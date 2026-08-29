package api

// POST /v1/internal/zk/verify — node-to-node peer doğrulama (spec Bölüm
// 19.3: "her proof en az 2 node tarafından doğrulanır").
//
// C10 #7 kanıtı: internal/zk/multi_verify.go'nun callPeerVerify'ı bu
// endpoint'e (eskiden aynı /v1/zk/verify'a) X-Internal-Secret header'ıyla
// istek atıyordu, ama alıcı (HandleVerifyZKProof, keys.go:340) bu header'ı
// HİÇ okumuyordu — sadece JWT (AuthMiddleware, priv subrouter) bekliyordu.
// callPeerVerify Authorization header GÖNDERMEDİĞİ için her node-to-node
// istek 401 alıyordu; MultiVerify bunu iletişim hatası sayıp approvals'a
// hiç eklemiyordu (approvals=0 < minVerifiers=1) → NODE_PEERS ayarlı HER
// production dağıtımında TÜM ZK proof doğrulamaları "Peer node'lar kanıtı
// reddetti" ile başarısız oluyordu — proof'un kendisi geçerli olsa bile.
// Ölü kod değildi, aktif kırık bir spec-uyumluluk regresyonuydu.
//
// Karar: node-auth GEREKLİ (multi_verify.go'nun tüm tasarımı ve spec Bölüm
// 19.3 bunu varsayıyor) — secret gönderimi silinmedi, alıcı tarafa N11
// deseninde (nodeMAC, storage_handlers.go/sharding.go'nun birebir kopyası)
// gerçek doğrulama eklendi, YENİ ayrı bir route'ta. Kullanıcı-facing
// /v1/zk/verify (JWT, DB'ye proof kaydeder) DEĞİŞTİRİLMEDİ — bu endpoint
// proof'u kaydetmez, sadece bağımsız bir ikinci Groth16 doğrulaması yapıp
// {"success":true/false} döner (replay/rate-limit anomaly detection
// user.DID'e bağlı olduğu için burada tekrarlanmaz — o kontrol proof'u ilk
// alan node'da zaten yapıldı; bu endpoint yalnızca "başka bir node aynı
// matematiksel sonuca varıyor mu" sorusuna cevap verir).

import (
	"encoding/json"
	"io"
	"net/http"

	"obscura.network/core/internal/zk"
)

func HandleVerifyZKProofInternal(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respond(w, 400, nil, "Body okunamadı")
		return
	}

	// nodeMAC doğrula (HMAC, N11 deseni) — JSON parse'dan önce.
	if !verifyNodeHMAC(r.Header.Get("X-Node-Ts"), r.Header.Get("X-Node-Sig"), body) {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req ZKVerifyRequest
	if err := json.Unmarshal(body, &req); err != nil || req.ProofJSON == "" || req.CircuitID == "" {
		respond(w, 400, nil, "proof_json ve circuit_id zorunlu")
		return
	}

	circuit, proofBytes, publicSignals, errMsg := parseZKVerifyProof(req)
	if errMsg != "" {
		respond(w, 400, nil, errMsg)
		return
	}

	if err := zk.VerifyGroth16(circuit, proofBytes, publicSignals); err != nil {
		respond(w, 400, nil, "Kanıt geçersiz: "+err.Error())
		return
	}

	respond(w, 200, map[string]interface{}{"success": true}, "")
}
