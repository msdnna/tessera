package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
)

// tagNameTaken is the refusal both write paths share. Russian, like every other
// sentence the API hands a reader (see userErr in task_ops.go); the web client
// shows `{"error": …}` verbatim.
const tagNameTaken = "тег с таким именем уже есть в проекте"

// tagNameEmpty is what a name made of nothing but spaces comes back as. `binding:
// "required"` only rejects the empty string, and the name is trimmed before it is
// stored (see tagNameFree), so " " would otherwise be saved as a nameless tag —
// an unclickable blank pill and a blank lane on a board grouped by tags.
const tagNameEmpty = "имя тега не может быть пустым"

// tagNameFree reports whether `name` is still free in this project, i.e. whether
// writing it would create a tag no reader can tell from one already there.
//
// Storage compares names byte-exactly (UNIQUE (project_id, name), migration 0023)
// and nothing downstream does: `canonPrefix` lowercases a namespace on all three
// clients, the web tag picker's `matchTagByName` resolves a typed name
// case-insensitively, and the MCP `resolveTags` matches the same way. So a twin
// differing only in case or in padding is one name to every consumer and two rows
// here — and the killer feature draws one lane per row, with `tagLaneLabel`
// printing only the value under a narrowed prefix: two lanes captioned identically,
// with the project's cards split between them and no way to tell which is which.
//
// `except` is the row being renamed (uuid.Nil on create): re-spelling a tag's own
// name («bug» → «Bug») is a legitimate edit, not a collision with itself.
//
// This is a check-then-write, so two simultaneous creates can still slip a twin
// past it; the byte-exact index stays as the backstop for the far likelier exact
// duplicate, and the sync's own door is closed in gitlab.go (ensureProjectTag).
func (h *API) tagNameFree(c *gin.Context, projectID uuid.UUID, name string, except uuid.UUID) (bool, error) {
	twin, err := h.q.GetProjectTagByName(c, db.GetProjectTagByNameParams{ProjectID: projectID, Name: name})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return true, nil
	case err != nil:
		return false, err
	}
	return twin.ID == except, nil
}

// tagNameFromBody trims the submitted name and refuses a blank one. Returns ok=false
// once the refusal has been written.
func tagNameFromBody(c *gin.Context, name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": tagNameEmpty})
		return "", false
	}
	return trimmed, true
}

// CreateTag adds a project-scoped tag.
func (h *API) CreateTag(c *gin.Context) {
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
	var req struct {
		Name  string `json:"name" binding:"required"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, ok := tagNameFromBody(c, req.Name)
	if !ok {
		return
	}
	free, err := h.tagNameFree(c, projectID, name, uuid.Nil)
	if err != nil {
		fail(c, err)
		return
	}
	if !free {
		c.JSON(http.StatusConflict, gin.H{"error": tagNameTaken})
		return
	}
	tag, err := h.q.CreateTag(c, db.CreateTagParams{WorkspaceID: wsID, ProjectID: projectID, Name: name, Color: req.Color})
	if err != nil {
		// UNIQUE(project_id, name). The name comes straight from a text field
		// that does not consult the project's tag list — the one in the task
		// modal offers «создать» for whatever is typed, and a project's tags are
		// grouped by prefix and scroll — so a reader hits this by typing a name
		// that already exists, which is an ordinary outcome and not a fault.
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": tagNameTaken})
			return
		}
		fail(c, err)
		return
	}
	h.broadcast(wsID, "tag.created", tag)
	c.JSON(http.StatusCreated, tag)
}

// ListTags lists a project's tags.
func (h *API) ListTags(c *gin.Context) {
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
	tags, err := h.q.ListTags(c, projectID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, tags)
}

// ListWorkspaceTags lists every tag across all projects in a workspace — for
// read-only views that span projects (Home, cross-project task lists).
func (h *API) ListWorkspaceTags(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	tags, err := h.q.ListWorkspaceTags(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, tags)
}

// UpdateTag edits a tag's name/color.
func (h *API) UpdateTag(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	tag, err := h.q.GetTag(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, tag.WorkspaceID) {
		return
	}
	var req struct {
		Name  string `json:"name" binding:"required"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, ok := tagNameFromBody(c, req.Name)
	if !ok {
		return
	}
	free, err := h.tagNameFree(c, tag.ProjectID, name, id)
	if err != nil {
		fail(c, err)
		return
	}
	if !free {
		c.JSON(http.StatusConflict, gin.H{"error": tagNameTaken})
		return
	}
	updated, err := h.q.UpdateTag(c, db.UpdateTagParams{ID: id, Name: name, Color: req.Color})
	if err != nil {
		// Same index as CreateTag, reached by renaming onto a name the project
		// already has. The rename field saves on blur as well as on Enter, so
		// clicking away from a collision is enough to get here.
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": tagNameTaken})
			return
		}
		fail(c, err)
		return
	}
	h.broadcast(tag.WorkspaceID, "tag.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// DeleteTag removes a tag (and its task associations via cascade).
func (h *API) DeleteTag(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	tag, err := h.q.GetTag(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, tag.WorkspaceID) {
		return
	}
	if err := h.q.DeleteTag(c, id); err != nil {
		fail(c, err)
		return
	}
	h.broadcast(tag.WorkspaceID, "tag.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}
