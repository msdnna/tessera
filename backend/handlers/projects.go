package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// ── Project groups ─────────────────────────────────────────

// CreateProjectGroup adds a group to a workspace, optionally nested in a parent.
func (h *API) CreateProjectGroup(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Name     string     `json:"name" binding:"required"`
		ParentID *uuid.UUID `json:"parent_id"`
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
		WorkspaceID: wsID, ParentID: req.ParentID, Name: req.Name, Position: positionBetween(&max, nil),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "group.created", g)
	c.JSON(http.StatusCreated, g)
}

// ListProjectGroups lists a workspace's groups (flat; the client builds the tree).
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
	g, ok := h.loadGroup(c)
	if !ok {
		return
	}
	var req struct {
		Name  string `json:"name" binding:"required"`
		Icon  string `json:"icon"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateProjectGroup(c, db.UpdateProjectGroupParams{
		ID: g.ID, Name: req.Name, Icon: req.Icon, Color: req.Color,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(g.WorkspaceID, "group.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// MoveProjectGroup re-parents and/or repositions a group among its siblings.
func (h *API) MoveProjectGroup(c *gin.Context) {
	g, ok := h.loadGroup(c)
	if !ok {
		return
	}
	var req struct {
		ParentID *uuid.UUID `json:"parent_id"`
		BeforeID *uuid.UUID `json:"before_id"`
		AfterID  *uuid.UUID `json:"after_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ParentID != nil && *req.ParentID == g.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a group cannot be its own parent"})
		return
	}
	prev, next, ok := h.neighborGroupPositions(c, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	updated, err := h.q.MoveProjectGroup(c, db.MoveProjectGroupParams{
		ID: g.ID, ParentID: req.ParentID, Position: positionBetween(prev, next),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(g.WorkspaceID, "group.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteProjectGroup removes a group (subgroups cascade; projects become ungrouped).
func (h *API) DeleteProjectGroup(c *gin.Context) {
	g, ok := h.loadGroup(c)
	if !ok {
		return
	}
	if err := h.q.DeleteProjectGroup(c, g.ID); err != nil {
		fail(c)
		return
	}
	h.broadcast(g.WorkspaceID, "group.deleted", gin.H{"id": g.ID})
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
		Icon    string     `json:"icon"`
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
		Color: req.Color, Icon: req.Icon, Slug: h.uniqueProjectSlug(c, req.Name),
		Position: positionBetween(&max, nil),
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
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, p)
}

// UpdateProject edits name/color/icon and group membership.
func (h *API) UpdateProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	var req struct {
		Name    string     `json:"name" binding:"required"`
		Color   string     `json:"color"`
		Icon    string     `json:"icon"`
		GroupID *uuid.UUID `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateProject(c, db.UpdateProjectParams{
		ID: p.ID, Name: req.Name, Color: req.Color, Icon: req.Icon, GroupID: req.GroupID,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(p.WorkspaceID, "project.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// MoveProject re-groups and/or repositions a project among its siblings.
func (h *API) MoveProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	var req struct {
		GroupID  *uuid.UUID `json:"group_id"`
		BeforeID *uuid.UUID `json:"before_id"`
		AfterID  *uuid.UUID `json:"after_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prev, next, ok := h.neighborProjectPositions(c, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	updated, err := h.q.MoveProject(c, db.MoveProjectParams{
		ID: p.ID, GroupID: req.GroupID, Position: positionBetween(prev, next),
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(p.WorkspaceID, "project.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteProject removes a project and everything under it.
func (h *API) DeleteProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	if err := h.q.DeleteProject(c, p.ID); err != nil {
		fail(c)
		return
	}
	h.broadcast(p.WorkspaceID, "project.deleted", gin.H{"id": p.ID})
	c.Status(http.StatusNoContent)
}

// ── helpers ────────────────────────────────────────────────

func (h *API) loadGroup(c *gin.Context) (db.ProjectGroup, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.ProjectGroup{}, false
	}
	g, err := h.q.GetProjectGroup(c, id)
	if notFound(c, err) {
		return db.ProjectGroup{}, false
	}
	if err != nil {
		fail(c)
		return db.ProjectGroup{}, false
	}
	if !h.requireMember(c, g.WorkspaceID) {
		return db.ProjectGroup{}, false
	}
	return g, true
}

func (h *API) loadProject(c *gin.Context) (db.Project, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.Project{}, false
	}
	p, err := h.q.GetProject(c, id)
	if notFound(c, err) {
		return db.Project{}, false
	}
	if err != nil {
		fail(c)
		return db.Project{}, false
	}
	if !h.requireMember(c, p.WorkspaceID) {
		return db.Project{}, false
	}
	return p, true
}

func (h *API) neighborGroupPositions(c *gin.Context, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		g, err := h.q.GetProjectGroup(c, *beforeID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &g.Position
	}
	if afterID != nil {
		g, err := h.q.GetProjectGroup(c, *afterID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &g.Position
	}
	return prev, next, true
}

func (h *API) neighborProjectPositions(c *gin.Context, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		p, err := h.q.GetProject(c, *beforeID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &p.Position
	}
	if afterID != nil {
		p, err := h.q.GetProject(c, *afterID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &p.Position
	}
	return prev, next, true
}
