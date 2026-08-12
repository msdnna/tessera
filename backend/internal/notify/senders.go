package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/containrrr/shoutrrr"

	"tessera/internal/netguard"
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

// defaultClient is the shared HTTP client for outbound webhook deliveries: a
// tight timeout so a slow endpoint can't wedge the delivery worker, and a
// guarded dialer so a workspace member can't point a webhook at the internal
// network (SSRF). NOTIFY_ALLOW_PRIVATE_URLS=true opts back in to private
// targets (e.g. ntfy/gotify on the same host).
var defaultClient = newWebhookClient()

// newWebhookClient builds an HTTP client whose dial is SSRF-guarded per the
// notify private-URL policy. The policy is resolved per-dial (not captured when
// the client is built), so NOTIFY_ALLOW_PRIVATE_URLS takes effect on the next
// connection — matching the per-call ValidateURL below, which reads it live.
func newWebhookClient() *http.Client {
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DialContext: netguard.DialerFunc(notifyAllowPrivateURLs).DialContext},
	}
}

// notifyAllowPrivateURLs reports whether webhook deliveries may target
// loopback/private addresses. Defaults to false: a webhook URL is arbitrary
// workspace-member input, so the safe default forbids the internal network. A
// typo or unparseable value keeps the safe default rather than flipping it.
func notifyAllowPrivateURLs() bool {
	v := strings.TrimSpace(os.Getenv("NOTIFY_ALLOW_PRIVATE_URLS"))
	if v == "" {
		return false
	}
	b, _ := strconv.ParseBool(v)
	return b
}

// ── shoutrrr (telegram + the long tail) ────────────────────

// ShoutrrrSender delivers through the shoutrrr library, which speaks a single
// URL DSL for ~20 services (telegram, slack, discord, ntfy, gotify, matrix, …).
// Two channel types use it: a friendly "telegram" (we build the URL from a bot
// token + chat id), and a generic "shoutrrr" whose secret IS a full shoutrrr URL
// — the universal escape hatch so new providers need no code change.
type ShoutrrrSender struct{}

// Send renders the message text and hands it to shoutrrr for the channel's URL.
func (ShoutrrrSender) Send(_ context.Context, ch Channel, msg Message) error {
	target, err := shoutrrrURL(ch)
	if err != nil {
		return Permanent(err)
	}
	// msg.Body is already the fully rendered message (the link is part of the
	// template), so send it verbatim.
	if err := shoutrrr.Send(target, msg.Body); err != nil {
		// shoutrrr errors can embed the service URL (token and all) — scrub it.
		return errors.New(redact(err.Error(), target, ch.Secret["url"], ch.Secret["bot_token"]))
	}
	return nil
}

// shoutrrrURL derives the shoutrrr service URL for a channel: the raw secret URL
// for a generic "shoutrrr" channel, or a built telegram URL from token + chat.
func shoutrrrURL(ch Channel) (string, error) {
	switch ch.Type {
	case "shoutrrr":
		u := strings.TrimSpace(ch.Secret["url"])
		if u == "" {
			return "", errors.New("shoutrrr channel needs a service URL")
		}
		return u, nil
	case "telegram":
		token := strings.TrimSpace(ch.Secret["bot_token"])
		chat := ch.configString("chat_id")
		if token == "" || chat == "" {
			return "", errors.New("telegram channel needs a bot_token and chat_id")
		}
		return fmt.Sprintf("telegram://%s@telegram/?chats=%s", token, url.QueryEscape(chat)), nil
	default:
		return "", fmt.Errorf("unsupported shoutrrr channel type %q", ch.Type)
	}
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
	// Reject a non-http(s) / userinfo / (by default) private-target URL as a
	// permanent error, so a misconfigured channel fails fast instead of
	// retrying forever. The client's guarded dialer is the backstop for a
	// hostname that resolves to a private address (DNS-rebinding).
	if _, err := netguard.ValidateURL(target, notifyAllowPrivateURLs()); err != nil {
		return Permanent(fmt.Errorf("webhook url: %w", err))
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
