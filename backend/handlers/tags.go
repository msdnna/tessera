package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
)

// CreateTag adds a workspace-scoped tag.
func (h *API) CreateTag(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
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
	tag, err := h.q.CreateTag(c, db.CreateTagParams{WorkspaceID: wsID, Name: req.Name, Color: req.Color})
	if err != nil {
		fail(c)
		return
	}
	h.broadcast(wsID, "tag.created", tag)
	c.JSON(http.StatusCreated, tag)
}

// ListTags lists a workspace's tags.
func (h *API) ListTags(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	tags, err := h.q.ListTags(c, wsID)
	if err != nil {
		fail(c)
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
		fail(c)
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
		fail(c)
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
		fail(c)
		return
	}
	if !h.requireMember(c, tag.WorkspaceID) {
		return
	}
	if err := h.q.DeleteTag(c, id); err != nil {
		fail(c)
		return
	}
	h.broadcast(tag.WorkspaceID, "tag.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}
