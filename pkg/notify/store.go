package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store keeps an app's notification channels and its routing choice in
// the app's own SQLite database. Every Beacon app uses the same tables,
// created here, so the feature is identical everywhere.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS notify_channels (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	kind       TEXT NOT NULL,
	enabled    INTEGER NOT NULL DEFAULT 1,
	settings   TEXT NOT NULL DEFAULT '{}',
	events     TEXT NOT NULL DEFAULT '[]',
	apps       TEXT NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS notify_settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`

// NewStore opens the store, creating its tables if needed.
func NewStore(ctx context.Context, db *sql.DB) (*Store, error) {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return nil, fmt.Errorf("notify: creating tables: %w", err)
	}
	return &Store{db: db}, nil
}

// ErrNotFound is returned for a channel that doesn't exist.
var ErrNotFound = errors.New("notify: channel not found")

// Routing choices for an app.
const (
	// ModeAuto sends through Pulse when the app is connected to a Pulse
	// that has channels, and through the app's own channels otherwise.
	ModeAuto = "auto"
	// ModeLocal always uses the app's own channels.
	ModeLocal = "local"
)

// Mode returns the app's routing choice (ModeAuto unless set).
func (s *Store) Mode(ctx context.Context) string {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM notify_settings WHERE key = 'mode'`).Scan(&v); err != nil || v != ModeLocal {
		return ModeAuto
	}
	return ModeLocal
}

