// Package notifyapi is the notifications settings API, the same in every
// Beacon app: channels, test sends, routing and the recent list. Pulse
// also accepts events from the apps here.
package notifyapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pulse/pkg/notify"
)

// Options wires the API to an app.
type Options struct {
	App    string // display name: "Pilot", "Pulse"
	Store  *notify.Store
	Outbox *notify.Outbox
	// Hub is true for Pulse: it receives events from apps and its channels
	// can be limited to some apps.
	Hub bool
}

// API types have their own names: huma names schemas after Go types, and
// "Field" or "Event" would clash with types in the apps that mount this.

type notifyField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
}

type notifyKind struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Help   string        `json:"help,omitempty"`
	Fields []notifyField `json:"fields"`
}

type notifyType struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Apps        []string `json:"apps"`
	Default     bool     `json:"default"`
}

type notifyRoute struct {
	Mode           string `json:"mode"`
	ViaPulse       bool   `json:"via_pulse"`
	PulseConnected bool   `json:"pulse_connected"`
	PulseChannels  int    `json:"pulse_channels"`
}

type notifyResult struct {
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Error       string `json:"error,omitempty"`
}

type notifySent struct {
	Time    time.Time      `json:"time"`
	App     string         `json:"app"`
	Type    string         `json:"type"`
	Title   string         `json:"title"`
	Level   string         `json:"level"`
	Via     string         `json:"via"`
	Results []notifyResult `json:"results"`
	Error   string         `json:"error,omitempty"`
	Pending bool           `json:"pending,omitempty"`
}

type notifyEvent struct {
	App      string            `json:"app" maxLength:"40"`
	Type     string            `json:"type" maxLength:"40"`
	Title    string            `json:"title" maxLength:"500"`
	Message  string            `json:"message,omitempty" maxLength:"4000"`
	Level    string            `json:"level,omitempty" enum:"info,success,warning,error,"`
	Time     time.Time         `json:"time,omitempty"`
	URL      string            `json:"url,omitempty" maxLength:"2000"`
	ImageURL string            `json:"image_url,omitempty" maxLength:"2000"`
	Fields   map[string]string `json:"fields,omitempty"`
}

func kindsBody() []notifyKind {
	var out []notifyKind
	for _, k := range notify.Kinds() {
		nk := notifyKind{ID: k.ID, Name: k.Name, Help: k.Help, Fields: []notifyField{}}
		for _, f := range k.Fields {
			nk.Fields = append(nk.Fields, notifyField{f.Key, f.Label, string(f.Type), f.Required, f.Placeholder, f.Help})
		}
		out = append(out, nk)
	}
	return out
}

func typesBody() []notifyType {
	out := make([]notifyType, 0, len(notify.Types))
	for _, t := range notify.Types {
		out = append(out, notifyType(t))
	}
	return out
}

func routeBody(r notify.Route) notifyRoute { return notifyRoute(r) }

func resultsBody(rs []notify.Result) []notifyResult {
	out := make([]notifyResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, notifyResult(r))
	}
	return out
}

func sentBody(ss []notify.Sent) []notifySent {
	out := make([]notifySent, 0, len(ss))
	for _, s := range ss {
		out = append(out, notifySent{s.Time, s.App, s.Type, s.Title, string(s.Level), s.Via, resultsBody(s.Results), s.Error, s.Pending})
	}
	return out
}

// Apps are the Beacon apps a Pulse channel can be limited to.
var Apps = []appInfo{{"pilot", "Pilot"}, {"prism", "Prism"}, {"haul", "Haul"}, {"pulse", "Pulse"}}

type appInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type catalogBody struct {
	App   string       `json:"app"`
	Hub   bool         `json:"hub"`
	Kinds []notifyKind `json:"kinds"`
	Types []notifyType `json:"types"`
	Apps  []appInfo    `json:"apps,omitempty"`
}

type statusBody struct {
	App      string      `json:"app"`
	Hub      bool        `json:"hub"`
	Route    notifyRoute `json:"route"`
	Channels int         `json:"channels"`
	Enabled  int         `json:"enabled"`
	Waiting  int         `json:"waiting"` // events held for Pulse
}

