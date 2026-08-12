package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
)

// ── FCM (background push to Android devices) ───────────────
//
// A "device" channel is normally ephemeral: the router flags the live WS event
// and the open app raises the notification itself. FCM is the transport that
// makes the same channel work while the app is closed — the device's push token
// is just another field in the channel config, so routing rules, quiet hours and
// the per-device settings screen keep working unchanged.
//
// We speak FCM HTTP v1 directly (one authenticated POST) rather than pulling in
// the Firebase Admin SDK, and we mint the access token from the service-account
// key with x/oauth2/jwt — the google helper would add a cloud metadata
// dependency for a code path that never runs off Google infrastructure.

// ErrPushUnregistered marks a push token FCM no longer accepts (app uninstalled,
// data cleared, token rotated, or the token belongs to another sender). The
// delivery worker uses it to drop the stale token from the channel config so it
// stops producing failures.
var ErrPushUnregistered = errors.New("push token is no longer registered")

// fcmScope is the OAuth2 scope FCM HTTP v1 requires.
const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// fcmClient is the HTTP client for FCM. Unlike webhooks this target is a fixed,
// public Google endpoint, so it needs no SSRF guard — just a timeout so a slow
// response can't wedge the delivery worker.
var fcmClient = &http.Client{Timeout: 15 * time.Second}

// FCMSender delivers a notification as a data-only push message to one device.
//
// Data-only is deliberate: a payload carrying a "notification" block is rendered
// by the system while the app is closed, bypassing our own code — which would
// skip both the deep link into the task and the dedup against the WS-delivered
// copy. We send data and draw the notification in the app.
type FCMSender struct {
	ProjectID string
	Tokens    oauth2.TokenSource
	Client    *http.Client
	// Endpoint overrides the FCM send URL (tests). Empty = the real endpoint
	// derived from ProjectID.
	Endpoint string
}

// serviceAccountKey is the subset of a Firebase service-account JSON we need:
// the credentials to mint an access token, plus the project the key belongs to
// (which is also the project the send endpoint is scoped to — no separate env).
type serviceAccountKey struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"`
}

// NewFCMSender builds a sender from the raw bytes of a service-account key. The
// returned sender caches and refreshes its access token internally.
func NewFCMSender(keyJSON []byte) (*FCMSender, error) {
	var k serviceAccountKey
	if err := json.Unmarshal(keyJSON, &k); err != nil {
		return nil, fmt.Errorf("parse service account key: %w", err)
	}
	if k.ClientEmail == "" || k.PrivateKey == "" || k.ProjectID == "" {
		return nil, errors.New("service account key needs client_email, private_key and project_id")
	}
	tokenURI := k.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}
	cfg := &jwt.Config{
		Email:        k.ClientEmail,
		PrivateKey:   []byte(k.PrivateKey),
		PrivateKeyID: k.PrivateKeyID,
		Scopes:       []string{fcmScope},
		TokenURL:     tokenURI,
	}
	return &FCMSender{ProjectID: k.ProjectID, Tokens: cfg.TokenSource(context.Background())}, nil
}

// LoadFCMSender reads a service-account key from disk.
func LoadFCMSender(path string) (*FCMSender, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read FCM credentials: %w", err)
	}
	return NewFCMSender(raw)
}

// Send pushes one message to the channel's device token.
func (s *FCMSender) Send(ctx context.Context, ch Channel, msg Message) error {
	token := strings.TrimSpace(ch.configString("fcm_token"))
	if token == "" {
		// A device channel without a token is the normal state for a browser or
		// a phone without Play Services — it lives on the WS path only. Permanent
		// so it fails fast instead of retrying for an hour.
		return Permanent(errors.New("device channel has no push token"))
	}
	payload, err := json.Marshal(fcmRequest(token, msg))
	if err != nil {
		return Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Tokens != nil {
		tok, terr := s.Tokens.Token()
		if terr != nil {
			// Minting the access token failed. Could be a clock skew or a network
			// blip (transient) as easily as a revoked key, so let the worker retry;
			// it gives up after maxDeliveryAttempts either way.
			return fmt.Errorf("fcm auth: %w", terr)
		}
		tok.SetAuthHeader(req)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return err // network — transient
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode/100 == 2 {
		return nil
	}
	return fcmError(resp.StatusCode, body)
}

// fcmRequest builds the FCM HTTP v1 message body. Every value under "data" must
// be a string — FCM rejects the whole request with 400 otherwise — so ids are
// rendered rather than embedded as JSON types.
func fcmRequest(token string, msg Message) map[string]any {
	data := map[string]string{
		"notification_id": msg.ID,
		"kind":            msg.Kind,
		"title":           msg.Title,
		"body":            msg.Body,
		"link":            msg.Link,
	}
	if msg.TaskID != "" {
		data["task_id"] = msg.TaskID
	}
	return map[string]any{
		"message": map[string]any{
			"token": token,
			// HIGH priority so a data-only message wakes the app under Doze
			// instead of waiting for the next maintenance window.
			"android": map[string]any{"priority": "HIGH"},
			"data":    data,
		},
	}
}

// endpoint is the FCM send URL for this sender's project.
func (s *FCMSender) endpoint() string {
	if s.Endpoint != "" {
		return s.Endpoint
	}
	return "https://fcm.googleapis.com/v1/projects/" + s.ProjectID + "/messages:send"
}

func (s *FCMSender) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return fcmClient
}

// fcmError classifies a non-2xx FCM response. Retrying only helps for rate
// limits and server-side failures; everything else is permanent, and the subset
// that means "this token is dead" is additionally tagged with
// ErrPushUnregistered so the caller can drop the token.
func fcmError(status int, body []byte) error {
	code := fcmErrorCode(body)
	err := fmt.Errorf("fcm returned %d (%s)", status, code)
	switch code {
	case "UNREGISTERED", "SENDER_ID_MISMATCH":
		return Permanent(fmt.Errorf("%w: %v", ErrPushUnregistered, err))
	}
	switch {
	case status == http.StatusNotFound:
		// 404 without a parseable error code is still "no such token".
		return Permanent(fmt.Errorf("%w: %v", ErrPushUnregistered, err))
	case status == http.StatusTooManyRequests, status/100 == 5:
		return err // rate limited / FCM outage — retry
	case status/100 == 4:
		return Permanent(err) // bad payload or bad credentials — won't fix itself
	}
	return err
}

// fcmErrorCode pulls the FCM-specific error code out of an error response,
// preferring the FcmError detail (UNREGISTERED, …) over the generic gRPC status.
func fcmErrorCode(body []byte) string {
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return "unknown"
	}
	for _, d := range e.Error.Details {
		if d.ErrorCode != "" {
			return d.ErrorCode
		}
	}
	if e.Error.Status != "" {
		return e.Error.Status
	}
	return "unknown"
}
