package qualityprofile

// seed_test.go — regression tests for the default-quality-profile seed
// data. Pulse seeds these profiles on first run; Prism and Pilot
// download them via their sync loops and decode the JSON blobs into
// `plugin.Quality` records. Any silent shape change here propagates
// to both downstream services and breaks scoring without warning.
//
// The Service CRUD methods are not covered here — they need a SQLite
// testdb infrastructure that doesn't exist in Pulse yet. The pure-Go
// seed helpers below are the most leverage we can get without that.

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

// quality is the decode target for the JSON blobs the seed helpers emit.
// Mirrors the shape Prism/Pilot's `plugin.Quality` unmarshals against.
type quality struct {
	Name       string `json:"name"`
	Resolution string `json:"resolution"`
	Source     string `json:"source"`
	Codec      string `json:"codec"`
	HDR        string `json:"hdr"`
}

// ── qualityJSON ──────────────────────────────────────────────────────────────

// qualityJSON's output must round-trip cleanly through json.Unmarshal
// into the shape Prism/Pilot expect. A regression in the field-name
// case or order would silently drop fields downstream.
func TestQualityJSON_RoundTripsToExpectedShape(t *testing.T) {
	got := qualityJSON("1080p Bluray", "1080p", "bluray", "x265", "none")

	var q quality
	if err := json.Unmarshal([]byte(got), &q); err != nil {
		t.Fatalf("unmarshal: %v\nblob=%s", err, got)
	}
	if q.Name != "1080p Bluray" {
		t.Errorf("Name = %q, want %q", q.Name, "1080p Bluray")
	}
	if q.Resolution != "1080p" {
		t.Errorf("Resolution = %q, want %q", q.Resolution, "1080p")
	}
	if q.Source != "bluray" {
		t.Errorf("Source = %q, want %q", q.Source, "bluray")
	}
	if q.Codec != "x265" {
		t.Errorf("Codec = %q, want %q", q.Codec, "x265")
	}
	if q.HDR != "none" {
		t.Errorf("HDR = %q, want %q", q.HDR, "none")
	}
}

// JSON injection guard: a `"` in the input must not break the produced
// JSON. The helper uses %q which escapes properly; this test ensures
// nobody accidentally swaps it for %s.
func TestQualityJSON_EscapesQuotesInInput(t *testing.T) {
	got := qualityJSON(`Bad"Name`, "1080p", "bluray", "x264", "none")

	var q quality
	if err := json.Unmarshal([]byte(got), &q); err != nil {
		t.Fatalf("unmarshal failed — %%q escaping was lost: %v\nblob=%s", err, got)
	}
	if q.Name != `Bad"Name` {
		t.Errorf("Name = %q, want %q", q.Name, `Bad"Name`)
	}
}

// ── qualitiesJSON ────────────────────────────────────────────────────────────

// qualitiesJSON produces a JSON array of the same shape, with as many
// elements as the input list. A bug in the comma-joining loop would
// surface here as either a parse error or a wrong-length array.
func TestQualitiesJSON_ProducesArrayWithAllEntries(t *testing.T) {
	entries := []qualityEntry{
		{"720p Bluray", "720p", "bluray", "x264", "none"},
		{"1080p Bluray", "1080p", "bluray", "x265", "none"},
		{"2160p Bluray HDR", "2160p", "bluray", "x265", "hdr10"},
	}
	got := qualitiesJSON(entries)

	var qs []quality
	if err := json.Unmarshal([]byte(got), &qs); err != nil {
		t.Fatalf("unmarshal: %v\nblob=%s", err, got)
	}
	if len(qs) != 3 {
		t.Fatalf("len(qs) = %d, want 3", len(qs))
	}
	if qs[0].Name != "720p Bluray" || qs[2].HDR != "hdr10" {
		t.Errorf("array contents wrong: %+v", qs)
	}
}

// Empty list edge case: must produce `[]`, not `[` or `null`. Prism/Pilot
// json.Unmarshal of an empty quality profile must succeed without
// special-casing.
func TestQualitiesJSON_EmptyList(t *testing.T) {
	got := qualitiesJSON(nil)
	if got != "[]" {
		t.Errorf("empty qualitiesJSON = %q, want %q", got, "[]")
	}
}

