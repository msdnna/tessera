package middleware

import "github.com/gin-gonic/gin"

// CORS sets permissive CORS headers, restricting the allowed origin to
// allowOrigin (pass "*" or "" to allow any origin).
func CORS(allowOrigin string) gin.HandlerFunc {
	if allowOrigin == "" {
		allowOrigin = "*"
	}
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowOrigin)
		// A non-wildcard origin makes the response vary by request origin.
		if allowOrigin != "*" {
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
