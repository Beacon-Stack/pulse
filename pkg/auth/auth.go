// Package auth decides who may use a Beacon app. Pilot, Prism, Haul and
// Pulse share it, so the rules and the settings page are the same in all
// four.
//
// There is one setting with three values:
//
//   - none (the default): anyone on the local network gets in without
//     logging in; connections from anywhere else are refused.
//   - disabled_for_local: the local network gets in without logging in;
//     connections from anywhere else log in with a username and password.
//   - enabled: everyone logs in.
//
// "Local" means loopback, the private address ranges, link-local, and
// any extra ranges the owner lists (Settings.LocalNetworks).
//
// The app's API key, in an X-Api-Key header or an apikey query parameter,
// is accepted in every mode: it is how scripts and the other apps get in.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultSessionTTL is how long a browser session stays valid after login.
const DefaultSessionTTL = 30 * 24 * time.Hour

// sessionVersion prefixes every token so the format can change later
// without old tokens being misparsed.
const sessionVersion = "v1"

// The values of Settings.Required.
const (
	// RequiredNone needs no login on the local network and refuses
	// everyone else. The default.
	RequiredNone = "none"
	// RequiredOutside needs no login on the local network; everyone
	// else logs in.
	RequiredOutside = "disabled_for_local"
	// RequiredEverywhere needs a login from every address.
	RequiredEverywhere = "enabled"
)

// Settings is the part of authentication the owner can change.
type Settings struct {
	// Required is RequiredNone, RequiredOutside or RequiredEverywhere.
	// Anything else is treated as RequiredNone.
	Required string `json:"required"`
	// Username and PasswordHash (from HashPassword) are the login. Both
	// empty means no login is set up.
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	// LocalNetworks are extra address ranges (CIDR) that count as the
	// local network, for homes that don't use a private range or setups
	// where the app sees an unusual address (a VPN, some container
	// networks).
	LocalNetworks []string `json:"local_networks"`
}

// CredentialsConfigured reports whether a username and password are set.
func (s Settings) CredentialsConfigured() bool {
	return s.Username != "" && s.PasswordHash != ""
}

// Mode is the rule actually applied. A mode that needs a login falls back
// to RequiredNone when no login is set up, so nobody is locked out by a
// half-finished change.
func (s Settings) Mode() string {
	switch s.Required {
	case RequiredOutside, RequiredEverywhere:
		if s.CredentialsConfigured() {
			return s.Required
		}
	}
	return RequiredNone
}

// Store saves settings changed from the settings page.
type Store interface {
	Save(Settings) error
}

// Options configures an Auth.
type Options struct {
	// App is the app's display name, used in messages ("Pilot").
	App string
	// CookieName names the session cookie. Defaults to "<app>_session".
	CookieName string
	// APIKey is the app's API key. Without one, API-key access and
	// logins are unavailable (sessions are signed with it); local
	// access still works.
	APIKey string
	// AllowedHosts are extra host names the app may be opened under
	// without a login from the local network (see host.go).
	AllowedHosts []string
	// LocalNetworks are address ranges (CIDR) from the app's config file
	// or environment that always count as local, on top of the ones in
	// Settings. They can't be removed from the settings page.
	LocalNetworks []string
	// Store, when set, is where SetSettings persists changes.
	Store Store
}

// Auth authenticates requests for one app.
//
// Sec-Fetch-Site is deliberately not a credential: any non-browser client
// can set it. It is only used as a cross-site check on requests let in
// by a cookie or by address, where the browser guarantees its value.
//
// Sessions are stateless HMAC-signed tokens. The signing key is derived
// from the API key and the password hash, so rotating the API key or
// changing the password ends every session.
type Auth struct {
	app        string
	cookieName string
	apiKey     []byte
	ttl        time.Duration
	now        func() time.Time
	limiter    *attemptLimiter
	store      Store

	fixedNets []netip.Prefix // from Options.LocalNetworks

	mu           sync.RWMutex
	settings     Settings
	localNets    []netip.Prefix // fixedNets plus the ones in settings
	signKey      []byte
	allowedHosts []string
}

