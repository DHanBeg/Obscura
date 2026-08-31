package api_test

// B10.2 Tuğla 1 — Epoch CAS. Faz 0'da bulunan açık: HandleMLSAddMember/
// HandleMLSCommitProposal/HandleMLSRemoveMember/HandleMlsUpdateKey hepsi
// `UPDATE mls_groups SET epoch = ? WHERE id = ?` ile KOŞULSUZ yazıyordu —
// iki eş-zamanlı istek aynı eski epoch'tan commit hesaplayıp ikisi de
// newEpoch=N+1 ile POST ederse ikisi de 200 alırdı (son yazan kazanır,
// çatallanma). advanceGroupEpoch (mls_handlers.go) artık
// `WHERE id = ? AND epoch = ?` (expectedOld = newEpoch-1) CAS'ı uyguluyor.
//
// Bu test derleniyor olmasını DEĞİL, eş-zamanlılık DAVRANIŞINI kanıtlıyor:
// gerçek HTTP sunucusuna gerçek goroutine'lerden aynı anda iki istek atılır.
//
// DİKKAT: post()/get() yardımcıları t.Fatalf çağırıyor — Go testing kuralı
// gereği t.FailNow (Fatal'ın alt katmanı) yalnızca test goroutine'inden
// çağrılabilir. Bu yüzden ırk (race) goroutine'leri ham http.Client kullanır,
// t.Fatal'ı SADECE ana goroutine'de (join sonrası) çağırır.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
)

type rawResp struct {
	code int
	body testResp
	err  error
}

// rawPost — post() ile AYNI sözleşme, ama t.Fatal YOK (goroutine-güvenli).
func rawPost(path string, body interface{}, token, xff string) rawResp {
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", testServer.URL+path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", xff)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rawResp{err: err}
	}
	defer resp.Body.Close()
	var tr testResp
	_ = json.NewDecoder(resp.Body).Decode(&tr)
	return rawResp{code: resp.StatusCode, body: tr}
}

