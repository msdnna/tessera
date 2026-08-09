package middleware

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// sensitiveQueryParams carry secrets in the URL and must never reach the access
// log. The OAuth callback (?code=…&state=…) is the reason this file exists: the
// stock gin.Logger prints the full path + query to stdout, which lands in the
// container log stream verbatim. Listed by the literal name OAuth/OIDC and our
// own token flows actually use.
var sensitiveQueryParams = map[string]struct{}{
	"code":          {},
	"state":         {},
	"token":         {},
	"secret":        {},
	"access_token":  {},
	"refresh_token": {},
}

// redactedQuery returns rawQuery with every sensitive parameter's value replaced
// by "***". Unrelated parameters are passed through untouched and in their
// original order so the log stays useful for debugging routing. Only the value
// is touched; a sensitive key with no '=' keeps its (value-less) form.
func redactedQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	var b strings.Builder
	for i, pair := range strings.Split(rawQuery, "&") {
		if i > 0 {
			b.WriteByte('&')
		}
		key, _, hasEq := strings.Cut(pair, "=")
		if _, secret := sensitiveQueryParams[key]; secret {
			b.WriteString(key)
			if hasEq {
				b.WriteString("=***")
			}
			continue
		}
		b.WriteString(pair)
	}
	return b.String()
}

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
		log.Printf("[GIN] %s | %d | %v | %s | %s %s",
			start.Format("2006/01/02 - 15:04:05"),
			c.Writer.Status(),
			time.Since(start),
			c.ClientIP(),
			c.Request.Method,
			full,
		)
	}
}
