package scraper

import (
	"context"
	"sync"
)

// Trace collects facts about one search as it runs, for indexer stats.
// Attach it with WithTrace before calling SearchWithQuery; the runner fills
// it in. Safe for concurrent use (multi-path definitions).
type Trace struct {
	mu         sync.Mutex
	challenged bool
	solved     bool
}

type traceKey struct{}

// WithTrace returns a context that carries t.
func WithTrace(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, traceKey{}, t)
}

func traceFrom(ctx context.Context) *Trace {
	t, _ := ctx.Value(traceKey{}).(*Trace)
	return t
}

// noteChallenge records a Cloudflare challenge and whether it was solved.
// One unsolved page marks the whole search unsolved.
func (t *Trace) noteChallenge(solved bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.challenged {
		t.solved = solved
	} else {
		t.solved = t.solved && solved
	}
	t.challenged = true
}

// Challenge reports whether Cloudflare challenged the search and, if so,
// whether every challenge was solved.
func (t *Trace) Challenge() (challenged, solved bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.challenged, t.solved
}

// NoteChallengeForTest records a challenge on t from outside the package.
// For tests that fake a search; the runner records challenges itself.
func NoteChallengeForTest(t *Trace, solved bool) {
	t.noteChallenge(solved)
}