type channelBody struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Enabled  bool           `json:"enabled"`
	Settings map[string]any `json:"settings"`
	Events   []string       `json:"events"`
	Apps     []string       `json:"apps"`
	Retired  bool           `json:"retired,omitempty" doc:"A channel type this version no longer has"`
}

type channelInput struct {
	Name     string         `json:"name" maxLength:"100"`
	Kind     string         `json:"kind" maxLength:"40"`
	Enabled  bool           `json:"enabled"`
	Settings map[string]any `json:"settings,omitempty"`
	Events   []string       `json:"events,omitempty" maxItems:"20"`
	Apps     []string       `json:"apps,omitempty" maxItems:"10"`
}

type testInput struct {
	Body struct {
		ID       string         `json:"id,omitempty" doc:"An existing channel; masked secrets are filled in from it"`
		Kind     string         `json:"kind" maxLength:"40"`
		Settings map[string]any `json:"settings,omitempty"`
	}
}

type testBody struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type sendBody struct {
	Delivered int            `json:"delivered"`
	Results   []notifyResult `json:"results"`
}

func toChannel(in channelInput) notify.Channel {
	s, _ := json.Marshal(in.Settings)
	if in.Settings == nil {
		s = []byte("{}")
	}
	return notify.Channel{Name: in.Name, Kind: in.Kind, Enabled: in.Enabled, Settings: s, Events: in.Events, Apps: in.Apps}
}

func fromChannel(c notify.Channel) channelBody {
	var m map[string]any
	_ = json.Unmarshal(notify.Mask(c.Kind, c.Settings), &m)
	if m == nil {
		m = map[string]any{}
	}
	_, err := notify.New(c.Kind, json.RawMessage(`{}`))
	return channelBody{
		ID: c.ID, Name: c.Name, Kind: c.Kind, Enabled: c.Enabled, Settings: m, Events: c.Events, Apps: c.Apps,
		Retired: errors.Is(err, notify.ErrUnknownKind),
	}
}

// badRequest turns a validation error into a plain 400.
func badRequest(err error) error {
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "notify: ")
	return huma.Error400BadRequest(msg)
}