// New builds an Auth. Invalid entries in settings.LocalNetworks are
// dropped.
func New(opts Options, settings Settings) *Auth {
	app := opts.App
	if app == "" {
		app = "This app"
	}
	cookie := opts.CookieName
	if cookie == "" {
		cookie = strings.ToLower(strings.ReplaceAll(app, " ", "_")) + "_session"
	}
	a := &Auth{
		app:          app,
		cookieName:   cookie,
		apiKey:       []byte(opts.APIKey),
		ttl:          DefaultSessionTTL,
		now:          time.Now,
		limiter:      newAttemptLimiter(),
		store:        opts.Store,
		allowedHosts: append([]string(nil), opts.AllowedHosts...),
	}
	for _, n := range opts.LocalNetworks {
		if p, err := parseNetwork(n); err == nil && p.Bits() >= 8 {
			a.fixedNets = append(a.fixedNets, p)
		}
	}
	a.apply(settings)
	return a
}

// Open builds an Auth whose settings live in dir/auth.json. When nothing
// has been saved there yet it starts from seed (for example values an
// older version kept in the app's config file).
func Open(opts Options, dir string, seed Settings) (*Auth, error) {
	store := FileStore{Path: filepath.Join(dir, "auth.json")}
	settings, found, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", store.Path, err)
	}
	if !found {
		settings = seed
	}
	opts.Store = store
	return New(opts, settings), nil
}

// CookieName is the name of the session cookie.
func (a *Auth) CookieName() string { return a.cookieName }

// Settings returns the current settings.
func (a *Auth) Settings() Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s := a.settings
	s.LocalNetworks = append([]string(nil), s.LocalNetworks...)
	return s
}

// SetSettings replaces the settings and saves them to the Store.
// Changing the password hash ends every session.
func (a *Auth) SetSettings(s Settings) error {
	nets, err := ParseNetworks(s.LocalNetworks)
	if err != nil {
		return err
	}
	s.LocalNetworks = networkStrings(nets)
	s = normalized(s)
	if a.store != nil {
		if err := a.store.Save(s); err != nil {
			return fmt.Errorf("saving login settings: %w", err)
		}
	}
	a.apply(s)
	return nil
}

func normalized(s Settings) Settings {
	switch s.Required {
	case RequiredOutside, RequiredEverywhere:
	default:
		s.Required = RequiredNone
	}
	return s
}

// apply installs settings without saving them; bad networks are dropped.
func (a *Auth) apply(s Settings) {
	s = normalized(s)
	var nets []netip.Prefix
	for _, n := range s.LocalNetworks {
		if p, err := parseNetwork(n); err == nil {
			nets = append(nets, p)
		}
	}
	s.LocalNetworks = networkStrings(nets)
	mac := hmac.New(sha256.New, a.apiKey)
	mac.Write([]byte(a.cookieName + "-" + sessionVersion + "\x00" + s.PasswordHash))
	a.mu.Lock()
	a.settings = s
	a.localNets = append(append([]netip.Prefix(nil), a.fixedNets...), nets...)
	a.signKey = mac.Sum(nil)
	a.mu.Unlock()
}

// SetAllowedHosts replaces the extra host names trusted for access
// without a login.
func (a *Auth) SetAllowedHosts(hosts []string) {
	a.mu.Lock()
	a.allowedHosts = append([]string(nil), hosts...)
	a.mu.Unlock()
}

func (a *Auth) hostTrusted(r *http.Request) bool {
	a.mu.RLock()
	allowed := a.allowedHosts
	a.mu.RUnlock()
	return localHostName(r.Host, allowed)
}

func (a *Auth) isLocal(r *http.Request) bool {
	a.mu.RLock()
	nets := a.localNets
	a.mu.RUnlock()
	return clientIsLocal(r, nets)
}

// Method is how a request was let in.
type Method int

// The ways a request can be let in.
const (
	MethodNone Method = iota
	MethodAPIKey
	MethodSession
	MethodLocal
)

