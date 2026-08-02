package federation

// Tests for federation.go (permissionless node registry CRUD + health probe).
// Previously zero test coverage. db/mu are package-level singletons (same
// pattern as sequencer.Global) — each test calls Init() with a fresh
// in-memory SQLite DB to avoid cross-test state leakage (tests run
// sequentially within the package, no t.Parallel()).

import (
	"context"
	"database/sql"
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
