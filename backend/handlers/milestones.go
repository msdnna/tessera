package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// Milestones («Этап»): a project-scoped planning unit. Native CRUD here; the GitLab
// pull/write-back map onto these in later steps via gitlab_milestone_links.

// milestoneReq is the create/update payload (dates are RFC3339, nullable).
type milestoneReq struct {
	Title       string     `json:"title" binding:"required"`
	Description string     `json:"description"`
	StartDate   *time.Time `json:"start_date"`
	DueDate     *time.Time `json:"due_date"`
	State       string     `json:"state"` // active | closed (defaults active)
}

// ListMilestones returns a project's milestones.
func (h *API) ListMilestones(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForProject(c, projectID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	ms, err := h.q.ListMilestones(c, projectID)
	if err != nil {
		fail(c)
		return
	}
	if ms == nil {
		ms = []db.Milestone{}
	}
	c.JSON(http.StatusOK, ms)
}

// CreateMilestone adds a milestone to a project.
func (h *API) CreateMilestone(c *gin.Context) {
	projectID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForProject(c, projectID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	var req milestoneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var state *string
	if req.State == "closed" || req.State == "active" {
		state = &req.State
	}
	m, err := h.q.CreateMilestone(c, db.CreateMilestoneParams{
		ProjectID: projectID, Title: req.Title, Description: req.Description,
		StartDate: req.StartDate, DueDate: req.DueDate, State: state,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "milestone.created", m)
	c.JSON(http.StatusCreated, m)
}

// milestoneWorkspace resolves a milestone's owning workspace (via its project) and
// enforces membership; returns the milestone on success.
func (h *API) milestoneWorkspace(c *gin.Context, id uuid.UUID) (db.Milestone, uuid.UUID, bool) {
	m, err := h.q.GetMilestone(c, id)
	if notFound(c, err) {
		return db.Milestone{}, uuid.Nil, false
	}
	if err != nil {
		fail(c)
		return db.Milestone{}, uuid.Nil, false
	}
	wsID, err := h.q.WorkspaceIDForProject(c, m.ProjectID)
	if err != nil {
		fail(c)
		return db.Milestone{}, uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return db.Milestone{}, uuid.Nil, false
	}
	return m, wsID, true
}

// UpdateMilestone edits a milestone (GitLab-sourced ones are guarded on the client;
// the server still allows local edits — the next pull re-asserts GitLab's values).
func (h *API) UpdateMilestone(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	var req milestoneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	state := req.State
	if state != "closed" {
		state = "active"
	}
	m, err := h.q.UpdateMilestone(c, db.UpdateMilestoneParams{
		ID: id, Title: req.Title, Description: req.Description,
		StartDate: req.StartDate, DueDate: req.DueDate, State: state,
	})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "milestone.updated", m)
	c.JSON(http.StatusOK, m)
}

// DeleteMilestone removes a milestone; tasks pointing at it are cleared (SET NULL).
func (h *API) DeleteMilestone(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	if err := h.q.DeleteMilestone(c, id); err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "milestone.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

// SetTaskMilestone assigns a milestone to a task (body {milestone_id}); a null
// milestone_id clears it.
func (h *API) SetTaskMilestone(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		MilestoneID *uuid.UUID `json:"milestone_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.SetTaskMilestone(c, db.SetTaskMilestoneParams{ID: id, MilestoneID: req.MilestoneID}); err != nil {
		fail(c)
		return
	}
	// A manual milestone change on a GitLab-linked task wins over the sync.
	_ = h.q.MarkGitlabMilestoneOverridden(c, id)
	if t, err := h.q.GetTask(c, id); err == nil {
		h.broadcast(wsID, "task.updated", t)
	}
	c.Status(http.StatusNoContent)
}

// ClearTaskMilestone removes a task's milestone.
func (h *API) ClearTaskMilestone(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	if err := h.q.SetTaskMilestone(c, db.SetTaskMilestoneParams{ID: id, MilestoneID: nil}); err != nil {
		fail(c)
		return
	}
	_ = h.q.MarkGitlabMilestoneOverridden(c, id)
	if t, err := h.q.GetTask(c, id); err == nil {
		h.broadcast(wsID, "task.updated", t)
	}
	c.Status(http.StatusNoContent)
}
