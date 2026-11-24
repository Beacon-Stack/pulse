package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Setting names match what Pilot and Prism stored before this package
// existed, so their saved channels keep working.

func init() {
	register(Kind{
		ID: "discord", Name: "Discord",
		Help: "In Discord: channel settings → Integrations → Webhooks → New Webhook → Copy Webhook URL.",
		Fields: []Field{
			{Key: "webhook_url", Label: "Webhook URL", Type: FieldURL, Required: true, Placeholder: "https://discord.com/api/webhooks/…"},
			{Key: "username", Label: "Bot name", Type: FieldText, Placeholder: "Beacon"},
		},
		secretKeys: []string{"webhook_url"},
		build:      decode(func(c discordConfig) Sender { return c }),
	})
	register(Kind{
		ID: "slack", Name: "Slack",
		Help: "In Slack: create an app with Incoming Webhooks turned on, then copy the webhook URL.",
		Fields: []Field{
			{Key: "webhook_url", Label: "Webhook URL", Type: FieldURL, Required: true, Placeholder: "https://hooks.slack.com/services/…"},
			{Key: "username", Label: "Bot name", Type: FieldText, Placeholder: "Beacon"},
		},
		secretKeys: []string{"webhook_url"},
		build:      decode(func(c slackConfig) Sender { return c }),
	})
	register(Kind{
		ID: "telegram", Name: "Telegram",
		Help: "Create a bot with @BotFather for the token; message the bot, then get your chat ID from @userinfobot.",
		Fields: []Field{
			{Key: "bot_token", Label: "Bot token", Type: FieldPassword, Required: true},
			{Key: "chat_id", Label: "Chat ID", Type: FieldText, Required: true},
		},
		build: decode(func(c telegramConfig) Sender { return c }),
	})
	register(Kind{
		ID: "ntfy", Name: "ntfy",
		Help: "Use https://ntfy.sh or your own ntfy server, and subscribe to the same topic in the ntfy app.",
		Fields: []Field{
			{Key: "url", Label: "Server", Type: FieldURL, Required: true, Placeholder: "https://ntfy.sh"},
			{Key: "topic", Label: "Topic", Type: FieldText, Required: true, Help: "Pick something hard to guess on the public server."},
			{Key: "token", Label: "Access token", Type: FieldPassword, Help: "Only if your server requires one."},
		},
		build: decode(func(c ntfyConfig) Sender { return c }),
	})
	register(Kind{
		ID: "pushover", Name: "Pushover",
		Help: "Your user key is on pushover.net; create an application there for the API token.",
		Fields: []Field{
			{Key: "user_key", Label: "User key", Type: FieldPassword, Required: true},
			{Key: "api_token", Label: "API token", Type: FieldPassword, Required: true},
		},
		build: decode(func(c pushoverConfig) Sender { return c }),
	})
	register(Kind{
		ID: "gotify", Name: "Gotify",
		Help: "In Gotify: Apps → Create application, then copy its token.",
		Fields: []Field{
			{Key: "url", Label: "Server", Type: FieldURL, Required: true, Placeholder: "http://gotify.local"},
			{Key: "token", Label: "App token", Type: FieldPassword, Required: true},
		},
		build: decode(func(c gotifyConfig) Sender { return c }),
	})
	register(Kind{
		ID: "email", Name: "Email",
		Help: "Any mail server that accepts SMTP. For Gmail, use an app password.",
		Fields: []Field{
			{Key: "host", Label: "SMTP server", Type: FieldText, Required: true, Placeholder: "smtp.gmail.com"},
			{Key: "port", Label: "Port", Type: FieldNumber, Placeholder: "587"},
			{Key: "username", Label: "Username", Type: FieldText},
			{Key: "password", Label: "Password", Type: FieldPassword},
			{Key: "from", Label: "From", Type: FieldText, Required: true, Placeholder: "beacon@example.com"},
			{Key: "to", Label: "To", Type: FieldList, Required: true, Placeholder: "you@example.com", Help: "Separate several addresses with commas."},
			{Key: "tls", Label: "Use TLS from the start (port 465)", Type: FieldCheckbox},
		},
		build: decode(func(c emailConfig) Sender { return c }),
	})
	register(Kind{
		ID: "webhook", Name: "Other app (URL)",
		Help: "For Home Assistant, n8n and similar tools: paste the URL they give you. Each event is sent as JSON.",
		Fields: []Field{
			{Key: "url", Label: "URL", Type: FieldURL, Required: true},
			{Key: "secret", Label: "Signing secret", Type: FieldPassword, Help: "Optional. Adds an X-Beacon-Signature header (sha256 HMAC of the body)."},
		},
		secretKeys: []string{"url"},
		build:      decode(func(c webhookConfig) Sender { return c }),
	})
}

