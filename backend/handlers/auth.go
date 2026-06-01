package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"tessera/internal/auth"
	"tessera/internal/db"
	"tessera/middleware"
)

type AuthHandler struct {
	q      *db.Queries
	secret string
}

func NewAuthHandler(q *db.Queries, secret string) *AuthHandler {
	return &AuthHandler{q: q, secret: secret}
}

type userDTO struct {
	ID      uuid.UUID `json:"id"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	IsAdmin bool      `json:"is_admin"`
}

func toUserDTO(u db.User) userDTO {
	return userDTO{ID: u.ID, Email: u.Email, Name: u.Name, IsAdmin: u.IsAdmin}
}

type authResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	User         userDTO `json:"user"`
}

// Register creates a user. The very first registered user becomes an admin.
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	count, err := h.q.CountUsers(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	user, err := h.q.CreateUser(c, db.CreateUserParams{
		Email:        strings.ToLower(req.Email),
		Name:         req.Name,
		PasswordHash: hash,
		IsAdmin:      count == 0,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Every new user gets a personal workspace they own. Shared workspaces are
	// created explicitly via POST /workspaces.
	if ws, err := h.q.CreateWorkspace(c, db.CreateWorkspaceParams{
		Name: "Личное пространство", OwnerID: user.ID,
	}); err == nil {
		_, _ = h.q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: ws.ID, UserID: user.ID, Role: "owner",
		})
	}

	h.issue(c, user)
}

// Login authenticates by email + password.
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.q.GetUserByEmail(c, strings.ToLower(req.Email))
	if err != nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	h.issue(c, user)
}

// Refresh rotates a refresh token: the presented one is revoked and a fresh
// access + refresh pair is issued.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash := auth.HashRefreshToken(req.RefreshToken)
	rt, err := h.q.GetRefreshToken(c, hash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	if rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token expired or revoked"})
		return
	}

	if err := h.q.RevokeRefreshToken(c, hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	user, err := h.q.GetUserByID(c, rt.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	h.issue(c, user)
}

// Me returns the authenticated user's profile.
func (h *AuthHandler) Me(c *gin.Context) {
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, toUserDTO(user))
}

// issue mints an access + refresh pair and stores the refresh hash.
func (h *AuthHandler) issue(c *gin.Context, user db.User) {
	access, err := auth.NewAccessToken(h.secret, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	refresh, hash, err := auth.NewRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if _, err := h.q.CreateRefreshToken(c, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(auth.RefreshTokenTTL),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, authResponse{AccessToken: access, RefreshToken: refresh, User: toUserDTO(user)})
}
