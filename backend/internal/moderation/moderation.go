// Package moderation provides content-moderation scoring for messages whose
// plaintext the server cannot read.
//
// Per ADR-0013, the server moderates on ciphertext METADATA only (length,
// entropy, repetition, fanout). Plaintext inspection is impossible by design;
// the long-term plan is client-side ONNX inference with a zero-knowledge proof
// of inference (ezkl) verified server-side. This package ships the heuristic
// baseline (Phase 0) behind an interface stable enough for Phase 2 (ZK-ML) to
// drop in without changing callers.
//
// The Score function returns a float in [0.0, 1.0] where higher means more
// likely abusive. IsSpam applies the configured threshold (default 0.7).
// Report persists a user-filed spam report into the existing spam_reports
// table and may invoke Score on the offending ciphertext to enrich the record
// in a later iteration.
package moderation

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"
)

// SpamThreshold is the score at or above which IsSpam returns true.
// Conservative default per ADR-0013; tune with Phase 1 telemetry.
const SpamThreshold = 0.7

// Metadata is the public, non-plaintext context the server already has about
// a message: sender, fanout count, prior-message gap, declared content type.
// None of this reveals payload contents.
type Metadata struct {
	SenderDID       string
	RecipientCount  int           // 1 for DM; N for group fanout
	TimeSinceLastMs int64         // gap since sender's previous message
	ContentType     string        // "text", "media-ref", "system"
	DeclaredLen     int           // sender-declared payload length (may differ from ciphertext for padding)
	Now             time.Time     // injectable clock; zero means time.Now()
	_               struct{}      // forbid positional literals
}

// Score returns an abuse score in [0.0, 1.0] for a ciphertext + metadata pair.
// Higher = more likely spam/abuse. Per ADR-0013 this is a heuristic composite;
// the ZK-ML implementation will replace the body without changing the signature.
//
// Heuristic components (each normalized to [0, 1], averaged with the weights
// below):
//
//   1. Length anomaly (weight 0.2): empty or absurdly large ciphertexts.
//      Typical OBS messages cipher to 64B-4KB; <16B or >64KB is suspicious.
//   2. Shannon entropy (weight 0.3): well-encrypted ciphertext should have
//      entropy ~8 bits/byte. Significantly lower entropy means either bad
//      crypto, a structured non-message payload, or repeated content. We map
//      entropy in [4.0, 8.0] linearly to abuse score [1.0, 0.0].
//   3. Repetition rate (weight 0.2): fraction of repeated byte-pairs. High
//      repetition suggests templated spam (the same encrypted-of-the-same
//      handshake material being re-blasted).
//   4. Fanout (weight 0.3): RecipientCount > 50 with TimeSinceLastMs < 1000
//      is the classic mass-blast signature. Scaled by log of recipient count.
//
// The function never returns an error in the Phase 0 implementation but keeps
// error in the signature for the ZK-verifier path (Phase 2 may fail to verify
// a malformed proof).
func Score(ctx context.Context, ciphertext []byte, metadata Metadata) (float64, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	lenScore := lengthAnomaly(len(ciphertext))
	entScore := entropyAnomaly(ciphertext)
	repScore := repetitionRate(ciphertext)
	fanScore := fanoutAnomaly(metadata)

	// Weighted composite. Weights sum to 1.0.
	score := 0.2*lenScore + 0.3*entScore + 0.2*repScore + 0.3*fanScore

	// Clamp defensively in case any sub-score escaped [0,1].
	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}
	return score, nil
}

// IsSpam applies the SpamThreshold.
func IsSpam(score float64) bool {
	return score >= SpamThreshold
}

