package federation

// Tests for federation.go (permissionless node registry CRUD + health probe).
// Previously zero test coverage. db/mu are package-level singletons (same
// pattern as sequencer.Global) — each test calls Init() with a fresh
// in-memory SQLite DB to avoid cross-test state leakage (tests run
// sequentially within the package, no t.Parallel()).

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestFederation(t *testing.T) {
	t.Helper()
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { testDB.Close() })
	if err := Init(testDB); err != nil {
		t.Fatalf("federation.Init: %v", err)
	}
}

func validRegisterReq(nodeID string) RegisterRequest {
	return RegisterRequest{
		NodeID:   nodeID,
		PeerAddr: "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/Qm" + nodeID,
		Pubkey:   "deadbeef",
		Version:  "3.0.0",
		Region:   "test",
	}
}

// genKeypair — test için gerçek bir Ed25519 anahtar çifti üretir.
func genKeypair(t *testing.T) (pubHex string, priv ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	return hex.EncodeToString(pub), priv
}

// signedRegisterReq — gerçek anahtarla İMZALANMIŞ, geçerli bir RegisterRequest
// üretir (Sig doğrulama testlerinin ortak "iyi hal" temeli). vrfPubkey boş
// bırakılabilir.
func signedRegisterReq(t *testing.T, nodeID, vrfPubkey string) RegisterRequest {
	t.Helper()
	pubHex, priv := genKeypair(t)
	req := RegisterRequest{
		NodeID:    nodeID,
		PeerAddr:  "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/Qm" + nodeID,
		Pubkey:    pubHex,
		Version:   "3.0.0",
		Region:    "test",
		VRFPubkey: vrfPubkey,
		Timestamp: time.Now().UTC().Unix(),
	}
	sig := ed25519.Sign(priv, SignaturePayload(req))
	req.Sig = hex.EncodeToString(sig)
	return req
}

// ─── Register / Get / List — CRUD ──────────────────────────────────────────

