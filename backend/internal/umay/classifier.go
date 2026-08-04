// Package umay implements the Kamusal İçerik Tarama Motoru (spec Bölüm 1.3-1.4,
// "Umay deseni"): monitor → brain → notify.
//
// Scope (Bölüm 1.1): only public content (conversations.is_public = 1).
// DM and invite-only groups are never touched — that boundary is enforced in
// monitor.go's query, not here. This package is independent of
// internal/scanner (heuristic rate-limit/keyword flags over all traffic) —
// no shared state, no shared tables beyond review_queue (via
// internal/moderation.EnqueueAutoScanReview).
package umay

import (
	"context"
	"sync"
)

// CategoryNone marks content the classifier found no violation in. It is
// deliberately NOT part of moderation's closed Bölüm 1.2 category list —
// that list is violations only; "clean" is an Umay-local sentinel.
const CategoryNone = "none"

// Verdict is a classifier's judgment on one piece of content.
type Verdict struct {
	// Category is CategoryNone or one of the closed Bölüm 1.2 categories
	// (see internal/moderation.CategorySpam etc).
	Category   string
	Confidence float64 // 0..1
}

// Classifier classifies public content into a Verdict. Implementations must
// be safe for concurrent use — monitor's scan loop may call Classify from a
// single goroutine today, but tests and future batching may not.
type Classifier interface {
	Classify(ctx context.Context, content string) (Verdict, error)
}

var (
	activeClassifierMu sync.RWMutex
	activeClassifier   Classifier = newDefaultClassifier()
)

// Classify runs the active classifier (production: DualClassifier wired to
// Ollama + Groq; tests: whatever SetClassifierForTest installed).
func Classify(ctx context.Context, content string) (Verdict, error) {
	activeClassifierMu.RLock()
	c := activeClassifier
	activeClassifierMu.RUnlock()
	return c.Classify(ctx, content)
}

// SetClassifierForTest swaps the active classifier and returns a restore
// func. Mirrors internal/airdrop's SetVerifyHookForTest seam: production
// code never calls this, tests use it to avoid any real Ollama/Groq call.
func SetClassifierForTest(c Classifier) func() {
	activeClassifierMu.Lock()
	prev := activeClassifier
	activeClassifier = c
	activeClassifierMu.Unlock()
	return func() {
		activeClassifierMu.Lock()
		activeClassifier = prev
		activeClassifierMu.Unlock()
	}
}

func newDefaultClassifier() Classifier {
	return NewDualClassifier(NewLocalOllamaClassifier(), NewGroqFallbackClassifier())
}
