package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ReadyHandler is the readiness probe (GET /health/ready): it pings the database
// so an orchestrator can tell a live-but-not-serving process from a healthy one.
// Liveness stays on the cheap /health — pinging the DB there would let a brief DB
// blip trigger a *restart* of a perfectly good process instead of just pulling it
// out of the load-balancer rotation. A DB failure here is a 503.
func (h *API) ReadyHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	start := time.Now()
	err := h.pool.Ping(ctx)
	latMS := float64(time.Since(start).Microseconds()) / 1000.0

	body := gin.H{
		"app":     "tessera",
		"version": h.version,
		"db":      gin.H{"ok": err == nil, "latency_ms": latMS},
	}
	if err != nil {
		body["ok"] = false
		c.JSON(http.StatusServiceUnavailable, body)
		return
	}
	body["ok"] = true
	c.JSON(http.StatusOK, body)
}

// MetricsHandler is an admin-only JSON snapshot of process internals (GET
// /admin/metrics): DB pool stats — the connection-pressure diagnostic behind the
// PAT-touch fix #2636 — plus live WebSocket clients, background jobs, and HTTP
// request/latency counters. In-process only; a second instance would report its
// own numbers.
func (h *API) MetricsHandler(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	s := h.pool.Stat()
	out := gin.H{
		"app":     "tessera",
		"version": h.version,
		"db_pool": gin.H{
			"acquired":            s.AcquiredConns(),
			"idle":                s.IdleConns(),
			"total":               s.TotalConns(),
			"max":                 s.MaxConns(),
			"empty_acquire_count": s.EmptyAcquireCount(),
		},
		"ws_clients": h.hub.ClientCount(),
		"jobs":       h.jobs.Snapshot(),
	}
	if h.metrics != nil {
		out["http"] = h.metrics.Snapshot()
	}
	c.JSON(http.StatusOK, out)
}
