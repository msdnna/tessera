package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ContextRequestID is the gin/context key under which the per-request id lives.
// It is a plain string because gin.Context.Value only resolves string keys
// (c.Set stores them in the Keys map), which is what lets GetRequestID read it
// back through the context.Context interface from non-gin call sites.
const ContextRequestID = "request_id"

// HeaderRequestID is the inbound/outbound header carrying the id, so a caller
// (or a fronting proxy) can supply one and see it echoed for correlation.
const HeaderRequestID = "X-Request-Id"

// RequestID assigns every request a stable id — taken from the X-Request-Id
// header when present, otherwise freshly minted — stores it in the context and
// echoes it in the response. Without it, a server-side error log can't be tied
// back to the user report that triggered it.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(ContextRequestID, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// GetRequestID returns the request id from a context (gin.Context or the plain
// context.Context handed to workers), or "" when none was set.
func GetRequestID(ctx context.Context) string {
	s, _ := ctx.Value(ContextRequestID).(string)
	return s
}
