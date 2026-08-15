package api_test

// L2 Tuğla 4e — Commit kalıcılığı.
//
// Sorun (launch-blocker): HandleMLSAddMember commit_b64'ü doğruluyor ama
// hiçbir yere YAZMIYORDU; yalnızca o an ONLINE olan üyelere WebSocket ile
// yayınlıyordu. Çevrimdışı MEVCUT bir üye (yeni katılan değil — o Welcome
// kuyruğundan besleniyor) commit'i kaçırınca epoch'u ilerletemiyor ve o
// epoch'tan sonraki HİÇBİR mesajı çözemiyor. Kalıcı, sessiz veri kaybı.
//
// Çözüm: commit de mls_messages'a yazılır (content_type='commit'), üye
// GET /v1/mls/group/{id}/messages ile geri çeker. Yeni tablo AÇILMADI —
// mls_messages.content_type kolonu (db/database.go 008_mls_messages) zaten
// vardı ve üzerinde CHECK kısıtı yok.
//
// Anayasa: yeni migration yok, CGO yok, auth bypass'ı yok (gerçek OTP → gerçek
// JWT → gerçek üyelik akışı; mls_group_members satırı elle YAZILMIYOR).

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"obscura.network/core/internal/db"
)

// mlsFetchedMessage — GET /v1/mls/group/{id}/messages yanıtının tek satırı.
// content_type alanı bu tuğlada eklendi: çağıran, commit'i application
// mesajından ayırt edemeden ikisini de aynı kuyruktan çekemez.
type mlsFetchedMessage struct {
	ID            string `json:"id"`
	SenderDID     string `json:"sender_did"`
	CiphertextB64 string `json:"ciphertext_b64"`
	ContentType   string `json:"content_type"`
	Epoch         int64  `json:"epoch"`
	CreatedAt     string `json:"created_at"`
}

func fetchMLSGroupMessages(t *testing.T, groupID, token string) []mlsFetchedMessage {
	t.Helper()
	r, code := get(t, "/v1/mls/group/"+groupID+"/messages", token)
	if code != 200 || !r.Success {
		t.Fatalf("grup mesajları okunamadı (code=%d): %s", code, r.Error)
	}
	var payload struct {
		GroupID  string              `json:"group_id"`
		Messages []mlsFetchedMessage `json:"messages"`
	}
	if err := json.Unmarshal(r.Data, &payload); err != nil {
		t.Fatalf("mesaj yanıtı parse edilemedi: %v", err)
	}
	return payload.Messages
}

func filterByContentType(msgs []mlsFetchedMessage, contentType string) []mlsFetchedMessage {
	var out []mlsFetchedMessage
	for _, m := range msgs {
		if m.ContentType == contentType {
			out = append(out, m)
		}
	}
	return out
}

