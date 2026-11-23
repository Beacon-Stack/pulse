// Package notify sends Beacon notifications to Discord, Slack, Telegram,
// ntfy, Pushover, Gotify, email and webhooks.
//
// Every Beacon app uses it: Pulse to broadcast on the apps' behalf, and
// Pilot, Prism and Haul to send on their own when they run without Pulse.
// One event vocabulary, one set of providers and one settings format, so
// a channel set up in one place behaves the same everywhere.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"
)

// Level says how an event should look: colour, priority, emoji.
type Level string

const (
	LevelInfo    Level = "info"
	LevelSuccess Level = "success"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
)

// Event types shared by every app. Apps translate their internal events
// into these; anything else is not sent.
const (
	TypeAdded           = "added"            // a show or movie was added to the library
	TypeGrabbed         = "grabbed"          // a release was sent to the download client
	TypeDownloaded      = "downloaded"       // a download finished
	TypeImported        = "imported"         // a file landed in the library
	TypeImportFailed    = "import_failed"    // a finished download couldn't be imported
	TypeDownloadProblem = "download_problem" // a download failed or stalled
	TypeHealthIssue     = "health_issue"     // something needs attention
	TypeHealthOK        = "health_ok"        // a problem cleared
	TypeTest            = "test"             // the Send test button
)

// TypeInfo describes an event type for settings screens.
type TypeInfo struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Apps        []string `json:"apps"`    // which apps send it
	Default     bool     `json:"default"` // on for new channels
}

// Types lists the event types in the order settings screens show them.
var Types = []TypeInfo{
	{TypeImported, "Imported", "A show or movie is in your library, ready to watch.", []string{"pilot", "prism"}, true},
	{TypeImportFailed, "Import failed", "A finished download couldn't be added to the library.", []string{"pilot", "prism"}, true},
	{TypeDownloaded, "Download finished", "A download completed.", []string{"pilot", "prism", "haul"}, false},
	{TypeDownloadProblem, "Download problem", "A download failed or stopped making progress.", []string{"pilot", "prism", "haul"}, true},
	{TypeGrabbed, "Release grabbed", "A release was sent to the download client.", []string{"pilot", "prism"}, false},
	{TypeAdded, "Added to library", "A show or movie was added to be tracked.", []string{"pilot", "prism"}, false},
	{TypeHealthIssue, "Health problem", "Something needs attention, like an unreachable indexer or a full disk.", []string{"pilot", "prism", "haul", "pulse"}, true},
	{TypeHealthOK, "Health restored", "A health problem cleared.", []string{"pilot", "prism", "haul", "pulse"}, false},
}

// DefaultTypes is the quiet, useful set new channels start with.
func DefaultTypes() []string {
	var out []string
	for _, t := range Types {
		if t.Default {
			out = append(out, t.ID)
		}
	}
	return out
}

// KnownType reports whether t is one of the shared event types.
func KnownType(t string) bool {
	if t == TypeTest {
		return true
	}
	for _, x := range Types {
		if x.ID == t {
			return true
		}
	}
	return false
}

// Event is one notification.
type Event struct {
	App      string            `json:"app"`                 // "Pilot", "Prism", "Haul", "Pulse"
	Type     string            `json:"type"`                // one of the Type constants
	Title    string            `json:"title"`               // one line: "Imported: Andor S02E03"
	Message  string            `json:"message,omitempty"`   // optional detail
	Level    Level             `json:"level"`               // info, success, warning, error
	Time     time.Time         `json:"time"`                // when it happened
	URL      string            `json:"url,omitempty"`       // a link for more (TMDB, IMDb, the app)
	ImageURL string            `json:"image_url,omitempty"` // a poster
	Fields   map[string]string `json:"fields,omitempty"`    // key facts: Quality, Size, Indexer
}

// Validate checks the fields every provider relies on.
func (e Event) Validate() error {
	switch {
	case e.App == "":
		return errors.New("notify: event has no app")
	case !KnownType(e.Type):
		return fmt.Errorf("notify: unknown event type %q", e.Type)
	case e.Title == "":
		return errors.New("notify: event has no title")
	}
	return nil
}

func (e Event) withDefaults() Event {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Level == "" {
		e.Level = LevelInfo
	}
	return e
}

// TestEvent is what the Send test button sends.
func TestEvent(app string) Event {
	return Event{
		App:     app,
		Type:    TypeTest,
		Title:   "Test notification",
		Level:   LevelSuccess,
		Time:    time.Now().UTC(),
		Message: fmt.Sprintf("%s can reach this channel. You'll get notifications here.", app),
	}
}

// Sender delivers events to one configured channel.
type Sender interface {
	Send(ctx context.Context, e Event) error
}

// FieldType tells a settings screen which input to show.
type FieldType string

const (
	FieldText     FieldType = "text"
	FieldPassword FieldType = "password" // a secret: masked when read back
	FieldURL      FieldType = "url"
	FieldNumber   FieldType = "number"
	FieldCheckbox FieldType = "checkbox"
	FieldList     FieldType = "list" // comma-separated in the form, a JSON array in settings
)

// Field is one setting of a provider.
type Field struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Type        FieldType `json:"type"`
	Required    bool      `json:"required,omitempty"`
	Placeholder string    `json:"placeholder,omitempty"`
	Help        string    `json:"help,omitempty"`
}

// Secret reports whether the field holds something that must not be shown.
func (f Field) Secret() bool { return f.Type == FieldPassword }