// ── defaultProfiles ──────────────────────────────────────────────────────────

// Every default profile's cutoff must appear in its qualities list.
// Forgetting to add a quality matching the cutoff means the cutoff
// can never be reached, so every release is "below cutoff" forever
// and upgrades fire indefinitely (or never, depending on profile
// behavior). This was a real Sonarr bug class in 2017.
func TestDefaultProfiles_CutoffPresentInQualities(t *testing.T) {
	for _, p := range defaultProfiles() {
		var cutoff quality
		if err := json.Unmarshal([]byte(p.CutoffJSON), &cutoff); err != nil {
			t.Errorf("%s: cutoff_json invalid: %v", p.Name, err)
			continue
		}
		var qs []quality
		if err := json.Unmarshal([]byte(p.QualitiesJSON), &qs); err != nil {
			t.Errorf("%s: qualities_json invalid: %v", p.Name, err)
			continue
		}
		if cutoff.Name == "" {
			t.Errorf("%s: cutoff has empty name", p.Name)
			continue
		}
		found := false
		for _, q := range qs {
			if q.Name == cutoff.Name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: cutoff %q not present in qualities list — releases would never satisfy the cutoff",
				p.Name, cutoff.Name)
		}
	}
}

// Profile names must be unique. Two profiles with the same name would
// shadow each other in the Prism/Pilot UI dropdown.
func TestDefaultProfiles_NamesAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range defaultProfiles() {
		if seen[p.Name] {
			t.Errorf("duplicate profile name %q", p.Name)
		}
		seen[p.Name] = true
	}
}

// Every profile must declare a non-empty name and at least one quality.
// A profile with no qualities is a contract violation — Prism/Pilot
// would skip every release for it.
func TestDefaultProfiles_NonEmpty(t *testing.T) {
	profiles := defaultProfiles()
	if len(profiles) == 0 {
		t.Fatal("defaultProfiles() returned no profiles")
	}
	for _, p := range profiles {
		if strings.TrimSpace(p.Name) == "" {
			t.Errorf("profile has empty name: %+v", p)
		}
		var qs []quality
		if err := json.Unmarshal([]byte(p.QualitiesJSON), &qs); err != nil || len(qs) == 0 {
			t.Errorf("profile %q has no qualities (parse err: %v, len: %d)", p.Name, err, len(qs))
		}
	}
}

// Pin the canonical profile set so we notice if someone accidentally
// drops one. The names are referenced by the Sonarr/Radarr import
// path and by user docs — silently changing them is user-visible.
func TestDefaultProfiles_PinnedProfileNames(t *testing.T) {
	want := []string{"SD", "HD-720p", "HD-1080p", "Ultra-HD", "Any"}
	got := defaultProfiles()
	if len(got) != len(want) {
		t.Fatalf("len(profiles) = %d, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Name != want[i] {
			t.Errorf("profile[%d].Name = %q, want %q", i, p.Name, want[i])
		}
	}
}

// ── nullStringFromPtr ────────────────────────────────────────────────────────

func TestNullStringFromPtr_Nil(t *testing.T) {
	got := nullStringFromPtr(nil)
	if got.Valid {
		t.Errorf("Valid = true, want false for nil input")
	}
	if got.String != "" {
		t.Errorf("String = %q, want empty", got.String)
	}
}

func TestNullStringFromPtr_NonNil(t *testing.T) {
	s := "hello"
	got := nullStringFromPtr(&s)
	want := sql.NullString{String: "hello", Valid: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Empty-string pointer is a real value that should round-trip as
// Valid=true. Only nil pointers should produce Valid=false.
func TestNullStringFromPtr_EmptyStringIsValid(t *testing.T) {
	s := ""
	got := nullStringFromPtr(&s)
	if !got.Valid {
		t.Errorf("Valid = false for empty-string pointer; want true (only nil should produce !Valid)")
	}
}