// Register adds the notifications API to api.
func Register(api huma.API, o Options) {
	tags := []string{"Notifications"}

	huma.Register(api, huma.Operation{
		OperationID: "notifications-catalog", Method: http.MethodGet, Path: "/api/v1/notifications/catalog",
		Summary: "Channel types, their settings and the events", Tags: tags,
	}, func(context.Context, *struct{}) (*struct{ Body catalogBody }, error) {
		b := catalogBody{App: o.App, Hub: o.Hub, Kinds: kindsBody(), Types: typesBody()}
		if o.Hub {
			b.Apps = Apps
		}
		return &struct{ Body catalogBody }{Body: b}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "notifications-status", Method: http.MethodGet, Path: "/api/v1/notifications/status",
		Summary: "Where notifications go right now", Tags: tags,
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body statusBody }, error) {
		total, on := o.Store.Count(ctx)
		b := statusBody{App: o.App, Hub: o.Hub, Channels: total, Enabled: on, Waiting: o.Outbox.Held()}
		if o.Hub {
			b.Route = notifyRoute{Mode: notify.ModeLocal}
		} else {
			b.Route = routeBody(o.Outbox.Route(ctx))
		}
		return &struct{ Body statusBody }{Body: b}, nil
	})

	if !o.Hub {
		huma.Register(api, huma.Operation{
			OperationID: "notifications-set-mode", Method: http.MethodPut, Path: "/api/v1/notifications/mode",
			Summary: "Send through Pulse when connected (auto), or always from this app (local)", Tags: tags,
		}, func(ctx context.Context, in *struct {
			Body struct {
				Mode string `json:"mode" enum:"auto,local"`
			}
		}) (*struct{ Body notifyRoute }, error) {
			if err := o.Store.SetMode(ctx, in.Body.Mode); err != nil {
				return nil, badRequest(err)
			}
			return &struct{ Body notifyRoute }{Body: routeBody(o.Outbox.Route(ctx))}, nil
		})
	}

	huma.Register(api, huma.Operation{
		OperationID: "notifications-list-channels", Method: http.MethodGet, Path: "/api/v1/notifications/channels",
		Summary: "List channels (secrets masked)", Tags: tags,
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []channelBody }, error) {
		list, err := o.Store.List(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("couldn't read channels", err)
		}
		out := make([]channelBody, 0, len(list))
		for _, c := range list {
			out = append(out, fromChannel(c))
		}
		return &struct{ Body []channelBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "notifications-create-channel", Method: http.MethodPost, Path: "/api/v1/notifications/channels",
		Summary: "Add a channel", Tags: tags, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct{ Body channelInput }) (*struct{ Body channelBody }, error) {
		c, err := o.Store.Create(ctx, toChannel(in.Body))
		if err != nil {
			return nil, badRequest(err)
		}
		return &struct{ Body channelBody }{Body: fromChannel(c)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "notifications-update-channel", Method: http.MethodPut, Path: "/api/v1/notifications/channels/{id}",
		Summary: "Change a channel (masked secrets are kept)", Tags: tags,
	}, func(ctx context.Context, in *struct {
		ID   string `path:"id"`
		Body channelInput
	}) (*struct{ Body channelBody }, error) {
		c, err := o.Store.Update(ctx, in.ID, toChannel(in.Body))
		if errors.Is(err, notify.ErrNotFound) {
			return nil, huma.Error404NotFound("channel not found")
		}
		if err != nil {
			return nil, badRequest(err)
		}
		return &struct{ Body channelBody }{Body: fromChannel(c)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "notifications-delete-channel", Method: http.MethodDelete, Path: "/api/v1/notifications/channels/{id}",
		Summary: "Remove a channel", Tags: tags, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		if err := o.Store.Delete(ctx, in.ID); errors.Is(err, notify.ErrNotFound) {
			return nil, huma.Error404NotFound("channel not found")
		} else if err != nil {
			return nil, huma.Error500InternalServerError("couldn't remove channel", err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "notifications-test", Method: http.MethodPost, Path: "/api/v1/notifications/test",
		Summary: "Send a test message with these settings (saved or not)", Tags: tags,
	}, func(ctx context.Context, in *testInput) (*struct{ Body testBody }, error) {
		s, _ := json.Marshal(in.Body.Settings)
		if in.Body.Settings == nil {
			s = []byte("{}")
		}
		if in.Body.ID != "" {
			if old, err := o.Store.Get(ctx, in.Body.ID); err == nil && old.Kind == in.Body.Kind {
				s = notify.Merge(old.Kind, old.Settings, s)
			}
		}
		ch := notify.Channel{Name: "test", Kind: in.Body.Kind, Enabled: true, Settings: s}
		b := testBody{OK: true}
		if err := notify.SendOne(ctx, ch, notify.TestEvent(o.App)); err != nil {
			b = testBody{Error: strings.TrimPrefix(err.Error(), "notify: ")}
		}
		return &struct{ Body testBody }{Body: b}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "notifications-recent", Method: http.MethodGet, Path: "/api/v1/notifications/recent",
		Summary: "Recently sent notifications, newest first", Tags: tags,
	}, func(context.Context, *struct{}) (*struct{ Body []notifySent }, error) {
		return &struct{ Body []notifySent }{Body: sentBody(o.Outbox.Recent())}, nil
	})

	if o.Hub {
		huma.Register(api, huma.Operation{
			OperationID: "notifications-send", Method: http.MethodPost, Path: "/api/v1/notifications/send",
			Summary:     "Broadcast an app's event to Pulse's channels",
			Description: "Called by Pilot, Prism and Haul when they send notifications through Pulse.",
			Tags:        tags,
		}, func(ctx context.Context, in *struct{ Body notifyEvent }) (*struct{ Body sendBody }, error) {
			e := notify.Event{App: in.Body.App, Type: in.Body.Type, Title: in.Body.Title, Message: in.Body.Message,
				Level: notify.Level(in.Body.Level), Time: in.Body.Time, URL: in.Body.URL, ImageURL: in.Body.ImageURL, Fields: in.Body.Fields}
			if err := e.Validate(); err != nil {
				return nil, badRequest(err)
			}
			s := o.Outbox.Deliver(ctx, e)
			b := sendBody{Results: resultsBody(s.Results)}
			for _, r := range s.Results {
				if r.OK() {
					b.Delivered++
				}
			}
			return &struct{ Body sendBody }{Body: b}, nil
		})
	}
}
