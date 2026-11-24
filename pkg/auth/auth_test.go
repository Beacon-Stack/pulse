package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testKey = "0123456789abcdef0123456789abcdef"

const (
	lanClient  = "192.168.1.20:5000"
	wanClient  = "203.0.113.9:5000"
	serverHost = "192.168.1.10:8383"
)

func fastHash(t *testing.T) {
	t.Helper()
	old := PasswordIterations
	PasswordIterations = 1000
	t.Cleanup(func() { PasswordIterations = old })
}

func withLogin(t *testing.T, required string, nets ...string) Settings {
	t.Helper()
	fastHash(t)
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	return Settings{Required: required, Username: "admin", PasswordHash: hash, LocalNetworks: nets}
}

func newAuth(s Settings) *Auth {
	return New(Options{App: "Pilot", APIKey: testKey}, s)
}

func request(method, remote string) *http.Request {
	r := httptest.NewRequest(method, "/api/v1/x", nil)
	r.RemoteAddr = remote
	r.Host = serverHost
	return r
}

func method(a *Auth, r *http.Request) Method {
	m, _ := a.authenticate(r)
	return m
}

// The default: the local network needs no login, everyone else is refused,
// and there is no login to offer them.
func TestNone_LocalInOutsideRefused(t *testing.T) {
	a := newAuth(Settings{})
	if method(a, request("GET", lanClient)) != MethodLocal {
		t.Fatal("local network should get in without a login")
	}
	for _, remote := range []string{"127.0.0.1:1", "10.0.0.5:1", "172.18.0.3:1", "[fd00::5]:1", "169.254.3.4:1"} {
		if method(a, request("GET", remote)) != MethodLocal {
			t.Errorf("%s is a local address", remote)
		}
	}
	m, reason := a.authenticate(request("GET", wanClient))
	if m != MethodNone || reason != reasonNotLocal {
		t.Fatalf("outside connection: %v %q, want refused as not local", m, reason)
	}

	w := httptest.NewRecorder()
	a.LoginHandler()(w, func() *http.Request {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"x"}`))
		r.RemoteAddr = wanClient
		return r
	}())
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("login with none set up: %d", w.Code)
	}
}

// Credentials alone don't change the rule: with required=none the outside
// stays refused even though a login exists.
func TestNone_StaysNoneWithCredentials(t *testing.T) {
	a := newAuth(withLogin(t, RequiredNone))
	if m, reason := a.authenticate(request("GET", wanClient)); m != MethodNone || reason != reasonNotLocal {
		t.Fatalf("outside: %v %q", m, reason)
	}
}

func TestOutsideMustLogIn(t *testing.T) {
	a := newAuth(withLogin(t, RequiredOutside))
	if method(a, request("GET", lanClient)) != MethodLocal {
		t.Fatal("local network should still need no login")
	}
	m, reason := a.authenticate(request("GET", wanClient))
	if m != MethodNone || reason != reasonLogin {
		t.Fatalf("outside without a session: %v %q", m, reason)
	}
	token, _ := a.issueToken()
	r := request("GET", wanClient)
	r.AddCookie(&http.Cookie{Name: a.CookieName(), Value: token})
	if method(a, r) != MethodSession {
		t.Fatal("outside with a session should get in")
	}
}

func TestEveryoneMustLogIn(t *testing.T) {
	a := newAuth(withLogin(t, RequiredEverywhere))
	for _, remote := range []string{lanClient, "127.0.0.1:1", wanClient} {
		if m, reason := a.authenticate(request("GET", remote)); m != MethodNone || reason != reasonLogin {
			t.Errorf("%s: %v %q, want login required", remote, m, reason)
		}
	}
	// The API key still works, and a wrong one never falls back to anything.
	r := request("GET", wanClient)
	r.Header.Set("X-Api-Key", testKey)
	if method(a, r) != MethodAPIKey {
		t.Fatal("API key should work in every mode")
	}
	r = request("GET", lanClient)
	r.Header.Set("X-Api-Key", "wrong")
	if method(newAuth(Settings{}), r) != MethodNone {
		t.Fatal("a wrong API key must be refused even from the local network")
	}
}

// A mode that needs a login but has none falls back to none, so a
// half-finished change can't lock anyone out.
func TestModeFallsBackWithoutCredentials(t *testing.T) {
	a := newAuth(Settings{Required: RequiredEverywhere})
	if a.Settings().Mode() != RequiredNone {
		t.Fatalf("mode = %q", a.Settings().Mode())
	}
	if method(a, request("GET", lanClient)) != MethodLocal {
		t.Fatal("local network should get in")
	}
}

// The owner can list ranges that also count as local.
func TestExtraLocalNetworks(t *testing.T) {
	a := newAuth(Settings{LocalNetworks: []string{"100.64.0.0/10", "203.0.113.9"}})
	for _, remote := range []string{"100.100.5.5:1", wanClient} {
		if method(a, request("GET", remote)) != MethodLocal {
			t.Errorf("%s is in a listed range", remote)
		}
	}
	if method(a, request("GET", "203.0.113.10:1")) != MethodNone {
		t.Error("a neighbouring address is not listed")
	}

	if _, err := ParseNetworks([]string{"0.0.0.0/0"}); err == nil {
		t.Error("the whole internet must not be accepted as a local network")
	}
	if _, err := ParseNetworks([]string{"not-a-range"}); err == nil {
		t.Error("junk must be rejected")
	}
	nets, err := ParseNetworks([]string{" 192.168.50.7/24 ", "", "192.168.50.0/24"})
	if err != nil || len(nets) != 1 || nets[0].String() != "192.168.50.0/24" {
		t.Errorf("normalising: %v %v", nets, err)
	}
}

// A proxy on the local network relaying an outside visitor is outside.
// Forwarded headers can only make a client look remote, never local.
func TestForwardedAddresses(t *testing.T) {
	a := newAuth(Settings{})
	cases := []struct {
		name   string
		remote string
		hdr    map[string]string
		want   Method
	}{
		{"proxy relaying the internet", "172.18.0.2:1", map[string]string{"X-Forwarded-For": "203.0.113.9"}, MethodNone},
		{"proxy relaying the LAN", "172.18.0.2:1", map[string]string{"X-Forwarded-For": "192.168.1.5"}, MethodLocal},
		{"chain with one outside hop", "172.18.0.2:1", map[string]string{"X-Forwarded-For": "192.168.1.5, 203.0.113.9"}, MethodNone},
		{"cloudflare header", "127.0.0.1:1", map[string]string{"Cf-Connecting-Ip": "203.0.113.9"}, MethodNone},
		{"rfc7239", "172.18.0.2:1", map[string]string{"Forwarded": "for=203.0.113.9;proto=https"}, MethodNone},
		{"unreadable address", "127.0.0.1:1", map[string]string{"X-Forwarded-For": "unknown"}, MethodNone},
		{"outside peer claiming to be local", wanClient, map[string]string{"X-Forwarded-For": "192.168.1.5"}, MethodNone},
	}
	for _, c := range cases {
		r := request("GET", c.remote)
		for k, v := range c.hdr {
			r.Header.Set(k, v)
		}
		if got := method(a, r); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// Sec-Fetch-Site is never a credential: the header alone gets nobody in.
func TestSecFetchSiteIsNotACredential(t *testing.T) {
	a := newAuth(Settings{})
	r := request("GET", wanClient)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if method(a, r) != MethodNone {
		t.Fatal("an outside request saying same-origin must be refused")
	}
}

func TestMiddleware_CrossSiteAndRebinding(t *testing.T) {
	a := newAuth(Settings{})
	reached := 0
	h := a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached++ }))
	serve := func(r *http.Request) int {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	// Another site's page making the visitor's browser call the app.
	r := request("POST", lanClient)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if code := serve(r); code != http.StatusForbidden || reached != 0 {
		t.Fatalf("cross-site: %d", code)
	}

	// DNS rebinding: the browser says same-origin, but the host is the
	// attacker's domain.
	for _, m := range []string{"GET", "POST"} {
		r = request(m, lanClient)
		r.Host = "evil.example.com:8383"
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Origin", "http://evil.example.com:8383")
		if code := serve(r); code != http.StatusUnauthorized || reached != 0 {
			t.Fatalf("rebinding %s: %d", m, code)
		}
	}

	// The same browser on the server's own address gets in.
	r = request("POST", lanClient)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if code := serve(r); reached != 1 {
		t.Fatalf("same-origin on the local network: %d", code)
	}

	// Another app (no browser headers) on the container network gets in.
	r = request("POST", "172.18.0.4:1")
	r.Host = "pilot:8383"
	if serve(r); reached != 2 {
		t.Fatal("app-to-app call on the container network should get in")
	}

	// A listed host name is trusted.
	a.SetAllowedHosts([]string{"evil.example.com"})
	r = request("GET", lanClient)
	r.Host = "evil.example.com"
	if serve(r); reached != 3 {
		t.Fatal("a listed host should be trusted")
	}
}

func TestLocalHostName(t *testing.T) {
	allowed := []string{"pilot.example.com", "*.home.example.org", ".tail1234.ts.net"}
	cases := map[string]bool{
		"192.168.1.10:8383": true, "127.0.0.1": true, "[::1]:8383": true, "[fd00::5]": true,
		"localhost:8383": true, "pilot:8383": true, "NAS": true, "nas.local": true,
		"media.lan:8383": true, "pilot.home.arpa": true, "box.internal.": true,
		"pilot.default.svc.cluster.local": true,
		"pilot.example.com":               true, "Pilot.Example.Com:443": true,
		"tv.home.example.org": true, "box.tail1234.ts.net": true,
		"evil.example.com:8383": false, "a1b2.rebind.attacker.net": false,
		"192.168.1.10.attacker.net": false, "localhost.attacker.net": false,
		"notpilot.example.com": false, "home.example.org": false, "evil-lan.com": false, "": false,
	}
	for host, want := range cases {
		if got := localHostName(host, allowed); got != want {
			t.Errorf("localHostName(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestLoginAndSessionLifecycle(t *testing.T) {
	a := newAuth(withLogin(t, RequiredEverywhere))
	login := func(remote, password string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		a.LoginHandler()(w, r)
		return w
	}

	w := login(wanClient, "correct horse")
	if w.Code != http.StatusNoContent {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	cookie := w.Result().Cookies()[0]
	if cookie.Name != "pilot_session" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure {
		t.Fatalf("cookie %+v", cookie)
	}
	r := request("GET", wanClient)
	r.AddCookie(cookie)
	if method(a, r) != MethodSession {
		t.Fatal("the session should be accepted")
	}

	// Tampered, expired, and after a password change.
	r = request("GET", wanClient)
	r.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value + "x"})
	if method(a, r) != MethodNone {
		t.Error("a tampered token must be refused")
	}
	a.now = func() time.Time { return time.Now().Add(DefaultSessionTTL + time.Hour) }
	r = request("GET", wanClient)
	r.AddCookie(cookie)
	if method(a, r) != MethodNone {
		t.Error("an expired token must be refused")
	}
	a.now = time.Now
	s := a.Settings()
	s.PasswordHash, _ = HashPassword("a new password")
	if err := a.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	r = request("GET", wanClient)
	r.AddCookie(cookie)
	if method(a, r) != MethodNone {
		t.Error("changing the password must end existing sessions")
	}

	// A different app's cookie signed with the same key is not ours.
	other := New(Options{App: "Prism", APIKey: testKey}, a.Settings())
	token, _ := other.issueToken()
	r = request("GET", wanClient)
	r.AddCookie(&http.Cookie{Name: a.CookieName(), Value: token})
	if method(a, r) != MethodNone {
		t.Error("a token issued by another app must be refused")
	}

	// Five misses lock that address out, even with the right password.
	for i := 0; i < freeAttempts; i++ {
		login("198.51.100.7:1", "guess")
	}
	if w := login("198.51.100.7:2", "a new password"); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("after %d misses: %d", freeAttempts, w.Code)
	}
	if w := login("198.51.100.8:1", "a new password"); w.Code != http.StatusNoContent {
		t.Fatalf("another address should not be locked: %d", w.Code)
	}
}

func TestStatusExplainsRefusal(t *testing.T) {
	a := newAuth(Settings{})
	status := func(r *http.Request) map[string]any {
		w := httptest.NewRecorder()
		a.StatusHandler()(w, r)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return body
	}
	if b := status(request("GET", lanClient)); b["authenticated"] != true || b["method"] != "local" || b["required"] != "none" {
		t.Errorf("local: %v", b)
	}
	if b := status(request("GET", wanClient)); b["authenticated"] != false || b["reason"] != reasonNotLocal {
		t.Errorf("outside: %v", b)
	}
	r := request("GET", lanClient)
	r.Host = "evil.example.com"
	if b := status(r); b["reason"] != reasonUntrusted || b["untrusted_host"] != "evil.example.com" {
		t.Errorf("untrusted host: %v", b)
	}
}

func TestWouldLockOut(t *testing.T) {
	a := newAuth(Settings{LocalNetworks: []string{"203.0.113.0/24"}})
	var got *http.Request
	h := a.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r }))
	h.ServeHTTP(httptest.NewRecorder(), request("GET", wanClient))
	if got == nil {
		t.Fatal("the listed range should let the caller in")
	}
	ctx := got.Context()
	if addr, local := a.CallerAddress(ctx); addr != "203.0.113.9" || !local {
		t.Errorf("caller %s local=%v", addr, local)
	}
	if !a.WouldLockOut(ctx, Settings{}) {
		t.Error("removing the caller's range with no login would lock them out")
	}
	if a.WouldLockOut(ctx, Settings{LocalNetworks: []string{"203.0.113.9"}}) {
		t.Error("keeping the caller's address is fine")
	}
	if a.WouldLockOut(ctx, withLogin(t, RequiredOutside)) {
		t.Error("with a login they can still get in")
	}
}

func TestFileStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "auth.json")
	store := FileStore{Path: path}
	if _, found, err := store.Load(); found || err != nil {
		t.Fatalf("empty store: found=%v err=%v", found, err)
	}
	a := New(Options{App: "Haul", APIKey: testKey, Store: store}, Settings{})
	want := withLogin(t, RequiredOutside, "100.64.0.0/10")
	if err := a.SetSettings(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Load()
	if err != nil || !found || got.Required != RequiredOutside || got.Username != "admin" || got.PasswordHash != want.PasswordHash || len(got.LocalNetworks) != 1 {
		t.Fatalf("loaded %+v found=%v err=%v", got, found, err)
	}
	if err := a.SetSettings(Settings{LocalNetworks: []string{"nonsense"}}); err == nil {
		t.Error("bad networks must not be saved")
	}
}

// Ranges from the app's config always count, and Open reads what was
// saved, falling back to the seed the first time.
func TestOpenAndFixedNetworks(t *testing.T) {
	dir := t.TempDir()
	seed := withLogin(t, RequiredOutside)
	a, err := Open(Options{App: "Prism", APIKey: testKey, LocalNetworks: []string{"203.0.113.0/24"}}, dir, seed)
	if err != nil {
		t.Fatal(err)
	}
	if a.Settings().Username != "admin" {
		t.Fatal("first start should use the seed")
	}
	if method(a, request("GET", wanClient)) != MethodLocal {
		t.Fatal("a range from the config should count as local")
	}
	if err := a.SetSettings(Settings{}); err != nil {
		t.Fatal(err)
	}
	if method(a, request("GET", wanClient)) != MethodLocal {
		t.Fatal("config ranges stay after the settings change")
	}
	b, err := Open(Options{App: "Prism", APIKey: testKey}, dir, seed)
	if err != nil {
		t.Fatal(err)
	}
	if b.Settings().Username != "" || b.Settings().Mode() != RequiredNone {
		t.Fatalf("second start should use what was saved, not the seed: %+v", b.Settings())
	}
}
