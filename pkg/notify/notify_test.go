package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// capture is a fake service that records the requests it gets.
type capture struct {
	mu   sync.Mutex
	reqs []captured
	code int
}

type captured struct {
	Path   string
	Header http.Header
	Body   string
}

func newCapture(t *testing.T) (*capture, *httptest.Server) {
	t.Helper()
	c := &capture{code: http.StatusOK}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.reqs = append(c.reqs, captured{Path: r.URL.RequestURI(), Header: r.Header.Clone(), Body: string(b)})
		code := c.code
		c.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return c, srv
}

func (c *capture) last(t *testing.T) captured {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reqs) == 0 {
		t.Fatal("no request received")
	}
	return c.reqs[len(c.reqs)-1]
}

// allowLocal lets providers reach the test server on 127.0.0.1.
func allowLocal(t *testing.T) {
	t.Helper()
	ic, hc := internetClient, homeClient
	internetClient, homeClient = http.DefaultClient, http.DefaultClient
	t.Cleanup(func() { internetClient, homeClient = ic, hc })
}

func settings(t *testing.T, m map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var sample = Event{
	App:      "Pilot",
	Type:     TypeImported,
	Title:    "Imported: Andor S02E03",
	Message:  "Added to TV <4K> & ready",
	Level:    LevelSuccess,
	Time:     time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	URL:      "https://www.themoviedb.org/tv/83867",
	ImageURL: "https://image.tmdb.org/t/p/w342/a.jpg",
	Fields:   map[string]string{"Quality": "WEB-DL 2160p", "Size": "4.1 GB"},
}

func TestRegistry_AllProvidersAndNoScripts(t *testing.T) {
	var ids []string
	for _, k := range Kinds() {
		ids = append(ids, k.ID)
	}
	got := strings.Join(ids, ",")
	for _, want := range []string{"discord", "slack", "telegram", "ntfy", "pushover", "gotify", "email", "webhook"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing provider %s in %s", want, got)
		}
	}
	if _, err := New("command", json.RawMessage(`{"path":"/x.sh"}`)); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("the retired script type must not be buildable: %v", err)
	}
}

func TestNew_RequiredSettings(t *testing.T) {
	_, err := New("telegram", settings(t, map[string]any{"bot_token": "x"}))
	if err == nil || !strings.Contains(err.Error(), "Chat ID is required") {
		t.Errorf("want a plain message about the missing chat ID, got %v", err)
	}
	if _, err := New("email", settings(t, map[string]any{"host": "h", "from": "a@b", "to": []string{}})); err == nil {
		t.Error("an empty To list should be rejected")
	}
	if _, err := New("discord", settings(t, map[string]any{"webhook_url": "https://discord.com/api/webhooks/1/x"})); err != nil {
		t.Error(err)
	}
}

func TestMaskAndMerge(t *testing.T) {
	stored := settings(t, map[string]any{"webhook_url": "https://discord.com/api/webhooks/1/secret", "username": "Beacon"})
	masked := string(Mask("discord", stored))
	if strings.Contains(masked, "secret") || !strings.Contains(masked, `"username":"Beacon"`) {
		t.Errorf("masked = %s", masked)
	}
	// Editing the name sends the masked URL back; the real one is kept.
	merged := Merge("discord", stored, settings(t, map[string]any{"webhook_url": Masked, "username": "Home"}))
	var m map[string]string
	_ = json.Unmarshal(merged, &m)
	if m["webhook_url"] != "https://discord.com/api/webhooks/1/secret" || m["username"] != "Home" {
		t.Errorf("merged = %s", merged)
	}
	// Leaving a secret out also keeps it; a new value replaces it.
	merged = Merge("pushover", settings(t, map[string]any{"user_key": "u", "api_token": "t"}), settings(t, map[string]any{"user_key": "u2"}))
	_ = json.Unmarshal(merged, &m)
	if m["api_token"] != "t" || m["user_key"] != "u2" {
		t.Errorf("merged = %s", merged)
	}
	// Unknown kinds mask everything.
	if s := string(Mask("command", settings(t, map[string]any{"path": "/x"}))); strings.Contains(s, "/x") {
		t.Errorf("unknown kind leaked %s", s)
	}
}

func TestChannelWants(t *testing.T) {
	ch := Channel{Enabled: true, Events: []string{TypeImported}, Apps: []string{"pilot"}}
	cases := []struct {
		e    Event
		want bool
	}{
		{Event{App: "Pilot", Type: TypeImported}, true},
		{Event{App: "Prism", Type: TypeImported}, false},
		{Event{App: "Pilot", Type: TypeGrabbed}, false},
		{Event{App: "Haul", Type: TypeTest}, true},
	}
	for _, c := range cases {
		if got := ch.Wants(c.e); got != c.want {
			t.Errorf("Wants(%s/%s) = %v", c.e.App, c.e.Type, got)
		}
	}
	all := Channel{Enabled: true}
	if !all.Wants(Event{App: "Haul", Type: TypeDownloadProblem}) {
		t.Error("no filters should mean every app and type")
	}
	if (Channel{}).Wants(Event{App: "Pilot", Type: TypeTest}) {
		t.Error("a disabled channel gets nothing")
	}
}