// TestMLSEpochCASRejectsConcurrentCommits — (a)+(b)+(c).
//
// Senaryo: Alice grubu kurar (epoch 0), TEK oturumdan aynı anda Bob'u ve
// Carol'ı eklemeye çalışır — iki client de "current epoch benim bildiğim
// 0, ben 1'e ilerletiyorum" varsayımıyla new_epoch=1 gönderiyor (tam Faz 0
// raporundaki çatallanma senaryosu). CAS'siz halde ikisi de 200 dönerdi.
func TestMLSEpochCASRejectsConcurrentCommits(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905557780601", "cas_alice")
	bobToken := loginAndRegister(t, "+905557780602", "cas_bob")
	carolToken := loginAndRegister(t, "+905557780603", "cas_carol")
	bobDID := currentUserDID(t, bobToken)
	carolDID := currentUserDID(t, carolToken)

	groupID := base64.StdEncoding.EncodeToString([]byte("epoch-cas-race-group"))

	r, code := post(t, "/v1/mls/group", map[string]any{
		"group_id": groupID,
		"name":     "Tuğla 1 CAS testi",
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}

	commitAddBob := base64.StdEncoding.EncodeToString([]byte("race-commit-adds-bob"))
	commitAddCarol := base64.StdEncoding.EncodeToString([]byte("race-commit-adds-carol"))

	var wg sync.WaitGroup
	results := make([]rawResp, 2)
	start := make(chan struct{})

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results[0] = rawPost("/v1/mls/group/"+groupID+"/add", map[string]any{
			"new_member_did": bobDID,
			"commit_b64":     commitAddBob,
			"welcome_b64":    base64.StdEncoding.EncodeToString([]byte("welcome-for-bob-race")),
			"new_epoch":      1,
		}, aliceToken, "cas-race-bob")
	}()
	go func() {
		defer wg.Done()
		<-start
		results[1] = rawPost("/v1/mls/group/"+groupID+"/add", map[string]any{
			"new_member_did": carolDID,
			"commit_b64":     commitAddCarol,
			"welcome_b64":    base64.StdEncoding.EncodeToString([]byte("welcome-for-carol-race")),
			"new_epoch":      1,
		}, aliceToken, "cas-race-carol")
	}()
	close(start) // ikisini AYNI ANDA serbest bırak
	wg.Wait()

	for i, res := range results {
		if res.err != nil {
			t.Fatalf("goroutine[%d] HTTP hatası: %v", i, res.err)
		}
	}

	// (a) Biri 200, diğeri 409 — İKİ 200 GÖRÜRSEN FIX ÇALIŞMIYOR.
	var winner, loser *rawResp
	var winnerIsBob bool
	codes := []int{results[0].code, results[1].code}
	switch {
	case codes[0] == 200 && codes[1] == 409:
		winner, loser, winnerIsBob = &results[0], &results[1], true
	case codes[0] == 409 && codes[1] == 200:
		winner, loser, winnerIsBob = &results[1], &results[0], false
	default:
		t.Fatalf("beklenen [200,409] veya [409,200], geldi %v (gövdeler: bob=%+v carol=%+v) — "+
			"iki 200 = CAS ÇALIŞMIYOR, iki 409 = CAS aşırı agresif/bozuk",
			codes, results[0].body, results[1].body)
	}
	t.Logf("kazanan=%s (code=%d), kaybeden code=%d", map[bool]string{true: "bob-ekleme", false: "carol-ekleme"}[winnerIsBob], winner.code, loser.code)

	if winner.body.Success == false {
		t.Fatalf("winner (200) success=false olamaz: %+v", winner.body)
	}
	var winnerData struct {
		NewEpoch int64 `json:"new_epoch"`
	}
	_ = json.Unmarshal(winner.body.Data, &winnerData)
	if winnerData.NewEpoch != 1 {
		t.Errorf("winner new_epoch = %d, beklenen 1", winnerData.NewEpoch)
	}

	// (c) 409 gövdesi mevcut (kazanan) epoch'u taşımalı — re-sync sözleşmesi.
	if loser.body.Success {
		t.Fatalf("loser (409) success=true olamaz: %+v", loser.body)
	}
	var loserData struct {
		Error        string `json:"error"`
		CurrentEpoch int64  `json:"current_epoch"`
	}
	if err := json.Unmarshal(loser.body.Data, &loserData); err != nil {
		t.Fatalf("409 gövdesi parse edilemedi: %v (ham: %s)", err, string(loser.body.Data))
	}
	if loserData.Error != "stale_epoch" {
		t.Errorf("409 error alanı = %q, beklenen \"stale_epoch\"", loserData.Error)
	}
	if loserData.CurrentEpoch != 1 {
		t.Errorf("409 current_epoch = %d, beklenen 1 (kazananın epoch'u) — client bunu görmeden re-sync edemez", loserData.CurrentEpoch)
	}

	// Sunucu grup state'i tutarlı mı — GET /v1/mls/group/{id}.epoch == 1 (2 değil).
	infoResp, infoCode := get(t, "/v1/mls/group/"+groupID, aliceToken)
	if infoCode != 200 || !infoResp.Success {
		t.Fatalf("grup bilgisi okunamadı (code=%d): %s", infoCode, infoResp.Error)
	}
	var info struct {
		Epoch int64 `json:"epoch"`
	}
	_ = json.Unmarshal(infoResp.Data, &info)
	if info.Epoch != 1 {
		t.Fatalf("grup epoch = %d, beklenen 1 (iki commit birbirini ezmiş olabilir)", info.Epoch)
	}

	// (b) mls_messages'ta epoch=1 için TEK commit satırı — UNIQUE tuttu, ve
	// içeriği KAZANANIN commit'i olmalı (kaybedenin commit'i asla yazılmadı,
	// tx CAS'ta rollback oldu).
	commits := filterByContentType(fetchMLSGroupMessages(t, groupID, aliceToken), "commit")
	var epoch1Commits []mlsFetchedMessage
	for _, c := range commits {
		if c.Epoch == 1 {
			epoch1Commits = append(epoch1Commits, c)
		}
	}
	if len(epoch1Commits) != 1 {
		t.Fatalf("epoch=1 için 1 commit satırı bekleniyordu, %d geldi: %+v — UNIQUE(group_id,epoch) tutmuyor",
			len(epoch1Commits), epoch1Commits)
	}
	wantCommit := commitAddCarol
	if winnerIsBob {
		wantCommit = commitAddBob
	}
	if epoch1Commits[0].CiphertextB64 != wantCommit {
		t.Errorf("kalıcılaşan commit = %q, beklenen kazananınki %q", epoch1Commits[0].CiphertextB64, wantCommit)
	}

	// (d) Regresyon — CAS meşru ardışık ilerlemeyi engellemiyor. Kaybedenin
	// client'ı re-sync ettikten sonra (gerçek akış Tuğla 2/3'te) sıradaki
	// üyeyi epoch=2 ile eklemeyi dener — bu normal, sıralı bir sonraki adım,
	// 200 dönmeli.
	thirdMemberToken := loginAndRegister(t, "+905557780604", "cas_dave")
	daveDID := currentUserDID(t, thirdMemberToken)
	r2, code2 := post(t, "/v1/mls/group/"+groupID+"/add", map[string]any{
		"new_member_did": daveDID,
		"commit_b64":     base64.StdEncoding.EncodeToString([]byte("followup-commit-adds-dave")),
		"welcome_b64":    base64.StdEncoding.EncodeToString([]byte("welcome-for-dave")),
		"new_epoch":      2,
	}, aliceToken)
	if code2 != 200 || !r2.Success {
		t.Fatalf("regresyon: sıralı ardışık add (epoch 1→2) başarısız olmamalıydı (code=%d): %s", code2, r2.Error)
	}

	t.Logf("✓ eş-zamanlı iki commit (aynı base epoch=0) → 1x200 + 1x409, epoch çatallanmadı, ardışık ilerleme kesintisiz sürdü")
}

