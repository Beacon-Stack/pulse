package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := NewStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	// Opening twice must be harmless (tables already exist).
	if _, err := NewStore(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStore_CreateEditKeepsSecrets(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	c, err := s.Create(ctx, Channel{Name: " Phone ", Kind: "ntfy", Enabled: true,
		Settings: json.RawMessage(`{"url":"https://ntfy.sh","topic":"t","token":"tok"}`),
		Events:   []string{TypeHealthIssue, "bogus", TypeImported, TypeImported}, Apps: []string{" Pilot", "pilot"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Phone" || strings.Join(c.Events, ",") != "imported,health_issue" || strings.Join(c.Apps, ",") != "pilot" {
		t.Errorf("created %+v", c)
	}
	// The settings screen sends the token back masked.
	up, err := s.Update(ctx, c.ID, Channel{Name: "Phone 2", Kind: "ntfy", Enabled: false, Settings: Mask("ntfy", c.Settings)})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, up.ID)
	if got.Name != "Phone 2" || got.Enabled || !strings.Contains(string(got.Settings), `"token":"tok"`) {
		t.Errorf("after edit %+v %s", got, got.Settings)
	}
	if total, on := s.Count(ctx); total != 1 || on != 0 || s.HasEnabled(ctx) {
		t.Errorf("count %d/%d", total, on)
	}
	if _, err := s.Create(ctx, Channel{Name: "x", Kind: "telegram", Settings: json.RawMessage(`{}`)}); err == nil {
		t.Error("missing required settings should be refused")
	}
	if _, err := s.Create(ctx, Channel{Name: "", Kind: "webhook", Settings: json.RawMessage(`{"url":"http://x"}`)}); err == nil {
		t.Error("a name is required")
	}
	if err := s.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestStore_Mode(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if s.Mode(ctx) != ModeAuto {
		t.Error("default should be auto")
	}
	if err := s.SetMode(ctx, ModeLocal); err != nil || s.Mode(ctx) != ModeLocal {
		t.Errorf("set local: %v", err)
	}
	if err := s.SetMode(ctx, "both"); err == nil {
		t.Error("unknown mode should be refused")
	}
}

func TestStore_ImportLegacyOnce(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	old := []LegacyChannel{
		{Name: "Discord", Kind: "discord", Enabled: true, Settings: json.RawMessage(`{"webhook_url":"https://discord.com/api/webhooks/1/x"}`), Events: []string{"import_complete", "download_done", "task_started"}},
		{Name: "Everything", Kind: "webhook", Enabled: true, Settings: json.RawMessage(`{"url":"http://ha.local"}`)},
		{Name: "Only junk", Kind: "webhook", Enabled: true, Settings: json.RawMessage(`{"url":"http://ha.local"}`), Events: []string{"task_started"}},
		{Name: "Script", Kind: "command", Enabled: true, Settings: json.RawMessage(`{"path":"/x.sh"}`)},
	}
	n, skipped, err := s.ImportLegacy(ctx, old)
	if err != nil || n != 3 || len(skipped) != 1 || skipped[0] != "Script (command)" {
		t.Fatalf("import n=%d skipped=%v err=%v", n, skipped, err)
	}
	list, _ := s.List(ctx)
	byName := map[string]Channel{}
	for _, c := range list {
		byName[c.Name] = c
	}
	if strings.Join(byName["Discord"].Events, ",") != "imported,downloaded" {
		t.Errorf("translated events %v", byName["Discord"].Events)
	}
	if len(byName["Everything"].Events) != 0 {
		t.Errorf("no filter should stay no filter: %v", byName["Everything"].Events)
	}
	if strings.Join(byName["Only junk"].Events, ",") != strings.Join(DefaultTypes(), ",") {
		t.Errorf("unmappable filter should become the defaults: %v", byName["Only junk"].Events)
	}
	// A second start doesn't import again.
	if n, _, _ := s.ImportLegacy(ctx, old); n != 0 {
		t.Errorf("imported again: %d", n)
	}
}

// fakeHub is a Pulse that can be up or down.
type fakeHub struct {
	mu       sync.Mutex
	up       bool
	channels int
	got      []Event
}

func (h *fakeHub) Notify(_ context.Context, e Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.up {
		return errors.New("connection refused")
	}
	h.got = append(h.got, e)
	return nil
}

func (h *fakeHub) Channels(context.Context) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.up {
		return 0, errors.New("connection refused")
	}
	return h.channels, nil
}

func (h *fakeHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.got)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func outboxWithLocal(t *testing.T, hub *fakeHub) (*Outbox, *capture) {
	t.Helper()
	allowLocal(t)
	s := newStore(t)
	c, srv := newCapture(t)
	if _, err := s.Create(context.Background(), Channel{Name: "Local", Kind: "webhook", Enabled: true, Settings: json.RawMessage(`{"url":"` + srv.URL + `"}`)}); err != nil {
		t.Fatal(err)
	}
	o := &Outbox{App: "Pilot", Store: s, HoldFor: 50 * time.Millisecond}
	if hub != nil {
		o.Hub = func() Hub { return hub }
	}
	return o, c
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

func ev() Event {
	return Event{Type: TypeImported, Title: "Imported: Andor S02E03", Level: LevelSuccess}
}

func TestOutbox_NoPulseUsesOwnChannels(t *testing.T) {
	o, local := outboxWithLocal(t, nil)
	o.Publish(ev())
	waitFor(t, "local send", func() bool { return local.count() == 1 })
	r := o.Recent()
	if len(r) != 1 || r[0].Via != "this app" || r[0].App != "Pilot" || len(r[0].Results) != 1 || !r[0].Results[0].OK() {
		t.Errorf("recent %+v", r)
	}
}

func TestOutbox_PulseWithChannelsGetsItAndLocalDoesNot(t *testing.T) {
	hub := &fakeHub{up: true, channels: 2}
	o, local := outboxWithLocal(t, hub)
	if r := o.Route(context.Background()); !r.ViaPulse || r.PulseChannels != 2 {
		t.Errorf("route %+v", r)
	}
	o.Publish(ev())
	waitFor(t, "pulse send", func() bool { return hub.count() == 1 })
	time.Sleep(50 * time.Millisecond)
	if local.count() != 0 {
		t.Error("an event must never go both ways")
	}
	if r := o.Recent(); len(r) != 1 || r[0].Via != "Pulse" {
		t.Errorf("recent %+v", r)
	}
}

func TestOutbox_PulseWithoutChannelsFallsToOwn(t *testing.T) {
	hub := &fakeHub{up: true, channels: 0}
	o, local := outboxWithLocal(t, hub)
	o.Publish(ev())
	waitFor(t, "local send", func() bool { return local.count() == 1 })
	if hub.count() != 0 {
		t.Error("a Pulse with no channels shouldn't swallow the event")
	}
}

func TestOutbox_LocalModeIgnoresPulse(t *testing.T) {
	hub := &fakeHub{up: true, channels: 3}
	o, local := outboxWithLocal(t, hub)
	_ = o.Store.SetMode(context.Background(), ModeLocal)
	o.Publish(ev())
	waitFor(t, "local send", func() bool { return local.count() == 1 })
	if hub.count() != 0 {
		t.Error("local mode should not use Pulse")
	}
}

func TestOutbox_PulseDownHoldsThenRecovers(t *testing.T) {
	hub := &fakeHub{up: true, channels: 1}
	o, local := outboxWithLocal(t, hub)
	o.HoldFor = time.Hour
	o.Route(context.Background()) // learn that Pulse has a channel
	hub.mu.Lock()
	hub.up = false
	hub.mu.Unlock()
	o.Publish(ev())
	waitFor(t, "held", func() bool { return o.Held() == 1 })
	if r := o.Recent(); len(r) != 1 || !r[0].Pending {
		t.Errorf("should show as waiting: %+v", r)
	}
	hub.mu.Lock()
	hub.up = true
	hub.mu.Unlock()
	o.retry(context.Background())
	if hub.count() != 1 || o.Held() != 0 || local.count() != 0 {
		t.Errorf("after recovery: pulse=%d held=%d local=%d", hub.count(), o.Held(), local.count())
	}
	if r := o.Recent(); len(r) != 1 || r[0].Pending || r[0].Via != "Pulse" {
		t.Errorf("recent %+v", r)
	}
}

func TestOutbox_PulseGoneFallsBackAfterHold(t *testing.T) {
	hub := &fakeHub{up: true, channels: 1}
	o, local := outboxWithLocal(t, hub)
	o.Route(context.Background())
	hub.mu.Lock()
	hub.up = false
	hub.mu.Unlock()
	o.Publish(ev())
	waitFor(t, "held", func() bool { return o.Held() == 1 })
	time.Sleep(60 * time.Millisecond) // past HoldFor
	o.retry(context.Background())
	if local.count() != 1 || o.Held() != 0 {
		t.Errorf("fallback: local=%d held=%d", local.count(), o.Held())
	}
	r := o.Recent()
	if len(r) != 1 || r[0].Pending || r[0].Via != "this app" || !strings.Contains(r[0].Error, "Pulse didn't answer") {
		t.Errorf("recent %+v", r)
	}
}

func TestOutbox_PulseGoneNoLocalDrops(t *testing.T) {
	hub := &fakeHub{up: true, channels: 1}
	allowLocal(t)
	o := &Outbox{App: "Haul", Store: newStore(t), Hub: func() Hub { return hub }, HoldFor: time.Millisecond}
	o.Route(context.Background())
	hub.mu.Lock()
	hub.up = false
	hub.mu.Unlock()
	o.Publish(Event{Type: TypeDownloadProblem, Title: "Stalled: x"})
	waitFor(t, "held", func() bool { return o.Held() == 1 })
	time.Sleep(5 * time.Millisecond)
	o.retry(context.Background())
	r := o.Recent()
	if o.Held() != 0 || len(r) != 1 || !strings.Contains(r[0].Error, "no channels of its own") {
		t.Errorf("held=%d recent=%+v", o.Held(), r)
	}
}

func TestOutbox_RejectsInternalEvents(t *testing.T) {
	o, local := outboxWithLocal(t, nil)
	o.Publish(Event{Type: "task_started", Title: "x"})
	time.Sleep(50 * time.Millisecond)
	if local.count() != 0 || len(o.Recent()) != 0 {
		t.Error("internal events must not be sent")
	}
}
