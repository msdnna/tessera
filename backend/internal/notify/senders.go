package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sender delivers one rendered message to one channel of a given type. A send
// error is transient by default (the worker retries); to fail fast and stop
// retrying, wrap the error with PermanentError.
type Sender interface {
	Send(ctx context.Context, ch Channel, msg Message) error
}

// PermanentError marks a delivery failure the worker should not retry (bad
// config, 4xx, …) versus a transient one (network, 5xx).
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent wraps err as non-retryable.
func Permanent(err error) error { return &PermanentError{Err: err} }

// IsPermanent reports whether err (or anything it wraps) is a PermanentError.
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// defaultClient is the shared HTTP client for outbound channel calls: a tight
// timeout so a slow endpoint can't wedge the delivery worker.
var defaultClient = &http.Client{Timeout: 15 * time.Second}

// ── Telegram ───────────────────────────────────────────────

// TelegramSender posts to the Bot API using a per-channel bot token + chat id
// (the Grafana/Alertmanager model: the user runs their own bot).
type TelegramSender struct{ Client *http.Client }

// Send posts the message to the channel's chat via the Bot API.
func (s TelegramSender) Send(ctx context.Context, ch Channel, msg Message) error {
	token := strings.TrimSpace(ch.Secret["bot_token"])
	chatID := ch.configString("chat_id")
	if token == "" || chatID == "" {
		return Permanent(errors.New("telegram channel needs a bot_token and chat_id"))
	}
	text := msg.Body
	if msg.Title != "" {
		text = msg.Title + "\n" + msg.Body
	}
	if msg.Link != "" {
		text += "\n" + msg.Link
	}
	payload, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return Permanent(errors.New(redact(err.Error(), token)))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		// The HTTP stack embeds the full request URL (which carries the bot token)
		// in its error string — redact it before the error reaches the UI / logs.
		return errors.New(redact(err.Error(), token)) // network — transient
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	err = fmt.Errorf("telegram api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
		return Permanent(err) // bad token / chat id — won't fix itself
	}
	return err
}

func (s TelegramSender) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return defaultClient
}

// ── Generic webhook ────────────────────────────────────────

// WebhookSender POSTs a small JSON envelope to a user-supplied URL — the
// universal escape hatch (Apprise / ntfy / n8n / a self-hosted relay).
type WebhookSender struct{ Client *http.Client }

// Send POSTs the message envelope as JSON to the channel's URL.
func (s WebhookSender) Send(ctx context.Context, ch Channel, msg Message) error {
	target := ch.configString("url")
	if target == "" {
		return Permanent(errors.New("webhook channel needs a url"))
	}
	method := strings.ToUpper(ch.configString("method"))
	if method == "" {
		method = http.MethodPost
	}
	payload, _ := json.Marshal(map[string]any{
		"kind":  msg.Kind,
		"title": msg.Title,
		"text":  msg.Body,
		"link":  msg.Link,
	})
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h := strings.TrimSpace(ch.Secret["auth_header"]); h != "" {
		req.Header.Set("Authorization", h)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return errors.New(redact(err.Error(), ch.Secret["auth_header"])) // network — transient
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode/100 == 2 {
		return nil
	}
	err = fmt.Errorf("webhook returned %d", resp.StatusCode)
	if resp.StatusCode/100 == 4 && resp.StatusCode != http.StatusTooManyRequests {
		return Permanent(err) // client error other than rate-limit — won't fix itself
	}
	return err
}

func (s WebhookSender) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return defaultClient
}

// trimFloat renders a whole float without a trailing ".0" (used to coerce a
// JSON-numeric chat_id back to a clean string).
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// redact replaces secret substrings in a message with "***". Channel secrets
// (a bot token, an auth header) can end up inside a transport's error string —
// e.g. the HTTP client embeds the request URL, token and all — and those errors
// surface in the UI and in last_error, so they must be scrubbed at the source.
func redact(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s = strings.TrimSpace(s); s != "" {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	return msg
}