// String is the name reported by the status endpoint.
func (m Method) String() string {
	switch m {
	case MethodAPIKey:
		return "api_key"
	case MethodSession:
		return "session"
	case MethodLocal:
		return "local"
	default:
		return "none"
	}
}

// Why a request with no valid credential was not let in.
const (
	reasonLogin     = "login_required"
	reasonNotLocal  = "not_local"
	reasonUntrusted = "untrusted_host"
	reasonBadKey    = "bad_api_key"
)

func (a *Auth) authenticate(r *http.Request) (Method, string) {
	if key := r.Header.Get("X-Api-Key"); key != "" {
		if a.keyMatches(key) {
			return MethodAPIKey, ""
		}
		return MethodNone, reasonBadKey
	}
	// The key may also come as ?apikey=, the Sonarr/Radarr convention that
	// dashboards built for those apps use. A value that isn't this app's
	// key is ignored, not refused: Torznab clients put an indexer's own
	// key in the same parameter.
	if key := r.URL.Query().Get("apikey"); key != "" && a.keyMatches(key) {
		return MethodAPIKey, ""
	}
	mode := a.Settings().Mode()
	if mode != RequiredNone {
		if c, err := r.Cookie(a.cookieName); err == nil && a.verifyToken(c.Value) {
			return MethodSession, ""
		}
		if mode == RequiredEverywhere {
			return MethodNone, reasonLogin
		}
	}
	if !a.isLocal(r) {
		if mode == RequiredNone {
			return MethodNone, reasonNotLocal
		}
		return MethodNone, reasonLogin
	}
	// A local address is only trusted under a host name a web page can't
	// own (see host.go): otherwise a DNS-rebinding page could use the
	// visitor's browser as its local address. Sessions and API keys don't
	// need this: a browser never sends the cookie to another domain.
	if !a.hostTrusted(r) {
		return MethodNone, reasonUntrusted
	}
	return MethodLocal, ""
}

func (a *Auth) keyMatches(key string) bool {
	return len(a.apiKey) > 0 && subtle.ConstantTimeCompare([]byte(key), a.apiKey) == 1
}

func (a *Auth) refusal(r *http.Request, reason string) string {
	switch reason {
	case reasonUntrusted:
		return a.app + " was opened as \"" + hostName(r.Host) + "\", a name it doesn't recognise as this server, so it won't skip the login for it. " +
			"Use the server's IP address or local name, log in, or add this name to the app's allowed hosts."
	case reasonNotLocal:
		return a.app + " only accepts connections from your local network. To use it from elsewhere, open it from your local network and set up a login in Settings, " +
			"or add your network there if this is your local network."
	case reasonBadKey:
		return "The X-Api-Key header doesn't match this app's API key."
	default:
		return "Authentication required: log in or provide a valid X-Api-Key header."
	}
}

// caller is who a request was let in as, for handlers that need to know.
type caller struct {
	method Method
	peer   string // the address that connected
	addrs  []netip.Addr
}

type callerKey struct{}

func withCaller(r *http.Request, m Method) *http.Request {
	c := caller{method: m, peer: peerAddr(r)}
	c.addrs, _ = clientAddrs(r)
	return r.WithContext(context.WithValue(r.Context(), callerKey{}, c))
}

// peerAddr is the address that connected, without the port.
func peerAddr(r *http.Request) string {
	if ip, ok := parseIP(r.RemoteAddr); ok {
		return ip.String()
	}
	return r.RemoteAddr
}

// ErrWrongPassword is returned by ConfirmPassword.
var ErrWrongPassword = errors.New("the current password is wrong")

// ErrTooManyAttempts is returned by ConfirmPassword and carries how long
// to wait.
type ErrTooManyAttempts struct{ Wait time.Duration }

func (e ErrTooManyAttempts) Error() string {
	return "Too many wrong passwords. Try again in " + waitText(e.Wait) + "."
}

