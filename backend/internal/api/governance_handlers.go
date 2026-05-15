package api

// Governance API — HTTP surface over the ZK-voting governance layer
// (ADR-0012). See internal/governance/governance.go.
//
// All routes require authentication (registered under the `priv` subrouter).
//
//   POST /v1/governance/proposals                   → create a proposal (burns 100 OBS)
//   GET  /v1/governance/proposals                   → list proposals
//   GET  /v1/governance/proposals/{id}              → proposal detail + current tally
//   GET  /v1/governance/proposals/{id}/voter-root   → eligibility merkle root (for ZK proof generation)
//   POST /v1/governance/proposals/{id}/vote         → anonymous ZK vote
//   POST /v1/governance/proposals/{id}/finalize     → tally after voting window ends
//   POST /v1/governance/proposals/{id}/veto         → Diamond-only veto submission
//   POST /v1/governance/proposals/{id}/execute      → execute after timelock

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/governance"
)

// governanceErrCode maps a governance sentinel error to an HTTP status.
func governanceErrCode(err error) int {
	switch {
	case errors.Is(err, governance.ErrProposalNotFound):
		return 404
	case errors.Is(err, governance.ErrNotEligible):
		return 403
	case errors.Is(err, governance.ErrDoubleVote):
		return 409
	case errors.Is(err, governance.ErrVotingClosed),
		errors.Is(err, governance.ErrVotingOpen),
		errors.Is(err, governance.ErrInvalidType),
		errors.Is(err, governance.ErrProofInvalid),
		errors.Is(err, governance.ErrPollMismatch),
		errors.Is(err, governance.ErrRootMismatch),
		errors.Is(err, governance.ErrAlreadyTallied),
		errors.Is(err, governance.ErrNotPassed),
		errors.Is(err, governance.ErrTimelockActive):
		return 400
	default:
		return 500
	}
}

// POST /v1/governance/proposals
// Body: { title, description, proposal_type ('param'|'protocol'), execution_payload }
func HandleGovernanceCreateProposal(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		Title            string `json:"title"`
		Description      string `json:"description"`
		ProposalType     string `json:"proposal_type"`
		ExecutionPayload string `json:"execution_payload"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.Title == "" {
		respond(w, 400, nil, "title zorunlu")
		return
	}

	id, err := governance.CreateProposal(r.Context(), user.DID, req.Title,
		req.Description, req.ProposalType, req.ExecutionPayload)
	if err != nil {
		respond(w, governanceErrCode(err), nil, err.Error())
		return
	}

	p, _ := governance.GetProposal(r.Context(), id)
	respond(w, 200, map[string]any{
		"proposal_id": id,
		"proposal":    p,
	}, "")
}

// GET /v1/governance/proposals
func HandleGovernanceListProposals(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	proposals, err := governance.ListProposals(r.Context(), 50)
	if err != nil {
		respond(w, 500, nil, "Öneriler listelenemedi: "+err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"proposals": proposals,
		"count":     len(proposals),
	}, "")
}

// GET /v1/governance/proposals/{id}
func HandleGovernanceGetProposal(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "proposal id zorunlu")
		return
	}

	p, err := governance.GetProposal(r.Context(), id)
	if err != nil {
		respond(w, governanceErrCode(err), nil, err.Error())
		return
	}

	tally, _ := governance.GetTally(r.Context(), id)

	// Voter root info (snapshot frozen at proposal creation).
	var root, snapAt string
	var voterCount int
	_ = db.DB.QueryRowContext(r.Context(), `
		SELECT merkle_root, voter_count, snapshot_at
		FROM governance_eligibility_snapshots WHERE proposal_id = ?`, id,
	).Scan(&root, &voterCount, &snapAt)

	respond(w, 200, map[string]any{
		"proposal": p,
		"tally":    tally,
		"voter_root": map[string]any{
			"merkle_root": root,
			"voter_count": voterCount,
			"snapshot_at": snapAt,
		},
	}, "")
}

// GET /v1/governance/proposals/{id}/voter-root
// Returns the eligibility merkle root + snapshot metadata. Clients need this to
// build a vote_proof that binds to the proposal's frozen voter set.
func HandleGovernanceVoterRoot(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "proposal id zorunlu")
		return
	}

	var root, snapAt string
	var voterCount int
	err := db.DB.QueryRowContext(r.Context(), `
		SELECT merkle_root, voter_count, snapshot_at
		FROM governance_eligibility_snapshots WHERE proposal_id = ?`, id,
	).Scan(&root, &voterCount, &snapAt)
	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Öneri bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Snapshot okunamadı: "+err.Error())
		return
	}

	// poll_id is needed inside the circuit too.
	var pollID string
	_ = db.DB.QueryRowContext(r.Context(),
		`SELECT poll_id FROM proposals WHERE id = ?`, id).Scan(&pollID)

	respond(w, 200, map[string]any{
		"proposal_id": id,
		"poll_id":     pollID,
		"merkle_root": root,
		"voter_count": voterCount,
		"snapshot_at": snapAt,
	}, "")
}

// POST /v1/governance/proposals/{id}/vote
// Body: { proof_json, public_inputs[], choice (0..3) }
//
// Anonymous: no voter identity is recorded. The nullifier inside public_inputs
// (index 3, derived in-circuit from voter secret + poll_id) prevents double
// voting. choice is the plaintext vote (0=yes, 1=no, 2=abstain, 3=veto) — in
// the FAZ 2 implementation it is stored as a JSON blob; FAZ 3 will replace this
// with threshold-decrypted aggregation (ADR-0012 tech debt).
func HandleGovernanceSubmitVote(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "proposal id zorunlu")
		return
	}

	var req struct {
		ProofJSON    string   `json:"proof_json"`
		PublicInputs []string `json:"public_inputs"`
		Choice       int      `json:"choice"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.ProofJSON == "" {
		respond(w, 400, nil, "proof_json zorunlu")
		return
	}
	if req.Choice < governance.ChoiceYes || req.Choice > governance.ChoiceVeto {
		respond(w, 400, nil, "choice 0..3 olmalı")
		return
	}

	// Support both bare proof and { "proof": {...} } envelope (mirrors airdrop).
	proofJSON := req.ProofJSON
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.ProofJSON), &raw); err == nil {
		if pj, ok := raw["proof"]; ok {
			proofJSON = string(pj)
		}
	}

	encChoice := governance.EncodeChoice(req.Choice)
	if err := governance.SubmitVote(r.Context(), id, proofJSON, req.PublicInputs, encChoice); err != nil {
		respond(w, governanceErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"proposal_id": id,
		"recorded":    true,
	}, "")
}

