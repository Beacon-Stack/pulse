package qualityprofile

import (
	"context"
	"fmt"

	cfgstore "github.com/beacon-stack/pulse/internal/core/config"
)

// seedNamespace and seedKey identify the one-shot marker that says
// "default quality profiles have been seeded, don't seed again".
const (
	seedNamespace = "system"
	seedKey       = "quality_profiles_seeded"
)

// SeedDefaults inserts a baseline set of quality profiles on first run so
// new users are never confronted with an empty dropdown when creating their
// first library in Prism or Pilot. Seeding is one-shot: once the marker is
// set, future restarts leave existing profiles alone, even if the user has
// deleted all of them.
//
// Seeded profiles are normal, fully editable Pulse profiles — the user can
// rename, adjust, or delete any of them, and advanced users can replace them
// wholesale via the TRaSH Guides integration when that lands.
func (s *Service) SeedDefaults(ctx context.Context, cfg *cfgstore.Store) error {
	// Already seeded once → no-op, even if the table is now empty.
	if _, err := cfg.Get(ctx, seedNamespace, seedKey); err == nil {
		return nil
	}

	// If the user somehow already has profiles without the marker (e.g. they
	// imported from Sonarr/Radarr pre-Pulse), don't clobber their work.
	// Set the marker and move on.
	existing, err := s.List(ctx)
	if err != nil {
		return fmt.Errorf("checking existing profiles before seeding: %w", err)
	}
	if len(existing) > 0 {
		_, err := cfg.Set(ctx, seedNamespace, seedKey, "true")
		return err
	}

	for _, p := range defaultProfiles() {
		if _, err := s.Create(ctx, p); err != nil {
			return fmt.Errorf("seeding quality profile %q: %w", p.Name, err)
		}
		s.logger.Info("seeded default quality profile", "name", p.Name)
	}

	if _, err := cfg.Set(ctx, seedNamespace, seedKey, "true"); err != nil {
		return fmt.Errorf("marking quality profiles as seeded: %w", err)
	}
	return nil
}

// defaultProfiles returns the baseline quality profile set. Every quality
// referenced here uses a slug present in BOTH Prism's and Pilot's
// quality_definitions tables, so the same profile works identically for
// both services. Quality slugs that aren't recognized locally are silently
// skipped during scoring — that's already the tolerance behavior.
//
// The set mirrors what Sonarr and Radarr ship out of the box so users
// coming from the *arr ecosystem see familiar names.
func defaultProfiles() []Input {
	return []Input{
		{
			Name:           "SD",
			CutoffJSON:     qualityJSON("SD DVD", "sd", "dvd", "xvid", "none"),
			QualitiesJSON:  qualitiesJSON([]qualityEntry{
				{"SD HDTV", "sd", "hdtv", "x264", "none"},
				{"SD DVD", "sd", "dvd", "xvid", "none"},
			}),
			UpgradeAllowed: true,
		},
		{
			Name:       "HD-720p",
			CutoffJSON: qualityJSON("720p Bluray", "720p", "bluray", "x264", "none"),
			QualitiesJSON: qualitiesJSON([]qualityEntry{
				{"720p HDTV", "720p", "hdtv", "x264", "none"},
				{"720p WEBDL", "720p", "webdl", "x264", "none"},
				{"720p WEBRip", "720p", "webrip", "x264", "none"},
				{"720p Bluray", "720p", "bluray", "x264", "none"},
			}),
			UpgradeAllowed: true,
		},
		{
			Name:       "HD-1080p",
			CutoffJSON: qualityJSON("1080p Bluray", "1080p", "bluray", "x265", "none"),
			QualitiesJSON: qualitiesJSON([]qualityEntry{
				{"1080p HDTV", "1080p", "hdtv", "x264", "none"},
				{"1080p WEBDL", "1080p", "webdl", "x264", "none"},
				{"1080p WEBRip", "1080p", "webrip", "x265", "none"},
				{"1080p Bluray", "1080p", "bluray", "x265", "none"},
				{"1080p Remux", "1080p", "remux", "x265", "none"},
			}),
			UpgradeAllowed: true,
		},
		{
			Name:       "Ultra-HD",
			CutoffJSON: qualityJSON("2160p Bluray HDR", "2160p", "bluray", "x265", "hdr10"),
			QualitiesJSON: qualitiesJSON([]qualityEntry{
				{"2160p WEBDL HDR", "2160p", "webdl", "x265", "hdr10"},
				{"2160p Bluray HDR", "2160p", "bluray", "x265", "hdr10"},
				{"2160p Remux HDR", "2160p", "remux", "x265", "hdr10"},
			}),
			UpgradeAllowed: true,
		},
		{
			Name:       "Any",
			CutoffJSON: qualityJSON("1080p Bluray", "1080p", "bluray", "x265", "none"),
			QualitiesJSON: qualitiesJSON([]qualityEntry{
				{"SD HDTV", "sd", "hdtv", "x264", "none"},
				{"SD DVD", "sd", "dvd", "xvid", "none"},
				{"720p HDTV", "720p", "hdtv", "x264", "none"},
				{"720p WEBDL", "720p", "webdl", "x264", "none"},
				{"720p WEBRip", "720p", "webrip", "x264", "none"},
				{"720p Bluray", "720p", "bluray", "x264", "none"},
				{"1080p HDTV", "1080p", "hdtv", "x264", "none"},
				{"1080p WEBDL", "1080p", "webdl", "x264", "none"},
				{"1080p WEBRip", "1080p", "webrip", "x265", "none"},
				{"1080p Bluray", "1080p", "bluray", "x265", "none"},
				{"1080p Remux", "1080p", "remux", "x265", "none"},
				{"2160p WEBDL HDR", "2160p", "webdl", "x265", "hdr10"},
				{"2160p Bluray HDR", "2160p", "bluray", "x265", "hdr10"},
				{"2160p Remux HDR", "2160p", "remux", "x265", "hdr10"},
			}),
			UpgradeAllowed: true,
		},
	}
}

// qualityEntry is a compact in-file tuple for readability in the default list.
// Fields: Name, Resolution, Source, Codec, HDR.
type qualityEntry struct {
	Name, Resolution, Source, Codec, HDR string
}

// qualityJSON produces the single-quality JSON blob used for `cutoff_json`,
// matching the shape Prism/Pilot's plugin.Quality unmarshals.
func qualityJSON(name, resolution, source, codec, hdr string) string {
	return fmt.Sprintf(
		`{"name":%q,"resolution":%q,"source":%q,"codec":%q,"hdr":%q}`,
		name, resolution, source, codec, hdr,
	)
}

// qualitiesJSON produces the JSON array blob used for `qualities_json`.
func qualitiesJSON(qs []qualityEntry) string {
	out := "["
	for i, q := range qs {
		if i > 0 {
			out += ","
		}
		out += qualityJSON(q.Name, q.Resolution, q.Source, q.Codec, q.HDR)
	}
	out += "]"
	return out
}
