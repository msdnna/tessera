package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/middleware"
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
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	ms, err := h.q.ListMilestones(c, projectID)
	if err != nil {
		fail(c, err)
		return
	}
	if ms == nil {
		ms = []db.ListMilestonesRow{}
	}
	c.JSON(http.StatusOK, ms)
}

// ListWorkspaceMilestones returns every milestone across the workspace's projects
// with task rollups (total / done / Σ estimate) for the dedicated «Этапы» screen.
func (h *API) ListWorkspaceMilestones(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	ms, err := h.q.ListWorkspaceMilestones(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	if ms == nil {
		ms = []db.ListWorkspaceMilestonesRow{}
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
		fail(c, err)
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
		Slug: h.uniqueMilestoneSlug(c, projectID, req.Title),
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "milestone.created", m)
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
		fail(c, err)
		return db.Milestone{}, uuid.Nil, false
	}
	wsID, err := h.q.WorkspaceIDForProject(c, m.ProjectID)
	if err != nil {
		fail(c, err)
		return db.Milestone{}, uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return db.Milestone{}, uuid.Nil, false
	}
	return m, wsID, true
}

// UpdateMilestone edits a milestone. A GitLab-sourced one (#3010) is editable only
// while its binding writes to GitLab (else 409 gitlab_readonly — the next pull would
// undo the edit); the change is then written to GitLab in the same request, and the
// response carries how that went (gl_sync: ok | failed | conflict).
func (h *API) UpdateMilestone(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	prev, wsID, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	var req milestoneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	link, lerr := h.q.GetGitlabMilestoneLink(c, id)
	linked := lerr == nil
	if lerr != nil && !errors.Is(lerr, pgx.ErrNoRows) {
		fail(c, lerr)
		return
	}
	if linked {
		if !h.requireWritableMilestone(c, link) {
			return
		}
		// A link without a baseline: the stage was read-only until now, so its
		// values before this edit are the last thing GitLab sent.
		if _, has := parseMilestoneSnap(link.GlSnapshot); !has {
			soft(c, "SetGitlabMilestoneSnapshot", h.q.SetGitlabMilestoneSnapshot(c, db.SetGitlabMilestoneSnapshotParams{
				MilestoneID: id, GlSnapshot: snapOfMilestone(prev).marshal(),
			}))
		}
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
		fail(c, err)
		return
	}
	out := milestoneWithSync{Milestone: m}
	if linked && snapOfMilestone(m) != snapOfMilestone(prev) {
		actor := middleware.CurrentUser(c)
		ctx, cancel := gitlabWriteCtx(c)
		res := h.syncMilestoneToGitlab(ctx, id, &actor)
		cancel()
		out.GlSync = &res
		// The round trip may have brought in what GitLab changed meanwhile.
		if fresh, gerr := h.q.GetMilestone(c, id); gerr == nil {
			out.Milestone = fresh
		}
	}
	h.broadcastAs(c, wsID, "milestone.updated", out.Milestone)
	c.JSON(http.StatusOK, out)
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
		fail(c, err)
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
	if err := h.applyMilestone(c, id, wsID, req.MilestoneID); err != nil {
		respondOpError(c, err)
		return
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
	if err := h.applyMilestone(c, id, wsID, nil); err != nil {
		respondOpError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
