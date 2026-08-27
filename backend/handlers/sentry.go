package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// MaxEnvelopeBytes caps the size of a tunnelled Sentry envelope. Browser error
// envelopes are small; this bounds abuse of the public tunnel endpoint. Kept in
// sync with the BodyLimit entry for the tunnel route in router.go — if the
// transport limit were the lower of the two, the handler's own answer would
// never be reached.
const MaxEnvelopeBytes = 1 << 20 // 1 MiB

// SentryConfigHandler serves the browser's Sentry runtime config and proxies
// ("tunnels") browser events to the upstream Sentry server.
//
// Why a tunnel: the self-hosted Sentry lives on the LAN only. If the browser
// posted events straight to the DSN host, any client off that LAN (or with an
// ad-blocker, which blocks hosts with "sentry" in the name) would fail with
// console errors. With the tunnel the browser only ever talks to the API it is
// already talking to, and the backend — which can reach Sentry — forwards the
// envelope. The DSN host the browser sees is therefore irrelevant.
//
// Why runtime config rather than a build-time constant: the DSN only exists
// once the operator has created the project in the Sentry UI. Serving it from
// the API means the web bundle never has to be rebuilt to point at a new Sentry
// (or to turn telemetry off).
type SentryConfigHandler struct {
	dsn         string  // frontend DSN handed to the browser
	environment string  // environment tag
	tracesRate  float64 // frontend trace sample rate
	enabled     bool    // false when no frontend DSN configured

	upstreamEnvelope string // resolved <scheme>://<host>/api/<project>/envelope/
	client           *http.Client
}

// NewSentryConfigHandler builds the handler from the frontend DSN. A blank DSN
// (or one that can't be parsed) yields a disabled handler: /api/client-config
// reports Sentry off and /api/sentry-tunnel becomes a no-op 204.
func NewSentryConfigHandler(dsn, environment string, tracesRate float64) *SentryConfigHandler {
	h := &SentryConfigHandler{
		dsn:         dsn,
		environment: environment,
		tracesRate:  tracesRate,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
	if dsn == "" {
		return h
	}
	envelope, err := envelopeURLFromDSN(dsn)
	if err != nil {
		log.Printf("WARNING: SENTRY_FRONTEND_DSN is set but unusable (%v) — frontend Sentry disabled", err)
		return h
	}
	h.upstreamEnvelope = envelope
	h.enabled = true
	return h
}

// envelopeURLFromDSN turns a Sentry DSN (scheme://key@host/projectID) into the
// upstream envelope ingest URL (scheme://host/api/projectID/envelope/).
//
// The scheme is restricted to http/https: the value comes from the operator's
// environment rather than from a request, but this is the one place where a
// string turns into a URL the server will fetch, and the cost of being strict
// is a typo caught at boot instead of a confusing runtime failure.
func envelopeURLFromDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported DSN scheme %q", u.Scheme)
	}
	projectID := strings.Trim(u.Path, "/")
	if u.Host == "" || projectID == "" {
		return "", fmt.Errorf("DSN is missing a host or project id")
	}
	return u.Scheme + "://" + u.Host + "/api/" + projectID + "/envelope/", nil
}

// ClientConfig serves the browser's runtime configuration. Public: the frontend
// reads it before authenticating (errors on the login screen are exactly the
// ones worth reporting). "sentry": null means telemetry is off and the frontend
// must not initialise the SDK.
func (h *SentryConfigHandler) ClientConfig(c *gin.Context) {
	if !h.enabled {
		c.JSON(http.StatusOK, gin.H{"sentry": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sentry": gin.H{
			"dsn":              h.dsn,
			"environment":      h.environment,
			"tracesSampleRate": h.tracesRate,
			"tunnel":           "/api/sentry-tunnel",
		},
	})
}

// Tunnel accepts a Sentry envelope from the browser SDK and forwards it to the
// upstream Sentry. Public and unauthenticated by necessity — an error thrown
// before login still has to be reportable — so it is body-limited and rate
// limited in the router.
func (h *SentryConfigHandler) Tunnel(c *gin.Context) {
	if !h.enabled {
		c.Status(http.StatusNoContent)
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, MaxEnvelopeBytes))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.upstreamEnvelope, bytes.NewReader(body))
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "application/x-sentry-envelope")

	resp, err := h.client.Do(req)
	if err != nil {
		// Upstream Sentry unreachable (not deployed yet, host down). Swallow it —
		// telemetry must never surface as an error in the user's browser, and a
		// failing tunnel would itself be reported, looping.
		c.Status(http.StatusOK)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	c.Status(resp.StatusCode)
}
