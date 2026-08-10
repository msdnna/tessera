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
	// Transactional: a workspace without its owner membership is invisible and
	// unreachable even to its creator — only removable by hand in the DB.
	var ws db.Workspace
	if err := h.inTx(c, func(q *db.Queries) error {
		var err error
		ws, err = q.CreateWorkspace(c, db.CreateWorkspaceParams{Name: req.Name, OwnerID: uid})
		if err != nil {
			return err
		}
		_, err = q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: ws.ID, UserID: uid, Role: "owner",
		})
		return err
	}); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, ws)
}

// ListWorkspaces returns workspaces the caller belongs to.
func (h *API) ListWorkspaces(c *gin.Context) {
	ws, err := h.q.ListWorkspacesForUser(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

// UpdateWorkspace renames a workspace (owner or admin).
func (h *API) UpdateWorkspace(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
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
		fail(c, err)
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
		fail(c, err)
		return
	}
	if ws.OwnerID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can delete the workspace"})
		return
	}
	if err := h.q.DeleteWorkspace(c, id); err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, members)
}

// manageableRole reports whether a role string is one a manager (owner/admin)
// may grant via add/update. Ownership (owner) is transferred separately, not
// assigned here.
func manageableRole(role string) bool {
	return role == "admin" || role == "member"
}

// AddMember invites an existing user (by email) to the workspace (owner/admin).
func (h *API) AddMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
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
	role := req.Role
	if role == "" {
		role = "member"
	}
	if !manageableRole(role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or member"})
		return
	}
	user, err := h.q.GetUserByEmail(c, strings.ToLower(req.Email))
	if notFound(c, err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no user with that email"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	m, err := h.q.CreateMembership(c, db.CreateMembershipParams{WorkspaceID: id, UserID: user.ID, Role: role})
	if err != nil {
		fail(c, err)
		return
	}
	// Live sockets snapshot their workspace set at connect; drop the new
	// member's so they reconnect and start receiving this workspace's events.
	h.hub.DropUser(user.ID)
	c.JSON(http.StatusCreated, m)
}

// UpdateMemberRole changes a member's role (owner/admin). The workspace owner's
// role is immutable here — ownership transfer is a separate, owner-only flow.
func (h *API) UpdateMemberRole(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	userID, ok := parseID(c, "userId")
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !manageableRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or member"})
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if userID == ws.OwnerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot change the workspace owner's role"})
		return
	}
	m, err := h.q.UpdateMembershipRole(c, db.UpdateMembershipRoleParams{WorkspaceID: id, UserID: userID, Role: req.Role})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// RemoveMember removes a member from the workspace (owner/admin). The workspace
// owner cannot be removed.
func (h *API) RemoveMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	userID, ok := parseID(c, "userId")
	if !ok {
		return
	}
	ws, err := h.q.GetWorkspace(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if userID == ws.OwnerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot remove the workspace owner"})
		return
	}
	if err := h.q.DeleteMembership(c, db.DeleteMembershipParams{WorkspaceID: id, UserID: userID}); err != nil {
		fail(c, err)
		return
	}
	// Cut the removed member's live sockets — otherwise they keep streaming
	// this workspace's events until they happen to reconnect.
	h.hub.DropUser(userID)
	c.Status(http.StatusNoContent)
}
