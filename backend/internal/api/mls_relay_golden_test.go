package api_test

// L2 Tuğla 4d — E1 kanıtı: DONMUŞ golden MLS wire, GERÇEK backend relay'inden
// geçip openmls tarafından çözülüyor. Üç halka da gerçek:
//
//	golden wire (Tuğla 2A'da ts-mls ile üretilip donduruldu)
//	  → gerçek Go relay (httptest + main.go'nun route bloğu + AuthMiddleware
//	    + gerçek SQLite test DB + gerçek OTP/JWT akışı)
//	  → openmls (crypto crate'inde Tuğla 3'ün kanıt gövdesi, cargo alt-süreci)
//
// Tuğla 4c'den farkı: orada relay bir JS mock'tu ve `setActor` ile kimlik
// taklit ediliyordu. Burada relay GERÇEK mls_handlers.go, kimlik GERÇEK Bearer
// JWT, üyelik GERÇEK add-member akışıyla oluşuyor (mls_group_members satırı
// elle YAZILMIYOR).
//
// Anayasa: CGO yok (modernc.org/sqlite + cargo alt-süreci), yeni FFI yok,
// auth bypass'ı yok.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	// Tuğla 2A'da ts-mls ile üretilip DONDURULMUŞ fixture — bu test onu
	// yalnızca OKUR, asla yeniden üretmez.
	goldenFixturePath = "../../../mobile/lib/mls/fixtures/two_party_golden.json"
	cryptoCrateDir    = "../../../crypto"

	// Rust tarafındaki karşılığı: crypto/src/mls/tests.rs
	relayedFixtureEnv = "OBSCURA_MLS_RELAYED_FIXTURE"
	relayedRustTest   = "mls::tests::interop_openmls_decrypts_relayed_golden_wire"

	// Golden fixture'ın MLS group_id'si (application-message wire'ının içinde
	// düz metin olarak taşınır; aşağıda doğrulanıyor). Backend'e giden
	// group_id bunun base64'ü — mls-e2e-mock-relay.test.ts'teki (Tuğla 4c)
	// `Buffer.from(groupIdBytes).toString("base64")` sözleşmesiyle aynı.
	goldenMLSGroupID = "obscura-golden-group"

	// Golden fixture'ın Commit wire'ı (Tuğla 4e'de eklendi): Bob, golden
	// Welcome'dan gruba katıldıktan sonra AYNI grupta (obscura-golden-group)
	// epoch 1 → 2 geçişini yapan gerçek Commit'i üretti. Tuğla 4d yazıldığında
	// fixture'da böyle bir alan yoktu ve handler commit'i hiçbir yere
	// yazmadığı için geri okunamıyordu; ikisi de 4e'de düzeldi, buradaki
	// "PLACEHOLDER" sabiti kalktı.
	//
	// B10.2 Tuğla 1 güncellemesi: fixture'ın KENDİ crypto tarihinde bu commit
	// epoch 1→2 geçişiydi (golden session'da grup bu commit'ten ÖNCE zaten
	// epoch 1'deydi — muhtemelen ayrı bir bootstrap commit'i vardı, bu testte
	// modellenmedi). Ama BU test sunucuda TAZE bir grup açıyor (HandleMLSCreateGroup
	// → epoch=0) ve bu add-çağrısı o grubun sunucu tarafından görülen İLK
	// commit'i — advanceGroupEpoch'un CAS'ı (mls_handlers.go) artık
	// expectedOld=newEpoch-1'i zorluyor, yani sunucu tarafında geçerli tek
	// değer new_epoch=1 (0→1). Bu SADECE sunucunun epoch SAYAÇ metadata'sı;
	// fixture'ın donmuş commit/welcome WIRE BYTE'LARI (CommitB64,
	// WelcomeWireB64) değişmedi ve HandleMLSGroupMessage (application mesajı,
	// aşağıda fixture.Epoch ile gönderiliyor) bu sayaca hiç bakmıyor — relay
	// byte-eşleşme iddiası (bu testin asıl amacı) etkilenmiyor.
	goldenCommitEpoch = 1
)

// goldenFixture — two_party_golden.json'un bu testte kullanılan alanları.
type goldenFixture struct {
	Plaintext                 string `json:"plaintext"`
	AliceKeyPackageWireB64    string `json:"alice_key_package_wire_b64"`
	BobKeyPackageWireB64      string `json:"bob_key_package_wire_b64"`
	WelcomeWireB64            string `json:"welcome_wire_b64"`
	ApplicationMessageWireB64 string `json:"application_message_wire_b64"`
	CommitB64                 string `json:"commit_b64"`
	Epoch                     uint64 `json:"epoch"`
}