// ReportInput carries a complaint's evidence (Bölüm 2.2/2.3 — kanıtsız
// şikayet işleme alınmaz). Grouped into a struct rather than positional
// params: too many same-typed strings to risk transposing.
type ReportInput struct {
	ID                     string // caller-assigned, so it can be threaded to EnqueueReview/RecordComplaintVerdict
	MessageID              string
	ReporterDID            string
	ReportedDID            string
	Reason                 string
	Category               string // one of the Bölüm 1.2 closed categories (see violations.go)
	EvidenceScreenshotURL  string
	EvidenceCiphertextHash string
	EvidenceVerified       bool // result of VerifyEvidence — must be true before calling Report
}

// Report records a user-filed spam report against ReportedDID, with the
// evidence required by Bölüm 2.2/2.3. Persists to the existing spam_reports
// table (see db/database.go schema), extended with evidence columns.
func Report(ctx context.Context, db *sql.DB, in ReportInput) error {
	if db == nil {
		return errors.New("moderation.Report: nil db")
	}
	if in.ReporterDID == "" || in.ReportedDID == "" {
		return errors.New("moderation.Report: reporter and reported DIDs required")
	}
	verified := 0
	if in.EvidenceVerified {
		verified = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO spam_reports (id, reporter_did, reported_did, reason, status, created_at,
			message_id, evidence_screenshot_url, evidence_ciphertext_hash, evidence_verified, category)
		VALUES (?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?)`,
		in.ID, in.ReporterDID, in.ReportedDID, in.Reason,
		time.Now().UTC().Format(time.RFC3339),
		in.MessageID, in.EvidenceScreenshotURL, in.EvidenceCiphertextHash, verified, in.Category,
	)
	return err
}

// ─── heuristic helpers ──────────────────────────────────────────────────────

// lengthAnomaly: 0 for typical sizes (16B-64KB), rising to 1 at the extremes.
func lengthAnomaly(n int) float64 {
	switch {
	case n < 16:
		return 1.0
	case n > 64*1024:
		return 1.0
	case n < 64:
		// short but plausible (acks, system messages)
		return 0.3
	default:
		return 0.0
	}
}

// entropyAnomaly: Shannon entropy on byte distribution. Well-encrypted bytes
// approach 8.0 bits/byte; <4.0 is very suspicious.
func entropyAnomaly(data []byte) float64 {
	if len(data) == 0 {
		return 1.0
	}
	var freq [256]int
	for _, b := range data {
		freq[b]++
	}
	n := float64(len(data))
	var h float64
	for _, c := range freq {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	// Map h in [4.0, 8.0] -> score in [1.0, 0.0]. Clamp outside.
	if h >= 8.0 {
		return 0.0
	}
	if h <= 4.0 {
		return 1.0
	}
	return (8.0 - h) / 4.0
}

// repetitionRate: fraction of duplicate adjacent byte-pairs. For random data
// this approaches 1/256 ~= 0.004; templated/structured data spikes much higher.
func repetitionRate(data []byte) float64 {
	if len(data) < 4 {
		return 0.0
	}
	dup := 0
	for i := 3; i < len(data); i++ {
		if data[i] == data[i-2] && data[i-1] == data[i-3] {
			dup++
		}
	}
	rate := float64(dup) / float64(len(data)-3)
	// Anything above ~5% adjacent-pair repetition is anomalous for ciphertext.
	if rate <= 0.01 {
		return 0.0
	}
	if rate >= 0.10 {
		return 1.0
	}
	return (rate - 0.01) / 0.09
}

// fanoutAnomaly: high recipient count + low inter-message gap = mass blast.
func fanoutAnomaly(m Metadata) float64 {
	if m.RecipientCount <= 1 {
		return 0.0
	}
	// Base score from log(recipients): 10 recipients = 0.33, 100 = 0.66, 1000 = 1.0
	base := math.Log10(float64(m.RecipientCount)) / 3.0
	if base > 1 {
		base = 1
	}
	// Burst multiplier: if last message was <1s ago, double-weight.
	if m.TimeSinceLastMs > 0 && m.TimeSinceLastMs < 1000 {
		base *= 1.5
	}
	if base > 1 {
		base = 1
	}
	return base
}
