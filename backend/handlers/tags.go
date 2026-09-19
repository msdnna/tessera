package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
)

// tagNameTaken is the refusal both write paths share. Russian, like every other
// sentence the API hands a reader (see userErr in task_ops.go); the web client
// shows `{"error": …}` verbatim.
const tagNameTaken = "тег с таким именем уже есть в проекте"

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
	tag, err := h.q.CreateTag(c, db.CreateTagParams{WorkspaceID: wsID, ProjectID: projectID, Name: req.Name, Color: req.Color})
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
	updated, err := h.q.UpdateTag(c, db.UpdateTagParams{ID: id, Name: req.Name, Color: req.Color})
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
