package log

// docker.go — reads logs from the Docker daemon socket. The in-app
// "Logs" panel uses this for "deep dive" history that exceeds the
// ring buffer's cap. Docker's per-container log retention (default:
// 10MB JSON file with rotation) gives us much more than the 1000-
// entry RAM cap.
//
// This is a self-introspection path: a service reads its OWN
// container's logs so the user sees a unified view that matches
// what `docker logs <service>` would show. Cross-container reading
// is intentionally out of scope here — that belongs in a central
// aggregator like Loki, not in each service's API surface.
//
// Security model: requires read-only access to /var/run/docker.sock
// (or wherever DOCKER_HOST points). The compose file mounts the
// socket; the endpoint's failure mode when the socket isn't
// reachable is a clean 503 with the reason, not a crash.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DockerLogsReader reads stdout from the running container via the
// Docker socket. Methods are safe to call concurrently from HTTP
// handlers — each call opens its own connection and drains it.
type DockerLogsReader struct {
	// SocketPath is the unix socket the daemon listens on. Default
	// "/var/run/docker.sock".
	SocketPath string

	// ContainerID is the container we're reading from. Determined
	// at startup by reading /etc/hostname (Docker stamps the short
	// container ID there) or via the env var BEACON_CONTAINER_ID.
	// Empty string disables the reader (returns "not in docker"
	// errors from every call).
	ContainerID string
}

// NewDockerLogsReader auto-discovers the socket + container ID.
// Returns a usable reader even when discovery fails — the failure
// surfaces only on the first call (so service startup doesn't
// hard-fail just because logs are unreadable).
func NewDockerLogsReader() *DockerLogsReader {
	r := &DockerLogsReader{
		SocketPath: socketPath(),
	}
	r.ContainerID = discoverContainerID()
	return r
}

// Available reports whether the reader is ready to serve. False
// means either the socket isn't mounted or we couldn't determine
// our container ID. The HTTP layer maps Available=false to 503 so
// the UI can fall back to the ring buffer cleanly.
func (r *DockerLogsReader) Available() bool {
	if r == nil || r.ContainerID == "" || r.SocketPath == "" {
		return false
	}
	if _, err := os.Stat(r.SocketPath); err != nil {
		return false
	}
	return true
}

// FetchOptions narrows what FetchLogs returns.
type FetchOptions struct {
	// Tail is the number of trailing lines to fetch. 0 = all.
	Tail int
	// Since is the lower bound on log timestamps. Zero = no bound.
	Since time.Time
	// Timestamps controls whether Docker prefixes each line with the
	// timestamp. Always true — we strip it on parse and use it for
	// the Entry.Time field.
}

// FetchLogs returns the container's stdout/stderr lines as Entries.
// Lines that parse as JSON (the slog handler emits JSON) populate
// the structured fields; everything else falls back to a plain
// "msg" with level=INFO so it still renders in the viewer.
func (r *DockerLogsReader) FetchLogs(ctx context.Context, opts FetchOptions) ([]Entry, error) {
	if !r.Available() {
		return nil, fmt.Errorf("docker socket reader unavailable (container_id=%q socket=%q)", r.ContainerID, r.SocketPath)
	}

	q := url.Values{}
	q.Set("stdout", "true")
	q.Set("stderr", "true")
	q.Set("timestamps", "true")
	if opts.Tail > 0 {
		q.Set("tail", fmt.Sprintf("%d", opts.Tail))
	} else {
		q.Set("tail", "all")
	}
	if !opts.Since.IsZero() {
		q.Set("since", fmt.Sprintf("%d", opts.Since.Unix()))
	}

	dialer := func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", r.SocketPath)
	}
	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: dialer},
		Timeout:   10 * time.Second,
	}

	uri := fmt.Sprintf("http://docker/containers/%s/logs?%s", r.ContainerID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker socket dial: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker socket returned %d: %s", resp.StatusCode, string(body))
	}

	return parseDockerLogStream(resp.Body)
}

// parseDockerLogStream decodes Docker's multiplexed log frame
// format. Each frame is an 8-byte header (1 byte stream type,
// 3 bytes padding, 4 bytes BE length) followed by the payload. The
// payload begins with an RFC3339Nano timestamp followed by a space.
//
// Spec: https://docs.docker.com/engine/api/v1.41/#tag/Container/operation/ContainerAttach
func parseDockerLogStream(rd io.Reader) ([]Entry, error) {
	var entries []Entry
	header := make([]byte, 8)

	for {
		if _, err := io.ReadFull(rd, header); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return entries, fmt.Errorf("read frame header: %w", err)
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(rd, payload); err != nil {
			return entries, fmt.Errorf("read frame payload: %w", err)
		}

		// Each payload may contain multiple newline-delimited lines
		// (when Docker batches). Split on \n and parse each.
		for _, line := range strings.Split(strings.TrimRight(string(payload), "\n"), "\n") {
			if line == "" {
				continue
			}
			entries = append(entries, parseDockerLine(line))
		}
	}
	return entries, nil
}

// parseDockerLine extracts the leading RFC3339Nano timestamp + the
// rest. The rest is JSON-decoded if possible (slog's output);
// otherwise it falls through as a plain INFO message.
func parseDockerLine(line string) Entry {
	// Format: "2026-05-04T12:34:56.123Z {payload}"
	idx := strings.IndexByte(line, ' ')
	if idx < 0 {
		return Entry{Time: time.Now(), Level: "INFO", Message: line}
	}
	tsStr, rest := line[:idx], line[idx+1:]
	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		ts = time.Now()
	}

	// slog's JSON format: {"time":"…","level":"INFO","msg":"…", …}
	if strings.HasPrefix(strings.TrimSpace(rest), "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(rest), &raw); err == nil {
			level, _ := raw["level"].(string)
			msg, _ := raw["msg"].(string)
			if level == "" {
				level = "INFO"
			}
			fields := make(map[string]any, len(raw))
			for k, v := range raw {
				if k == "time" || k == "level" || k == "msg" {
					continue
				}
				fields[k] = v
			}
			return Entry{Time: ts, Level: level, Message: msg, Fields: fields}
		}
	}

	return Entry{Time: ts, Level: "INFO", Message: rest}
}

// socketPath returns the docker socket path, honoring DOCKER_HOST if
// set (only the unix:// form). Falls back to the standard location.
func socketPath() string {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://")
	}
	return "/var/run/docker.sock"
}

// discoverContainerID reads the container's own ID from /etc/hostname.
// Docker stamps the 12-char short ID there by default. Honors the
// BEACON_CONTAINER_ID env var if set (handy for compose
// configurations where hostname is overridden).
func discoverContainerID() string {
	if id := strings.TrimSpace(os.Getenv("BEACON_CONTAINER_ID")); id != "" {
		return id
	}
	data, err := os.ReadFile("/etc/hostname")
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(data))
	// /etc/hostname is the short ID under Docker; but operators
	// sometimes use compose's `hostname:` directive to set
	// readable names ("pulse"). In that case we won't find the
	// container by that name on the docker API and FetchLogs will
	// 404. The 404 is surfaced cleanly to the UI, so the caller
	// just sees "container not found" and falls back to the ring
	// buffer.
	return id
}
