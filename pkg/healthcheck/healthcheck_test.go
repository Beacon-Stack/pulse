package healthcheck

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer bad.Close()

	cases := []struct {
		name    string
		args    []string
		code    int
		handled bool
	}{
		{"normal start", []string{"/pilot"}, 0, false},
		{"normal start with flags", []string{"/pilot", "-config", "/config/x.yaml"}, 0, false},
		{"healthcheck default", []string{"/pilot", "healthcheck"}, 0, true},
		{"healthcheck explicit bad", []string{"/pilot", "healthcheck", bad.URL}, 1, true},
		{"old compose wget", []string{"/usr/bin/wget", "--no-verbose", "--tries=1", "--spider", ok.URL + "/health"}, 0, true},
		{"old compose wget, unhealthy", []string{"wget", "--spider", bad.URL}, 1, true},
		{"wget without a URL", []string{"wget", "-q"}, 2, true},
		{"nothing listening", []string{"/pilot", "healthcheck", "http://127.0.0.1:1/health"}, 1, true},
	}
	for _, c := range cases {
		var stderr bytes.Buffer
		code, handled := run(c.args, ok.URL+"/health", &stderr)
		if code != c.code || handled != c.handled {
			t.Errorf("%s: code=%d handled=%v (%s), want %d %v", c.name, code, handled, strings.TrimSpace(stderr.String()), c.code, c.handled)
		}
	}
}