func TestRegister_CreatesNewNode(t *testing.T) {
	setupTestFederation(t)
	rec, err := Register(validRegisterReq("node-a"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if rec.NodeID != "node-a" || rec.Status != "active" {
		t.Fatalf("beklenmedik kayıt: %+v", rec)
	}
}

func TestRegister_MissingRequiredFieldsFails(t *testing.T) {
	setupTestFederation(t)
	cases := []RegisterRequest{
		{PeerAddr: "x", Pubkey: "y"}, // node_id eksik
		{NodeID: "n", Pubkey: "y"},   // peer_addr eksik
		{NodeID: "n", PeerAddr: "x"}, // pubkey eksik
	}
	for i, req := range cases {
		if _, err := Register(req); err == nil {
			t.Fatalf("case %d: eksik alanla Register hata vermeliydi", i)
		}
	}
}

func TestRegister_UpsertsExistingNode(t *testing.T) {
	setupTestFederation(t)
	if _, err := Register(validRegisterReq("node-a")); err != nil {
		t.Fatalf("ilk register: %v", err)
	}
	updated := validRegisterReq("node-a")
	updated.PeerAddr = "/ip4/9.9.9.9/udp/9001/quic-v1/p2p/QmYeni"
	updated.Region = "yeni-bolge"
	if _, err := Register(updated); err != nil {
		t.Fatalf("ikinci register (upsert): %v", err)
	}

	got, err := Get("node-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("node-a bulunamadı")
	}
	if got.PeerAddr != updated.PeerAddr {
		t.Fatalf("upsert peer_addr'ı güncellememiş: got=%s want=%s", got.PeerAddr, updated.PeerAddr)
	}
	if got.Region != "yeni-bolge" {
		t.Fatalf("upsert region'ı güncellememiş: got=%s", got.Region)
	}

	// Aynı node_id ile ikinci bir satır OLUŞMAMALI (gerçek upsert, insert değil).
	all, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	count := 0
	for _, n := range all {
		if n.NodeID == "node-a" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("node-a için tam olarak 1 satır olmalıydı, got=%d", count)
	}
}

// TestRegister_StoresVRFPubkey — ADR-0017 adım 5: sequencer VRF proof
// doğrulaması için diğer node'ların bu node'un vrf_pubkey'ini federation'dan
// öğrenebilmesi gerekiyor.
func TestRegister_StoresVRFPubkey(t *testing.T) {
	setupTestFederation(t)
	req := validRegisterReq("node-vrf")
	req.VRFPubkey = "deadbeefcafe"
	if _, err := Register(req); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := Get("node-vrf")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.VRFPubkey != "deadbeefcafe" {
		t.Fatalf("vrf_pubkey beklenmedik: got=%q", got.VRFPubkey)
	}
}

// TestRegister_EmptyVRFPubkeyDoesNotWipeExisting — heartbeat/eski
// istemciler vrf_pubkey göndermeden yeniden register olabilir; bu durumda
// önceden kaydedilmiş vrf_pubkey silinmemeli (COALESCE/NULLIF fix).
func TestRegister_EmptyVRFPubkeyDoesNotWipeExisting(t *testing.T) {
	setupTestFederation(t)
	req := validRegisterReq("node-vrf2")
	req.VRFPubkey = "01020304"
	if _, err := Register(req); err != nil {
		t.Fatalf("ilk register: %v", err)
	}

	reReq := validRegisterReq("node-vrf2")
	reReq.VRFPubkey = "" // eski istemci simülasyonu
	if _, err := Register(reReq); err != nil {
		t.Fatalf("ikinci register: %v", err)
	}

	got, err := Get("node-vrf2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.VRFPubkey != "01020304" {
		t.Fatalf("boş vrf_pubkey ile re-register mevcut değeri silmemeliydi, got=%q", got.VRFPubkey)
	}
}

// ─── Sig doğrulama (yumuşak geçiş) ─────────────────────────────────────────

// TestRegister_EmptySigAcceptedLegacyPath — geriye dönük uyumluluk: mevcut
// (frontend manuel formundan gelen, hiç Sig göndermeyen) kayıtlar reddedilmemeli.
func TestRegister_EmptySigAcceptedLegacyPath(t *testing.T) {
	setupTestFederation(t)
	req := validRegisterReq("node-legacy")
	if req.Sig != "" {
		t.Fatalf("test öncüllü: validRegisterReq Sig içermemeli, got=%q", req.Sig)
	}
	if _, err := Register(req); err != nil {
		t.Fatalf("boş Sig ile Register reddedilmemeliydi: %v", err)
	}
	got, _ := Get("node-legacy")
	if got == nil {
		t.Fatal("boş Sig'li kayıt oluşmadı")
	}
}

// TestRegister_ValidSignatureAccepted — doğru anahtarla, doğru payload'la
// üretilmiş imza kabul edilmeli.
func TestRegister_ValidSignatureAccepted(t *testing.T) {
	setupTestFederation(t)
	req := signedRegisterReq(t, "node-signed", "vrfpub-aaaa")
	if _, err := Register(req); err != nil {
		t.Fatalf("geçerli imzalı kayıt reddedildi: %v", err)
	}
	got, _ := Get("node-signed")
	if got == nil || got.VRFPubkey != "vrfpub-aaaa" {
		t.Fatalf("kayıt beklenmedik: %+v", got)
	}
}

// TestRegister_WrongSignerRejected — Pubkey alanı A'ya ait ama imza B'nin
// private key'iyle üretilmiş (Pubkey'nin karşılığı olmayan bir anahtar) —
// reddedilmeli, kayıt oluşmamalı.
func TestRegister_WrongSignerRejected(t *testing.T) {
	setupTestFederation(t)
	pubA, _ := genKeypair(t)
	_, privB := genKeypair(t)

	req := RegisterRequest{
		NodeID:    "node-wrongsigner",
		PeerAddr:  "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/QmX",
		Pubkey:    pubA,
		Timestamp: time.Now().UTC().Unix(),
	}
	req.Sig = hex.EncodeToString(ed25519.Sign(privB, SignaturePayload(req)))

	if _, err := Register(req); err == nil {
		t.Fatal("Pubkey'in sahibi olmayan bir anahtarla imzalanmış kayıt kabul edildi")
	}
	if got, _ := Get("node-wrongsigner"); got != nil {
		t.Fatalf("reddedilen kayıt DB'ye yazılmamalıydı, got=%+v", got)
	}
}

// TestRegister_TamperedVRFPubkeyRejected — KRİTİK senaryo: VRF fix'in
// (sequencer.handleIncomingVRFProof) adım-3 doğrulaması "iddia edilen
// node'un federation'da bilinen GERÇEK vrf_pubkey'i" varsayımına dayanıyor.
// SignaturePayload vrf_pubkey'i İÇERMESEYDİ, bir saldırgan BAŞKA bir node'un
// tamamen geçerli (node_id, peer_addr, pubkey) imzasını yakalayıp üzerine
// KENDİ vrf_pubkey'ini ekleyerek gönderebilir, federation bunu kabul eder,
// ve saldırgan artık o node_id ADINA (asıl sahibinin bilgisi/onayı olmadan)
// kendi ürettiği VRF proof'larını "meşru" gösterebilirdi — VRF adillik
// fix'ini dolaylı olarak atlatmanın yolu bu olurdu. SignaturePayload
// vrf_pubkey'i dahil ettiği için bu imzayı GEÇERSİZ kılar: vrf_pubkey
// değişince payload değişir, imza artık o payload'la eşleşmez.
func TestRegister_TamperedVRFPubkeyRejected(t *testing.T) {
	setupTestFederation(t)
	req := signedRegisterReq(t, "node-victim", "vrfpub-real-owner")

	// Saldırgan: imza ÜRETİLDİKTEN SONRA vrf_pubkey'i kendi anahtarıyla değiştiriyor.
	req.VRFPubkey = "vrfpub-attacker-controlled"

	if _, err := Register(req); err == nil {
		t.Fatal("imzalandıktan sonra vrf_pubkey değiştirilmiş kayıt kabul edildi — VRF adillik fix'i dolaylı atlatılabilir")
	}
	if got, _ := Get("node-victim"); got != nil {
		t.Fatalf("tahrif edilmiş kayıt DB'ye yazılmamalıydı, got=%+v", got)
	}
}

// TestRegister_TamperedPeerAddrRejected — genel tahrifat sağlaması: sadece
// vrf_pubkey değil, imzaya dahil HERHANGİ bir alan (örn. peer_addr, node'un
// trafiğinin yönlendirileceği adres) imzalandıktan sonra değiştirilirse
// reddedilmeli.
func TestRegister_TamperedPeerAddrRejected(t *testing.T) {
	setupTestFederation(t)
	req := signedRegisterReq(t, "node-c", "")
	req.PeerAddr = "/ip4/10.0.0.1/udp/9001/quic-v1/p2p/QmAttacker"

	if _, err := Register(req); err == nil {
		t.Fatal("imzalandıktan sonra peer_addr değiştirilmiş kayıt kabul edildi")
	}
}

// TestRegister_OldTimestampRejected — replay guard: eski (5dk penceresi
// dışı) bir timestamp'le üretilmiş — imza kendisi geçerli olsa bile —
// reddedilmeli (yakalanmış eski bir kaydın tekrar oynatılması senaryosu).
func TestRegister_OldTimestampRejected(t *testing.T) {
	setupTestFederation(t)
	pubHex, priv := genKeypair(t)
	req := RegisterRequest{
		NodeID:    "node-replay",
		PeerAddr:  "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/QmR",
		Pubkey:    pubHex,
		Timestamp: time.Now().UTC().Add(-10 * time.Minute).Unix(),
	}
	req.Sig = hex.EncodeToString(ed25519.Sign(priv, SignaturePayload(req)))

	if _, err := Register(req); err == nil {
		t.Fatal("10dk eski timestamp'li (imzası geçerli) kayıt kabul edildi — replay guard çalışmıyor")
	}
}

// TestRegister_FutureTimestampRejected — pencere simetrik: aşırı ileri
// tarihli bir timestamp de (clock skew istismarı/önceden-imzalanmış replay
// hazırlığı) reddedilmeli.
func TestRegister_FutureTimestampRejected(t *testing.T) {
	setupTestFederation(t)
	pubHex, priv := genKeypair(t)
	req := RegisterRequest{
		NodeID:    "node-future",
		PeerAddr:  "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/QmF",
		Pubkey:    pubHex,
		Timestamp: time.Now().UTC().Add(10 * time.Minute).Unix(),
	}
	req.Sig = hex.EncodeToString(ed25519.Sign(priv, SignaturePayload(req)))

	if _, err := Register(req); err == nil {
		t.Fatal("10dk ileri tarihli timestamp'li kayıt kabul edildi")
	}
}

// TestRegister_SigWithoutTimestampRejected — Sig dolu ama Timestamp=0
// (eski/uyumsuz client, ya da alan atlanmış) net biçimde reddedilmeli,
// sessizce "timestamp yokmuş gibi" kabul edilmemeli.
func TestRegister_SigWithoutTimestampRejected(t *testing.T) {
	setupTestFederation(t)
	pubHex, priv := genKeypair(t)
	req := RegisterRequest{
		NodeID:   "node-notimestamp",
		PeerAddr: "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/QmT",
		Pubkey:   pubHex,
		// Timestamp kasıtlı olarak 0 bırakıldı.
	}
	req.Sig = hex.EncodeToString(ed25519.Sign(priv, SignaturePayload(req)))

	if _, err := Register(req); err == nil {
		t.Fatal("timestamp=0 ile Sig'li kayıt kabul edildi")
	}
}

// TestRegister_MalformedPubkeyWithSigRejected — Sig doluyken Pubkey geçerli
// hex/32-byte Ed25519 formatında değilse panic ETMEDEN düzgün reddedilmeli.
func TestRegister_MalformedPubkeyWithSigRejected(t *testing.T) {
	setupTestFederation(t)
	req := RegisterRequest{
		NodeID:    "node-badpub",
		PeerAddr:  "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/QmB",
		Pubkey:    "not-valid-hex-zzz",
		Sig:       "deadbeef",
		Timestamp: time.Now().UTC().Unix(),
	}
	if _, err := Register(req); err == nil {
		t.Fatal("bozuk pubkey formatlı (Sig dolu) kayıt kabul edildi")
	}
}

// TestRegister_MalformedSigRejected — Sig geçerli hex/64-byte formatında
// değilse (bozuk/kısaltılmış) reddedilmeli.
func TestRegister_MalformedSigRejected(t *testing.T) {
	setupTestFederation(t)
	pubHex, _ := genKeypair(t)
	req := RegisterRequest{
		NodeID:    "node-badsig",
		PeerAddr:  "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/QmS",
		Pubkey:    pubHex,
		Sig:       "zz",
		Timestamp: time.Now().UTC().Unix(),
	}
	if _, err := Register(req); err == nil {
		t.Fatal("bozuk sig formatlı kayıt kabul edildi")
	}
}

// TestSignaturePayload_FieldSensitive — SignaturePayload'ın imzaya dahil
// TÜM alanlara (node_id, peer_addr, pubkey, vrf_pubkey, timestamp) duyarlı
// olduğunu doğrudan doğrular — Register üzerinden dolaylı değil, payload
// üretiminin kendisinde. TamperedVRFPubkey testinin dayandığı özelliğin
// kök nedenini burada izole ediyoruz.
func TestSignaturePayload_FieldSensitive(t *testing.T) {
	base := RegisterRequest{
		NodeID:    "node-x",
		PeerAddr:  "/ip4/1.2.3.4/udp/9001/quic-v1",
		Pubkey:    "aabbcc",
		VRFPubkey: "ddeeff",
		Timestamp: 1000,
	}
	basePayload := string(SignaturePayload(base))

	variants := map[string]RegisterRequest{
		"node_id":    {NodeID: "node-y", PeerAddr: base.PeerAddr, Pubkey: base.Pubkey, VRFPubkey: base.VRFPubkey, Timestamp: base.Timestamp},
		"peer_addr":  {NodeID: base.NodeID, PeerAddr: "/ip4/9.9.9.9/udp/9001/quic-v1", Pubkey: base.Pubkey, VRFPubkey: base.VRFPubkey, Timestamp: base.Timestamp},
		"pubkey":     {NodeID: base.NodeID, PeerAddr: base.PeerAddr, Pubkey: "112233", VRFPubkey: base.VRFPubkey, Timestamp: base.Timestamp},
		"vrf_pubkey": {NodeID: base.NodeID, PeerAddr: base.PeerAddr, Pubkey: base.Pubkey, VRFPubkey: "attacker-vrf-pubkey", Timestamp: base.Timestamp},
		"timestamp":  {NodeID: base.NodeID, PeerAddr: base.PeerAddr, Pubkey: base.Pubkey, VRFPubkey: base.VRFPubkey, Timestamp: 2000},
	}
	for field, variant := range variants {
		if string(SignaturePayload(variant)) == basePayload {
			t.Fatalf("%s alanı değişince payload AYNI kaldı — bu alan imza tarafından korunmuyor", field)
		}
	}

	// Determinizm: aynı girdi her zaman aynı payload'ı üretmeli.
	if string(SignaturePayload(base)) != basePayload {
		t.Fatal("SignaturePayload deterministik değil")
	}
}

func TestGet_ReturnsNilForUnknownNode(t *testing.T) {
	setupTestFederation(t)
	got, err := Get("hic-boyle-bir-node-yok")
	if err != nil {
		t.Fatalf("bilinmeyen node için err beklenmezdi: %v", err)
	}
	if got != nil {
		t.Fatalf("bilinmeyen node için nil beklenirdi, got=%+v", got)
	}
}

func TestGet_ReturnsRegisteredNode(t *testing.T) {
	setupTestFederation(t)
	Register(validRegisterReq("node-b"))
	got, err := Get("node-b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.NodeID != "node-b" {
		t.Fatalf("node-b dönmedi: %+v", got)
	}
}

func TestList_ReturnsOnlyActiveNodes(t *testing.T) {
	setupTestFederation(t)
	Register(validRegisterReq("node-active"))
	Register(validRegisterReq("node-inactive"))

	if _, err := db.Exec(`UPDATE federation_nodes SET status = 'inactive' WHERE node_id = ?`, "node-inactive"); err != nil {
		t.Fatalf("manuel inactive işaretleme: %v", err)
	}

	nodes, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "node-active" {
		t.Fatalf("sadece node-active dönmeliydi, got=%+v", nodes)
	}
}

func TestUpdateLatency_PersistsValue(t *testing.T) {
	setupTestFederation(t)
	Register(validRegisterReq("node-lat"))
	if err := UpdateLatency("node-lat", 42); err != nil {
		t.Fatalf("UpdateLatency: %v", err)
	}
	got, _ := Get("node-lat")
	if got.LatencyMs != 42 {
		t.Fatalf("latency=42 beklenirdi, got=%d", got.LatencyMs)
	}
}

func TestHeartbeat_UpdatesLastSeen(t *testing.T) {
	setupTestFederation(t)
	Register(validRegisterReq("node-hb"))
	before, _ := Get("node-hb")

	time.Sleep(1100 * time.Millisecond) // DATETIME saniye çözünürlüğü — fark görünür olsun
	if err := Heartbeat("node-hb"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	after, _ := Get("node-hb")

	if !after.LastSeen.After(before.LastSeen) {
		t.Fatalf("heartbeat sonrası last_seen ilerlemeliydi: before=%v after=%v", before.LastSeen, after.LastSeen)
	}
}

func TestPruneInactive_MarksOldNodesInactiveButKeepsRecent(t *testing.T) {
	setupTestFederation(t)
	Register(validRegisterReq("node-old"))
	Register(validRegisterReq("node-recent"))

	// node-old'u 11 dakika önce görülmüş gibi geriye tarihle (prune eşiği 10dk).
	oldTime := time.Now().UTC().Add(-11 * time.Minute)
	if _, err := db.Exec(`UPDATE federation_nodes SET last_seen = ? WHERE node_id = ?`, oldTime, "node-old"); err != nil {
		t.Fatalf("last_seen geri tarihleme: %v", err)
	}

	PruneInactive()

	old, _ := Get("node-old")
	if old.Status != "inactive" {
		t.Fatalf("10dk'dan eski node inactive olmalıydı, got=%s", old.Status)
	}
	recent, _ := Get("node-recent")
	if recent.Status != "active" {
		t.Fatalf("yeni görülen node active kalmalıydı, got=%s", recent.Status)
	}
}

// ─── ProbeHealth — mock HTTP sunucusuyla ───────────────────────────────────

func TestProbeHealth_HealthyServerReturnsTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/node/status" {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer srv.Close()

	healthy, latencyMs := ProbeHealth(NodeRecord{HTTPURL: srv.URL})
	if !healthy {
		t.Fatal("sağlıklı sunucu için healthy=true beklenirdi")
	}
	if latencyMs < 0 {
		t.Fatalf("latency negatif olamaz, got=%d", latencyMs)
	}
}

func TestProbeHealth_UnhealthySuccessFalseReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{"success": false})
	}))
	defer srv.Close()

	healthy, _ := ProbeHealth(NodeRecord{HTTPURL: srv.URL})
	if healthy {
		t.Fatal("success:false döndüren sunucu için healthy=false beklenirdi")
	}
}