// decode turns a typed constructor into a Kind.build.
func decode[T any](mk func(T) Sender) func(json.RawMessage) (Sender, error) {
	return func(raw json.RawMessage) (Sender, error) {
		var c T
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("invalid settings: %w", err)
		}
		return mk(c), nil
	}
}

// Shared formatting

func heading(e Event) string { return e.App + ": " + e.Title }

func emoji(l Level) string {
	switch l {
	case LevelSuccess:
		return "✅"
	case LevelWarning:
		return "⚠️"
	case LevelError:
		return "❌"
	}
	return "ℹ️"
}

// sortedFields returns the event's fields in a stable order.
func sortedFields(e Event) [][2]string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, k := range keys {
		if v := e.Fields[k]; v != "" {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

// plainBody is the message plus fields, for text-only services.
func plainBody(e Event) string {
	var b strings.Builder
	b.WriteString(e.Message)
	for _, f := range sortedFields(e) {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(f[0] + ": " + f[1])
	}
	if e.URL != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(e.URL)
	}
	return b.String()
}

func postJSON(ctx context.Context, c *http.Client, url string, body any, hdr map[string]string) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return do(c, req)
}

func do(c *http.Client, req *http.Request) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s answered %d %s", req.URL.Host, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Discord

type discordConfig struct {
	WebhookURL string `json:"webhook_url"`
	Username   string `json:"username"`
}

func discordColor(l Level) int {
	switch l {
	case LevelSuccess:
		return 0x57F287
	case LevelWarning:
		return 0xFEE75C
	case LevelError:
		return 0xED4245
	}
	return 0x5865F2
}

func (c discordConfig) Send(ctx context.Context, e Event) error {
	type field struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Inline bool   `json:"inline"`
	}
	type image struct {
		URL string `json:"url"`
	}
	embed := map[string]any{
		"title":       truncate(heading(e), 256),
		"description": truncate(e.Message, 4000),
		"color":       discordColor(e.Level),
		"timestamp":   e.Time.UTC().Format(time.RFC3339),
	}
	if e.URL != "" {
		embed["url"] = e.URL
	}
	if e.ImageURL != "" {
		embed["thumbnail"] = image{URL: e.ImageURL}
	}
	var fs []field
	for _, f := range sortedFields(e) {
		fs = append(fs, field{Name: f[0], Value: truncate(f[1], 1000), Inline: true})
	}
	if len(fs) > 0 {
		embed["fields"] = fs
	}
	body := map[string]any{"embeds": []any{embed}}
	body["username"] = orDefault(c.Username, "Beacon")
	return postJSON(ctx, internetClient, c.WebhookURL, body, nil)
}

// Slack

type slackConfig struct {
	WebhookURL string `json:"webhook_url"`
	Username   string `json:"username"`
}

func (c slackConfig) Send(ctx context.Context, e Event) error {
	title := emoji(e.Level) + " *" + slackEscape(heading(e)) + "*"
	if e.URL != "" {
		title = emoji(e.Level) + " *<" + e.URL + "|" + slackEscape(heading(e)) + ">*"
	}
	text := title
	if body := plainBodyNoURL(e); body != "" {
		text += "\n" + slackEscape(body)
	}
	return postJSON(ctx, internetClient, c.WebhookURL, map[string]any{
		"username": orDefault(c.Username, "Beacon"),
		"text":     text,
	}, nil)
}

func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func plainBodyNoURL(e Event) string {
	e.URL = ""
	return plainBody(e)
}

// Telegram

type telegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// telegramAPI is a variable so tests can point it at a local server.
var telegramAPI = "https://api.telegram.org"

func (c telegramConfig) Send(ctx context.Context, e Event) error {
	// HTML mode: everything from the event is escaped so a title with
	// "<" or "&" can't break the message.
	text := emoji(e.Level) + " <b>" + html.EscapeString(heading(e)) + "</b>"
	if body := plainBodyNoURL(e); body != "" {
		text += "\n" + html.EscapeString(body)
	}
	if e.URL != "" {
		text += "\n<a href=\"" + html.EscapeString(e.URL) + "\">More</a>"
	}
	return postJSON(ctx, internetClient, telegramAPI+"/bot"+url.PathEscape(c.BotToken)+"/sendMessage", map[string]any{
		"chat_id":    c.ChatID,
		"text":       truncate(text, 4000),
		"parse_mode": "HTML",
	}, nil)
}

// ntfy

type ntfyConfig struct {
	URL   string `json:"url"`
	Topic string `json:"topic"`
	Token string `json:"token"`
}

