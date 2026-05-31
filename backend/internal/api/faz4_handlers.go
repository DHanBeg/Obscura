package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/ai"
	"obscura.network/core/internal/dao"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/pqcrypto"
	"obscura.network/core/internal/sequencer"
	"obscura.network/core/internal/zk"
)

// ── DAO (Full governance) ─────────────────────────────────────────────────────

// HandleDAOCreateProposal — POST /v1/dao/proposals
func HandleDAOCreateProposal(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	cat := dao.ProposalCategory(req.Category)
	if cat == "" {
		cat = dao.CategoryParam
	}
	p, err := dao.CreateProposal(user.DID, req.Title, req.Description, cat)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, p, "")
}

// HandleDAOListProposals — GET /v1/dao/proposals
func HandleDAOListProposals(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	proposals, err := dao.ListProposals(status)
	if err != nil {
		respond(w, 500, nil, "Liste alınamadı")
		return
	}
	if proposals == nil {
		proposals = []dao.DAOProposal{}
	}
	respond(w, 200, map[string]interface{}{"proposals": proposals, "count": len(proposals)}, "")
}

// HandleDAOFinalize — POST /v1/dao/proposals/{id}/finalize
func HandleDAOFinalize(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	id := mux.Vars(r)["id"]
	p, err := dao.Finalize(id)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, p, "")
}

// HandleDAOExecute — POST /v1/dao/proposals/{id}/execute
func HandleDAOExecute(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	id := mux.Vars(r)["id"]
	p, err := dao.Execute(id)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, p, "")
}

// HandleDAOVeto — POST /v1/dao/proposals/{id}/veto (guardian only)
func HandleDAOVeto(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	id := mux.Vars(r)["id"]
	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if err := dao.GuardianVeto(id, user.DID, req.Reason); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, map[string]string{"status": "vetoed"}, "")
}

// ── Post-quantum (Dilithium) ──────────────────────────────────────────────────

// HandleDilithiumKeygen — POST /v1/pq/dilithium/keygen
// Anahtar çiftini üretir, DB'ye kaydeder ve mesajları otomatik imzalamayı etkinleştirir.
func HandleDilithiumKeygen(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	kp, err := pqcrypto.DilithiumGenerateKeyPair()
	if err != nil {
		respond(w, 500, nil, "Dilithium keygen hatası")
		return
	}
	// Her iki anahtarı da DB'ye kaydet — private key server-side auto-signing için
	if _, dbErr := db.DB.Exec(
		"UPDATE users SET dilithium_pub_key = ?, dilithium_priv_key = ? WHERE did = ?",
		kp.PublicKey, kp.PrivateKey, user.DID,
	); dbErr != nil {
		respond(w, 500, nil, "Anahtar kaydedilemedi")
		return
	}
	respond(w, 200, map[string]interface{}{
		"algorithm":  "Dilithium3",
		"public_key": kp.PublicKey,
		"did":        user.DID,
		"note":       "Anahtar çifti oluşturuldu. Gönderdiğin mesajlar artık otomatik imzalanacak.",
	}, "")
}

// HandleDilithiumVerify — POST /v1/pq/dilithium/verify
func HandleDilithiumVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey string `json:"public_key"`
		Message   string `json:"message"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	valid, err := pqcrypto.DilithiumVerify(req.PublicKey, req.Message, req.Signature)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, map[string]bool{"valid": valid}, "")
}

// ── AI Optimizer ─────────────────────────────────────────────────────────────

// HandleAIMetrics — GET /v1/ai/metrics (node metrikleri + yük tahmini)
func HandleAIMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := ai.GlobalOptimizer.AllMetrics()
	predicted := ai.GlobalOptimizer.PredictLoad()
	best := ai.GlobalOptimizer.SelectPeers(5)
	respond(w, 200, map[string]interface{}{
		"all_nodes":      metrics,
		"predicted_load": predicted,
		"best_peers":     best,
	}, "")
}

// HandleAIPeers — GET /v1/ai/peers?n=5 (en iyi N peer seç)
func HandleAIPeers(w http.ResponseWriter, r *http.Request) {
	n := 5
	if v := r.URL.Query().Get("n"); v != "" {
		if err := json.Unmarshal([]byte(v), &n); err != nil || n <= 0 {
			n = 5
		}
	}
	peers := ai.GlobalOptimizer.SelectPeers(n)
	respond(w, 200, map[string]interface{}{"peers": peers, "count": len(peers)}, "")
}

// ── Sequencer ─────────────────────────────────────────────────────────────────

// HandleSequencerRegister — POST /v1/sequencer/register
func HandleSequencerRegister(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req sequencer.SequencerCandidate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	req.DID = user.DID
	if err := sequencer.Global.Register(req); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, map[string]string{"status": "registered"}, "")
}

// HandleSequencerList — GET /v1/sequencer/candidates
func HandleSequencerList(w http.ResponseWriter, r *http.Request) {
	candidates := sequencer.Global.Candidates()
	active := sequencer.Global.ActiveSequencer()
	respond(w, 200, map[string]interface{}{
		"candidates": candidates,
		"active":     active,
	}, "")
}

// HandleSequencerBatches — GET /v1/sequencer/batches
func HandleSequencerBatches(w http.ResponseWriter, r *http.Request) {
	batches := sequencer.Global.RecentBatches(20)
	respond(w, 200, map[string]interface{}{"batches": batches}, "")
}

// HandleSequencerSubmitBatch — POST /v1/sequencer/batch
func HandleSequencerSubmitBatch(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req struct {
		StateRoot string `json:"state_root"`
		TxCount   int    `json:"tx_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	active := sequencer.Global.ActiveSequencer()
	if active == nil || active.DID != user.DID {
		respond(w, 403, nil, "Aktif sequencer değilsiniz")
		return
	}

	batch, err := sequencer.Global.SubmitBatch(active.NodeID, req.StateRoot, req.TxCount)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	respond(w, 200, batch, "")
}

// ── Location Proof ────────────────────────────────────────────────────────────

// HandleLocationVerify — POST /v1/location/verify (GPS+ZK konum doğrulama)
func HandleLocationVerify(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req struct {
		ProofJSON    string   `json:"proof_json"`
		PublicInputs []string `json:"public_inputs"`
		// public_inputs: [region_lat_min, region_lat_max, region_lon_min, region_lon_max, epoch, location_commitment]
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}

	if req.ProofJSON == "" || len(req.PublicInputs) < 6 {
		respond(w, 400, nil, "Geçersiz kanıt formatı (6 public input gerekli: region_lat_min/max, region_lon_min/max, epoch, location_commitment)")
		return
	}

	// location_proof circuit yüklüyse gerçek ZK doğrulaması yap
	if zk.IsCircuitLoaded("location_proof") {
		ok, err := zk.VerifyProof("location_proof", req.ProofJSON, req.PublicInputs)
		if err != nil || !ok {
			respond(w, 400, nil, "ZK location kanıtı geçersiz")
			return
		}
		// public_inputs[4] = in_region output (son eleman)
		inRegion := len(req.PublicInputs) > 6 && req.PublicInputs[6] == "1"
		respond(w, 200, map[string]interface{}{
			"verified":  true,
			"in_region": inRegion,
		}, "")
		return
	}

	// Circuit henüz yüklü değil (trusted setup tamamlanmamış)
	respond(w, 503, nil, "location_proof circuit trusted setup tamamlanmamış — circuit kullanılamıyor")
}
