package notify

import (
	"context"
	"sync"
	"time"
)

// Problem is one thing an app's health check found wrong.
type Problem struct {
	// Key identifies the problem across checks ("indexer:1337x"), so a
	// reworded message for the same thing isn't announced again.
	Key     string
	Message string
	Level   Level // warning or error
}

// HealthWatcher runs an app's health check on a schedule and announces
// problems as they appear and clear.
//
//   - The first check only learns what's already wrong, so restarts and
//     deploys don't repeat old news.
//   - A change has to hold for Confirm checks in a row before it's
//     announced, so something that flickers (an indexer behind
//     Cloudflare) doesn't send "problem" and "restored" all day.
type HealthWatcher struct {
	Outbox  *Outbox
	Check   func(ctx context.Context) []Problem
	Every   time.Duration // default 5 minutes
	Confirm int           // default 2

	mu      sync.Mutex
	started bool
	known   map[string]Problem // what's been announced (or was there at start)
	seen    map[string]int     // consecutive checks a new problem has been present
	gone    map[string]int     // consecutive checks a known problem has been absent
}

// Run checks until ctx ends.
func (w *HealthWatcher) Run(ctx context.Context) {
	every := w.Every
	if every <= 0 {
		every = 5 * time.Minute
	}
	w.step(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.step(ctx)
		}
	}
}

func (w *HealthWatcher) step(ctx context.Context) {
	now := map[string]Problem{}
	for _, p := range w.Check(ctx) {
		if p.Key == "" {
			p.Key = p.Message
		}
		if p.Level == "" {
			p.Level = LevelWarning
		}
		now[p.Key] = p
	}
	confirm := w.Confirm
	if confirm <= 0 {
		confirm = 2
	}

	w.mu.Lock()
	if !w.started {
		w.started, w.known, w.seen, w.gone = true, now, map[string]int{}, map[string]int{}
		w.mu.Unlock()
		return
	}
	var appeared, cleared []Problem
	for k, p := range now {
		delete(w.gone, k)
		if _, ok := w.known[k]; ok {
			w.known[k] = p // keep the latest wording
			continue
		}
		w.seen[k]++
		if w.seen[k] >= confirm {
			delete(w.seen, k)
			w.known[k] = p
			appeared = append(appeared, p)
		}
	}
	for k := range w.seen {
		if _, ok := now[k]; !ok {
			delete(w.seen, k) // a blip that went away before it counted
		}
	}
	for k, p := range w.known {
		if _, ok := now[k]; ok {
			continue
		}
		w.gone[k]++
		if w.gone[k] >= confirm {
			delete(w.gone, k)
			delete(w.known, k)
			cleared = append(cleared, p)
		}
	}
	w.mu.Unlock()

	for _, p := range appeared {
		w.Outbox.Publish(Event{Type: TypeHealthIssue, Level: p.Level, Title: "Health problem", Message: p.Message})
	}
	for _, p := range cleared {
		w.Outbox.Publish(Event{Type: TypeHealthOK, Level: LevelSuccess, Title: "Health restored", Message: p.Message})
	}
}
