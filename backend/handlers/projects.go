package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// ── Project groups ─────────────────────────────────────────

// CreateProjectGroup adds a group to a workspace (appended to the end).
func (h *API) CreateProjectGroup(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	max, err := h.q.MaxProjectGroupPosition(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	g, err := h.q.CreateProjectGroup(c, db.CreateProjectGroupParams{
		WorkspaceID: wsID, Name: req.Name, Position: positionBetween(&max, nil),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "group.created", g)
	c.JSON(http.StatusCreated, g)
}

// ListProjectGroups lists a workspace's groups.
func (h *API) ListProjectGroups(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	groups, err := h.q.ListProjectGroups(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, groups)
}

// UpdateProjectGroup renames a group.
func (h *API) UpdateProjectGroup(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	g, err := h.q.GetProjectGroup(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, g.WorkspaceID) {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateProjectGroup(c, db.UpdateProjectGroupParams{
		ID: id, Name: req.Name, Position: g.Position,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(g.WorkspaceID, "group.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteProjectGroup removes a group (its projects become ungrouped).
func (h *API) DeleteProjectGroup(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	g, err := h.q.GetProjectGroup(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, g.WorkspaceID) {
		return
	}
	if err := h.q.DeleteProjectGroup(c, id); err != nil {
		fail(c)
		return
	}
	h.broadcast(g.WorkspaceID, "group.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// ── Projects ───────────────────────────────────────────────

// CreateProject adds a project to a workspace, optionally inside a group.
func (h *API) CreateProject(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Name    string     `json:"name" binding:"required"`
		Color   string     `json:"color"`
		GroupID *uuid.UUID `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	max, err := h.q.MaxProjectPosition(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	p, err := h.q.CreateProject(c, db.CreateProjectParams{
		WorkspaceID: wsID, GroupID: req.GroupID, Name: req.Name,
		Color: req.Color, Position: positionBetween(&max, nil),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "project.created", p)
	c.JSON(http.StatusCreated, p)
}

// ListProjects lists a workspace's projects.
func (h *API) ListProjects(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	projects, err := h.q.ListProjects(c, wsID)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, projects)
}

// GetProject returns a single project.
func (h *API) GetProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	p, err := h.q.GetProject(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return
	}
	c.JSON(http.StatusOK, p)
}

// UpdateProject edits name/color and optionally moves it between groups.
func (h *API) UpdateProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	p, err := h.q.GetProject(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return
	}
	var req struct {
		Name    string     `json:"name" binding:"required"`
		Color   string     `json:"color"`
		GroupID *uuid.UUID `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateProject(c, db.UpdateProjectParams{
		ID: id, Name: req.Name, Color: req.Color, GroupID: req.GroupID, Position: p.Position,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(p.WorkspaceID, "project.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteProject removes a project and everything under it.
func (h *API) DeleteProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	p, err := h.q.GetProject(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return
	}
	if err := h.q.DeleteProject(c, id); err != nil {
		fail(c)
		return
	}
	h.broadcast(p.WorkspaceID, "project.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}
