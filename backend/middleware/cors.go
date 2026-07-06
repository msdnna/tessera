package middleware

import "github.com/gin-gonic/gin"

// CORS sets CORS headers. The first argument is the primary allowed origin
// (pass "*" or "" to allow any origin, matching the legacy behaviour); `extra`
// lists additional allowed origins (e.g. the desktop app's Tauri origins). When
// the allowlist is not a wildcard, the request Origin is reflected only if it is
// in the list — so multiple distinct clients (web + desktop) are supported
// without opening the API to every origin.
func CORS(allowOrigin string, extra ...string) gin.HandlerFunc {
	wildcard := allowOrigin == "" || allowOrigin == "*"
	allowed := make(map[string]bool, len(extra)+1)
	if !wildcard {
		allowed[allowOrigin] = true
	}
	for _, o := range extra {
		if o == "" || o == "*" {
			wildcard = true
			continue
		}
		allowed[o] = true
	}

	return func(c *gin.Context) {
		switch {
		case wildcard:
			c.Header("Access-Control-Allow-Origin", "*")
		case allowed[c.GetHeader("Origin")]:
			// A reflected origin makes the response vary by request origin.
			c.Header("Access-Control-Allow-Origin", c.GetHeader("Origin"))
			c.Header("Vary", "Origin")
		default:
			// Not allowed: still set Vary so caches don't serve a cross-origin
			// response, but omit Allow-Origin (browser blocks the read).
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
