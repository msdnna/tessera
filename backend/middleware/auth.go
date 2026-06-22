// Package middleware holds gin middleware (JWT auth, request context helpers).
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/auth"
)

// ContextUserID is the gin context key holding the authenticated user's uuid.
const ContextUserID = "user_id"

// Auth validates the Bearer access token and stores the user id in the context.
func Auth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		uid, err := auth.ParseAccessToken(secret, strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ContextUserID, uid)
		c.Next()
	}
}

// CurrentUser returns the authenticated user id set by Auth (uuid.Nil if absent).
func CurrentUser(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ContextUserID)
	id, _ := v.(uuid.UUID)
	return id
}
