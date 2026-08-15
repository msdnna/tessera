// Package converter talks to the LibreOffice sidecar (converter/server.py) that
// turns office documents into HTML and back.
//
// The service is optional by design (#2733). Documents are a first-class
// section of Tessera and must keep working on an install that did not deploy a
// gigabyte of LibreOffice: with no CONVERTER_URL the client reports itself
// disabled and every import/export route answers "conversion is unavailable"
// instead of failing in a way the user has to guess about.
package converter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrDisabled is returned by every call when no converter is configured.
var ErrDisabled = errors.New("converter is not configured")

// Error is a refusal the sidecar itself produced — a format it will not take, a
// document LibreOffice could not parse, a run that timed out. It is separated
// from transport failures because the two mean different things to a caller:
// this one is about the file, and repeating the request will not help.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("converter: %s (status %d)", e.Message, e.Status) }

// Info is the sidecar's self-description, used to tell the client which file
// extensions the picker may offer. Asking the service rather than hard-coding
// the list keeps the two from drifting when the sidecar is upgraded.
type Info struct {
	OK      bool     `json:"ok"`
	Sources []string `json:"sources"`
	Targets []string `json:"targets"`
}

// Client is safe for concurrent use.
type Client struct {
	baseURL string
	http    *http.Client
}

// DefaultTimeout bounds one conversion from the caller's side. LibreOffice is
// slow on a large document and the sidecar has its own (shorter) ceiling; this
// exists so a wedged sidecar cannot pin a request goroutine indefinitely.
const DefaultTimeout = 150 * time.Second

// New builds a client for baseURL. An empty baseURL yields a disabled client
// rather than nil, so callers never have to nil-check before asking Enabled.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: DefaultTimeout},
	}
}

// Enabled reports whether a converter was configured at all.
func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

// Health asks the sidecar what it can convert. A disabled client returns
// ErrDisabled; an unreachable one returns the transport error, and callers
// surface both as "unavailable" rather than as a server fault — a missing
// optional sidecar is a deployment choice, not a bug.
func (c *Client) Health(ctx context.Context) (Info, error) {
	if !c.Enabled() {
		return Info{}, ErrDisabled
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return Info{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return Info{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Info{}, &Error{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	var info Info
	if err := json.Unmarshal(body, &info); err != nil {
		return Info{}, err
	}
	return info, nil
}

// maxResponseBytes caps what a conversion may return. A PDF of a document is
// bounded by the document, but the ceiling keeps a misbehaving sidecar from
// turning one request into an unbounded allocation.
const maxResponseBytes = 64 << 20

// Convert sends data (a file whose extension is fromExt, without the dot) and
// returns it rendered as to ("html", "pdf", "docx", "odt").
func (c *Client) Convert(ctx context.Context, data []byte, fromExt, to string) ([]byte, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	if len(data) == 0 {
		return nil, &Error{Status: http.StatusBadRequest, Message: "empty document"}
	}
	endpoint := c.baseURL + "/convert?" + url.Values{
		"from": {strings.TrimPrefix(strings.ToLower(fromExt), ".")},
		"to":   {strings.ToLower(to)},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &Error{Status: resp.StatusCode, Message: sidecarMessage(body)}
	}
	if len(body) == 0 {
		// A 200 with nothing in it is the sidecar disagreeing with itself. Do not
		// hand an empty file to the user as if it were a successful export.
		return nil, &Error{Status: http.StatusBadGateway, Message: "converter returned an empty document"}
	}
	return body, nil
}

// sidecarMessage pulls the error text out of the sidecar's JSON body, falling
// back to the raw payload. The message is shown to the user, so it is trimmed:
// a LibreOffice stack trace in a toast helps nobody.
func sidecarMessage(body []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	msg := strings.TrimSpace(string(body))
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		msg = payload.Error
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	if msg == "" {
		msg = "conversion failed"
	}
	return msg
}
