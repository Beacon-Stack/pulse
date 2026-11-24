package authapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/beacon-stack/pulse/pkg/auth"
	"github.com/beacon-stack/pulse/pkg/auth/authapi"
)

const key = "0123456789abcdef0123456789abcdef"

type env struct {
	t     *testing.T
	srv   *httptest.Server
	store auth.FileStore
}

// newEnv serves the settings API behind the real middleware, as an app
// does. The test client connects from 127.0.0.1, so it is local.
func newEnv(t *testing.T) *env {
	t.Helper()
	old := auth.PasswordIterations
	auth.PasswordIterations = 1000
	t.Cleanup(func() { auth.PasswordIterations = old })

	store := auth.FileStore{Path: filepath.Join(t.TempDir(), "auth.json")}
	a := auth.New(auth.Options{App: "Pilot", APIKey: key, Store: store}, auth.Settings{})
	r := chi.NewRouter()
	r.Get("/api/v1/auth/status", a.StatusHandler())
	r.Post("/api/v1/auth/login", a.LoginHandler())
	api := humachi.New(r.With(a.Middleware), huma.DefaultConfig("test", "1"))
	authapi.Register(api, a)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, store: store}
}

func (e *env) do(method, path, body string, hdr map[string]string) (int, map[string]any) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestSettingsFlow(t *testing.T) {
	e := newEnv(t)

	// Fresh app: none, no login, and it reports the address it sees.
	code, got := e.do("GET", "/api/v1/system/auth", "", nil)
	if code != 200 || got["required"] != "none" || got["password_set"] != false || got["client_address"] != "127.0.0.1" || got["client_is_local"] != true {
		t.Fatalf("initial: %d %v", code, got)
	}

	// A login can't be required before one exists; bad input is refused.
	for name, body := range map[string]string{
		"require without login": `{"required":"enabled"}`,
		"short password":        `{"required":"none","username":"me","password":"short"}`,
		"password, no username": `{"required":"none","password":"long enough pw"}`,
		"junk network":          `{"required":"none","local_networks":["nope"]}`,
		"whole internet":        `{"required":"none","local_networks":["0.0.0.0/0"]}`,
	} {
		if code, _ := e.do("PUT", "/api/v1/system/auth", body, nil); code != 400 && code != 422 {
			t.Errorf("%s: status %d, want 400", name, code)
		}
	}

	// Set a login and an extra range; it is saved, hashed.
	code, got = e.do("PUT", "/api/v1/system/auth", `{"required":"disabled_for_local","username":"me","password":"long enough pw","local_networks":["100.64.0.0/10"]}`, nil)
	if code != 200 || got["required"] != "disabled_for_local" || got["password_set"] != true {
		t.Fatalf("set login: %d %v", code, got)
	}
	saved, found, err := e.store.Load()
	if err != nil || !found || !strings.HasPrefix(saved.PasswordHash, "pbkdf2-sha256$") || len(saved.LocalNetworks) != 1 {
		t.Fatalf("saved %+v found=%v err=%v", saved, found, err)
	}

	// Once a login exists, changes need the current password...
	if code, _ := e.do("PUT", "/api/v1/system/auth", `{"required":"none"}`, nil); code != 403 {
		t.Fatalf("change without the current password: %d, want 403", code)
	}
	if code, _ := e.do("PUT", "/api/v1/system/auth", `{"required":"none","current_password":"a guess"}`, nil); code != 403 {
		t.Fatalf("change with a wrong password: %d, want 403", code)
	}
	// ...or the API key, which is how a forgotten password is reset.
	code, got = e.do("PUT", "/api/v1/system/auth", `{"required":"enabled","username":"me"}`, map[string]string{"X-Api-Key": key})
	if code != 200 || got["required"] != "enabled" {
		t.Fatalf("change with the API key: %d %v", code, got)
	}

	// Now everyone logs in, including this local client.
	if code, _ := e.do("GET", "/api/v1/system/auth", "", nil); code != 401 {
		t.Fatalf("local client with login required: %d, want 401", code)
	}
	if code, _ := e.do("POST", "/api/v1/auth/login", `{"username":"me","password":"long enough pw"}`, nil); code != 204 {
		t.Fatalf("login: %d", code)
	}

	// Back to none with the right password; the login is removed.
	code, got = e.do("PUT", "/api/v1/system/auth", `{"required":"none","username":"","current_password":"long enough pw"}`, map[string]string{"X-Api-Key": key})
	if code != 200 || got["password_set"] != false || got["required"] != "none" {
		t.Fatalf("remove login: %d %v", code, got)
	}
}
