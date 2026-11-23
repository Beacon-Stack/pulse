package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Hub is Pulse as seen by an app: something that broadcasts on its behalf.
type Hub interface {
	// Notify hands an event to Pulse.
	Notify(ctx context.Context, e Event) error
	// Channels reports how many enabled channels Pulse has.
	Channels(ctx context.Context) (int, error)
}

// Route says where an app's events go right now.
type Route struct {
	Mode           string `json:"mode"`            // auto or local
	ViaPulse       bool   `json:"via_pulse"`       // events go to Pulse
	PulseConnected bool   `json:"pulse_connected"` // the app is connected to Pulse
	PulseChannels  int    `json:"pulse_channels"`  // enabled channels in Pulse
}

// Sent is one entry in the "recently sent" list.
type Sent struct {
	Time    time.Time `json:"time"`
	App     string    `json:"app"`
	Type    string    `json:"type"`
	Title   string    `json:"title"`
	Level   Level     `json:"level"`
	Via     string    `json:"via"`     // "Pulse" or "this app"
	Results []Result  `json:"results"` // per channel (local sends)
	Error   string    `json:"error,omitempty"`
	Pending bool      `json:"pending,omitempty"` // waiting for Pulse
}

// Outbox decides where each event goes and keeps a short history.
//
//   - Mode auto, connected to a Pulse with channels: send to Pulse. If
//     Pulse doesn't answer, hold the event and retry; after HoldFor, use
//     the app's own channels instead (or drop it, logged, if it has none).
//   - Otherwise: send to the app's own channels.
//
// It never sends the same event both ways.
type Outbox struct {
	App   string
	Store *Store
	// Hub returns Pulse, or nil when the app isn't connected to one.
	Hub func() Hub
	Log *slog.Logger

	// HoldFor is how long an event waits for Pulse before falling back.
	HoldFor time.Duration
	// RetryEvery is how often held events are retried.
	RetryEvery time.Duration

	mu      sync.Mutex
	held    []heldEvent
	recent  []Sent
	route   Route
	checked time.Time
}

type heldEvent struct {
	e     Event
	since time.Time
}

const (
	maxHeld   = 200
	maxRecent = 50
	routeTTL  = time.Minute
)

func (o *Outbox) hub() Hub {
	if o.Hub == nil {
		return nil
	}
	return o.Hub()
}

func (o *Outbox) logger() *slog.Logger {
	if o.Log == nil {
		return slog.Default()
	}
	return o.Log
}

// Route works out where events go now. Pulse's channel count is cached
// for a minute.
func (o *Outbox) Route(ctx context.Context) Route {
	mode := o.Store.Mode(ctx)
	h := o.hub()
	o.mu.Lock()
	cached, fresh := o.route, time.Since(o.checked) < routeTTL && o.route.PulseConnected == (h != nil)
	o.mu.Unlock()
	r := Route{Mode: mode, PulseConnected: h != nil}
	if h != nil {
		if fresh {
			r.PulseChannels = cached.PulseChannels
		} else if n, err := h.Channels(ctx); err == nil {
			r.PulseChannels = n
		} else {
			r.PulseChannels = cached.PulseChannels // keep the last answer through a blip
		}
	}
	r.ViaPulse = mode == ModeAuto && h != nil && r.PulseChannels > 0
	o.mu.Lock()
	o.route, o.checked = r, time.Now()
	o.mu.Unlock()
	return r
}

// Publish sends an event without blocking the caller.
func (o *Outbox) Publish(e Event) {
	e.App = o.App
	e = e.withDefaults()
	if err := e.Validate(); err != nil {
		o.logger().Warn("notify: not sending", "error", err)
		return
	}
	go o.publish(e)
}

func (o *Outbox) publish(e Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*SendTimeout)
	defer cancel()
	if o.Route(ctx).ViaPulse {
		if h := o.hub(); h != nil {
			if err := h.Notify(ctx, e); err == nil {
				o.remember(Sent{Time: e.Time, App: e.App, Type: e.Type, Title: e.Title, Level: e.Level, Via: "Pulse"})
				return
			}
		}
		o.hold(e)
		return
	}
	o.sendLocal(ctx, e, "")
}

// Deliver sends an event that arrived from elsewhere (Pulse receiving
// from an app) to this app's own channels, waiting for the result.
func (o *Outbox) Deliver(ctx context.Context, e Event) Sent {
	e = e.withDefaults()
	return o.sendLocal(ctx, e, "")
}

