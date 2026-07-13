package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/ai"
	"obscura.network/core/internal/api"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/bots"
	"obscura.network/core/internal/bridge"
	"obscura.network/core/internal/consensus"
	"obscura.network/core/internal/dao"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/federation"
	"obscura.network/core/internal/gossip"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/mls"
	"obscura.network/core/internal/p2p"
	"obscura.network/core/internal/scanner"
	"obscura.network/core/internal/sequencer"
	"obscura.network/core/internal/staking"
	"obscura.network/core/internal/subscriber"
	"obscura.network/core/internal/zk"
)

func main() {
	// ─── BAŞLATMA ─────────────────────────────────────────────────────────────
	log.Println("🦅 Obscura Node v3.0 başlatılıyor...")

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Veritabanı
	if err := db.Init(dataDir); err != nil {
		log.Fatalf("❌ Veritabanı hatası: %v", err)
	}
	defer db.Close()

	// Abone kimlik katmanı — mesaj düzleminden AYRI, yasal erişime açık,
	// alan bazında şifreli kimlik deposu ("identity at the door").
	subDB, err := subscriber.OpenSubscriberDB(dataDir)
	if err != nil {
		log.Fatalf("❌ Subscriber veritabanı hatası: %v", err)
	}
	defer subDB.Close()
	if err := subscriber.InitCryptoFromEnv(); err != nil {
		log.Fatalf("❌ Subscriber şifreleme başlatılamadı: %v", err)
	}
	subscriber.InitStore(subDB)
	phonePepper, err := subscriber.PepperFromEnv()
	if err != nil {
		log.Fatalf("❌ Subscriber pepper hatası: %v", err)
	}
	// Idempotent backfill: plaintext telefonları users'tan şifreli depoya taşır.
	if err := subscriber.MigratePhoneToSubscriberStore(db.DB, subDB, phonePepper); err != nil {
		log.Printf("⚠️  Subscriber phone migration hatası: %v", err)
	} else {
		log.Println("🔐 Subscriber kimlik katmanı hazır")
	}

	// WebSocket Hub
	go messaging.GlobalHub.Run()

	// Mesaj sona-erme zamanlayıcısı — saatlik tarama, TTL dolmuş mesajları
	// status='expired' işaretler ve içeriklerini temizler (Spec Bölüm 6.6).
	// Önceden hiç başlatılmıyordu; mesajlar süresiz birikiyordu.
	go messaging.StartMessageExpiryScheduler(db.DB, messaging.GlobalHub, time.Hour)
	log.Println("⏳ Mesaj sona-erme zamanlayıcısı aktif (1h aralık)")

	// Federation — permissionless node kaydı (FAZ 3)
	if err := federation.Init(db.DB); err != nil {
		log.Printf("⚠️  Federation başlatılamadı: %v", err)
	} else {
		federation.StartPruner()
		log.Println("🌐 Federation node kaydı aktif")
	}

	// BFT konsensüs — Tendermint-style Propose/Prevote/Precommit (FAZ 3)
	// Federation kaydından SONRA başlatılır; quorum = 2f+1 (peer sayısından türetilir).
	// Transport: şimdilik in-process LocalTransport (tek-node/dev). Çok-node üretimde
	// main, p2p.Publish/Subscribe köprüsünü buraya enjekte etmelidir.
	// TODO(FAZ3-BFT): P2P aktifken LocalTransport yerine GossipSub köprüsü geç.
	//
	// FAZ 3 BFT consensus iskeleti — BİLİNÇLİ olarak devre dışı (kararlaştırıldı,
	// tarih: 2026-07-03 teşhis + 2026-07-14 karar). "Engine'de bug" değil, eksik
	// wiring: ProposeBlock() kod tabanının hiçbir yerinden çağrılmıyor (grep ile
	// doğrulandı) — bu yüzden engine hiçbir zaman kendi bloğunu öneremiyor, tek
	// yaptığı 8sn'de bir advanceRound() (round say, logla, sıfırla) sonsuza dek
	// tekrarlamak; tek node'da bile (NODE_PEERS boş, quorum=1) bu yüzden hiç
	// commit'e ulaşmıyordu — 25dk'da round 200, log spam + gereksiz CPU.
	//
	// Eksik olan 4 parça (hiçbiri henüz yok):
	//   1. Blok-üretim scheduler'ı — periyodik/koşullu ProposeBlock() çağıran
	//      kod hiç yazılmadı.
	//   2. Gerçek transport — LocalTransport yalnızca in-process; çok-node için
	//      p2p.Publish/Subscribe köprüsü (TODO(FAZ3-BFT), yukarıda) bağlanmadı.
	//   3. Validator-set/stake bağlantısı — quorum salt peer-sayısından
	//      türetiliyor, internal/staking ile sıfır coupling (grep doğrulandı).
	//   4. Mempool — TxRoot'u besleyecek gerçek bir pending-tx kaynağı yok.
	//
	// İzolasyon doğrulandı: internal/consensus paketi main.go dışında hiçbir
	// yerden import edilmiyor (grep ile doğrulandı) — mesajlaşma, moderation,
	// X3DH, staking, sequencer bundan TAMAMEN bağımsız, kapalı kalması hiçbir
	// şeyi bozmaz.
	//
	// Madde 8 (blockchain katmanı) geldiğinde: yukarıdaki 4 parça eklenip bu
	// blok aktifleştirilecek. O zamana kadar açmak FAYDASIZ — yalnızca aynı
	// log-spam/CPU sorununu geri getirir, tek başına hiçbir şeyi düzeltmez.
	if false {
		selfID := os.Getenv("NODE_ID")
		if selfID == "" {
			selfID = "node-1"
		}
		// NODE_PEERS virgülle ayrılmış peer listesi. Toplam node = peer + 1 (self).
		peerCount := 0
		if peers := strings.TrimSpace(os.Getenv("NODE_PEERS")); peers != "" {
			peerCount = len(strings.Split(peers, ","))
		}
		totalNodes := peerCount + 1
		// Tendermint güvenliği: quorum = 2f+1 = ceil(2N/3). Tek-node'da quorum=1.
		quorum := (2*totalNodes)/3 + 1
		if quorum < 1 {
			quorum = 1
		}

		bftTransport := consensus.NewLocalTransport()
		bftEngine := consensus.NewEngine(
			selfID,
			quorum,
			func(b consensus.Block) {
				log.Printf("🧱 BFT blok commit edildi — height=%d, tx_root=%s", b.Height, b.TxRoot)
			},
			bftTransport.Publish,
			bftTransport.Subscribe,
		)
		if err := bftEngine.Start(); err != nil {
			log.Printf("⚠️  BFT konsensüs başlatılamadı: %v", err)
		} else {
			log.Printf("🗳️  BFT konsensüs aktif — selfID=%s, totalNodes=%d, quorum=%d", selfID, totalNodes, quorum)
		}
	}

	// DAO — tam yönetim (FAZ 4)
	if err := dao.Init(db.DB); err != nil {
		log.Printf("⚠️  DAO başlatılamadı: %v", err)
	} else {
		log.Println("🗳️  DAO tam yönetim aktif")
	}

	if err := bots.Init(db.DB); err != nil {
		log.Printf("⚠️  Bot tabloları başlatılamadı: %v", err)
	} else {
		log.Println("🤖 Bot ekosistemi aktif")
	}

	// Shard Storage — Reed-Solomon 4-of-6 (Spec Bölüm 3.2)
	if err := api.InitStorage(); err != nil {
		log.Printf("⚠️  Shard storage başlatılamadı: %v", err)
	} else {
		log.Println("🗄️  Shard storage aktif (256KB · 4-of-6 Reed-Solomon · 30 gün TTL)")
	}

	// Signal Protocol session store (Double Ratchet state — spec Bölüm 4.3)
	api.InitSignalStore()
	log.Println("🔒 Signal Protocol session store aktif")

	// MLS CLI subprocess (RFC 9420 grup şifrelemesi — isteğe bağlı)
	if mlsClient := mls.InitFromPath(os.Getenv("MLS_CLI_PATH")); mlsClient != nil {
		defer mlsClient.Close()
	}

	// MLS KeyPackage rotation scanner — günlük tarama, 90 gün TTL (spec Bölüm 4.2)
	mls.StartRotationScanner(context.Background())
	log.Println("🔄 MLS KeyPackage rotasyon tarayıcısı aktif (24h aralık)")

	// MLS proaktif rotasyon zamanlayıcısı — sunucu tarafı: süresi dolmaya yakın
	// KeyPackage'ları olan kullanıcılar için otomatik yeni KP üretir/tetikler
	// (spec Bölüm 4.2). Scanner sadece kapatıyordu; bu scheduler yenilemeyi sürdürür.
	mls.StartKeyPackageRotationScheduler(context.Background(), db.DB, os.Getenv("NODE_ID"))
	log.Println("🔄 MLS KeyPackage rotasyon zamanlayıcısı aktif")

	// Sequencer — on-chain staking entegrasyonu + otomatik epoch rotasyonu (FAZ 4)
	sequencer.SetStakeLookup(staking.NodeOperatorStakeOBS)
	sequencer.Global.StartEpochRotation(context.Background())

	// AI Node Optimizer — gerçek latency probing (FAZ 4)
	ai.StartNodeProber(context.Background())
	log.Println("🤖 AI node optimizer aktif (30s probe aralığı)")

	// 7/24 Tarama Sistemi — mesaj, grup, kullanıcı, node, ZK anomali, spam
	globalScanner := scanner.New(db.DB)
	globalScanner.Start(context.Background())
	log.Println("[Scanner] 7/24 tarama sistemi aktif")

	// P2P — libp2p host + GossipSub + DHT (FAZ 3)
	// Default: aktif. P2P_ENABLED=false ile devre dışı bırakılabilir.
	// Hata durumunda node crash etmez — HTTP gossip devam eder (graceful degrade).
	p2pCfg := p2p.ConfigFromEnv()
	if p2pCfg.Enabled {
		// ZK node_proof doğrulayıcıyı inject et (import cycle önlemek için)
		p2p.SetNodeProofVerifier(func(proofJSON string, publicSignals []string) error {
			return zk.VerifyGroth16(zk.CircuitNodeProof, []byte(proofJSON), publicSignals)
		})
		if err := p2p.Start(context.Background(), p2pCfg); err != nil {
			log.Printf("P2P baslatılamadı (HTTP gossip devam ediyor): %v", err)
		} else {
			// GossipSub → WebSocket hub köprüsü
			if err := p2p.StartHubBridge(context.Background(), func(did, typ string, payload interface{}) bool {
				return messaging.GlobalHub.SendTo(did, typ, payload)
			}); err != nil {
				log.Printf("P2P hub bridge başlatılamadı: %v", err)
			} else {
				log.Println("P2P: GossipSub ↔ WebSocket hub köprüsü aktif")
			}
		}
	} else {
		log.Println("P2P devre disi (P2P_ENABLED=false) — HTTP gossip aktif")
	}

	// Cross-chain bridge relayer (ETH→DOT, FAZ 3)
	bridge.StartRelayer(context.Background(), db.DB)

	// ZK verification keys (Groth16 vkey JSONs)
	zkKeysDir := os.Getenv("ZK_KEYS_DIR")
	if zkKeysDir == "" {
		zkKeysDir = "./internal/zk/keys"
	}
	if err := zk.LoadVerificationKeys(zkKeysDir); err != nil {
		log.Printf("⚠️  ZK verification keys yüklenemedi: %v (embedded fallback denenecek)", err)
	} else {
		log.Printf("🔐 ZK circuits yüklü: %v", zk.LoadedCircuits())
	}

	// ZK proof anomaly detector — replay/rate-limit/şüpheli proof tespiti.
	// Önceden hiç başlatılmıyordu; GlobalDetector() nil dönüyordu.
	zk.InitDetector(zk.NewProofAnomalyDetector())
	log.Println("🕵️  ZK proof anomaly detector aktif")

	// ─── ROUTER ───────────────────────────────────────────────────────────────
	r := mux.NewRouter()
	r.Use(api.CORSMiddleware)
	r.Use(api.LoggerMiddleware)

	// Global OPTIONS preflight yakalayıcı — gorilla/mux v1.8.1'de middleware
	// sadece EŞLEŞEN route'a uygulanıyor, 404/405 fallback'e uygulanmıyor.
	// Aşağıdaki route'ların çoğu Methods("GET"/"POST"/"PATCH") ile kayıtlı,
	// "OPTIONS" değil — bu yüzden CORSMiddleware'in OPTIONS kısayolu onlara
	// hiç erişemiyordu, tarayıcıdan (mobil'de değil, sadece web'de) auth'lu
	// isteklerin preflight'ı CORS header'sız 404 alıp bloklanıyordu. Bu route
	// herhangi bir path+OPTIONS için eşleşme sağlıyor; asıl 204+header yanıtı
	// CORSMiddleware'in kendi OPTIONS kısayolundan geliyor (r.Use zinciri
	// eşleşen handler'ı sarmalıyor), bu yüzden handler gövdesi hiç çalışmaz.
	r.PathPrefix("/").Methods("OPTIONS").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(204)
	})

	// ─── PUBLIC ROUTES ────────────────────────────────────────────────────────
	pub := r.PathPrefix("/v1").Subrouter()
	pub.HandleFunc("/auth/request-otp", api.HandleRequestOTP).Methods("POST", "OPTIONS")
	pub.HandleFunc("/auth/verify-otp", api.HandleVerifyOTP).Methods("POST", "OPTIONS")
	pub.HandleFunc("/dev/otp", api.HandleDevOTP).Methods("GET") // dev only — production'da OBSCURA_ENV=development değil
	pub.HandleFunc("/node/status", api.HandleNodeStatus).Methods("GET")
	// ZK-ID güncelleme — auth gerektirir (priv subrouter ile kayıtlı aşağıda)
	// Cross-signing — public start + public status polling (Bölüm 5.4)
	pub.HandleFunc("/devices/pair/start", api.HandlePairStart).Methods("POST", "OPTIONS")
	pub.HandleFunc("/devices/pair/status/{id}", api.HandlePairStatus).Methods("GET")

	// ─── PROTECTED ROUTES ─────────────────────────────────────────────────────
	priv := r.PathPrefix("/v1").Subrouter()
	priv.Use(api.AuthMiddleware)

	// ZK-ID güncelleme (spec Bölüm 5.2-5.3 — sonradan ZK kimliği ekle)
	priv.HandleFunc("/auth/zk-id-update", api.HandleZKIDUpdate).Methods("POST", "OPTIONS")

	// Kullanıcı
	priv.HandleFunc("/users/me", api.HandleGetMe).Methods("GET")
	priv.HandleFunc("/users/me", api.HandleUpdateMe).Methods("PATCH")
	priv.HandleFunc("/users/search", api.HandleSearchUser).Methods("GET")
	priv.HandleFunc("/users/{did}", api.HandleGetUser).Methods("GET")

	// Mesajlaşma
	priv.HandleFunc("/conversations", api.HandleGetConversations).Methods("GET")
	priv.HandleFunc("/conversations", api.HandleCreateConversation).Methods("POST")
	// Grup moderasyon — rapor gönder
	priv.HandleFunc("/groups/{id}/report", api.HandleReportGroup).Methods("POST")
	priv.HandleFunc("/conversations/{id}/messages", api.HandleGetMessages).Methods("GET")
	// Keşfet — public konuşmalar (statik route, {id} parametresinden önce kayıt edilmeli)
	priv.HandleFunc("/conversations/discover", api.HandleDiscoverConversations).Methods("GET")
	// Grup/kanal/topluluk profil-yönetimi
	priv.HandleFunc("/conversations/{id}", api.HandleGetConversationDetail).Methods("GET")
	priv.HandleFunc("/conversations/{id}", api.HandleUpdateConversation).Methods("PATCH")
	priv.HandleFunc("/conversations/{id}/leave", api.HandleLeaveConversation).Methods("POST")
	priv.HandleFunc("/conversations/{id}/members", api.HandleGetConversationMembers).Methods("GET")
	priv.HandleFunc("/conversations/{id}/members/{did}", api.HandleRemoveConvMember).Methods("DELETE")
	priv.HandleFunc("/conversations/{id}/members/{did}", api.HandleUpdateMemberRole).Methods("PATCH")
	priv.HandleFunc("/messages", api.HandleSendMessage).Methods("POST")
	priv.HandleFunc("/messages/{id}", api.HandleDeleteMessage).Methods("DELETE")
	// Mesaj durum sistemi (Spec Bölüm 6.4)
	priv.HandleFunc("/messages/{id}/read", api.HandleMarkMessageRead).Methods("POST")
	priv.HandleFunc("/messages/{id}/status", api.HandleGetMessageStatus).Methods("GET")
	// Mesaj geri çağırma — tüm node'larda sil (Spec Bölüm 6.5)
	priv.HandleFunc("/messages/{id}/recall", api.HandleGlobalRecall).Methods("POST")

	// Kredi
	priv.HandleFunc("/credit/score", api.HandleGetCreditScore).Methods("GET")
	priv.HandleFunc("/credit/history", api.HandleGetCreditHistory).Methods("GET")
	priv.HandleFunc("/credit/binding", api.HandleCreditBinding).Methods("POST")   // user_hash binding upload (one-time)
	priv.HandleFunc("/credit/upgrade", api.HandleCreditUpgrade).Methods("POST")  // ZK ispatlı tier upgrade
	priv.HandleFunc("/credit/claim-zk", api.HandleClaimZKCredit).Methods("POST") // ZK proof tabanlı kredi claim (Spec Bölüm 5.5)
	priv.HandleFunc("/spam/report", api.HandleSpamReport).Methods("POST")

	// Denetim ve Topluluk Katmanı — Oturum 1 (docs/spec/obscura_denetim_topluluk_katmani.md)
	priv.HandleFunc("/complaints/{id}/verdict", api.HandleComplaintVerdict).Methods("POST")
	priv.HandleFunc("/moderation/my-status", api.HandleModerationMyStatus).Methods("GET")

	// Medya yükleme
	priv.HandleFunc("/media/upload", api.HandleMediaUpload).Methods("POST")

	// Cihaz kaydı (FCM/APNs push)
	priv.HandleFunc("/devices/register", api.HandleRegisterDevice).Methods("POST")

	// Cross-signing approve + cihaz yönetimi (Bölüm 5.4)
	priv.HandleFunc("/devices/pair/approve", api.HandlePairApprove).Methods("POST")
	priv.HandleFunc("/devices", api.HandleListDevices).Methods("GET")
	priv.HandleFunc("/devices/{id}", api.HandleRevokeDevice).Methods("DELETE")

	// PreKey (X3DH anahtar değişimi)
	priv.HandleFunc("/keys/upload", api.HandleUploadPreKeyBundle).Methods("POST")
	priv.HandleFunc("/keys/{did}", api.HandleGetPreKeyBundle).Methods("GET")
	priv.HandleFunc("/keys/opk/replenish", api.HandleReplenishOPK).Methods("POST")
	priv.HandleFunc("/keys/opk/count", api.HandleGetOPKCount).Methods("GET")

	// Signal Protocol — session state + X3DH prekey distribution (spec Bölüm 4.3)
	priv.HandleFunc("/signal/session", api.HandleSignalSaveSession).Methods("POST")
	priv.HandleFunc("/signal/session/{did}", api.HandleSignalGetSession).Methods("GET")
	priv.HandleFunc("/signal/session/{did}", api.HandleSignalDeleteSession).Methods("DELETE")
	priv.HandleFunc("/signal/sessions", api.HandleSignalListSessions).Methods("GET")
	priv.HandleFunc("/signal/prekey/{did}", api.HandleSignalGetPrekey).Methods("GET")

	// ZK Kanıt
	priv.HandleFunc("/zk/verify", api.HandleVerifyZKProof).Methods("POST")
	pub.HandleFunc("/zk/circuits", api.HandleZKCircuitList).Methods("GET", "OPTIONS")
	priv.HandleFunc("/zk/prove", api.HandleZKProve).Methods("POST")
	// ZK audit log bütünlük kontrolü — X-Internal-Secret header ile korunur (JWT değil).
	// Sadece node operatörü / monitoring altyapısı çağırır; dışarıya açık değildir.
	r.HandleFunc("/v1/zk/audit", api.HandleZKAuditLog).Methods("GET")

	// MLS (RFC 9420) — grup şifrelemesi (spec Bölüm 6.3, ADR-0007)
	priv.HandleFunc("/mls/key-package", api.HandleMLSUploadKeyPackage).Methods("POST")
	priv.HandleFunc("/mls/key-package/{did}", api.HandleMLSGetKeyPackage).Methods("GET")
	priv.HandleFunc("/mls/group", api.HandleMLSCreateGroup).Methods("POST")
	priv.HandleFunc("/mls/group/{id}", api.HandleMLSGroupInfo).Methods("GET")
	priv.HandleFunc("/mls/group/{id}/add", api.HandleMLSAddMember).Methods("POST")
	priv.HandleFunc("/mls/group/{id}/message", api.HandleMLSGroupMessage).Methods("POST")
	priv.HandleFunc("/mls/group/{id}/messages", api.HandleMLSGroupMessages).Methods("GET")
	priv.HandleFunc("/mls/welcomes", api.HandleMLSPendingWelcomes).Methods("GET")
	priv.HandleFunc("/mls/welcomes/ack", api.HandleMLSAckWelcome).Methods("POST")
	// Yeni: join / commit / remove / state (spec Bölüm 17 EK B tam set)
	priv.HandleFunc("/mls/group/{id}/join", api.HandleMLSJoinGroup).Methods("POST")
	priv.HandleFunc("/mls/group/{id}/commit", api.HandleMLSCommitProposal).Methods("POST")
	priv.HandleFunc("/mls/group/{id}/state", api.HandleMLSGroupState).Methods("GET")
	priv.HandleFunc("/mls/group/{id}/member/{did}", api.HandleMLSRemoveMember).Methods("DELETE")

	// WebRTC (TURN credentials — auth gerektirir)
	priv.HandleFunc("/rtc/turn-credentials", api.HandleGetTURNCredentials).Methods("GET")

	// WebRTC arama sinyalizasyonu (spec Bölüm 9)
	priv.HandleFunc("/call/invite", api.HandleCallInvite).Methods("POST")
	priv.HandleFunc("/call/answer", api.HandleCallAnswer).Methods("POST")
	priv.HandleFunc("/call/end", api.HandleCallEnd).Methods("POST")

	// NFC — fiziksel etkileşim (Spec Bölüm 11.4): check-in, eşleştirme, ödeme.
	// Önceden handler'lar yazılmıştı ama route'ları kaydedilmemişti (dead code).
	priv.HandleFunc("/nfc/checkin", api.HandleNFCCheckin).Methods("POST")
	priv.HandleFunc("/nfc/pair", api.HandleNFCPair).Methods("POST")
	priv.HandleFunc("/nfc/payment", api.HandleNFCPayment).Methods("POST")

	// Kişi Rehberi
	priv.HandleFunc("/contacts", api.HandleGetContacts).Methods("GET")
	priv.HandleFunc("/contacts", api.HandleAddContact).Methods("POST")
	priv.HandleFunc("/contacts/{did}", api.HandleRemoveContact).Methods("DELETE")

	// Davet linki
	priv.HandleFunc("/conversations/{id}/invite/create", api.HandleCreateConvInvite).Methods("POST")
	priv.HandleFunc("/conversations/join", api.HandleJoinViaInvite).Methods("POST")

	// OBS Cüzdan (token state layer — ADR-0010)
	priv.HandleFunc("/wallet/balance", api.HandleWalletBalance).Methods("GET")
	priv.HandleFunc("/wallet/transfer", api.HandleWalletTransfer).Methods("POST")
	priv.HandleFunc("/wallet/transactions", api.HandleWalletTransactions).Methods("GET")
	priv.HandleFunc("/wallet/supply", api.HandleWalletSupply).Methods("GET")

	// Shielded transfers (spec Bölüm 8.3 — Gizli Transfer Akışı / ZK)
	priv.HandleFunc("/wallet/shield", api.HandleWalletShield).Methods("POST")
	priv.HandleFunc("/wallet/shielded-transfer", api.HandleWalletShieldedTransfer).Methods("POST")
	priv.HandleFunc("/wallet/unshield", api.HandleWalletUnshield).Methods("POST")
	priv.HandleFunc("/wallet/shielded/root", api.HandleWalletShieldedRoot).Methods("GET")
	priv.HandleFunc("/wallet/shielded/notes", api.HandleWalletShieldedNotes).Methods("GET")
	priv.HandleFunc("/wallet/shielded/proof/{leaf_index}", api.HandleWalletMerkleProof).Methods("GET")

	// OBS Staking + Slashing (ADR-0011)
	priv.HandleFunc("/staking/stake", api.HandleStakingStake).Methods("POST")
	priv.HandleFunc("/staking/unstake", api.HandleStakingUnstake).Methods("POST")
	priv.HandleFunc("/staking/withdraw", api.HandleStakingWithdraw).Methods("POST")
	priv.HandleFunc("/staking/positions", api.HandleStakingPositions).Methods("GET")
	priv.HandleFunc("/staking/slashes", api.HandleStakingSlashes).Methods("GET")
	priv.HandleFunc("/staking/slash/review", api.HandleStakingSlashReview).Methods("POST")

	// Airdrop Dağıtımı (spec Bölüm 12.2) — ZK-gated, Sybil-resistant
	priv.HandleFunc("/airdrop/campaigns", api.HandleAirdropCreateCampaign).Methods("POST")
	priv.HandleFunc("/airdrop/campaigns", api.HandleAirdropListCampaigns).Methods("GET")
	priv.HandleFunc("/airdrop/campaigns/{id}", api.HandleAirdropGetCampaign).Methods("GET")
	priv.HandleFunc("/airdrop/campaigns/{id}/claim", api.HandleAirdropClaim).Methods("POST")
	priv.HandleFunc("/airdrop/campaigns/{id}/end", api.HandleAirdropEndCampaign).Methods("POST")

	// Governance — ZK voting + tier-gated eligibility (ADR-0012)
	priv.HandleFunc("/governance/proposals", api.HandleGovernanceCreateProposal).Methods("POST")
	priv.HandleFunc("/governance/proposals", api.HandleGovernanceListProposals).Methods("GET")
	priv.HandleFunc("/governance/proposals/{id}", api.HandleGovernanceGetProposal).Methods("GET")
	priv.HandleFunc("/governance/proposals/{id}/voter-root", api.HandleGovernanceVoterRoot).Methods("GET")
	priv.HandleFunc("/governance/proposals/{id}/vote", api.HandleGovernanceSubmitVote).Methods("POST")
	priv.HandleFunc("/governance/proposals/{id}/finalize", api.HandleGovernanceFinalize).Methods("POST")
	priv.HandleFunc("/governance/proposals/{id}/veto", api.HandleGovernanceVeto).Methods("POST")
	priv.HandleFunc("/governance/proposals/{id}/execute", api.HandleGovernanceExecute).Methods("POST")
	priv.HandleFunc("/governance/community/join", api.HandleJoinOBSCommunity).Methods("POST")

	// Mini App Motoru (spec Bölüm 10) — FAZ 2 skeleton
	priv.HandleFunc("/apps", api.HandlePublishApp).Methods("POST")
	priv.HandleFunc("/apps", api.HandleListApps).Methods("GET")
	priv.HandleFunc("/apps/{id}", api.HandleGetApp).Methods("GET")
	priv.HandleFunc("/apps/{id}/install", api.HandleInstallApp).Methods("POST")
	priv.HandleFunc("/apps/{id}/install", api.HandleUninstallApp).Methods("DELETE")
	priv.HandleFunc("/apps/{id}/run", api.HandleRunApp).Methods("POST")
	priv.HandleFunc("/miniapp/bridge", api.HandleBridgeHTTP).Methods("POST")

	// Bot ekosistemi — webhook tabanlı otomatik ajanlar
	bots.RegisterRoutes(priv)

	// Prometheus metrics (iç ağda erişilebilir)
	r.HandleFunc("/v1/metrics", api.HandleMetrics).Methods("GET")

	// WebRTC Sinyalizasyon (token query param ile)
	r.HandleFunc("/v1/rtc/signal", api.HandleRTCSignal)

	// ─── WEBSOCKET ────────────────────────────────────────────────────────────
	r.HandleFunc("/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		// Token query param veya header'dan al
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			tokenStr = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}

		claims, err := auth.ValidateToken(tokenStr)
		if err != nil {
			http.Error(w, "Yetkisiz", 401)
			return
		}

		conn, err := messaging.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WS yükseltme hatası: %v", err)
			return
		}

		client := &messaging.Client{
			DID:    claims.DID,
			UserID: claims.UserID,
			Tier:   claims.Tier,
			Conn:   conn,
			Send:   make(chan []byte, 256),
			Hub:    messaging.GlobalHub,
		}

		messaging.GlobalHub.Register <- client
		go client.WritePump()
		go client.ReadPump()
	})

	// ─── FAZ 3: FEDERATION + P2P + BRIDGE + PQ ───────────────────────────────
	// Node kaydı (permissionless — auth gerektirmez)
	pub.HandleFunc("/nodes/register", api.HandleNodeRegister).Methods("POST", "OPTIONS")
	pub.HandleFunc("/nodes", api.HandleNodeList).Methods("GET")
	pub.HandleFunc("/nodes/{id}", api.HandleNodeGet).Methods("GET")
	pub.HandleFunc("/nodes/{id}/heartbeat", api.HandleNodeHeartbeat).Methods("POST", "OPTIONS")

	// Cross-chain bridge (FAZ 3 — real relayer)
	priv.HandleFunc("/bridge/status", api.HandleBridgeStatus).Methods("GET")
	priv.HandleFunc("/bridge/lock", api.HandleBridgeLock).Methods("POST")
	priv.HandleFunc("/bridge/transfers/{id}", api.HandleBridgeTransferStatus).Methods("GET")
	priv.HandleFunc("/bridge/transfers", api.HandleBridgeTransferList).Methods("GET")

	// Post-quantum anahtar hazırlığı (FAZ 3)
	priv.HandleFunc("/pq/keygen", api.HandlePQKeyGen).Methods("POST")

	// ─── FAZ 4: DAO + PQ + AI + SEQUENCER + LOCATION ────────────────────────
	// DAO (tam yönetim — FAZ 2 governance'ın üstünde)
	priv.HandleFunc("/dao/proposals", api.HandleDAOCreateProposal).Methods("POST")
	priv.HandleFunc("/dao/proposals", api.HandleDAOListProposals).Methods("GET")
	priv.HandleFunc("/dao/proposals/{id}/finalize", api.HandleDAOFinalize).Methods("POST")
	priv.HandleFunc("/dao/proposals/{id}/execute", api.HandleDAOExecute).Methods("POST")
	priv.HandleFunc("/dao/proposals/{id}/veto", api.HandleDAOVeto).Methods("POST")

	// Post-quantum imzalama (Dilithium3)
	priv.HandleFunc("/pq/dilithium/keygen", api.HandleDilithiumKeygen).Methods("POST")
	pub.HandleFunc("/pq/dilithium/verify", api.HandleDilithiumVerify).Methods("POST")

	// AI node optimizasyonu
	priv.HandleFunc("/ai/metrics", api.HandleAIMetrics).Methods("GET")
	priv.HandleFunc("/ai/peers", api.HandleAIPeers).Methods("GET")

	// Decentralized sequencer
	priv.HandleFunc("/sequencer/register", api.HandleSequencerRegister).Methods("POST")
	pub.HandleFunc("/sequencer/candidates", api.HandleSequencerList).Methods("GET")
	pub.HandleFunc("/sequencer/batches", api.HandleSequencerBatches).Methods("GET")
	priv.HandleFunc("/sequencer/batch", api.HandleSequencerSubmitBatch).Methods("POST")

	// GPS + ZK location proof
	priv.HandleFunc("/location/verify", api.HandleLocationVerify).Methods("POST")

	// ─── Fiziksel Etkinlik Entegrasyonu (Spec Bölüm 11) ──────────────────────
	priv.HandleFunc("/events", api.HandleCreateEvent).Methods("POST")
	priv.HandleFunc("/events", api.HandleListEvents).Methods("GET")
	priv.HandleFunc("/events/nearby", api.HandleNearbyEvents).Methods("GET")
	priv.HandleFunc("/events/{id}", api.HandleGetEvent).Methods("GET")
	priv.HandleFunc("/events/{id}/join", api.HandleJoinEvent).Methods("POST")
	priv.HandleFunc("/events/{id}/join", api.HandleLeaveEvent).Methods("DELETE")
	priv.HandleFunc("/events/{id}/checkin", api.HandleCheckIn).Methods("POST")
	priv.HandleFunc("/events/{id}/attendees", api.HandleListAttendees).Methods("GET")
	priv.HandleFunc("/events/{id}/qr", api.HandleGetCheckinQR).Methods("GET")

	// ─── Identity + BIP39 + Sosyal Kurtarma ──────────────────────────────────
	priv.HandleFunc("/identity/mnemonic/generate", api.HandleGenerateMnemonic).Methods("POST")
	pub.HandleFunc("/identity/mnemonic/validate", api.HandleValidateMnemonic).Methods("POST", "OPTIONS")
	priv.HandleFunc("/identity/derive", api.HandleDeriveIdentity).Methods("POST")
	priv.HandleFunc("/identity/shamir/split", api.HandleShamirSplit).Methods("POST")
	pub.HandleFunc("/identity/shamir/combine", api.HandleShamirCombine).Methods("POST", "OPTIONS")

	// ─── Binding Rotation — timelocked DID migration (ADR-0009 N3) ───────────
	priv.HandleFunc("/identity/rotation/request", api.HandleRotationRequest).Methods("POST")
	priv.HandleFunc("/identity/rotation/{id}", api.HandleRotationCancel).Methods("DELETE")
	priv.HandleFunc("/identity/rotation/{id}/confirm", api.HandleRotationConfirm).Methods("POST")

	// ─── Shard Storage (Spec Bölüm 3.2) ─────────────────────────────────────
	priv.HandleFunc("/storage/shard", api.HandleShardUpload).Methods("POST")
	priv.HandleFunc("/storage/shard/{content_id}", api.HandleShardRetrieve).Methods("GET")
	priv.HandleFunc("/storage/shard/{content_id}", api.HandleShardDelete).Methods("DELETE")
	pub.HandleFunc("/storage/stats", api.HandleShardStats).Methods("GET")
	r.HandleFunc("/v1/storage/local-shard", api.HandleLocalShard).Methods("POST")                  // node'dan node'a shard yaz
	r.HandleFunc("/v1/storage/local-shard/{shard_id}", api.HandleFetchLocalShard).Methods("GET") // node'dan node'a shard oku

	// ─── INTERNAL SHARD STORAGE (node'lar arası) ──────────────────────────────
	r.HandleFunc("/v1/internal/store-shard", api.HandleStoreShard).Methods("POST")
	r.HandleFunc("/v1/internal/fetch-shard", api.HandleFetchShardInternal).Methods("GET")

	// ─── INTERNAL RELAY (node'lar arası) ─────────────────────────────────────
	r.HandleFunc("/v1/internal/relay",
		gossip.BuildRelayHandler(func(targetDID, msgType string, payload interface{}) {
			messaging.GlobalHub.SendTo(targetDID, msgType, payload)
		}),
	).Methods("POST")

	// ─── BAŞLAT ───────────────────────────────────────────────────────────────
	log.Printf("✅ Obscura Node hazır → http://localhost:%s", port)
	log.Printf("📡 WebSocket  → ws://localhost:%s/v1/stream", port)
	log.Printf("🔐 E2EE aktif | ZK-ID aktif | Kredi sistemi aktif")

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("❌ Sunucu başlatılamadı: %v", err)
	}
}