// Kind is a provider: its settings and how to build a Sender.
type Kind struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Help   string  `json:"help,omitempty"`
	Fields []Field `json:"fields"`

	build func(json.RawMessage) (Sender, error)
	// secretKeys are settings to mask besides password fields (a webhook
	// URL is a secret: anyone holding it can post to the channel).
	secretKeys []string
}

func (k Kind) isSecret(key string) bool {
	for _, f := range k.Fields {
		if f.Key == key && f.Secret() {
			return true
		}
	}
	return slices.Contains(k.secretKeys, key)
}

var (
	kindsMu sync.RWMutex
	kinds   = map[string]Kind{}
)

func register(k Kind) {
	kindsMu.Lock()
	defer kindsMu.Unlock()
	kinds[k.ID] = k
}

// Kinds lists the providers, sorted by name, for settings screens.
func Kinds() []Kind {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	out := make([]Kind, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ErrUnknownKind is returned for a provider this version doesn't have
// (including the retired "command" type).
var ErrUnknownKind = errors.New("notify: unknown channel type")

func lookup(kind string) (Kind, error) {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	k, ok := kinds[kind]
	if !ok {
		return Kind{}, fmt.Errorf("%w %q", ErrUnknownKind, kind)
	}
	return k, nil
}

// New builds a Sender for a provider from its JSON settings, checking
// required settings.
func New(kind string, settings json.RawMessage) (Sender, error) {
	k, err := lookup(kind)
	if err != nil {
		return nil, err
	}
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	var m map[string]any
	if err := json.Unmarshal(settings, &m); err != nil {
		return nil, fmt.Errorf("%s: settings are not valid JSON: %w", k.Name, err)
	}
	for _, f := range k.Fields {
		if !f.Required {
			continue
		}
		if v, ok := m[f.Key]; !ok || isEmpty(v) {
			return nil, fmt.Errorf("%s: %s is required", k.Name, f.Label)
		}
	}
	return k.build(settings)
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// Masked is what secrets look like when settings are read back.
const Masked = "***"

// Mask replaces secret settings with "***" so they can be shown or
// returned by an API.
func Mask(kind string, settings json.RawMessage) json.RawMessage {
	k, err := lookup(kind)
	var m map[string]json.RawMessage
	if json.Unmarshal(settings, &m) != nil {
		return json.RawMessage("{}")
	}
	for key, v := range m {
		// Unknown kinds: mask everything that isn't obviously harmless.
		if (err == nil && k.isSecret(key)) || (err != nil && string(v) != `""`) {
			m[key] = json.RawMessage(`"` + Masked + `"`)
		}
	}
	out, _ := json.Marshal(m)
	return out
}

// Merge applies edited settings over stored ones. A secret sent back as
// "***" (or left out) keeps the stored value, so editing a channel's name
// doesn't wipe its token.
func Merge(kind string, stored, incoming json.RawMessage) json.RawMessage {
	k, _ := lookup(kind)
	var old, inc map[string]json.RawMessage
	_ = json.Unmarshal(stored, &old)
	if json.Unmarshal(incoming, &inc) != nil || inc == nil {
		inc = map[string]json.RawMessage{}
	}
	for key, v := range old {
		cur, sent := inc[key]
		if !sent && k.isSecret(key) {
			inc[key] = v
		} else if sent && string(cur) == `"`+Masked+`"` {
			inc[key] = v
		}
	}
	out, _ := json.Marshal(inc)
	return out
}

// Channel is a configured destination plus what it wants to hear about.
type Channel struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	Enabled  bool            `json:"enabled"`
	Settings json.RawMessage `json:"settings"`
	Events   []string        `json:"events"` // empty: every shared type
	Apps     []string        `json:"apps"`   // lower-case app names; empty: every app
}

// Wants reports whether the channel should receive e.
func (c Channel) Wants(e Event) bool {
	if !c.Enabled {
		return false
	}
	if e.Type == TypeTest {
		return true
	}
	if len(c.Events) > 0 && !slices.Contains(c.Events, e.Type) {
		return false
	}
	if len(c.Apps) > 0 && !slices.ContainsFunc(c.Apps, func(a string) bool { return equalFold(a, e.App) }) {
		return false
	}
	return true
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// Result is the outcome of sending to one channel.
type Result struct {
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Error       string `json:"error,omitempty"`
}

// OK reports whether the send succeeded.
func (r Result) OK() bool { return r.Error == "" }

// SendTimeout bounds each delivery, so one slow service can't hold up
// the rest.
const SendTimeout = 20 * time.Second

// Send delivers e to every channel that wants it, in parallel, and
// reports each outcome. Channels that don't want e are skipped silently.
func Send(ctx context.Context, channels []Channel, e Event) []Result {
	e = e.withDefaults()
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []Result
	)
	for _, ch := range channels {
		if !ch.Wants(e) {
			continue
		}
		wg.Add(1)
		go func(ch Channel) {
			defer wg.Done()
			r := Result{ChannelID: ch.ID, ChannelName: ch.Name}
			if err := SendOne(ctx, ch, e); err != nil {
				r.Error = err.Error()
			}
			mu.Lock()
			out = append(out, r)
			mu.Unlock()
		}(ch)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].ChannelName < out[j].ChannelName })
	return out
}

// SendOne delivers e to one channel regardless of its event filter (for
// the Send test button).
func SendOne(ctx context.Context, ch Channel, e Event) error {
	e = e.withDefaults()
	if err := e.Validate(); err != nil {
		return err
	}
	s, err := New(ch.Kind, ch.Settings)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, SendTimeout)
	defer cancel()
	return s.Send(ctx, e)
}
