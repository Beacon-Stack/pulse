package notify

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHealthWatcher_AnnouncesChangesOnly(t *testing.T) {
	allowLocal(t)
	s := newStore(t)
	c, srv := newCapture(t)
	if _, err := s.Create(context.Background(), Channel{Name: "Phone", Kind: "webhook", Enabled: true, Settings: json.RawMessage(`{"url":"` + srv.URL + `"}`)}); err != nil {
		t.Fatal(err)
	}
	o := &Outbox{App: "Pilot", Store: s}

	var mu sync.Mutex
	problems := []Problem{{Key: "disk", Message: "Library TV has 2 GB free", Level: LevelError}}
	w := &HealthWatcher{Outbox: o, Check: func(context.Context) []Problem {
		mu.Lock()
		defer mu.Unlock()
		return append([]Problem(nil), problems...)
	}}
	set := func(p ...Problem) {
		mu.Lock()
		problems = p
		mu.Unlock()
	}
	titles := func() []string {
		c.mu.Lock()
		defer c.mu.Unlock()
		var out []string
		for _, r := range c.reqs {
			var e Event
			_ = json.Unmarshal([]byte(r.Body), &e)
			out = append(out, string(e.Type)+": "+e.Message)
		}
		return out
	}
	ctx := context.Background()

	w.step(ctx) // startup: learns the disk problem, says nothing
	w.step(ctx) // unchanged: nothing
	time.Sleep(50 * time.Millisecond)
	if got := titles(); len(got) != 0 {
		t.Fatalf("startup and unchanged checks should be quiet: %v", got)
	}

	// A new problem has to be there twice before it's announced; a known
	// one has to be gone twice before it's cleared.
	set(Problem{Key: "disk", Message: "Library TV has 1 GB free", Level: LevelError}, Problem{Key: "indexer:1337x", Message: "Can't reach 1337x"})
	w.step(ctx)
	set(Problem{Key: "indexer:1337x", Message: "Can't reach 1337x"})
	w.step(ctx)
	time.Sleep(50 * time.Millisecond)
	if got := titles(); len(got) != 1 || got[0] != "health_issue: Can't reach 1337x" {
		t.Fatalf("after two checks: %v", got)
	}
	w.step(ctx) // disk gone a second time: cleared
	waitFor(t, "two announcements", func() bool { return len(titles()) == 2 })
	got := strings.Join(titles(), " | ")
	if !strings.Contains(got, "health_issue: Can't reach 1337x") || !strings.Contains(got, "health_ok: Library TV has 1 GB free") || strings.Contains(got, "health_issue: Library TV") {
		t.Errorf("announcements %s", got)
	}
}

// Something that flickers on and off doesn't announce anything.
func TestHealthWatcher_IgnoresFlicker(t *testing.T) {
	allowLocal(t)
	s := newStore(t)
	c, srv := newCapture(t)
	_, _ = s.Create(context.Background(), Channel{Name: "Phone", Kind: "webhook", Enabled: true, Settings: json.RawMessage(`{"url":"` + srv.URL + `"}`)})
	on := false
	w := &HealthWatcher{Outbox: &Outbox{App: "Pulse", Store: s}, Check: func(context.Context) []Problem {
		if on {
			return []Problem{{Key: "indexer:1337x", Message: "1337x is failing"}}
		}
		return nil
	}}
	ctx := context.Background()
	w.step(ctx)
	for i := 0; i < 6; i++ {
		on = !on
		w.step(ctx)
	}
	time.Sleep(50 * time.Millisecond)
	if c.count() != 0 {
		t.Errorf("a flickering indexer sent %d notifications", c.count())
	}
}
