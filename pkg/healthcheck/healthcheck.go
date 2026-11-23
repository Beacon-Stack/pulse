// Package healthcheck lets a Beacon app check its own health from inside
// its container, so images need no shell or wget.
//
//	pilot healthcheck            → GET http://127.0.0.1:8383/health
//	wget --spider http://…/health → the same, for compose files written
//	                                 when the images still had wget
//
// Call Run first thing in main.
package healthcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run handles the health-check invocations and exits; for anything else
// it returns false and main carries on. defaultURL is the app's own
// health URL, e.g. "http://127.0.0.1:8383/health".
func Run(args []string, defaultURL string) bool {
	code, handled := run(args, defaultURL, os.Stderr)
	if handled {
		os.Exit(code)
	}
	return false
}

func run(args []string, defaultURL string, stderr io.Writer) (code int, handled bool) {
	if len(args) == 0 {
		return 0, false
	}
	var url string
	switch {
	case filepath.Base(args[0]) == "wget":
		// Called through the wget link: the URL is the argument that looks
		// like one; flags (--spider, -q, --tries=1 …) are ignored.
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
				url = a
			}
		}
		if url == "" {
			_, _ = fmt.Fprintln(stderr, "wget: only health checks are supported in this image (no URL given)")
			return 2, true
		}
	case len(args) > 1 && args[1] == "healthcheck":
		url = defaultURL
		if len(args) > 2 {
			url = args[2]
		}
	default:
		return 0, false
	}
	if err := check(url); err != nil {
		_, _ = fmt.Fprintln(stderr, "unhealthy:", err)
		return 1, true
	}
	return 0, true
}

func check(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s answered %d", url, resp.StatusCode)
	}
	return nil
}
