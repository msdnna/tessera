package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NoBodyLimit marks a route as exempt from the body ceiling. Only /api/ws needs
// it: the upgrade hijacks the connection, and wrapping its body would cap the
// WebSocket stream itself.
const NoBodyLimit int64 = -1

// BodyLimit caps how much request body a route will read. defaultMax applies to
// everything not named in byRoute (keys are gin FullPaths); a NoBodyLimit entry
// exempts a route entirely.
//
// Two layers on purpose:
//
//   - a Content-Length check, which refuses an oversized request up front with a
//     truthful 413 and without reading a byte;
//   - http.MaxBytesReader, which is the actual enforcement — Content-Length is
//     client-supplied and absent on chunked bodies. A body that lies its way past
//     the first check dies mid-read, and the handler reports it as a 400 (the
//     read fails inside binding, where we no longer control the status code).
func BodyLimit(defaultMax int64, byRoute map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := defaultMax
		if v, ok := byRoute[c.FullPath()]; ok {
			limit = v
		}
		if limit <= 0 || c.Request == nil || c.Request.Body == nil {
			c.Next()
			return
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