// POST /v1/governance/proposals/{id}/finalize
// Anyone can trigger finalization once the voting window has ended.
func HandleGovernanceFinalize(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "proposal id zorunlu")
		return
	}

	tally, err := governance.Finalize(r.Context(), id)
	if err != nil {
		respond(w, governanceErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, tally, "")
}

// POST /v1/governance/proposals/{id}/veto
// PLACEHOLDER: ADR-0012 reserves veto for the Diamond cohort. Because votes
// are anonymous, the canonical veto path is for Diamond holders to cast a
// vote with choice=Veto via the normal vote endpoint, and Tally enforces the
// 1/5 threshold. This endpoint exists as a Diamond-tier-gated convenience that
// records a veto vote *outside* the ZK envelope — it is intentionally lighter
// (no ZK proof required) but only Diamond-tier identities can call it. It is
// rate-limited by the proposal's UNIQUE nullifier just like a normal vote.
//
// Body: { proof_json, public_inputs[] }   (vote choice is forced to Veto)
func HandleGovernanceVeto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if user.Tier < governance.DiamondTier {
		respond(w, 403, nil, "Veto yalnızca Elmas (tier 5) kullanıcılar içindir")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "proposal id zorunlu")
		return
	}

	var req struct {
		ProofJSON    string   `json:"proof_json"`
		PublicInputs []string `json:"public_inputs"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.ProofJSON == "" {
		respond(w, 400, nil, "proof_json zorunlu")
		return
	}

	proofJSON := req.ProofJSON
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.ProofJSON), &raw); err == nil {
		if pj, ok := raw["proof"]; ok {
			proofJSON = string(pj)
		}
	}

	encChoice := governance.EncodeChoice(governance.ChoiceVeto)
	if err := governance.SubmitVote(r.Context(), id, proofJSON, req.PublicInputs, encChoice); err != nil {
		respond(w, governanceErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"proposal_id": id,
		"vetoed_by":   user.DID,
		"recorded":    true,
	}, "")
}

// POST /v1/governance/proposals/{id}/execute
// Anyone can trigger execution once status='passed' and the 48h timelock has
// elapsed. The actual payload application is a FAZ 3 stub (see governance.Execute).
func HandleGovernanceExecute(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	id := mux.Vars(r)["id"]
	if id == "" {
		respond(w, 400, nil, "proposal id zorunlu")
		return
	}

	if err := governance.Execute(r.Context(), id); err != nil {
		respond(w, governanceErrCode(err), nil, err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"proposal_id": id,
		"status":      "executed",
	}, "")
}
