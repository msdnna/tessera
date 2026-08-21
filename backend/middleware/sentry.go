package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

// maxCapturedBody caps how much of a 5xx response body we buffer for the Sentry
// event. Error payloads are tiny gin.H{"error": …} JSON, so a few KiB is plenty
// and bounds the per-request memory cost.
const maxCapturedBody = 4 << 10 // 4 KiB

// capturingWriter tees a bounded prefix of the response body so SentryReport can
// attach what the client actually saw to the event.
type capturingWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *capturingWriter) Write(b []byte) (int, error) {
	if room := maxCapturedBody - w.body.Len(); room > 0 {
		if room > len(b) {
			room = len(b)
		}
		w.body.Write(b[:room])
	}
	return w.ResponseWriter.Write(b)
}

// SentryReport captures every 5xx response — and any error attached via
// c.Error() — as a Sentry event. Handlers in this codebase signal server-side
// failures through handlers.fail(), which writes a generic 500 and attaches the
// real cause with c.Error(); capturing by status code therefore reports the
// whole failure surface without touching each handler. sentrygin already covers
// panics; this fills the gap for handled errors.
//
// Must be registered after sentrygin.New so the request-scoped hub exists. When
// Sentry is disabled (no DSN, hence no client) it short-circuits with zero
// overhead — no body buffering.
func SentryReport() gin.HandlerFunc {
	return func(c *gin.Context) {
		if sentry.CurrentHub().Client() == nil {
			c.Next() // Sentry disabled — skip buffering entirely.
			return
		}

		cw := &capturingWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = cw

		c.Next()

		status := c.Writer.Status()
		if status < http.StatusInternalServerError && len(c.Errors) == 0 {
			return
		}

		hub := sentrygin.GetHubFromContext(c)
		if hub == nil {
			hub = sentry.CurrentHub()
		}

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelError)
			scope.SetTag("http.status_code", strconv.Itoa(status))
			scope.SetTag("http.method", c.Request.Method)
			scope.SetTag("http.route", route)
			// The request id ties a Sentry issue back to the access log line and
			// to the slog record fail() wrote for the same request.
			if id := GetRequestID(c); id != "" {
				scope.SetTag("request_id", id)
			}
			// Attach the error body / gin errors as a context block (sentry-go
			// dropped SetExtra in favour of SetContext).
			details := sentry.Context{}
			if body := cw.body.String(); body != "" {
				details["response_body"] = body
			}
			if len(c.Errors) > 0 {
				details["gin_errors"] = c.Errors.String()
			}
			if len(details) > 0 {
				scope.SetContext("tessera", details)
			}
			// Group per route+status rather than per concrete URL (task and
			// workspace IDs vary per request), so all 500s on the same endpoint
			// collapse into one issue instead of thousands.
			scope.SetFingerprint([]string{"http", c.Request.Method, route, strconv.Itoa(status)})

			// Prefer the real error (carries a type) when a handler attached
			// one; otherwise describe the failed request.
			if len(c.Errors) > 0 {
				hub.CaptureException(c.Errors.Last().Err)
			} else {
				hub.CaptureMessage(fmt.Sprintf("HTTP %d %s %s", status, c.Request.Method, route))
			}
		})
	}
}