func ntfyPriority(l Level) string {
	switch l {
	case LevelError:
		return "4"
	case LevelWarning:
		return "3"
	}
	return "3"
}

func ntfyTag(l Level) string {
	switch l {
	case LevelSuccess:
		return "white_check_mark"
	case LevelWarning:
		return "warning"
	case LevelError:
		return "x"
	}
	return "information_source"
}

func (c ntfyConfig) Send(ctx context.Context, e Event) error {
	body := plainBodyNoURL(e)
	if body == "" {
		body = e.Title
	}
	u := strings.TrimRight(c.URL, "/") + "/" + url.PathEscape(c.Topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		return err
	}
	// Header values must be single-line; ntfy reads UTF-8 titles fine.
	req.Header.Set("Title", oneLine(heading(e)))
	req.Header.Set("Priority", ntfyPriority(e.Level))
	req.Header.Set("Tags", ntfyTag(e.Level))
	if e.URL != "" {
		req.Header.Set("Click", e.URL)
	}
	if e.ImageURL != "" {
		req.Header.Set("Icon", e.ImageURL)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return do(homeClient, req)
}

// Pushover

type pushoverConfig struct {
	UserKey  string `json:"user_key"`
	APIToken string `json:"api_token"`
}

var pushoverAPI = "https://api.pushover.net/1/messages.json"

func (c pushoverConfig) Send(ctx context.Context, e Event) error {
	msg := plainBodyNoURL(e)
	if msg == "" {
		msg = e.Title
	}
	prio := 0
	if e.Level == LevelError {
		prio = 1
	}
	body := map[string]any{
		"token":    c.APIToken,
		"user":     c.UserKey,
		"title":    truncate(heading(e), 250),
		"message":  truncate(msg, 1024),
		"priority": prio,
	}
	if e.URL != "" {
		body["url"] = e.URL
	}
	return postJSON(ctx, internetClient, pushoverAPI, body, nil)
}

// Gotify

type gotifyConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func (c gotifyConfig) Send(ctx context.Context, e Event) error {
	prio := 5
	switch e.Level {
	case LevelError:
		prio = 8
	case LevelWarning:
		prio = 6
	}
	msg := plainBody(e)
	if msg == "" {
		msg = e.Title
	}
	u := strings.TrimRight(c.URL, "/") + "/message"
	return postJSON(ctx, homeClient, u, map[string]any{
		"title":    heading(e),
		"message":  msg,
		"priority": prio,
	}, map[string]string{"X-Gotify-Key": c.Token})
}

// Email

type emailConfig struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
	TLS      bool     `json:"tls"`
}

func (c emailConfig) Send(ctx context.Context, e Event) error {
	port := c.Port
	if port == 0 {
		port = 587
		if c.TLS {
			port = 465
		}
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(port))
	conn, err := homeDial(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("email: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	tlsCfg := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if c.TLS {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return fmt.Errorf("email: TLS: %w", err)
		}
		conn = tc
	}
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: %w", err)
	}
	defer func() { _ = client.Close() }()
	if !c.TLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("email: STARTTLS: %w", err)
			}
		} else if c.Username != "" {
			return fmt.Errorf("email: %s doesn't offer encryption; not sending the password in the clear", c.Host)
		}
	}
	if c.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return fmt.Errorf("email: sign-in failed: %w", err)
		}
	}
	if err := client.Mail(oneLine(c.From)); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	for _, to := range c.To {
		if err := client.Rcpt(oneLine(strings.TrimSpace(to))); err != nil {
			return fmt.Errorf("email: recipient %s: %w", to, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: %w", err)
	}
	if _, err := w.Write(emailMessage(c, e)); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	return client.Quit()
}

func emailMessage(c emailConfig, e Event) []byte {
	to := make([]string, len(c.To))
	for i, t := range c.To {
		to[i] = oneLine(strings.TrimSpace(t))
	}
	body := plainBody(e)
	if body == "" {
		body = e.Title
	}
	var b strings.Builder
	b.WriteString("From: " + oneLine(c.From) + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + oneLine(heading(e)) + "\r\n")
	b.WriteString("Date: " + e.Time.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	b.WriteString("\r\n")
	return []byte(b.String())
}

// Webhook ("Other app")

type webhookConfig struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

func (c webhookConfig) Send(ctx context.Context, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Beacon-Notify/1")
	if c.Secret != "" {
		req.Header.Set("X-Beacon-Signature", "sha256="+Sign(c.Secret, body))
	}
	return do(homeClient, req)
}

// Sign returns the hex sha256 HMAC of body, as sent in X-Beacon-Signature.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// Small helpers

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