// TestMLSEpochCASSequentialFlowUnaffected — (d)'nin ayrı, dar testi: CAS
// eklenmeden ÖNCE de yeşil olan sıralı add/commit/remove akışlarının hâlâ
// yeşil olduğunu doğrular (mevcut mls_commit_persist_test.go'daki senaryonun
// CAS sonrası tekrarı — regresyon yoksa bu zaten dolaylı kanıtlanıyor, burada
// commit+remove yollarını da (add'e ek olarak) tek testte ayrıca doğruluyoruz).
func TestMLSEpochCASSequentialFlowUnaffected(t *testing.T) {
	aliceToken := loginAndRegister(t, "+905557780701", "cas_seq_alice")
	bobToken := loginAndRegister(t, "+905557780702", "cas_seq_bob")
	bobDID := currentUserDID(t, bobToken)

	groupID := base64.StdEncoding.EncodeToString([]byte("epoch-cas-sequential-group"))

	r, code := post(t, "/v1/mls/group", map[string]any{"group_id": groupID, "name": "seq"}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("grup oluşturulamadı (code=%d): %s", code, r.Error)
	}

	// epoch 0 → 1: add
	r, code = post(t, "/v1/mls/group/"+groupID+"/add", map[string]any{
		"new_member_did": bobDID,
		"commit_b64":     base64.StdEncoding.EncodeToString([]byte("seq-commit-add-bob")),
		"welcome_b64":    base64.StdEncoding.EncodeToString([]byte("seq-welcome-bob")),
		"new_epoch":      1,
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("sıralı add (epoch 0→1) başarısız (code=%d): %s", code, r.Error)
	}

	// epoch 1 → 2: update-key commit (proposal_type=update, client-supplied commit)
	r, code = post(t, "/v1/mls/group/"+groupID+"/commit", map[string]any{
		"commit_b64":    base64.StdEncoding.EncodeToString([]byte("seq-commit-update")),
		"new_epoch":      2,
		"proposal_type": "update",
	}, aliceToken)
	if code != 200 || !r.Success {
		t.Fatalf("sıralı commit (epoch 1→2) başarısız (code=%d): %s", code, r.Error)
	}

	// epoch 2 → 3: remove. post()/get() yardımcıları DELETE desteklemiyor,
	// ham istekle atılıyor.
	req, _ := http.NewRequest("DELETE", testServer.URL+"/v1/mls/group/"+groupID+"/member/"+bobDID,
		bytes.NewReader(mustJSON(map[string]any{
			"commit_b64": base64.StdEncoding.EncodeToString([]byte("seq-commit-remove-bob")),
			"new_epoch":  3,
		})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aliceToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE isteği başarısız: %v", err)
	}
	defer resp.Body.Close()
	var delResp testResp
	_ = json.NewDecoder(resp.Body).Decode(&delResp)
	if resp.StatusCode != 200 || !delResp.Success {
		t.Fatalf("sıralı remove (epoch 2→3) başarısız (code=%d): %s", resp.StatusCode, delResp.Error)
	}

	infoResp, infoCode := get(t, "/v1/mls/group/"+groupID, aliceToken)
	if infoCode != 200 || !infoResp.Success {
		t.Fatalf("grup bilgisi okunamadı (code=%d): %s", infoCode, infoResp.Error)
	}
	var info struct {
		Epoch int64 `json:"epoch"`
	}
	_ = json.Unmarshal(infoResp.Data, &info)
	if info.Epoch != 3 {
		t.Fatalf("dört ardışık epoch-ilerleten çağrıdan sonra epoch = %d, beklenen 3 — CAS meşru ilerlemeyi bozuyor", info.Epoch)
	}

	t.Logf("✓ add→commit(update)→remove sıralı zinciri (epoch 0→1→2→3) CAS ile hâlâ kesintisiz")
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