// loadGoldenFixture — fixture'ı hem tiplenmiş hem ham (alan alan) okur.
// Ham hâli, relay'den dönen wire'lar yerine konarak Rust'a verilecek geçici
// fixture'ı üretmek için gerekir (diğer alanlar birebir korunur).
func loadGoldenFixture(t *testing.T) (goldenFixture, map[string]json.RawMessage) {
	t.Helper()
	raw, err := os.ReadFile(goldenFixturePath)
	if err != nil {
		t.Fatalf("golden fixture okunamadı (%s): %v", goldenFixturePath, err)
	}
	var typed goldenFixture
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatalf("golden fixture parse edilemedi: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("golden fixture ham parse edilemedi: %v", err)
	}
	if typed.BobKeyPackageWireB64 == "" || typed.WelcomeWireB64 == "" ||
		typed.ApplicationMessageWireB64 == "" || typed.Plaintext == "" ||
		typed.CommitB64 == "" {
		t.Fatal("golden fixture beklenen wire alanlarını içermiyor")
	}
	return typed, fields
}

// assertWireIdentical — bu ticket'ın çekirdeği: POST edilen değerle GET'ten
// dönen değer BYTE-BYTE aynı mı. "istek başarılı oldu" yetmez. Eşleşmezse
// hatanın NEREDE olduğunu (kırpma, whitespace, TEXT truncation, yeniden
// kodlama) ayırt edecek tanı bilgisi basar.
func assertWireIdentical(t *testing.T, label, sent, got string) {
	t.Helper()

	if sent == got {
		// Base64 metni aynı — çözülmüş byte'lar da aynı olmalı (paranoyak
		// kontrol: farklıysa base64 alfabesi/padding sorunu vardır).
		sentBytes, errS := base64.StdEncoding.DecodeString(sent)
		gotBytes, errG := base64.StdEncoding.DecodeString(got)
		if errS != nil || errG != nil {
			t.Fatalf("%s: base64 çözülemedi (sent=%v got=%v)", label, errS, errG)
		}
		if !bytes.Equal(sentBytes, gotBytes) {
			t.Fatalf("%s: base64 metinleri aynı ama çözülmüş byte'lar farklı (imkânsıza yakın)", label)
		}
		t.Logf("✓ %s byte-eşleşme: %d base64 karakter / %d ham byte birebir aynı",
			label, len(sent), len(sentBytes))
		return
	}

	// ── Eşleşmedi: nedenini daralt ──
	var diag []string
	diag = append(diag, "gönderilen uzunluk="+strconv.Itoa(len(sent))+" alınan uzunluk="+strconv.Itoa(len(got)))

	switch {
	case strings.TrimSpace(got) == sent:
		diag = append(diag, "NEDEN: alınan değerde baştaki/sondaki whitespace var (trim edilince eşleşiyor)")
	case len(got) < len(sent) && strings.HasPrefix(sent, got):
		diag = append(diag, "NEDEN: alınan değer KIRPILMIŞ (gönderilenin ön eki) — DB kolon uzunluğu/TEXT truncation şüphesi")
	case len(got) > len(sent) && strings.HasPrefix(got, sent):
		diag = append(diag, "NEDEN: alınan değere sonradan ek yapılmış")
	case strings.EqualFold(got, sent):
		diag = append(diag, "NEDEN: yalnızca harf büyüklüğü farklı — bir yerde case dönüşümü var")
	default:
		for i := 0; i < len(sent) && i < len(got); i++ {
			if sent[i] != got[i] {
				diag = append(diag, "İLK FARKLI İNDEKS="+strconv.Itoa(i)+
					" gönderilen="+quoteByte(sent[i])+" alınan="+quoteByte(got[i]))
				break
			}
		}
	}

	t.Fatalf("%s BYTE-EŞLEŞME BAŞARISIZ\n  %s\n  gönderilen(ilk 80)=%s\n  alınan(ilk 80)=%s",
		label, strings.Join(diag, "\n  "), head(sent, 80), head(got, 80))
}

