package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
	"tessera/middleware"
)

// CreateWorkspace makes a new (shared) workspace owned by the caller.
func (h *API) CreateWorkspace(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	ws, err := h.q.CreateWorkspace(c, db.CreateWorkspaceParams{Name: req.Name, OwnerID: uid})
	if err != nil {
		fail(c)
		return
	}
	if _, err := h.q.CreateMembership(c, db.CreateMembershipParams{
		WorkspaceID: ws.ID, UserID: uid, Role: "owner",
	}); err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusCreated, ws)
}

// ListWorkspaces returns workspaces the caller belongs to.
func (h *API) ListWorkspaces(c *gin.Context) {
	ws, err := h.q.ListWorkspacesForUser(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, ws)
}

// GetWorkspace returns a single workspace the caller belongs to.
func (h *API) GetWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, ws)
}

// UpdateWorkspace renames a workspace (any member).
func (h *API) UpdateWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ws, err := h.q.UpdateWorkspace(c, db.UpdateWorkspaceParams{ID: id, Name: req.Name})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(id, "workspace.updated", ws)
	c.JSON(http.StatusOK, ws)
}

// DeleteWorkspace removes a workspace (owner only).
func (h *API) DeleteWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if ws.OwnerID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can delete the workspace"})
		return
	}
	if err := h.q.DeleteWorkspace(c, id); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListMembers lists the members of a workspace.
func (h *API) ListMembers(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	members, err := h.q.ListMembers(c, id)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, members)
}

// AddMember invites an existing user (by email) to the workspace.
func (h *API) AddMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.q.GetUserByEmail(c, strings.ToLower(req.Email))
	if notFound(c, err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no user with that email"})
		return
	}
	if err != nil {
		fail(c)
		return
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	m, err := h.q.CreateMembership(c, db.CreateMembershipParams{WorkspaceID: id, UserID: user.ID, Role: role})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusCreated, m)
}

// RemoveMember removes a member from the workspace.
func (h *API) RemoveMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, id) {
		return
	}
	userID, ok := parseID(c, "userId")
	if !ok {
		return
	}
	if err := h.q.DeleteMembership(c, db.DeleteMembershipParams{WorkspaceID: id, UserID: userID}); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}