func waitText(d time.Duration) string {
	if d <= time.Minute {
		return "a minute"
	}
	return fmt.Sprintf("%d minutes", int((d+time.Minute-1)/time.Minute))
}

// ConfirmPassword checks the current password before a change to the
// login settings, so a device that is merely on the network (let in
// without a login) can't replace the password and lock the owner out. A
// request made with the API key doesn't need it: the key is the stronger
// credential and is how a forgotten password gets reset. With no login
// set up there is nothing to confirm.
//
// ctx must come from a request that passed Middleware.
func (a *Auth) ConfirmPassword(ctx context.Context, password string) error {
	s := a.Settings()
	if !s.CredentialsConfigured() {
		return nil
	}
	c, _ := ctx.Value(callerKey{}).(caller)
	if c.method == MethodAPIKey {
		return nil
	}
	if d := a.limiter.wait(c.peer); d > 0 {
		return ErrTooManyAttempts{Wait: d}
	}
	if !CheckPassword(s.PasswordHash, password) {
		a.limiter.fail(c.peer)
		return ErrWrongPassword
	}
	a.limiter.ok(c.peer)
	return nil
}

// ViaAPIKey reports whether the request behind ctx was made with the
// app's API key, as scripts and the other apps do, rather than from a
// browser let in by address or by a login. Handlers use it to keep
// secrets out of answers to browsers. ctx must come from a request that
// passed Middleware.
func ViaAPIKey(ctx context.Context) bool {
	c, _ := ctx.Value(callerKey{}).(caller)
	return c.method == MethodAPIKey
}

// CallerAddress is the address the app saw the request come from, and
// whether it counts as local. The settings page shows it, because behind
// some container setups and proxies it is not the address the owner
// expects. ctx must come from a request that passed Middleware.
func (a *Auth) CallerAddress(ctx context.Context) (addr string, local bool) {
	c, _ := ctx.Value(callerKey{}).(caller)
	if len(c.addrs) == 0 {
		return c.peer, false
	}
	a.mu.RLock()
	nets := a.localNets
	a.mu.RUnlock()
	// The last address is the original client when a proxy reported one.
	return c.addrs[len(c.addrs)-1].String(), allLocal(c.addrs, nets)
}

// WouldLockOut reports whether the caller behind ctx could no longer
// reach the app under next: it got in by address, and next neither
// counts that address as local nor offers a login.
func (a *Auth) WouldLockOut(ctx context.Context, next Settings) bool {
	c, _ := ctx.Value(callerKey{}).(caller)
	if c.method != MethodLocal {
		return false
	}
	if next.Mode() != RequiredNone {
		return false // they can log in
	}
	nets, err := ParseNetworks(next.LocalNetworks)
	if err != nil {
		return false
	}
	nets = append(nets, a.fixedNets...)
	return len(c.addrs) > 0 && !allLocal(c.addrs, nets)
}

// Middleware rejects requests that are not authenticated. Mount it on the
// route group that holds every protected route, not as a global
// path-prefix filter, so a route can't slip past it through path tricks.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, reason := a.authenticate(r)
		r = withCaller(r, method)
		switch method {
		case MethodAPIKey:
			next.ServeHTTP(w, r)
		case MethodSession, MethodLocal:
			// Both are credentials a browser supplies on its own, so a
			// page on another site must not be able to ride them.
			if !crossSiteSafe(r) {
				writeError(w, http.StatusForbidden, "Cross-site request rejected.")
				return
			}
			next.ServeHTTP(w, r)
		default:
			writeError(w, http.StatusUnauthorized, a.refusal(r, reason))
		}
	})
}

// crossSiteSafe is the cross-site check for requests let in by a cookie
// or by address. SameSite=Strict already stops browsers attaching the
// cookie to cross-site requests; this also covers address-based access
// and cross-site WebSocket handshakes.
func crossSiteSafe(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
		// Not a browser, an older one without Fetch Metadata, or a
		// WebSocket handshake (browsers don't all send Fetch Metadata on
		// those). Fall back to Origin.
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			// Same machine, any port: a dev server proxying the socket
			// has its own port. Another site has another name.
			return strings.EqualFold(u.Hostname(), hostName(r.Host))
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return true
		}
		return u.Host == r.Host
	default: // "same-site", "cross-site", or anything unexpected
		return false
	}
}