func TestSend_RoutesAndReports(t *testing.T) {
	allowLocal(t)
	ok, okSrv := newCapture(t)
	bad, badSrv := newCapture(t)
	bad.code = http.StatusInternalServerError
	chans := []Channel{
		{ID: "1", Name: "Kitchen", Kind: "webhook", Enabled: true, Settings: settings(t, map[string]any{"url": okSrv.URL})},
		{ID: "2", Name: "Broken", Kind: "webhook", Enabled: true, Settings: settings(t, map[string]any{"url": badSrv.URL})},
		{ID: "3", Name: "Prism only", Kind: "webhook", Enabled: true, Apps: []string{"prism"}, Settings: settings(t, map[string]any{"url": okSrv.URL})},
	}
	res := Send(context.Background(), chans, sample)
	if len(res) != 2 {
		t.Fatalf("results %+v", res)
	}
	if res[0].ChannelName != "Broken" || res[0].OK() || !strings.Contains(res[0].Error, "500") {
		t.Errorf("broken result %+v", res[0])
	}
	if res[1].ChannelName != "Kitchen" || !res[1].OK() {
		t.Errorf("kitchen result %+v", res[1])
	}
	if len(ok.reqs) != 1 {
		t.Errorf("the Prism-only channel should have been skipped: %d requests", len(ok.reqs))
	}
}

func TestSendOne_RejectsBadEvents(t *testing.T) {
	err := SendOne(context.Background(), Channel{Kind: "webhook", Settings: json.RawMessage(`{"url":"http://x"}`)}, Event{App: "Pilot", Type: "task_started", Title: "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown event type") {
		t.Errorf("internal events must not go out: %v", err)
	}
}

func TestDiscord(t *testing.T) {
	allowLocal(t)
	c, srv := newCapture(t)
	if err := SendOne(context.Background(), Channel{Kind: "discord", Settings: settings(t, map[string]any{"webhook_url": srv.URL})}, sample); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Username string `json:"username"`
		Embeds   []struct {
			Title     string                         `json:"title"`
			URL       string                         `json:"url"`
			Color     int                            `json:"color"`
			Thumbnail struct{ URL string }           `json:"thumbnail"`
			Fields    []struct{ Name, Value string } `json:"fields"`
		} `json:"embeds"`
	}
	if err := json.Unmarshal([]byte(c.last(t).Body), &body); err != nil {
		t.Fatal(err)
	}
	e := body.Embeds[0]
	if body.Username != "Beacon" || e.Title != "Pilot: Imported: Andor S02E03" || e.Color != 0x57F287 || e.Thumbnail.URL == "" || e.URL == "" {
		t.Errorf("discord body %+v", body)
	}
	if len(e.Fields) != 2 || e.Fields[0].Name != "Quality" {
		t.Errorf("fields %+v", e.Fields)
	}
}

func TestSlack(t *testing.T) {
	allowLocal(t)
	c, srv := newCapture(t)
	if err := SendOne(context.Background(), Channel{Kind: "slack", Settings: settings(t, map[string]any{"webhook_url": srv.URL})}, sample); err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	_ = json.Unmarshal([]byte(c.last(t).Body), &body)
	if !strings.Contains(body["text"], "&lt;4K&gt; &amp;") || !strings.Contains(body["text"], "<https://www.themoviedb.org/tv/83867|Pilot: Imported") {
		t.Errorf("slack text %q", body["text"])
	}
}

func TestTelegram_EscapesHTML(t *testing.T) {
	allowLocal(t)
	c, srv := newCapture(t)
	old := telegramAPI
	telegramAPI = srv.URL
	t.Cleanup(func() { telegramAPI = old })
	if err := SendOne(context.Background(), Channel{Kind: "telegram", Settings: settings(t, map[string]any{"bot_token": "123:abc", "chat_id": "42"})}, sample); err != nil {
		t.Fatal(err)
	}
	r := c.last(t)
	var body map[string]string
	_ = json.Unmarshal([]byte(r.Body), &body)
	if r.Path != "/bot123:abc/sendMessage" || body["chat_id"] != "42" || body["parse_mode"] != "HTML" {
		t.Errorf("telegram request %s %+v", r.Path, body)
	}
	if !strings.Contains(body["text"], "&lt;4K&gt; &amp; ready") || strings.Contains(body["text"], "<4K>") {
		t.Errorf("event text must be escaped: %q", body["text"])
	}
}