// SetMode stores the routing choice.
func (s *Store) SetMode(ctx context.Context, mode string) error {
	if mode != ModeAuto && mode != ModeLocal {
		return fmt.Errorf("notify: unknown mode %q", mode)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notify_settings (key, value) VALUES ('mode', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, mode)
	return err
}

const columns = `id, name, kind, enabled, settings, events, apps`

func scanChannel(row interface{ Scan(...any) error }) (Channel, error) {
	var (
		c                    Channel
		enabled              int
		settings, evts, apps string
	)
	if err := row.Scan(&c.ID, &c.Name, &c.Kind, &enabled, &settings, &evts, &apps); err != nil {
		return Channel{}, err
	}
	c.Enabled = enabled != 0
	c.Settings = json.RawMessage(settings)
	_ = json.Unmarshal([]byte(evts), &c.Events)
	_ = json.Unmarshal([]byte(apps), &c.Apps)
	if c.Events == nil {
		c.Events = []string{}
	}
	if c.Apps == nil {
		c.Apps = []string{}
	}
	return c, nil
}

// List returns every channel, by name, with settings as stored.
func (s *Store) List(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM notify_channels ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one channel.
func (s *Store) Get(ctx context.Context, id string) (Channel, error) {
	c, err := scanChannel(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM notify_channels WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return c, err
}

// HasEnabled reports whether any channel is switched on.
func (s *Store) HasEnabled(ctx context.Context) bool {
	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notify_channels WHERE enabled = 1`).Scan(&n)
	return n > 0
}

// Count returns how many channels exist and how many are on.
func (s *Store) Count(ctx context.Context) (total, enabled int) {
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(enabled), 0) FROM notify_channels`).Scan(&total, &enabled)
	return total, enabled
}

// cleanEvents keeps known types, in a stable order, without repeats.
func cleanEvents(in []string) []string {
	out := []string{}
	for _, t := range Types {
		for _, e := range in {
			if e == t.ID {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func cleanApps(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, a := range in {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// Validate checks a channel before it's saved.
func (c Channel) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("give the channel a name")
	}
	_, err := New(c.Kind, c.Settings)
	return err
}

// Create saves a new channel.
func (s *Store) Create(ctx context.Context, c Channel) (Channel, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Events, c.Apps = cleanEvents(c.Events), cleanApps(c.Apps)
	if len(c.Settings) == 0 {
		c.Settings = json.RawMessage("{}")
	}
	if err := c.validate(); err != nil {
		return Channel{}, err
	}
	c.ID = uuid.NewString()
	return c, s.write(ctx, c, true)
}

// Update replaces a channel's fields. Secrets sent back masked are kept.
func (s *Store) Update(ctx context.Context, id string, c Channel) (Channel, error) {
	old, err := s.Get(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	c.ID = id
	c.Name = strings.TrimSpace(c.Name)
	c.Events, c.Apps = cleanEvents(c.Events), cleanApps(c.Apps)
	if old.Kind == c.Kind {
		c.Settings = Merge(c.Kind, old.Settings, c.Settings)
	}
	if err := c.validate(); err != nil {
		return Channel{}, err
	}
	return c, s.write(ctx, c, false)
}

func (s *Store) write(ctx context.Context, c Channel, insert bool) error {
	evts, _ := json.Marshal(c.Events)
	apps, _ := json.Marshal(c.Apps)
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	if insert {
		_, err := s.db.ExecContext(ctx, `INSERT INTO notify_channels (id, name, kind, enabled, settings, events, apps, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, c.ID, c.Name, c.Kind, enabled, string(c.Settings), string(evts), string(apps), now, now)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notify_channels SET name = ?, kind = ?, enabled = ?, settings = ?, events = ?, apps = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Kind, enabled, string(c.Settings), string(evts), string(apps), now, c.ID)
	return err
}

// Delete removes a channel.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notify_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// legacyEventNames maps the event names Pilot and Prism stored before the
// shared vocabulary existed.
var legacyEventNames = map[string]string{
	"download_done":   TypeDownloaded,
	"import_complete": TypeImported,
	"import_done":     TypeImported,
	"import_failed":   TypeImportFailed,
	"grab_started":    TypeGrabbed,
	"episode_grabbed": TypeGrabbed,
	"grab_failed":     TypeDownloadProblem,
	"grab_stalled":    TypeDownloadProblem,
	"health_issue":    TypeHealthIssue,
	"health_ok":       TypeHealthOK,
	"series_added":    TypeAdded,
	"show_added":      TypeAdded,
	"movie_added":     TypeAdded,
}

// LegacyChannel is a channel as Pilot and Prism stored it before.
type LegacyChannel struct {
	Name     string
	Kind     string
	Enabled  bool
	Settings json.RawMessage
	Events   []string // old event names
}

// ImportLegacy copies an app's old channels in, once. Old event names are
// translated; a channel that listed none keeps "every event". Channels of
// a retired type (the old "command" script type) are skipped and
// returned, so the app can say so.
func (s *Store) ImportLegacy(ctx context.Context, old []LegacyChannel) (imported int, skipped []string, err error) {
	var done string
	if s.db.QueryRowContext(ctx, `SELECT value FROM notify_settings WHERE key = 'legacy_imported'`).Scan(&done) == nil {
		return 0, nil, nil
	}
	for _, o := range old {
		if _, err := lookup(o.Kind); err != nil {
			skipped = append(skipped, o.Name+" ("+o.Kind+")")
			continue
		}
		var evts []string
		for _, e := range o.Events {
			if n, ok := legacyEventNames[e]; ok {
				evts = append(evts, n)
			}
		}
		if len(o.Events) > 0 && len(evts) == 0 {
			// It picked only events that no longer exist; don't turn that
			// into "everything".
			evts = DefaultTypes()
		}
		c := Channel{ID: uuid.NewString(), Name: o.Name, Kind: o.Kind, Enabled: o.Enabled, Settings: o.Settings, Events: cleanEvents(evts), Apps: []string{}}
		if len(c.Settings) == 0 {
			c.Settings = json.RawMessage("{}")
		}
		if err := s.write(ctx, c, true); err != nil {
			return imported, skipped, err
		}
		imported++
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO notify_settings (key, value) VALUES ('legacy_imported', ?)`, time.Now().UTC().Format(time.RFC3339))
	return imported, skipped, err
}