// StatusHandler tells the UI whether it is signed in and whether a login
// form makes sense. Unauthenticated.
//
//	GET /api/v1/auth/status
func (a *Auth) StatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := a.Settings()
		method, reason := a.authenticate(r)
		if (method == MethodLocal || method == MethodSession) && !crossSiteSafe(r) {
			method, reason = MethodNone, reasonLogin
		}
		body := map[string]any{
			"app":                    a.app,
			"authenticated":          method != MethodNone,
			"method":                 method.String(),
			"required":               s.Mode(),
			"credentials_configured": s.CredentialsConfigured(),
		}
		if method == MethodNone {
			// Tell the login screen why the UI didn't just open.
			body["reason"] = reason
			body["detail"] = a.refusal(r, reason)
			if reason == reasonUntrusted {
				body["untrusted_host"] = hostName(r.Host)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(body)
	}
}

// LoginHandler exchanges a username and password for a session cookie.
//
//	POST /api/v1/auth/login  {"username": "...", "password": "..."}
func (a *Auth) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !crossSiteSafe(r) {
			writeError(w, http.StatusForbidden, "Cross-site request rejected.")
			return
		}
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "Request body must be JSON with username and password fields.")
			return
		}
		s := a.Settings()
		if len(a.apiKey) == 0 || s.Mode() == RequiredNone {
			writeError(w, http.StatusUnauthorized,
				"No login is set up. Open "+a.app+" from your local network and set one in Settings.")
			return
		}
		peer := peerAddr(r)
		if d := a.limiter.wait(peer); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, ErrTooManyAttempts{Wait: d}.Error())
			return
		}
		// Always run the password hash so a wrong username takes as
		// long as a wrong password.
		passOK := CheckPassword(s.PasswordHash, body.Password)
		userOK := subtle.ConstantTimeCompare([]byte(body.Username), []byte(s.Username)) == 1
		if !passOK || !userOK {
			a.limiter.fail(peer)
			writeError(w, http.StatusUnauthorized, "Incorrect username or password.")
			return
		}
		a.limiter.ok(peer)
		token, err := a.issueToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create session.")
			return
		}
		http.SetCookie(w, a.cookie(r, token, int(a.ttl.Seconds())))
		w.WriteHeader(http.StatusNoContent)
	}
}

// LogoutHandler clears the session cookie. Sessions are stateless, so a
// copied cookie stays valid until it expires, the API key is rotated or
// the password changes.
//
//	POST /api/v1/auth/logout
func (a *Auth) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, a.cookie(r, "", -1))
		w.WriteHeader(http.StatusNoContent)
	}
}

// cookie builds the session cookie. Secure is set only over HTTPS: these
// apps are usually reached over plain HTTP on a LAN, where a Secure cookie
// would never be sent back and login would silently fail. HttpOnly and
// SameSite=Strict are always set.
func (a *Auth) cookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // G124: Secure is conditional on purpose
		Name:     a.cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	}
}

// issueToken returns "v1.<expiry-unix>.<nonce>.<sig>".
func (a *Auth) issueToken() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := sessionVersion + "." +
		strconv.FormatInt(a.now().Add(a.ttl).Unix(), 10) + "." +
		base64.RawURLEncoding.EncodeToString(nonce)
	return payload + "." + a.sign(payload), nil
}

func (a *Auth) verifyToken(token string) bool {
	if len(a.apiKey) == 0 {
		return false
	}
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return false
	}
	payload, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(a.sign(payload))) {
		return false
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 3 || parts[0] != sessionVersion {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	return a.now().Unix() < exp
}

func (a *Auth) sign(payload string) string {
	a.mu.RLock()
	key := a.signKey
	a.mu.RUnlock()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"title":  http.StatusText(status),
		"detail": detail,
	})
}