func TestProbeHealth_MalformedJSONReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bu gecerli bir json degil {{{"))
	}))
	defer srv.Close()

	healthy, _ := ProbeHealth(NodeRecord{HTTPURL: srv.URL})
	if healthy {
		t.Fatal("bozuk JSON için healthy=false beklenirdi")
	}
}

func TestProbeHealth_UnreachableServerReturnsFalseAndZeroLatency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // sunucuyu kapat — artık ulaşılamaz

	healthy, latencyMs := ProbeHealth(NodeRecord{HTTPURL: url})
	if healthy {
		t.Fatal("ulaşılamayan sunucu için healthy=false beklenirdi")
	}
	if latencyMs != 0 {
		t.Fatalf("hata durumunda latency=0 beklenirdi, got=%d", latencyMs)
	}
}

func TestProbeHealth_EmptyHTTPURLReturnsFalseWithoutRequest(t *testing.T) {
	healthy, latencyMs := ProbeHealth(NodeRecord{HTTPURL: ""})
	if healthy || latencyMs != 0 {
		t.Fatalf("boş HTTPURL için (false,0) beklenirdi, got=(%v,%d)", healthy, latencyMs)
	}
}

// ─── StartHealthProbe entegrasyonu — gerçek probe döngüsü latency'yi yazıyor mu ─