// TestMLSCommitPersistedForOfflineMember — Faz 1'in kırmızısı.
//
// Senaryo: Alice grup kurar, Bob'u ekler (epoch 1), sonra Carol'ı ekler
// (epoch 2). Bob bu sırada ÇEVRİMDIŞI — hiçbir WebSocket bağlantısı yok, yani
// mls_handlers.go'daki broadcast onu hiç görmüyor. Bob geri döndüğünde
// Carol'ı ekleyen commit'i sunucudan çekebilmeli; yoksa epoch 2'ye
// geçemez ve sonraki mesajları çözemez.
func TestMLSCommitPersistedForOfflineMember(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905557770501", "mls_commit_alice")
	bobToken := loginAndRegister(t, "+905557770502", "mls_commit_bob")
	carolToken := loginAndRegister(t, "+905557770503", "mls_commit_carol")
	aliceDID := currentUserDID(t, aliceToken)
	bobDID := currentUserDID(t, bobToken)
	carolDID := currentUserDID(t, carolToken)

	groupID := base64.StdEncoding.EncodeToString([]byte("commit-persist-offline-member"))

	r, code := post(t, "/v1/mls/group", map[string]any{
		"group_id": groupID,
		"name":     "Tuğla 4e commit kalıcılığı",
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}

	// Opak wire yerine geçen sabitler — bu test kripto DEĞİL, taşıma katmanı
	// (relay byte-eşleşmesi Faz 3'te gerçek golden commit wire'ıyla yapılıyor).
	commitAddBob := base64.StdEncoding.EncodeToString([]byte("commit-epoch-1-adds-bob"))
	commitAddCarol := base64.StdEncoding.EncodeToString([]byte("commit-epoch-2-adds-carol"))

	r, code = post(t, "/v1/mls/group/"+groupID+"/add", map[string]any{
		"new_member_did": bobDID,
		"commit_b64":     commitAddBob,
		"welcome_b64":    base64.StdEncoding.EncodeToString([]byte("welcome-for-bob")),
		"new_epoch":      1,
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("Bob eklenemedi (code=%d): %s", code, r.Error)
	}

	// Bob buradan itibaren ÇEVRİMDIŞI kabul ediliyor (WS bağlantısı hiç
	// kurulmadı) — Carol'ı ekleyen commit'i canlı yayından alamaz.
	r, code = post(t, "/v1/mls/group/"+groupID+"/add", map[string]any{
		"new_member_did": carolDID,
		"commit_b64":     commitAddCarol,
		"welcome_b64":    base64.StdEncoding.EncodeToString([]byte("welcome-for-carol")),
		"new_epoch":      2,
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("Carol eklenemedi (code=%d): %s", code, r.Error)
	}

	// Alice epoch 2'de bir uygulama mesajı yollar — commit ile application
	// aynı kuyrukta ama content_type ile ayrışmalı.
	appCiphertext := base64.StdEncoding.EncodeToString([]byte("opak-uygulama-mesaji"))
	r, code = post(t, "/v1/mls/group/"+groupID+"/message", map[string]any{
		"ciphertext_b64": appCiphertext,
		"epoch":          2,
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("uygulama mesajı gönderilemedi (code=%d): %s", code, r.Error)
	}

	// ── Bob geri döner ve kaçırdıklarını çeker ──
	msgs := fetchMLSGroupMessages(t, groupID, bobToken)
	commits := filterByContentType(msgs, "commit")
	if len(commits) != 2 {
		t.Fatalf("2 commit bekleniyordu, %d geldi (toplam %d satır: %+v)\n"+
			"→ commit_b64 kalıcı değilse çevrimdışı üye epoch atlar ve sonraki mesajları ÇÖZEMEZ",
			len(commits), len(msgs), msgs)
	}

	if commits[0].Epoch != 1 || commits[1].Epoch != 2 {
		t.Fatalf("commit epoch sırası = [%d %d], beklenen [1 2]", commits[0].Epoch, commits[1].Epoch)
	}
	if commits[0].CiphertextB64 != commitAddBob {
		t.Errorf("epoch 1 commit wire'ı = %q, beklenen %q", commits[0].CiphertextB64, commitAddBob)
	}
	if commits[1].CiphertextB64 != commitAddCarol {
		t.Errorf("epoch 2 commit wire'ı = %q, beklenen %q", commits[1].CiphertextB64, commitAddCarol)
	}
	for i, c := range commits {
		if c.SenderDID != aliceDID {
			t.Errorf("commit[%d].sender_did = %q, beklenen komiter Alice %q", i, c.SenderDID, aliceDID)
		}
	}

	// Uygulama mesajı commit'lerle karışmamalı.
	apps := filterByContentType(msgs, "application")
	if len(apps) != 1 {
		t.Fatalf("1 application mesajı bekleniyordu, %d geldi", len(apps))
	}
	if apps[0].CiphertextB64 != appCiphertext {
		t.Errorf("application ciphertext = %q, beklenen %q", apps[0].CiphertextB64, appCiphertext)
	}

	// Carol da (yeni üye) aynı kuyruğu görebilmeli — üyelik kapısı commit
	// satırlarını gizlemiyor.
	if got := len(filterByContentType(fetchMLSGroupMessages(t, groupID, carolToken), "commit")); got != 2 {
		t.Errorf("Carol için 2 commit bekleniyordu, %d geldi", got)
	}

	t.Logf("✓ çevrimdışı üye Bob 2 commit'i (epoch 1, 2) sunucudan geri çekti; application mesajı ayrı")
}

// TestMLSCommitFetchOrdersByEpochDespiteClockSkew — Faz 2.
//
// Tuğla 3'ün "tek mesaj değil, çok mesaj ve SIRA" dersinin epoch versiyonu.
// Commit'ler MLS'te sıkı sıralıdır: epoch N+1'in commit'i, epoch N'inki
// uygulanmadan işlenemez. Sunucunun created_at'e göre sıralaması yeterli
// DEĞİL — düğümler arası saat kayması (veya aynı saniyeye düşen kayıtlar)
// sırayı bozar. Bu test saat kaymasını bilerek üretir: epoch 3'ün satırı
// epoch 1'inkinden ÖNCE bir created_at taşır.
func TestMLSCommitFetchOrdersByEpochDespiteClockSkew(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905557770504", "mls_skew_alice")
	bobToken := loginAndRegister(t, "+905557770505", "mls_skew_bob")
	bobDID := currentUserDID(t, bobToken)
	aliceDID := currentUserDID(t, aliceToken)

	groupID := base64.StdEncoding.EncodeToString([]byte("commit-order-clock-skew"))

	r, code := post(t, "/v1/mls/group", map[string]any{
		"group_id": groupID,
		"name":     "Tuğla 4e saat kayması",
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}
	r, code = post(t, "/v1/mls/group/"+groupID+"/add", map[string]any{
		"new_member_did": bobDID,
		"commit_b64":     base64.StdEncoding.EncodeToString([]byte("commit-bootstrap-adds-bob")),
		"welcome_b64":    base64.StdEncoding.EncodeToString([]byte("welcome-for-bob-skew")),
		"new_epoch":      1,
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("Bob eklenemedi (code=%d): %s", code, r.Error)
	}

	// Bob'un kaçırdığı 3 commit — SAAT KAYMASI simülasyonu: created_at sırası
	// epoch sırasının TAM TERSİ. Bu satırlar bilerek doğrudan DB'ye yazılıyor;
	// handler kendi created_at'ini üretir, ters saat üretemez.
	skewed := []struct {
		id        string
		epoch     int64
		createdAt string
	}{
		{"skew-commit-epoch-11", 11, "2026-08-14T10:00:30Z"}, // en GEÇ epoch, en ERKEN saat
		{"skew-commit-epoch-22", 22, "2026-08-14T10:00:20Z"},
		{"skew-commit-epoch-33", 33, "2026-08-14T10:00:10Z"},
	}
	// Ekleme sırası da karışık olsun ki rowid sıralaması "kazara doğru"
	// çıkmasın: 33, 11, 22.
	for _, i := range []int{2, 0, 1} {
		s := skewed[i]
		if _, err := db.DB.Exec(`
			INSERT INTO mls_messages (id, group_id, sender_did, ciphertext_b64, content_type, epoch, created_at)
			VALUES (?, ?, ?, ?, 'commit', ?, ?)`,
			s.id, groupID, aliceDID,
			base64.StdEncoding.EncodeToString([]byte(s.id)),
			s.epoch, s.createdAt,
		); err != nil {
			t.Fatalf("saat kaymalı commit satırı yazılamadı (%s): %v", s.id, err)
		}
	}

	msgs := fetchMLSGroupMessages(t, groupID, bobToken)
	commits := filterByContentType(msgs, "commit")

	var gotEpochs []int64
	for _, c := range commits {
		if c.Epoch >= 11 { // bootstrap commit'i (epoch 1) hariç
			gotEpochs = append(gotEpochs, c.Epoch)
		}
	}
	want := []int64{11, 22, 33}
	if len(gotEpochs) != len(want) {
		t.Fatalf("3 saat kaymalı commit bekleniyordu, %d geldi (%+v)", len(gotEpochs), commits)
	}
	for i := range want {
		if gotEpochs[i] != want[i] {
			t.Fatalf("commit sırası = %v, beklenen %v — created_at karışıklığına rağmen epoch sırası korunmalı\n"+
				"→ ORDER BY created_at tek başına YETMEZ (saat kayması sırayı bozar)", gotEpochs, want)
		}
	}

	// Bütün liste genelinde epoch monoton artmalı (tie-break created_at).
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Epoch < msgs[i-1].Epoch {
			t.Fatalf("liste epoch'a göre monoton değil: index %d epoch=%d < index %d epoch=%d",
				i, msgs[i].Epoch, i-1, msgs[i-1].Epoch)
		}
	}

	t.Logf("✓ created_at sırası %v iken epoch sırası %v döndü — birincil sıralama anahtarı epoch",
		[]string{skewed[2].createdAt, skewed[1].createdAt, skewed[0].createdAt}, gotEpochs)
}
