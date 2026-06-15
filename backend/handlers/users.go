package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/auth"
	"tessera/internal/db"
	"tessera/middleware"
)

// maxAvatarBytes caps an uploaded avatar (stored as a DB blob).
const maxAvatarBytes = 2 << 20 // 2 MiB

// userDTO is the public shape of a user (identity + legal-name profile + avatar
// link). Email is the login and is not self-editable; password never leaves.
type userDTO struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	IsAdmin       bool      `json:"is_admin"`
	EmailVerified bool      `json:"email_verified"`
	Provider      string    `json:"provider"`
	LastName      string    `json:"last_name"`
	FirstName     string    `json:"first_name"`
	MiddleName    string    `json:"middle_name"`
	Bio           string    `json:"bio"`
	Company       string    `json:"company"`
	JobTitle      string    `json:"job_title"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
}

// prefsDTO mirrors user_preferences (localizing + personalizing settings).
type prefsDTO struct {
	Language        string `json:"language"`
	Timezone        string `json:"timezone"`
	Country         string `json:"country"`
	TimeFormat      string `json:"time_format"`
	DateFormat      string `json:"date_format"`
	WeekStart       int16  `json:"week_start"`
	Theme           string `json:"theme"`
	Accent          string `json:"accent"`
	BoardBackground string `json:"board_background"`
}

// defaultPrefsDTO matches the column defaults in migration 0014 — returned when
// a user has no preferences row yet.
func defaultPrefsDTO() prefsDTO {
	return prefsDTO{
		Language: "ru", TimeFormat: "24h", DateFormat: "dd.MM.yyyy",
		WeekStart: 1, Theme: "system", Accent: "purple",
	}
}

func toPrefsDTO(p db.UserPreference) prefsDTO {
	return prefsDTO{
		Language: p.Language, Timezone: p.Timezone, Country: p.Country,
		TimeFormat: p.TimeFormat, DateFormat: p.DateFormat, WeekStart: p.WeekStart,
		Theme: p.Theme, Accent: p.Accent, BoardBackground: p.BoardBackground,
	}
}

// buildUserDTO maps a db.User and attaches the avatar URL when one is stored.
func buildUserDTO(c *gin.Context, q *db.Queries, u db.User) userDTO {
	dto := userDTO{
		ID: u.ID, Email: u.Email, Name: u.Name, IsAdmin: u.IsAdmin, EmailVerified: u.EmailVerified,
		Provider: u.Provider, LastName: u.LastName, FirstName: u.FirstName, MiddleName: u.MiddleName,
		Bio: u.Bio, Company: u.Company, JobTitle: u.JobTitle,
	}
	if has, err := q.UserHasAvatar(c, u.ID); err == nil && has {
		dto.AvatarURL = "/api/users/" + u.ID.String() + "/avatar"
	}
	return dto
}

// loadPrefsDTO reads a user's preferences, falling back to defaults if unset.
func loadPrefsDTO(c *gin.Context, q *db.Queries, uid uuid.UUID) prefsDTO {
	p, err := q.GetUserPreferences(c, uid)
	if err != nil {
		return defaultPrefsDTO()
	}
	return toPrefsDTO(p)
}

// UpdateMyProfile updates the caller's own profile (display + legal name and the
// free-text business fields). Email/role are not editable here. Full-replace:
// the client sends the whole profile bundle.
func (h *API) UpdateMyProfile(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		Name       string `json:"name" binding:"required"`
		LastName   string `json:"last_name"`
		FirstName  string `json:"first_name"`
		MiddleName string `json:"middle_name"`
		Bio        string `json:"bio"`
		Company    string `json:"company"`
		JobTitle   string `json:"job_title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.q.UpdateUserProfile(c, db.UpdateUserProfileParams{
		ID: uid, Name: req.Name, LastName: req.LastName, FirstName: req.FirstName,
		MiddleName: req.MiddleName, Bio: req.Bio, Company: req.Company, JobTitle: req.JobTitle,
	})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, buildUserDTO(c, h.q, u))
}

// ChangeMyPassword changes the caller's password after verifying the current one.
func (h *API) ChangeMyPassword(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.q.GetUserByID(c, uid)
	if err != nil {
		fail(c)
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.CurrentPassword) {
		c.JSON(http.StatusForbidden, gin.H{"error": "current password is incorrect"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		fail(c)
		return
	}
	if err := h.q.UpdateUserPassword(c, db.UpdateUserPasswordParams{ID: uid, PasswordHash: hash}); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// UpdateMyPreferences upserts the caller's localizing + personalizing settings.
func (h *API) UpdateMyPreferences(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req prefsDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate the behaviour-driving fields; free-form ones (language/accent/
	// timezone/country/board) are stored as-is.
	if req.Theme != "system" && req.Theme != "light" && req.Theme != "dark" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "theme must be system, light or dark"})
		return
	}
	if req.TimeFormat != "24h" && req.TimeFormat != "12h" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "time_format must be 24h or 12h"})
		return
	}
	if req.WeekStart < 0 || req.WeekStart > 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "week_start must be 0-6"})
		return
	}
	p, err := h.q.UpsertUserPreferences(c, db.UpsertUserPreferencesParams{
		UserID: uid, Language: req.Language, Timezone: req.Timezone, Country: req.Country,
		TimeFormat: req.TimeFormat, DateFormat: req.DateFormat, WeekStart: req.WeekStart,
		Theme: req.Theme, Accent: req.Accent, BoardBackground: req.BoardBackground,
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, toPrefsDTO(p))
}

// UploadMyAvatar stores the caller's avatar image (multipart field "avatar") as
// a DB blob.
func (h *API) UploadMyAvatar(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	file, err := c.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing avatar file"})
		return
	}
	if file.Size > maxAvatarBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "avatar exceeds 2 MiB"})
		return
	}
	f, err := file.Open()
	if err != nil {
		fail(c)
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxAvatarBytes+1))
	if err != nil || len(data) > maxAvatarBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "avatar exceeds 2 MiB"})
		return
	}
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "avatar must be a PNG, JPEG, GIF or WebP image"})
		return
	}
	if err := h.q.UpsertUserAvatar(c, db.UpsertUserAvatarParams{UserID: uid, ContentType: contentType, Bytes: data}); err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar_url": "/api/users/" + uid.String() + "/avatar"})
}

// DeleteMyAvatar removes the caller's avatar.
func (h *API) DeleteMyAvatar(c *gin.Context) {
	if err := h.q.DeleteUserAvatar(c, middleware.CurrentUser(c)); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// SetUserActive activates/deactivates an account (global admin only). A
// deactivated user can't log in. You can't change your own active state.
func (h *API) SetUserActive(c *gin.Context) {
	caller, ok := h.requireGlobalAdmin(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if id == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot change your own active state"})
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.SetUserActive(c, db.SetUserActiveParams{ID: id, Active: req.Active}); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetUserAvatar serves a user's avatar blob. Unauthenticated (served straight
// into <img>, like /uploads) — avatars are low-sensitivity and keyed by UUID.
func (h *API) GetUserAvatar(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	av, err := h.q.GetUserAvatar(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, av.ContentType, av.Bytes)
}
