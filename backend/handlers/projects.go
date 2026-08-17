package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// normIconMode validates the icon colouring mode, defaulting unknown/empty
// values (e.g. older clients that don't send the field) to "badge".
func normIconMode(m string) string {
	if m == "icon" {
		return "icon"
	}
	return "badge"
}

// normTreeMode validates the sidebar tree mode (what a project shows as children),
// defaulting unknown/empty to "boards".
func normTreeMode(m string) string {
	switch m {
	case "milestones", "both":
		return m
	default:
		return "boards"
	}
}

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
	maxPos, err := h.q.MaxProjectGroupPosition(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	g, err := h.q.CreateProjectGroup(c, db.CreateProjectGroupParams{
		WorkspaceID: wsID, ParentID: req.ParentID, Name: req.Name, Position: positionBetween(&maxPos, nil),
	})
	if err != nil {
		fail(c, err)
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
		fail(c, err)
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
		Name     string `json:"name" binding:"required"`
		Icon     string `json:"icon"`
		Color    string `json:"color"`
		IconMode string `json:"icon_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateProjectGroup(c, db.UpdateProjectGroupParams{
		ID: g.ID, Name: req.Name, Icon: req.Icon, Color: req.Color,
		IconMode: normIconMode(req.IconMode),
	})
	if err != nil {
		fail(c, err)
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
		fail(c, err)
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
		fail(c, err)
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
		Slug    string     `json:"slug"`
		GroupID *uuid.UUID `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// An explicit address is a request for that exact address: reject a taken
	// one instead of silently handing back "name-2". Empty means "derive it".
	projectSlug := ""
	if strings.TrimSpace(req.Slug) != "" {
		if !h.requireManager(c, wsID) {
			return
		}
		s, ok := normalizeProjectSlug(req.Slug)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
			return
		}
		taken, err := h.q.ProjectSlugExists(c, s)
		if err != nil {
			fail(c, err)
			return
		}
		if taken {
			c.JSON(http.StatusConflict, gin.H{"error": "slug already taken"})
			return
		}
		projectSlug = s
	} else {
		projectSlug = h.uniqueProjectSlug(c, req.Name)
	}
	maxPos, err := h.q.MaxProjectPosition(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	p, err := h.q.CreateProject(c, db.CreateProjectParams{
		WorkspaceID: wsID, GroupID: req.GroupID, Name: req.Name,
		Color: req.Color, Icon: req.Icon, Slug: projectSlug,
		Position: positionBetween(&maxPos, nil),
	})
	if err != nil {
		fail(c, err)
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
		fail(c, err)
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
		Name     string     `json:"name" binding:"required"`
		Color    string     `json:"color"`
		Icon     string     `json:"icon"`
		IconMode string     `json:"icon_mode"`
		TreeMode string     `json:"tree_mode"`
		GroupID  *uuid.UUID `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateProject(c, db.UpdateProjectParams{
		ID: p.ID, Name: req.Name, Color: req.Color, Icon: req.Icon,
		GroupID: req.GroupID, IconMode: normIconMode(req.IconMode),
		TreeMode: normTreeMode(req.TreeMode),
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(p.WorkspaceID, "project.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// SetProjectSlug changes a project's URL address. Owners and admins only —
// links already handed out stop resolving, so it isn't a per-member edit.
// The slug is deliberately left out of UpdateProject: recolouring a project
// shouldn't silently rewrite its URL.
func (h *API) SetProjectSlug(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	if !h.requireManager(c, p.WorkspaceID) {
		return
	}
	var req struct {
		Slug string `json:"slug" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s, ok := normalizeProjectSlug(req.Slug)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid slug"})
		return
	}
	if s == p.Slug {
		c.JSON(http.StatusOK, p)
		return
	}
	// Slugs are unique across the whole instance, so the holder may sit in a
	// workspace the caller can't see — keep the message free of specifics.
	taken, err := h.q.ProjectSlugExists(c, s)
	if err != nil {
		fail(c, err)
		return
	}
	if taken {
		c.JSON(http.StatusConflict, gin.H{"error": "slug already taken"})
		return
	}
	if err := h.q.SetProjectSlug(c, db.SetProjectSlugParams{ID: p.ID, Slug: s}); err != nil {
		fail(c, err)
		return
	}
	updated, err := h.q.GetProject(c, p.ID)
	if err != nil {
		fail(c, err)
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
		fail(c, err)
		return
	}
	h.broadcast(p.WorkspaceID, "project.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// TransferProject moves a project — with all its boards, tasks, tags, notes and
// GitLab bindings — to another workspace. A dangerous operation: it re-stamps the
// denormalized workspace_id across the project's tags/tag_prefixes/notes and any
// GitLab integrations, and strips assignees who aren't members of the target
// workspace. Requires membership in BOTH the source (checked by loadProject) and
// the target workspace. The project lands ungrouped at the target's root.
func (h *API) TransferProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	var req struct {
		WorkspaceID uuid.UUID `json:"workspace_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.WorkspaceID == p.WorkspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project is already in this workspace"})
		return
	}
	if !h.requireMember(c, req.WorkspaceID) {
		return
	}

	// Position at the end of the target workspace's root.
	maxPos, err := h.q.MaxProjectPosition(c, req.WorkspaceID)
	if err != nil {
		fail(c, err)
		return
	}

	tx, err := h.pool.Begin(c)
	if err != nil {
		fail(c, err)
		return
	}
	defer tx.Rollback(c) //nolint:errcheck // no-op after a successful Commit
	qtx := h.q.WithTx(tx)

	updated, err := qtx.TransferProject(c, db.TransferProjectParams{
		ID: p.ID, WorkspaceID: req.WorkspaceID, Position: positionBetween(&maxPos, nil),
	})
	if err != nil {
		fail(c, err)
		return
	}
	if err := qtx.ReassignProjectTagsWorkspace(c, db.ReassignProjectTagsWorkspaceParams{
		ProjectID: p.ID, WorkspaceID: req.WorkspaceID,
	}); err != nil {
		fail(c, err)
		return
	}
	if err := qtx.ReassignProjectTagPrefixesWorkspace(c, db.ReassignProjectTagPrefixesWorkspaceParams{
		ProjectID: p.ID, WorkspaceID: req.WorkspaceID,
	}); err != nil {
		fail(c, err)
		return
	}
	if err := qtx.ReassignProjectNotesWorkspace(c, db.ReassignProjectNotesWorkspaceParams{
		ProjectID: &p.ID, WorkspaceID: req.WorkspaceID,
	}); err != nil {
		fail(c, err)
		return
	}
	// Documents follow the project, exactly as notes do. requireMember
	// authorizes on documents.workspace_id, so leaving it stale would keep the
	// documents visible to the team the project left and hide them from the one
	// that now owns it.
	if err := qtx.ReassignProjectDocumentsWorkspace(c, db.ReassignProjectDocumentsWorkspaceParams{
		ProjectID: &p.ID, WorkspaceID: req.WorkspaceID,
	}); err != nil {
		fail(c, err)
		return
	}
	if err := qtx.ReassignProjectGitlabWorkspace(c, db.ReassignProjectGitlabWorkspaceParams{
		ProjectID: p.ID, WorkspaceID: req.WorkspaceID,
	}); err != nil {
		fail(c, err)
		return
	}
	stripped, err := qtx.StripNonMemberAssignees(c, db.StripNonMemberAssigneesParams{
		ProjectID: p.ID, WorkspaceID: req.WorkspaceID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		fail(c, err)
		return
	}

	// The project leaves the source workspace and joins the target — tell both.
	h.broadcast(p.WorkspaceID, "project.deleted", gin.H{"id": p.ID})
	h.broadcast(req.WorkspaceID, "project.created", updated)
	c.JSON(http.StatusOK, gin.H{"project": updated, "stripped_assignees": stripped})
}

// DeleteProject removes a project and everything under it.
func (h *API) DeleteProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	if err := h.q.DeleteProject(c, p.ID); err != nil {
		fail(c, err)
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
		fail(c, err)
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
		fail(c, err)
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