func TestNtfy(t *testing.T) {
	allowLocal(t)
	c, srv := newCapture(t)
	if err := SendOne(context.Background(), Channel{Kind: "ntfy", Settings: settings(t, map[string]any{"url": srv.URL + "/", "topic": "beacon home", "token": "tk"})}, sample); err != nil {
		t.Fatal(err)
	}
	r := c.last(t)
	if r.Path != "/beacon%20home" || r.Header.Get("Title") != "Pilot: Imported: Andor S02E03" || r.Header.Get("Click") != sample.URL || r.Header.Get("Authorization") != "Bearer tk" || r.Header.Get("Tags") != "white_check_mark" {
		t.Errorf("ntfy request %s %v", r.Path, r.Header)
	}
	if !strings.HasPrefix(r.Body, "Added to TV <4K> & ready") || !strings.Contains(r.Body, "Quality: WEB-DL 2160p") {
		t.Errorf("ntfy body %q", r.Body)
	}
}

func TestPushover(t *testing.T) {
	allowLocal(t)
	c, srv := newCapture(t)
	old := pushoverAPI
	pushoverAPI = srv.URL
	t.Cleanup(func() { pushoverAPI = old })
	e := sample
	e.Level = LevelError
	if err := SendOne(context.Background(), Channel{Kind: "pushover", Settings: settings(t, map[string]any{"user_key": "u", "api_token": "t"})}, e); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(c.last(t).Body), &body)
	if body["token"] != "t" || body["user"] != "u" || body["priority"] != float64(1) || body["url"] != sample.URL {
		t.Errorf("pushover body %+v", body)
	}
}

func TestGotify(t *testing.T) {
	allowLocal(t)
	c, srv := newCapture(t)
	if err := SendOne(context.Background(), Channel{Kind: "gotify", Settings: settings(t, map[string]any{"url": srv.URL, "token": "gt"})}, sample); err != nil {
		t.Fatal(err)
	}
	r := c.last(t)
	// The token goes in a header, not the URL, so it stays out of logs.
	if r.Path != "/message" || r.Header.Get("X-Gotify-Key") != "gt" {
		t.Errorf("gotify request %s %v", r.Path, r.Header)
	}
}

func TestWebhook_SignedJSON(t *testing.T) {
	allowLocal(t)
	c, srv := newCapture(t)
	if err := SendOne(context.Background(), Channel{Kind: "webhook", Settings: settings(t, map[string]any{"url": srv.URL, "secret": "s3"})}, sample); err != nil {
		t.Fatal(err)
	}
	r := c.last(t)
	var got Event
	if err := json.Unmarshal([]byte(r.Body), &got); err != nil || got.Title != sample.Title || got.App != "Pilot" || got.Fields["Size"] != "4.1 GB" {
		t.Errorf("webhook body %s (%v)", r.Body, err)
	}
	if r.Header.Get("X-Beacon-Signature") != "sha256="+Sign("s3", []byte(r.Body)) {
		t.Errorf("signature %q", r.Header.Get("X-Beacon-Signature"))
	}
}

func TestEmailMessage_NoHeaderInjection(t *testing.T) {
	e := sample
	e.Title = "Imported\r\nBcc: victim@example.com"
	msg := string(emailMessage(emailConfig{From: "beacon@example.com", To: []string{" you@example.com "}}, e))
	if strings.Contains(msg, "\r\nBcc:") {
		t.Errorf("a title must not add headers:\n%s", msg)
	}
	if !strings.Contains(msg, "Subject: Pilot: Imported  Bcc: victim@example.com\r\n") || !strings.Contains(msg, "To: you@example.com\r\n") {
		t.Errorf("message:\n%s", msg)
	}
}

func TestNetworkRules(t *testing.T) {
	_, srv := newCapture(t) // listens on 127.0.0.1
	ctx := context.Background()
	// Internet-only services can't be pointed at the local machine.
	err := SendOne(ctx, Channel{Kind: "discord", Settings: settings(t, map[string]any{"webhook_url": srv.URL})}, sample)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("discord to loopback should be refused, got %v", err)
	}
	// Self-hosted services can reach the home network…
	if err := SendOne(ctx, Channel{Kind: "ntfy", Settings: settings(t, map[string]any{"url": srv.URL, "topic": "t"})}, sample); err != nil {
		t.Errorf("ntfy on the local network should work: %v", err)
	}
	// …but never the cloud metadata address.
	err = SendOne(ctx, Channel{Kind: "webhook", Settings: settings(t, map[string]any{"url": "http://169.254.169.254/latest/meta-data"})}, sample)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("metadata address should be refused, got %v", err)
	}
}

func TestTypes(t *testing.T) {
	d := DefaultTypes()
	if strings.Join(d, ",") != "imported,import_failed,download_problem,health_issue" {
		t.Errorf("defaults %v", d)
	}
	if KnownType("task_started") || !KnownType(TypeTest) {
		t.Error("KnownType")
	}
}
