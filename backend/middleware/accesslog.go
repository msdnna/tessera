package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"tessera/internal/logsafe"
	"tessera/internal/observability"
)

// redactedQuery blanks the value of every secret-bearing query parameter. The
// OAuth callback (?code=…&state=…) is the reason this file exists: the stock
// gin.Logger prints the full path + query to stdout, which lands in the
// container log stream verbatim.
//
// The list and the redaction itself live in internal/observability, which uses
// them for the same job on the Sentry side — the access log was only one of the
// two ways a query string leaves the process.
var redactedQuery = observability.RedactQuery

// AccessLog is a drop-in for gin.Logger that records the same one-line access
// trace (time, status, latency, client IP, method, path) but redacts secrets
// from the query string via redactedQuery. newRouter used to call gin.Default,
// which wired gin.Logger and printed ?code=…&state=… in the clear; it now uses
// gin.New with Recovery + this middleware.
//
// Silent under gin.TestMode to keep test output clean — the redaction itself is
// covered by redactedQuery's unit tests, and the timing/fields here are not
// security-relevant.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		test := gin.Mode() == gin.TestMode
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if test {
			return
		}
		full := path
		if q := redactedQuery(raw); q != "" {
			full = path + "?" + q
		}
		// logsafe: the path arrives percent-decoded, so %0A in a URL is a real
		// newline here — enough to forge a whole access-log line of its own.
		logsafe.Printf("[GIN] %s | %d | %v | %s | %s | %s %s",
			start.Format("2006/01/02 - 15:04:05"),
			c.Writer.Status(),
			time.Since(start),
			c.ClientIP(),
			GetRequestID(c),
			c.Request.Method,
			full,
		)
	}
}