func quoteByte(b byte) string {
	q, _ := json.Marshal(string(b))
	return string(q)
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// TestMLSGoldenWireSurvivesRealRelay — E1'in uçtan uca kanıtı.
func TestMLSGoldenWireSurvivesRealRelay(t *testing.T) {
	fixture, fields := loadGoldenFixture(t)

	// ── Gerçek kullanıcılar: gerçek OTP akışı → gerçek imzalı JWT ──
	// (createTestUserAndToken gibi OTP'yi atlayan kısa yol KULLANILMIYOR.)
	aliceToken := loginAndRegister(t, "+905557770401", "mls_relay_alice")
	bobToken := loginAndRegister(t, "+905557770402", "mls_relay_bob")
	aliceDID := currentUserDID(t, aliceToken)
	bobDID := currentUserDID(t, bobToken)

	// Backend group_id = MLS group_id'nin base64'ü (Tuğla 4c sözleşmesi).
	groupIDBytes := []byte(goldenMLSGroupID)
	groupID := base64.StdEncoding.EncodeToString(groupIDBytes)
	if strings.ContainsAny(groupID, "/") {
		t.Fatalf("group_id base64'ü '/' içeriyor, URL path'e gömülemez: %s", groupID)
	}

	// Kullandığımız group_id gerçekten golden wire'ların taşıdığı grup mu?
	// (Commit için de aynı kontrol: 4e'de eklenen commit_b64 başka bir koşudan
	// kopyalanmış yabancı bir blob olsaydı bu kontrol yakalardı.)
	for _, w := range []struct{ label, wire string }{
		{"application-message", fixture.ApplicationMessageWireB64},
		{"commit", fixture.CommitB64},
	} {
		wireBytes, err := base64.StdEncoding.DecodeString(w.wire)
		if err != nil {
			t.Fatalf("%s wire base64 çözülemedi: %v", w.label, err)
		}
		if !bytes.Contains(wireBytes, groupIDBytes) {
			t.Fatalf("golden %s wire'ı %q group_id'sini taşımıyor — yanlış grup kimliğiyle relay ediyoruz", w.label, goldenMLSGroupID)
		}
	}

	// ── 1) Alice ve Bob KeyPackage'larını yayınlar ──
	for _, kp := range []struct {
		who   string
		token string
		wire  string
	}{
		{"alice", aliceToken, fixture.AliceKeyPackageWireB64},
		{"bob", bobToken, fixture.BobKeyPackageWireB64},
	} {
		r, code := post(t, "/v1/mls/key-package", map[string]any{
			"key_package_b64": kp.wire,
		}, kp.token)
		if code != 200 || !r.Success {
			t.Fatalf("%s KeyPackage yüklenemedi (code=%d): %s", kp.who, code, r.Error)
		}
	}

	// ── 2) Alice, Bob'un KeyPackage'ını çeker (tek kullanımlık) ──
	r, code := get(t, "/v1/mls/key-package/"+bobDID, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("Bob KeyPackage çekilemedi (code=%d): %s", code, r.Error)
	}
	var kpResp struct {
		KeyPackageB64 string `json:"key_package_b64"`
		TargetDID     string `json:"target_did"`
	}
	if err := json.Unmarshal(r.Data, &kpResp); err != nil {
		t.Fatalf("KeyPackage yanıtı parse edilemedi: %v", err)
	}
	assertWireIdentical(t, "Bob KeyPackage wire (POST → GET)",
		fixture.BobKeyPackageWireB64, kpResp.KeyPackageB64)

	// ── 3) Alice grubu kaydeder (kurucu olarak ilk üye satırı burada oluşur) ──
	r, code = post(t, "/v1/mls/group", map[string]any{
		"group_id": groupID,
		"name":     "Tuğla 4d golden relay",
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}

	// ── 4) Alice, Bob'u ekler: Welcome + Commit relay'e yazılır, üyelik
	// GERÇEKTEN oluşur. Commit artık placeholder değil, golden grubun kendi
	// epoch 1 → 2 Commit wire'ı (Tuğla 4e).
	r, code = post(t, "/v1/mls/group/"+groupID+"/add", map[string]any{
		"new_member_did": bobDID,
		"commit_b64":     fixture.CommitB64,
		"welcome_b64":    fixture.WelcomeWireB64,
		"new_epoch":      goldenCommitEpoch,
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("üye eklenemedi (code=%d): %s", code, r.Error)
	}

	// ── 5) Bob bekleyen Welcome'ını çeker ──
	r, code = get(t, "/v1/mls/welcomes", bobToken)
	if code != 200 || !r.Success {
		t.Fatalf("Welcome kuyruğu okunamadı (code=%d): %s", code, r.Error)
	}
	var welcomes []struct {
		ID         string `json:"id"`
		GroupID    string `json:"group_id"`
		WelcomeB64 string `json:"welcome_b64"`
	}
	if err := json.Unmarshal(r.Data, &welcomes); err != nil {
		t.Fatalf("Welcome yanıtı parse edilemedi: %v", err)
	}
	if len(welcomes) != 1 {
		t.Fatalf("Bob için 1 Welcome beklendi, %d geldi", len(welcomes))
	}
	if welcomes[0].GroupID != groupID {
		t.Errorf("Welcome group_id = %q, beklenen %q", welcomes[0].GroupID, groupID)
	}
	assertWireIdentical(t, "Welcome wire (POST /add → GET /welcomes)",
		fixture.WelcomeWireB64, welcomes[0].WelcomeB64)

	// ── 6) Alice şifreli application-message'ı relay'e yollar ──
	r, code = post(t, "/v1/mls/group/"+groupID+"/message", map[string]any{
		"ciphertext_b64": fixture.ApplicationMessageWireB64,
		"epoch":          fixture.Epoch,
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("grup mesajı gönderilemedi (code=%d): %s", code, r.Error)
	}

	// ── 7) Bob kuyruğu geri okur: hem application mesajı hem Commit ──
	// Commit'in de burada olması Tuğla 4e'nin çekirdeği: çevrimdışıyken WS
	// yayınını kaçıran mevcut üye epoch geçişini ancak buradan alabilir.
	msgs := fetchMLSGroupMessages(t, groupID, bobToken)
	if len(msgs) != 2 {
		t.Fatalf("2 kayıt beklendi (1 application + 1 commit), %d geldi: %+v", len(msgs), msgs)
	}
	// Sıra epoch'a göre: application epoch 1, commit epoch 2.
	if msgs[0].Epoch > msgs[1].Epoch {
		t.Fatalf("kuyruk epoch'a göre sıralı değil: %d, %d", msgs[0].Epoch, msgs[1].Epoch)
	}

	apps := filterByContentType(msgs, "application")
	commits := filterByContentType(msgs, "commit")
	if len(apps) != 1 || len(commits) != 1 {
		t.Fatalf("1 application + 1 commit beklendi, %d/%d geldi: %+v", len(apps), len(commits), msgs)
	}
	if apps[0].SenderDID != aliceDID {
		t.Errorf("application sender_did = %q, beklenen %q", apps[0].SenderDID, aliceDID)
	}
	if commits[0].SenderDID != aliceDID {
		t.Errorf("commit sender_did = %q, beklenen komiter %q", commits[0].SenderDID, aliceDID)
	}
	if commits[0].Epoch != goldenCommitEpoch {
		t.Errorf("commit epoch = %d, beklenen %d", commits[0].Epoch, goldenCommitEpoch)
	}
	assertWireIdentical(t, "Application-message ciphertext (POST → GET)",
		fixture.ApplicationMessageWireB64, apps[0].CiphertextB64)
	assertWireIdentical(t, "Commit wire (POST /add → GET /messages)",
		fixture.CommitB64, commits[0].CiphertextB64)

	// ── 8) openmls, relay'den GERİ OKUNAN wire'ları çözüyor mu ──
	// Rust'a verilen fixture: golden'ın birebir kopyası, ama wire alanları
	// relay'den dönen değerlerle DEĞİŞTİRİLMİŞ. Özel anahtarlar (Bob'un init/
	// hpke/signature) golden'dan gelir — onlar hiçbir zaman relay'e girmez.
	fields["bob_key_package_wire_b64"] = mustJSONString(t, kpResp.KeyPackageB64)
	fields["welcome_wire_b64"] = mustJSONString(t, welcomes[0].WelcomeB64)
	fields["application_message_wire_b64"] = mustJSONString(t, apps[0].CiphertextB64)
	fields["commit_b64"] = mustJSONString(t, commits[0].CiphertextB64)
	runOpenMLSDecodeOfRelayedWire(t, fields, fixture.Plaintext)
}

func mustJSONString(t *testing.T, s string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("string JSON'a çevrilemedi: %v", err)
	}
	return b
}

// runOpenMLSDecodeOfRelayedWire — relay'den dönen wire'ları openmls'e çözdürür.
// Köprü: cargo alt-süreci (subprocess deseni; yeni FFI/CGO yok). Çalıştırdığı
// test Tuğla 3'ün kanıt gövdesinin ta kendisidir — tek farkı wire'ların artık
// relay'den gelmesi.
func runOpenMLSDecodeOfRelayedWire(t *testing.T, fields map[string]json.RawMessage, expectedPlaintext string) {
	t.Helper()

	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skipf("cargo PATH'te yok — openmls çözme halkası atlanıyor (Go relay halkaları yine de doğrulandı): %v", err)
	}

	blob, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatalf("relay fixture serileştirilemedi: %v", err)
	}
	fixturePath := filepath.Join(t.TempDir(), "relayed_two_party.json")
	if err := os.WriteFile(fixturePath, blob, 0o600); err != nil {
		t.Fatalf("relay fixture yazılamadı: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, cargo, "test", "--lib", relayedRustTest,
		"--", "--exact", "--nocapture")
	cmd.Dir = cryptoCrateDir
	absFixture, err := filepath.Abs(fixturePath)
	if err != nil {
		t.Fatalf("fixture mutlak yolu alınamadı: %v", err)
	}
	cmd.Env = append(os.Environ(), relayedFixtureEnv+"="+absFixture)

	out, runErr := cmd.CombinedOutput()
	output := string(out)
	if runErr != nil {
		t.Fatalf("openmls çözme başarısız (%v)\n%s", runErr, output)
	}
	// Test env yokken kendini no-op yapıyor — sessizce "geçti" sanmayalım.
	if strings.Contains(output, "[atlandı]") {
		t.Fatalf("Rust testi fixture'ı görmedi (env geçmemiş)\n%s", output)
	}
	if !strings.Contains(output, "1 passed") {
		t.Fatalf("Rust testi geçti raporlamadı\n%s", output)
	}
	if !strings.Contains(output, "[çözüldü] plaintext = "+expectedPlaintext) {
		t.Fatalf("çözülen plaintext golden ile eşleşmedi (beklenen %q)\n%s", expectedPlaintext, output)
	}
	t.Logf("✓ openmls, relay'den dönen wire'ı çözdü — plaintext = %q", expectedPlaintext)
}

// TestMLSGroupMessageRejectsEmptyCiphertext — negatif kontrol. Doğrulamanın
// GERÇEK handler yolunda (mls_handlers.go) çalıştığını, mock taklidinde değil,
// gerçek üyelik durumu + gerçek JWT ile kanıtlar.
func TestMLSGroupMessageRejectsEmptyCiphertext(t *testing.T) {
	token := loginAndRegister(t, "+905557770403", "mls_relay_empty")
	groupID := base64.StdEncoding.EncodeToString([]byte("empty-ciphertext-negative-control"))

	// Gerçek üyelik: kurucu olarak gruba kaydolur (mls_group_members satırı
	// elle YAZILMIYOR, handler yazıyor).
	r, code := post(t, "/v1/mls/group", map[string]any{
		"group_id": groupID,
		"name":     "Negatif kontrol",
	}, token)
	if code != 200 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}

	r, code = post(t, "/v1/mls/group/"+groupID+"/message", map[string]any{
		"ciphertext_b64": "",
		"epoch":          1,
	}, token)

	if code != 400 {
		t.Fatalf("HTTP 400 beklendi, %d geldi (body error=%q)", code, r.Error)
	}
	if r.Success {
		t.Error("success=false beklendi")
	}
	if r.Error != "ciphertext_b64 zorunlu" {
		t.Errorf("hata mesajı = %q, beklenen %q", r.Error, "ciphertext_b64 zorunlu")
	}
	t.Logf("✓ negatif kontrol: HTTP %d, body error=%q", code, r.Error)
}

// TestMLSGroupMessageRejectsNonMember — üyelik kapısının gerçekten çalıştığını
// gösterir; testte üyelik satırı uydurulmadığının kanıtı.
func TestMLSGroupMessageRejectsNonMember(t *testing.T) {
	ownerToken := loginAndRegister(t, "+905557770404", "mls_relay_owner")
	outsiderToken := loginAndRegister(t, "+905557770405", "mls_relay_outsider")
	groupID := base64.StdEncoding.EncodeToString([]byte("non-member-negative-control"))

	r, code := post(t, "/v1/mls/group", map[string]any{
		"group_id": groupID,
		"name":     "Üyelik kapısı",
	}, ownerToken)
	if code != 200 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}

	r, code = post(t, "/v1/mls/group/"+groupID+"/message", map[string]any{
		"ciphertext_b64": base64.StdEncoding.EncodeToString([]byte("opak")),
		"epoch":          1,
	}, outsiderToken)

	if code != 403 {
		t.Fatalf("HTTP 403 beklendi, %d geldi (body error=%q)", code, r.Error)
	}
	if r.Error != "Grubun üyesi değilsiniz" {
		t.Errorf("hata mesajı = %q, beklenen %q", r.Error, "Grubun üyesi değilsiniz")
	}
}