func (o *Outbox) sendLocal(ctx context.Context, e Event, note string) Sent {
	chans, err := o.Store.List(ctx)
	s := Sent{Time: e.Time, App: e.App, Type: e.Type, Title: e.Title, Level: e.Level, Via: "this app", Error: note}
	if err != nil {
		s.Error = "couldn't read channels: " + err.Error()
	} else {
		s.Results = Send(ctx, chans, e)
		for _, r := range s.Results {
			if !r.OK() {
				o.logger().Warn("notify: send failed", "channel", r.ChannelName, "title", e.Title, "error", r.Error)
			}
		}
	}
	if len(s.Results) > 0 || s.Error != "" {
		o.remember(s)
	}
	return s
}

func (o *Outbox) hold(e Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.held) >= maxHeld {
		o.logger().Warn("notify: too many events waiting for Pulse; dropping the oldest", "title", o.held[0].e.Title)
		o.held = o.held[1:]
	}
	o.held = append(o.held, heldEvent{e: e, since: time.Now()})
	o.addRecent(Sent{Time: e.Time, App: e.App, Type: e.Type, Title: e.Title, Level: e.Level, Via: "Pulse", Pending: true})
}

// Run retries held events until ctx ends. Start it once per app.
func (o *Outbox) Run(ctx context.Context) {
	every := o.RetryEvery
	if every <= 0 {
		every = 30 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			o.retry(ctx)
		}
	}
}

func (o *Outbox) retry(ctx context.Context) {
	o.mu.Lock()
	held := o.held
	o.held = nil
	o.mu.Unlock()
	if len(held) == 0 {
		return
	}
	holdFor := o.HoldFor
	if holdFor <= 0 {
		holdFor = 5 * time.Minute
	}
	h := o.hub()
	var still []heldEvent
	for _, he := range held {
		if h != nil && o.Store.Mode(ctx) == ModeAuto {
			sctx, cancel := context.WithTimeout(ctx, SendTimeout)
			err := h.Notify(sctx, he.e)
			cancel()
			if err == nil {
				o.resolve(he.e, Sent{Via: "Pulse"})
				continue
			}
		}
		if time.Since(he.since) < holdFor && h != nil {
			still = append(still, he)
			continue
		}
		// Pulse didn't come back in time: the app's own channels, if any.
		if o.Store.HasEnabled(ctx) {
			s := o.sendLocal(ctx, he.e, "Pulse didn't answer; sent from this app instead")
			o.resolve(he.e, s)
			o.dropRecent(s) // resolve updated the pending entry; avoid a duplicate
		} else {
			o.logger().Warn("notify: Pulse unreachable and no channels of its own; dropping", "title", he.e.Title)
			o.resolve(he.e, Sent{Via: "Pulse", Error: "Pulse didn't answer, and this app has no channels of its own"})
		}
	}
	o.mu.Lock()
	o.held = append(still, o.held...)
	o.mu.Unlock()
}

// resolve updates the pending "recently sent" entry for e.
func (o *Outbox) resolve(e Event, s Sent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := range o.recent {
		r := &o.recent[i]
		if r.Pending && r.Time.Equal(e.Time) && r.Title == e.Title && r.Type == e.Type {
			r.Pending = false
			r.Via, r.Results, r.Error = s.Via, s.Results, s.Error
			return
		}
	}
}

// dropRecent removes the newest entry equal to s (added by sendLocal).
func (o *Outbox) dropRecent(s Sent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := len(o.recent) - 1; i >= 0; i-- {
		r := o.recent[i]
		if !r.Pending && r.Time.Equal(s.Time) && r.Title == s.Title && r.Via == "this app" && r.Error == s.Error {
			o.recent = append(o.recent[:i], o.recent[i+1:]...)
			return
		}
	}
}

func (o *Outbox) remember(s Sent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.addRecent(s)
}

func (o *Outbox) addRecent(s Sent) {
	o.recent = append(o.recent, s)
	if len(o.recent) > maxRecent {
		o.recent = o.recent[len(o.recent)-maxRecent:]
	}
}

// Recent returns the latest sends, newest first.
func (o *Outbox) Recent() []Sent {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Sent, len(o.recent))
	for i, s := range o.recent {
		out[len(o.recent)-1-i] = s
	}
	return out
}

// Held reports how many events are waiting for Pulse.
func (o *Outbox) Held() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.held)
}