func TestStartHealthProbe_UpdatesLatencyForHealthyNode(t *testing.T) {
	setupTestFederation(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer srv.Close()

	req := validRegisterReq("node-probed")
	req.HTTPURL = srv.URL
	Register(req)
	// Ayırt edici bir sentinel değer yaz — probe sonrası bunun DEĞİŞTİĞİNİ
	// doğrulayacağız (gerçek ölçüm 0ms bile çıksa "hiç yazılmadı" ile
	// "0ms ölçüldü" durumunu böyle ayırt ederiz).
	const sentinel = -999999
	if err := UpdateLatency("node-probed", sentinel); err != nil {
		t.Fatalf("sentinel UpdateLatency: %v", err)
	}

	// StartHealthProbe döngüsü İLK taramayı hemen yapar (30sn beklemeden) —
	// bu yüzden gerçek fonksiyonu çağırıp kısa bir an bekleyip iptal etmek
	// yeterli, 30sn'lik ticker'ı beklemeye gerek yok.
	ctx, cancel := context.WithCancel(context.Background())
	StartHealthProbe(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()

	got, _ := Get("node-probed")
	if got.LatencyMs == sentinel {
		t.Fatal("StartHealthProbe sağlıklı node'un latency'sini güncellemeliydi (sentinel hâlâ duruyor)")
	}
}
