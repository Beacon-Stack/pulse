package log

// api.go — registers the /api/v1/system/logs and
// /api/v1/system/log-level endpoints on a Huma API. Every Beacon
// service calls log.RegisterRoutes(api, system) once during HTTP
// setup; the routes look identical across services so the in-app
// viewer is the same component regardless of which service it's
// inspecting.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// logEntryBody is the wire shape for one log entry. Mirrors the
// internal Entry but with json-friendly types.
type logEntryBody struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`   // "DEBUG" | "INFO" | "WARN" | "ERROR"
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type logListInput struct {
	Level string `query:"level" doc:"Minimum level to include (debug | info | warn | error). Default: any."`
	Limit int    `query:"limit" doc:"Max entries (default 200, max 5000)"`
	// Search is a substring match across the message and any string
	// field values. Case-insensitive. Empty matches everything.
	Search string `query:"q" doc:"Substring search across message + string fields (case-insensitive)"`
}

type logListOutput struct {
	Body struct {
		Entries []*logEntryBody `json:"entries"`
		// Total is the total entries that match the filter, before
		// the limit cap. Lets the UI show "showing N of M".
		Total int `json:"total"`
	}
}

type logLevelOutput struct {
	Body struct {
		Level string `json:"level"`
	}
}

type logLevelInput struct {
	Body struct {
		Level string `json:"level" doc:"debug | info | warn | error"`
	}
}

type dockerLogsInput struct {
	Tail int    `query:"tail" doc:"Trailing line count. 0 = all available history. Default 500."`
	Q    string `query:"q"    doc:"Substring search across message + string fields"`
}

type dockerLogsOutput struct {
	Body struct {
		Available bool            `json:"available"`
		Entries   []*logEntryBody `json:"entries"`
		// Reason carries the "why not" string when Available=false:
		// "container not detected", "socket not mounted", etc. Lets
		// the UI render a helpful tip instead of a generic error.
		Reason string `json:"reason,omitempty"`
	}
}

// RegisterRoutes wires the system-log endpoints onto api. The System
// returned by log.New is the source of truth for the buffer + level.
// Idempotent: safe to call once per service from main.go.
//
// Pass docker=nil to omit the Docker-stdout endpoint (e.g. unit
// tests, services that intentionally don't read their own
// container).
func RegisterRoutes(api huma.API, sys *System) {
	registerRoutesWithDocker(api, sys, nil)
}

// RegisterRoutesWithDocker is RegisterRoutes plus the Docker
// stdout reader for full history. Pass NewDockerLogsReader() (or
// nil to skip).
func RegisterRoutesWithDocker(api huma.API, sys *System, docker *DockerLogsReader) {
	registerRoutesWithDocker(api, sys, docker)
}

func registerRoutesWithDocker(api huma.API, sys *System, docker *DockerLogsReader) {
	huma.Register(api, huma.Operation{
		OperationID: "list-system-logs",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/logs",
		Summary:     "Recent log entries from the in-memory ring buffer",
		Description: "Returns the most recent log entries captured in the service's ring buffer. For longer history, read the container's stdout via Docker. Filterable by minimum level and substring search.",
		Tags:        []string{"System"},
	}, func(_ context.Context, in *logListInput) (*logListOutput, error) {
		entries := sys.Buffer.Entries()

		minLevel := levelOrdinal(in.Level)
		needle := strings.ToLower(in.Search)
		limit := in.Limit
		if limit <= 0 {
			limit = 200
		}
		if limit > 5000 {
			limit = 5000
		}

		// Walk newest-first so the limit cap shows the most recent
		// matches first; reverse at the end so the response is
		// chronological (UI scrolls up to see older).
		matched := make([]*logEntryBody, 0, limit)
		total := 0
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if levelOrdinal(e.Level) < minLevel {
				continue
			}
			if needle != "" && !entryMatchesSearch(e, needle) {
				continue
			}
			total++
			if len(matched) < limit {
				matched = append(matched, &logEntryBody{
					Time:    e.Time,
					Level:   e.Level,
					Message: e.Message,
					Fields:  e.Fields,
				})
			}
		}
		// Reverse to chronological.
		for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
			matched[i], matched[j] = matched[j], matched[i]
		}

		out := &logListOutput{}
		out.Body.Entries = matched
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-log-level",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/log-level",
		Summary:     "Current minimum log level",
		Tags:        []string{"System"},
	}, func(_ context.Context, _ *struct{}) (*logLevelOutput, error) {
		out := &logLevelOutput{}
		out.Body.Level = sys.Level.String()
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-log-level",
		Method:      http.MethodPut,
		Path:        "/api/v1/system/log-level",
		Summary:     "Change minimum log level at runtime (no restart)",
		Description: "Bumping to debug is the standard troubleshooting move. Reverting to info afterwards keeps the steady-state log volume manageable.",
		Tags:        []string{"System"},
	}, func(_ context.Context, in *logLevelInput) (*logLevelOutput, error) {
		if err := sys.Level.Set(in.Body.Level); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		out := &logLevelOutput{}
		out.Body.Level = sys.Level.String()
		return out, nil
	})

	// Docker stdout: full history, much larger than the ring buffer.
	// Returns Available=false (with a Reason) instead of erroring
	// when the socket isn't reachable, so the UI can fall back to
	// the ring-buffer endpoint cleanly.
	if docker != nil {
		huma.Register(api, huma.Operation{
			OperationID: "list-docker-logs",
			Method:      http.MethodGet,
			Path:        "/api/v1/system/logs/docker",
			Summary:     "Container stdout via the Docker socket",
			Description: "Reads the service's own container logs through /var/run/docker.sock. Provides much more history than the in-memory ring buffer. Returns Available=false when the socket isn't mounted or the container can't be self-identified.",
			Tags:        []string{"System"},
		}, func(ctx context.Context, in *dockerLogsInput) (*dockerLogsOutput, error) {
			out := &dockerLogsOutput{}
			if !docker.Available() {
				out.Body.Available = false
				out.Body.Reason = "Docker socket not reachable. Mount /var/run/docker.sock read-only on this container to enable, or use the ring-buffer endpoint at /api/v1/system/logs."
				return out, nil
			}

			tail := in.Tail
			if tail <= 0 {
				tail = 500
			}
			entries, err := docker.FetchLogs(ctx, FetchOptions{Tail: tail})
			if err != nil {
				out.Body.Available = false
				out.Body.Reason = err.Error()
				return out, nil
			}

			needle := strings.ToLower(in.Q)
			matched := make([]*logEntryBody, 0, len(entries))
			for _, e := range entries {
				if needle != "" && !entryMatchesSearch(e, needle) {
					continue
				}
				matched = append(matched, &logEntryBody{
					Time:    e.Time,
					Level:   e.Level,
					Message: e.Message,
					Fields:  e.Fields,
				})
			}
			out.Body.Available = true
			out.Body.Entries = matched
			return out, nil
		})
	}
}

// entryMatchesSearch returns true when needle (already lowercased)
// appears in the message or any string-valued field. Searches are
// substring matches; we don't try regex because a typo in regex
// syntax shouldn't fail the request.
func entryMatchesSearch(e Entry, needle string) bool {
	if strings.Contains(strings.ToLower(e.Message), needle) {
		return true
	}
	for _, v := range e.Fields {
		if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), needle) {
			return true
		}
	}
	return false
}

// levelOrdinal returns slog's int level value for a level string.
// Returns -100 (well below DEBUG) for empty/unknown so a bare
// /api/v1/system/logs returns everything.
func levelOrdinal(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return -4
	case "INFO":
		return 0
	case "WARN", "WARNING":
		return 4
	case "ERROR", "ERR":
		return 8
	default:
		return -100
	}
}
