package api

// OBS staking + slashing API — the HTTP surface over the staking state layer.
//
// See internal/staking/staking.go and docs/adr/0011-staking-slashing.md.
// All routes require authentication (registered under the `priv` subrouter).
//
//   POST /v1/staking/stake         → lock OBS into a new stake
//   POST /v1/staking/unstake       → request unstake (starts 7-day cooldown)
//   POST /v1/staking/withdraw      → withdraw principal + reward after cooldown
//   GET  /v1/staking/positions     → caller's stakes + accrued rewards
//   GET  /v1/staking/slashes       → caller's slash events
//   POST /v1/staking/slash/review  → multisig review of a pending slash
//
// NOTE on slash/review authorization: ADR-0011 specifies a 3/5 multisig of
// governance-elected signers. Proper signer identity does not exist yet, so as
// a PLACEHOLDER any Diamond-tier (tier 5 / "Elmas") user may cast a review
// vote. This must be replaced with real multisig node identity before mainnet.

import (
	"errors"
	"net/http"

	"obscura.network/core/internal/staking"
	"obscura.network/core/internal/token"
)

// stakeView pairs a stake with its currently-accrued reward for API responses.
type stakeView struct {
	*staking.Position
	AccruedReward string `json:"accrued_reward"`
}

// POST /v1/staking/stake
// Body: { amount, stake_type }
func HandleStakingStake(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		Amount    string `json:"amount"`
		StakeType string `json:"stake_type"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz JSON")
		return
	}
	if req.StakeType == "" {
		respond(w, 400, nil, "stake_type zorunlu ('user' veya 'node_operator')")
		return
	}

	// Validate amount: positive base-10 decimal string that fits in 256 bits.
	amount, err := token.ParseAmount(req.Amount)
	if err != nil {
		respond(w, 400, nil, "Geçersiz tutar: "+err.Error())
		return
	}

	stakeID, err := staking.Stake(r.Context(), user.DID, amount, req.StakeType)
	if err != nil {
		switch {
		case errors.Is(err, staking.ErrInvalidStakeType):
			respond(w, 400, nil, "Geçersiz stake türü")
		case errors.Is(err, staking.ErrBelowMinimum):
			respond(w, 400, nil, "Minimum stake altında: "+err.Error())
		case errors.Is(err, staking.ErrInsufficientFunds):
			respond(w, 400, nil, "Yetersiz bakiye")
		case errors.Is(err, staking.ErrInvalidAmount):
			respond(w, 400, nil, "Geçersiz tutar")
		default:
			respond(w, 500, nil, "Stake başarısız: "+err.Error())
		}
		return
	}

	respond(w, 200, map[string]any{
		"stake_id":   stakeID,
		"amount":     amount.String(),
		"stake_type": req.StakeType,
		"currency":   "OBS",
	}, "")
}

// POST /v1/staking/unstake
// Body: { stake_id }
func HandleStakingUnstake(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		StakeID string `json:"stake_id"`
	}
	if err := decodeBody(r, &req); err != nil || req.StakeID == "" {
		respond(w, 400, nil, "stake_id zorunlu")
		return
	}

	if err := staking.RequestUnstake(r.Context(), user.DID, req.StakeID); err != nil {
		respondStakingErr(w, err)
		return
	}
	respond(w, 200, map[string]any{
		"stake_id": req.StakeID,
		"status":   "unstaking",
	}, "")
}

// POST /v1/staking/withdraw
// Body: { stake_id }
func HandleStakingWithdraw(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		StakeID string `json:"stake_id"`
	}
	if err := decodeBody(r, &req); err != nil || req.StakeID == "" {
		respond(w, 400, nil, "stake_id zorunlu")
		return
	}

	if err := staking.Withdraw(r.Context(), user.DID, req.StakeID); err != nil {
		respondStakingErr(w, err)
		return
	}
	respond(w, 200, map[string]any{
		"stake_id": req.StakeID,
		"status":   "withdrawn",
	}, "")
}

// GET /v1/staking/positions
func HandleStakingPositions(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	stakes, err := staking.Positions(r.Context(), user.DID)
	if err != nil {
		respond(w, 500, nil, "Pozisyonlar okunamadı: "+err.Error())
		return
	}

	views := make([]stakeView, 0, len(stakes))
	for _, s := range stakes {
		reward, err := staking.AccruedReward(s.ID)
		rewardStr := "0"
		if err == nil {
			rewardStr = reward.String()
		}
		views = append(views, stakeView{Position: s, AccruedReward: rewardStr})
	}

	respond(w, 200, map[string]any{
		"positions": views,
		"count":     len(views),
	}, "")
}

// GET /v1/staking/slashes
func HandleStakingSlashes(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	events, err := staking.SlashEvents(r.Context(), user.DID)
	if err != nil {
		respond(w, 500, nil, "Slash olayları okunamadı: "+err.Error())
		return
	}
	respond(w, 200, map[string]any{
		"slashes": events,
		"count":   len(events),
	}, "")
}

// POST /v1/staking/slash/review
// Body: { slash_event_id, approve }
//
// PLACEHOLDER auth: any Diamond-tier (tier 5) user may review. Replace with
// proper 3/5 multisig signer identity before mainnet (ADR-0011).
func HandleStakingSlashReview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if user.Tier < 5 {
		respond(w, 403, nil, "Slash incelemesi yalnızca Elmas (tier 5) kullanıcılar içindir")
		return
	}

	var req struct {
		SlashEventID string `json:"slash_event_id"`
		Approve      bool   `json:"approve"`
	}
	if err := decodeBody(r, &req); err != nil || req.SlashEventID == "" {
		respond(w, 400, nil, "slash_event_id zorunlu")
		return
	}

	if err := staking.ReviewSlash(r.Context(), req.SlashEventID, user.DID, req.Approve); err != nil {
		switch {
		case errors.Is(err, staking.ErrSlashNotFound):
			respond(w, 404, nil, "Slash olayı bulunamadı")
		case errors.Is(err, staking.ErrSlashNotPending):
			respond(w, 400, nil, "Slash olayı incelemede değil")
		case errors.Is(err, staking.ErrAlreadyReviewed):
			respond(w, 409, nil, "Bu slash olayını zaten incelediniz")
		default:
			respond(w, 500, nil, "İnceleme başarısız: "+err.Error())
		}
		return
	}
	respond(w, 200, map[string]any{
		"slash_event_id": req.SlashEventID,
		"approve":        req.Approve,
		"reviewer":       user.DID,
	}, "")
}

// respondStakingErr maps the shared staking errors used by unstake/withdraw.
func respondStakingErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, staking.ErrStakeNotFound):
		respond(w, 404, nil, "Stake bulunamadı")
	case errors.Is(err, staking.ErrNotStakeOwner):
		respond(w, 403, nil, "Bu stake size ait değil")
	case errors.Is(err, staking.ErrWrongStatus):
		respond(w, 400, nil, "Stake bu işlem için uygun durumda değil")
	case errors.Is(err, staking.ErrCooldownActive):
		respond(w, 400, nil, "Bekleme süresi (7 gün) henüz dolmadı")
	default:
		respond(w, 500, nil, "İşlem başarısız: "+err.Error())
	}
}
