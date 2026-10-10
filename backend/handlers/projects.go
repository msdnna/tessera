package handlers

import (
	"net/http"
	"strings"
	"time"

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

// groupParentAllowed authorizes the parent_id a caller sends in the *body* of a
// create/move. The route's :id is checked by the caller (requireMember or
// loadGroup); the parent is a second reference to a group the caller may have no
// rights to, and nothing else ties it to the workspace being edited.
//
// Two things go wrong without it. A parent in someone else's workspace drops the
// branch out of both trees at once — the client lists the groups of one
// workspace, so a parent it never sees makes the branch unreachable, while
// workspace_id (and with it every authorization check) still points at the old
// workspace. And a parent inside the moved group's own subtree closes the tree
// into a cycle with no root: both clients walk parent_id by recursion, so what
// the database holds as one odd row lands as an unbounded walk in the sidebar of
// every member — broadcast to them and surviving a reload, with the sidebar
// itself being the only UI that could undo it.
//
// moved is uuid.Nil on create (nothing can be below a group that does not exist
// yet). Mirrors reparentAllowed for documents, which is the same tree.
//
// Projects hang off the same tree and pass their group_id through here with
// moved = uuid.Nil: only the workspace half applies (a project is a leaf, so it
// has no subtree to close a cycle with), and a project under a foreign group
// falls out of the sidebar exactly the same way. TransferProject states the
// invariant from the other side — it clears group_id on the way out, because
// groups belong to the source workspace.
func (h *API) groupParentAllowed(c *gin.Context, wsID, moved uuid.UUID, parentID *uuid.UUID) bool {
	if parentID == nil {
		return true
	}
	parent, err := h.q.GetProjectGroup(c, *parentID)
	if notFound(c, err) {
		return false
	}
	if err != nil {
		fail(c, err)
		return false
	}
	if parent.WorkspaceID != wsID {
		c.JSON(http.StatusForbidden, gin.H{"error": "parent group belongs to another workspace"})
		return false
	}
	if moved == uuid.Nil {
		return true
	}
	// Walk up from the proposed parent: meeting the moved group on the way to the
	// root means the parent sits below it. The walk starts at the parent itself,
	// so "be your own parent" is the same case and needs no separate branch.
	ancestors, err := h.q.ProjectGroupAncestorIDs(c, *parentID)
	if err != nil {
		fail(c, err)
		return false
	}
	for _, id := range ancestors {
		if id == moved {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot move a group into itself or its own subtree"})
			return false
		}
	}
	return true
}

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
	if !h.groupParentAllowed(c, wsID, uuid.Nil, req.ParentID) {
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
	h.broadcastAs(c, wsID, "group.created", g)
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
	h.broadcastAs(c, g.WorkspaceID, "group.updated", updated)
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
	if !h.groupParentAllowed(c, g.WorkspaceID, g.ID, req.ParentID) {
		return
	}
	if rejectSameNeighbour(c, req.BeforeID, req.AfterID) {
		return
	}
	prev, next, ok := h.neighborGroupPositions(c, g.WorkspaceID, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	pos, ok := h.positionForReorder(c, prev, next,
		func() error {
			return h.q.RebalanceProjectGroupPositions(c, db.RebalanceProjectGroupPositionsParams{
				WorkspaceID: g.WorkspaceID, Gap: positionGap,
			})
		},
		func() (*float64, *float64, bool) {
			return h.neighborGroupPositions(c, g.WorkspaceID, req.BeforeID, req.AfterID)
		})
	if !ok {
		return
	}
	updated, err := h.q.MoveProjectGroup(c, db.MoveProjectGroupParams{
		ID: g.ID, ParentID: req.ParentID, Position: pos,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, g.WorkspaceID, "group.moved", updated)
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
	if !h.groupParentAllowed(c, wsID, uuid.Nil, req.GroupID) {
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
			h.respondSlugTaken(c, s)
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
	var p db.Project
	if err := h.inTx(c, func(q *db.Queries) error {
		var err error
		p, err = q.CreateProject(c, db.CreateProjectParams{
			WorkspaceID: wsID, GroupID: req.GroupID, Name: req.Name,
			Color: req.Color, Icon: req.Icon, Slug: projectSlug,
			Position: positionBetween(&maxPos, nil),
		})
		if err != nil {
			return err
		}
		// A project taking an address another one left behind owns it from now
		// on (#3012): the old links lead here, not to the one that moved away.
		return q.DeleteProjectSlugAlias(c, projectSlug)
	}); err != nil {
		// Two creates racing for one address: the index arbitrates, and the
		// loser hears what a pre-checked collision would have told it.
		if isUniqueViolation(err) {
			h.respondSlugTaken(c, projectSlug)
			return
		}
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "project.created", p)
	c.JSON(http.StatusCreated, p)
}

// ListProjects lists a workspace's projects.
func (h *API) ListProjects(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	// ?archived=1 is the «Архив проектов» list (0086); the default is the
	// sidebar tree, which leaves archived projects out.
	list := h.q.ListProjects
	if c.Query("archived") == "1" {
		list = h.q.ListArchivedProjects
	}
	projects, err := list(c, wsID)
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

// UpdateProject edits name/color/icon and group membership. It writes group_id
// just like create and move do, so it needs the same guard: this is the third
// door into the project tree, and a foreign group here drops the project out of
// the sidebar exactly as it would there (groupParentAllowed, #2932).
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
	if !h.groupParentAllowed(c, p.WorkspaceID, uuid.Nil, req.GroupID) {
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
	h.broadcastAs(c, p.WorkspaceID, "project.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// SetProjectSlug changes a project's URL address. Owners and admins only — it
// rewrites every link to the project, so it isn't a per-member edit. The old
// address keeps leading here for projectSlugAliasTTL (#3012), unless another
// project takes it first. The slug is deliberately left out of UpdateProject:
// recolouring a project shouldn't silently rewrite its URL.
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
	// workspace the caller can't see — respondSlugTaken names it only to members.
	taken, err := h.q.ProjectSlugExists(c, s)
	if err != nil {
		fail(c, err)
		return
	}
	if taken {
		h.respondSlugTaken(c, s)
		return
	}
	if err := h.inTx(c, func(q *db.Queries) error {
		if err := q.SetProjectSlug(c, db.SetProjectSlugParams{ID: p.ID, Slug: s}); err != nil {
			return err
		}
		// The new address may be one this or another project left behind: the
		// live slug owns it now.
		if err := q.DeleteProjectSlugAlias(c, s); err != nil {
			return err
		}
		if p.Slug == "" {
			return nil
		}
		return q.UpsertProjectSlugAlias(c, db.UpsertProjectSlugAliasParams{
			Slug: p.Slug, ProjectID: p.ID, ExpiresAt: time.Now().Add(projectSlugAliasTTL),
		})
	}); err != nil {
		if isUniqueViolation(err) {
			h.respondSlugTaken(c, s)
			return
		}
		fail(c, err)
		return
	}
	updated, err := h.q.GetProject(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, p.WorkspaceID, "project.updated", updated)
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
	if !h.groupParentAllowed(c, p.WorkspaceID, uuid.Nil, req.GroupID) {
		return
	}
	if rejectSameNeighbour(c, req.BeforeID, req.AfterID) {
		return
	}
	prev, next, ok := h.neighborProjectPositions(c, p.WorkspaceID, req.BeforeID, req.AfterID)
	if !ok {
		return
	}
	pos, ok := h.positionForReorder(c, prev, next,
		func() error {
			return h.q.RebalanceProjectPositions(c, db.RebalanceProjectPositionsParams{
				WorkspaceID: p.WorkspaceID, Gap: positionGap,
			})
		},
		func() (*float64, *float64, bool) {
			return h.neighborProjectPositions(c, p.WorkspaceID, req.BeforeID, req.AfterID)
		})
	if !ok {
		return
	}
	updated, err := h.q.MoveProject(c, db.MoveProjectParams{
		ID: p.ID, GroupID: req.GroupID, Position: pos,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, p.WorkspaceID, "project.moved", updated)
	c.JSON(http.StatusOK, updated)
}

// TransferProject moves a project — with all its boards, tasks, tags, notes and
// GitLab bindings — to another workspace. A dangerous operation: it re-stamps the
// denormalized workspace_id across the project's tags/tag_prefixes/notes and any
// GitLab integrations, re-numbers its tasks out of the target's counter, strips
// assignees who aren't members of the target workspace, severs the relations,
// milestone links, document links and task tags whose two ends the move would put
// in different workspaces, and cancels the document approval routes the move leaves
// waiting on somebody who can no longer open the document.
// Requires membership in BOTH the source (checked by loadProject) and
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
	// Notes and documents are addressed by a slug unique *per workspace*, and the
	// re-stamps below carry them into one that may already use those addresses —
	// a title shared by both sides («Регламент» is not a rare one) aborted the
	// whole transfer on the unique index, with nothing in the 500 naming the row
	// that caused it and every retry failing the same way. Freeing the clashing
	// addresses first leaves the target's own rows where they are.
	if err := h.freeProjectSlugs(c, qtx, p.ID, req.WorkspaceID); err != nil {
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
	// A task's #N is issued by its workspace's counter, and the arrivals carry the
	// source workspace's numbering. Nothing in the schema can catch the clash —
	// tasks has no workspace_id, the number is scoped through boards → projects,
	// so there is no unique index to violate — and GetTaskByNumber is a `:one`
	// with neither LIMIT nor ORDER BY: with two matches it silently returns
	// whichever row came first. That is a #-link, `/blocks #42` or `/parent #42`
	// landing on the wrong task, for the *resident* task as much as the arriving
	// one. Leaving the target's counter alone would keep minting the clash for
	// every task created afterwards, so the numbers are re-stamped from a block
	// reserved out of that counter, inside this transaction.
	renumbered, err := qtx.CountProjectTasks(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if renumbered > 0 {
		last, rerr := qtx.ReserveWorkspaceTaskNumbers(c, db.ReserveWorkspaceTaskNumbersParams{
			ID: req.WorkspaceID, Count: renumbered,
		})
		if rerr != nil {
			fail(c, rerr)
			return
		}
		if rerr := qtx.RenumberProjectTasks(c, db.RenumberProjectTasksParams{
			ProjectID: p.ID, Base: last - renumbered,
		}); rerr != nil {
			fail(c, rerr)
			return
		}
	}
	// Two of a task's links were authorized once, when they were made, against a
	// workspace boundary this transfer just moved out from under them: a relation
	// (AddRelation resolves the other end through GetTaskByNumber scoped to the
	// caller's workspace) and a milestone (applyMilestone refuses one from another
	// workspace outright). Neither read re-checks that later — ListTaskRelations
	// joins tasks for related_number/related_title, ListWorkspaceMilestones rolls
	// up over t.milestone_id, both without a workspace filter — so a link left
	// hanging across the new boundary keeps handing each side the other's task
	// titles and keeps a stranger's task in a milestone's count and Σ estimate.
	// Severed here, in the same transaction, in both directions. Parent links need
	// no such pass: applyParent adopts the parent's board, so a parent and its
	// subtree share a project and travel together.
	unlinked, err := qtx.DeleteRelationsAcrossWorkspaces(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	unpinned, err := qtx.ClearMilestonesAcrossWorkspaces(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	// A document↔task link is the third of the kind, and the only one whose two
	// ends are different objects: the project's documents travel with it while a
	// task in a neighbouring project does not, so either end can be the one left
	// behind. CreateDocumentTaskLink refuses a cross-workspace pair and says why
	// — the row would "leak the task's title to people who cannot see the task" —
	// and both reads join straight through it for task_title and document
	// title/slug.
	unlinkedDocs, err := qtx.DeleteDocumentLinksAcrossWorkspaces(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	// The fourth is a document's own parent, and unlike a task's it does not
	// travel with the project: nesting and project are independent columns, so a
	// document of this project can be nested under one that stays behind, or be
	// the parent of one. parentInWorkspace refuses such a pair at the door, and
	// the reads that walk parent_id assume it never exists — CountDocumentChildren
	// blocks the delete of a parent over a child its workspace cannot see,
	// DeleteDocumentSubtree follows the link straight into the other workspace
	// and erases documents there (with no room to drop, since documentSubtreeIDs
	// only walks the caller's own list), and ListDocuments plus a client-side tree
	// render a parentless-looking child nowhere at all. Detached after the
	// re-stamp above, so both ends are compared at their final workspace.
	detachedDocs, err := qtx.DetachDocumentParentsAcrossWorkspaces(c, &p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	// The fifth is the tag on a task, and it is the one the per-project scoping
	// of tags makes easy to miss: applyTag lets a tag cross *projects* inside a
	// workspace on purpose ("which the board views rely on"), so the moving
	// project's tasks may carry a neighbour's tag and the neighbour's tasks may
	// carry one of this project's — re-stamped to the target a few lines up. The
	// same door refuses a tag from another workspace and names the harm: the task
	// detail "reads its name and colour back, a cross-workspace read of data the
	// caller was never shown", and ListTaskTags joins straight through task_tags
	// with no filter of its own. Only the pairing is dropped; both tags stay.
	strippedTags, err := qtx.DeleteTaskTagsAcrossWorkspaces(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	// The sixth is the conference pinned to a task, and it is the one link that
	// cannot travel at all: a conference carries its own workspace_id and belongs
	// to no project, so the call stays where it was held while the task it was
	// about walks into another workspace. CreateConference refuses such a pair at
	// the door and names the harm — the link "is what later files the protocol
	// back into the task, and a cross-workspace link would put it in front of a
	// team that was never in the call" — and ListConferencesByTask is exactly that
	// read: it authorizes on the *task's* workspace, the target from here on, then
	// lists by task_id alone, handing the arriving team the title, description and
	// schedule of a meeting they were not in. Only the link is cleared; the
	// conference, its chat and its recordings stay in the workspace that held it.
	unpinnedConfs, err := qtx.ClearConferenceTasksAcrossWorkspaces(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	// The eighth and ninth are a document's stage and meeting (#3044, #3046).
	// The stage travels when it is this project's and the document does not (or
	// the reverse); a meeting never travels at all. Either pin left across the
	// boundary would put one team's stage or meeting title in the other's
	// document head, and a stranger's document on the stage page. Unpinned; the
	// document, the stage and the meeting all stay.
	unpinnedDocStages, err := qtx.ClearDocumentMilestonesAcrossWorkspaces(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	unpinnedDocMeetings, err := qtx.ClearDocumentConferencesAcrossWorkspaces(c, p.ID)
	// …and the same for the stages that travel with the project (#3053): a
	// meeting left behind would keep naming a stage its team can no longer open.
	unstagedConfs, err := qtx.ClearConferenceMilestonesAcrossWorkspaces(c, p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	// The seventh is the open approval route on a document that travels, and it is
	// the one with nothing to sever: both of its ends move together. What breaks is
	// the rule CreateDocumentApproval enforces at the door — every approver must be
	// a member of the document's workspace, because "someone who cannot open it can
	// only ever stall it" — which this transfer invalidates from behind, leaving the
	// route pointing at the people the *source* workspace asked. They are answered
	// 403 by loadDocumentApproval from here on, nothing adds or replaces a step in a
	// route already raised, and the one-open-route rule then refuses the new owners
	// a route of their own. So the stalled ones are closed here rather than left for
	// a manager to find: the protocol stays in the журнал, as a cancellation always
	// does, and the document is free to be sent round its new workspace.
	cancelledApprovals, err := qtx.CancelApprovalsAcrossWorkspaces(c, db.CancelApprovalsAcrossWorkspacesParams{
		ProjectID: &p.ID, WorkspaceID: req.WorkspaceID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	// The travelling documents are also live rooms, and a transfer is as invisible
	// to them as a delete is: ConnectDocument authorizes once, at the upgrade,
	// against the workspace the document had then. Everyone sitting in these rooms
	// is by definition a member of the *source* workspace — the target's people
	// could not open the document before this call — so leaving the rooms up hands
	// them the presence badges of the new owners' editors, keeps announcing every
	// save of a document they are now answered 403 for, and keeps their block locks
	// held, which is a veto on the new workspace's editing that nobody there can
	// lift. Dropped after the commit, for the reason DeleteWorkspace drops its own.
	movedDocs, err := qtx.DocumentIDsInProject(c, &p.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		fail(c, err)
		return
	}
	h.dropDocRooms(movedDocs)

	// The project leaves the source workspace and joins the target — tell both.
	h.broadcast(p.WorkspaceID, "project.deleted", gin.H{"id": p.ID})
	h.broadcastAs(c, req.WorkspaceID, "project.created", updated)
	c.JSON(http.StatusOK, gin.H{
		"project": updated, "stripped_assignees": stripped, "renumbered_tasks": renumbered,
		"unlinked_relations": unlinked, "unpinned_milestones": unpinned,
		"unlinked_documents": unlinkedDocs, "detached_documents": detachedDocs,
		"stripped_tags": strippedTags, "unpinned_conferences": unpinnedConfs,
		"cancelled_approvals":        cancelledApprovals,
		"unpinned_document_stages":   unpinnedDocStages,
		"unpinned_document_meetings": unpinnedDocMeetings,
		"unstaged_conferences":       unstagedConfs,
	})
}

// DeleteProject removes a project and everything under it.
func (h *API) DeleteProject(c *gin.Context) {
	p, ok := h.loadProject(c)
	if !ok {
		return
	}
	// A project's tasks are two cascades away (project → board → task), so the
	// sweep has to resolve them rather than read them off the id in the URL.
	// Transactional for the same reason as DeleteBoard: the list must be what the
	// delete actually took.
	var gone []uuid.UUID
	if err := h.inTx(c, func(q *db.Queries) error {
		var err error
		if gone, err = q.TaskIDsInProject(c, p.ID); err != nil {
			return err
		}
		return q.DeleteProject(c, p.ID)
	}); err != nil {
		fail(c, err)
		return
	}
	h.purgeTaskUploads(gone)
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

// projectInWorkspace authorizes a project_id that arrived in a request *body*,
// where the route vouches for the workspace but nothing vouches for the
// project. Same shape and same role as parentInWorkspace for a document's
// parent_id: 404 when there is no such project, 400 when it lives elsewhere.
//
// Pinning a note or a document across the boundary is not a cosmetic mismatch,
// because two writes read the relation by project_id alone. notes.project_id is
// ON DELETE CASCADE, so a stranger deleting their own project takes the note
// with it; and TransferProject re-stamps workspace_id on a project's notes and
// documents (`WHERE project_id = $1`), carrying them out of the workspace that
// wrote them and into one whose members never saw them.
func (h *API) projectInWorkspace(c *gin.Context, projectID, wsID uuid.UUID) bool {
	p, err := h.q.GetProject(c, projectID)
	if notFound(c, err) {
		return false
	}
	if err != nil {
		fail(c, err)
		return false
	}
	if p.WorkspaceID != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project belongs to another workspace"})
		return false
	}
	return true
}

// neighborGroupPositions / neighborProjectPositions resolve the before/after
// bounds of a sidebar reorder. The neighbour must live in the SAME workspace as
// the item being moved — that is the scope its ladder is numbered on
// (RebalanceProjectGroupPositions / RebalanceProjectPositions respace per
// workspace), so a neighbour from another one contributes a position that says
// nothing about where the drop should land, and the item ends up at an arbitrary
// end of the sidebar. `GetProjectGroup`/`GetProject` take a bare id, so without
// the check the reply also answers "does this id exist?" for a workspace the
// caller isn't a member of — same 400 now, whichever it was (#2932).
func (h *API) neighborGroupPositions(c *gin.Context, wsID uuid.UUID, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		g, err := h.q.GetProjectGroup(c, *beforeID)
		if err != nil || g.WorkspaceID != wsID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &g.Position
	}
	if afterID != nil {
		g, err := h.q.GetProjectGroup(c, *afterID)
		if err != nil || g.WorkspaceID != wsID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &g.Position
	}
	return prev, next, true
}

func (h *API) neighborProjectPositions(c *gin.Context, wsID uuid.UUID, beforeID, afterID *uuid.UUID) (prev, next *float64, ok bool) {
	if beforeID != nil {
		p, err := h.q.GetProject(c, *beforeID)
		if err != nil || p.WorkspaceID != wsID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before_id"})
			return nil, nil, false
		}
		prev = &p.Position
	}
	if afterID != nil {
		p, err := h.q.GetProject(c, *afterID)
		if err != nil || p.WorkspaceID != wsID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_id"})
			return nil, nil, false
		}
		next = &p.Position
	}
	return prev, next, true
}
