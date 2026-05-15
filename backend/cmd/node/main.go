package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/api"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/gossip"
	"obscura.network/core/internal/messaging"
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

	// WebSocket Hub
	go messaging.GlobalHub.Run()

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

	// ─── ROUTER ───────────────────────────────────────────────────────────────
	r := mux.NewRouter()
	r.Use(api.CORSMiddleware)
	r.Use(api.LoggerMiddleware)

	// ─── PUBLIC ROUTES ────────────────────────────────────────────────────────
	pub := r.PathPrefix("/v1").Subrouter()
	pub.HandleFunc("/auth/request-otp", api.HandleRequestOTP).Methods("POST", "OPTIONS")
	pub.HandleFunc("/auth/verify-otp", api.HandleVerifyOTP).Methods("POST", "OPTIONS")
	pub.HandleFunc("/node/status", api.HandleNodeStatus).Methods("GET")
	// Cross-signing — public start + public status polling (Bölüm 5.4)
	pub.HandleFunc("/devices/pair/start", api.HandlePairStart).Methods("POST", "OPTIONS")
	pub.HandleFunc("/devices/pair/status/{id}", api.HandlePairStatus).Methods("GET")

	// ─── PROTECTED ROUTES ─────────────────────────────────────────────────────
	priv := r.PathPrefix("/v1").Subrouter()
	priv.Use(api.AuthMiddleware)

	// Kullanıcı
	priv.HandleFunc("/users/me", api.HandleGetMe).Methods("GET")
	priv.HandleFunc("/users/me", api.HandleUpdateMe).Methods("PATCH")
	priv.HandleFunc("/users/search", api.HandleSearchUser).Methods("GET")
	priv.HandleFunc("/users/{did}", api.HandleGetUser).Methods("GET")

	// Mesajlaşma
	priv.HandleFunc("/conversations", api.HandleGetConversations).Methods("GET")
	priv.HandleFunc("/conversations", api.HandleCreateConversation).Methods("POST")
	priv.HandleFunc("/conversations/{id}/messages", api.HandleGetMessages).Methods("GET")
	priv.HandleFunc("/messages", api.HandleSendMessage).Methods("POST")
	priv.HandleFunc("/messages/{id}", api.HandleDeleteMessage).Methods("DELETE")

	// Kredi
	priv.HandleFunc("/credit/score", api.HandleGetCreditScore).Methods("GET")
	priv.HandleFunc("/credit/history", api.HandleGetCreditHistory).Methods("GET")
	priv.HandleFunc("/credit/binding", api.HandleCreditBinding).Methods("POST")  // user_hash binding upload (one-time)
	priv.HandleFunc("/credit/upgrade", api.HandleCreditUpgrade).Methods("POST")  // ZK ispatlı tier upgrade
	priv.HandleFunc("/spam/report", api.HandleSpamReport).Methods("POST")

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

	// ZK Kanıt
	priv.HandleFunc("/zk/verify", api.HandleVerifyZKProof).Methods("POST")

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

	// WebRTC (TURN credentials — auth gerektirir)
	priv.HandleFunc("/rtc/turn-credentials", api.HandleGetTURNCredentials).Methods("GET")

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

	// Mini App Motoru (spec Bölüm 10) — FAZ 2 skeleton
	priv.HandleFunc("/apps", api.HandlePublishApp).Methods("POST")
	priv.HandleFunc("/apps", api.HandleListApps).Methods("GET")
	priv.HandleFunc("/apps/{id}", api.HandleGetApp).Methods("GET")
	priv.HandleFunc("/apps/{id}/install", api.HandleInstallApp).Methods("POST")
	priv.HandleFunc("/apps/{id}/install", api.HandleUninstallApp).Methods("DELETE")
	priv.HandleFunc("/apps/{id}/run", api.HandleRunApp).Methods("POST")

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
