package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

const (
	// maxDocTemplates caps a workspace's gallery. The limit is not about storage
	// — it is that the gallery is a flat grid a person picks from, and a
	// thousand-tile picker is not a picker. Hitting it is a signal to delete, not
	// something to raise silently.
	maxDocTemplates = 200

	maxDocTemplateTitle = 200
	maxDocTemplateDesc  = 500
)

// documentTemplateView ships content as JSON rather than the []byte sqlc scans
// jsonb into, mirroring documentView.
type documentTemplateView struct {
	db.DocumentTemplate
	Content json.RawMessage `json:"content"`
}

func viewDocumentTemplate(t db.DocumentTemplate) documentTemplateView {
	content := json.RawMessage(t.Content)
	if len(content) == 0 {
		content = json.RawMessage(`{"type":"doc","content":[]}`)
	}
	return documentTemplateView{DocumentTemplate: t, Content: content}
}

// ListDocumentTemplates returns the workspace gallery, without bodies.
func (h *API) ListDocumentTemplates(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListDocumentTemplates(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// CreateDocumentTemplate saves a starting point for future documents, either
// from an existing document (document_id) or from a body the client parsed
// itself (content) — the second form is what uploading a .md or .json file
// lands on.
//
// Both forms go through validateDocContent. That matters for the upload path in
// particular: an uploaded file is arbitrary JSON from outside the editor, and
// the schema check is the only thing standing between it and the database.
func (h *API) CreateDocumentTemplate(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Icon        string          `json:"icon"`
		Content     json.RawMessage `json:"content"`
		DocumentID  *uuid.UUID      `json:"document_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	count, err := h.q.CountDocumentTemplates(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	if count >= maxDocTemplates {
		c.JSON(http.StatusConflict, gin.H{"error": "template limit reached"})
		return
	}

	title := req.Title
	raw := req.Content
	if req.DocumentID != nil {
		doc, err := h.q.GetDocument(c, *req.DocumentID)
		if notFound(c, err) {
			return
		}
		if err != nil {
			fail(c, err)
			return
		}
		// Authorize against the *document's* workspace as well: without this a
		// member of workspace A could template any document whose id they know.
		if doc.WorkspaceID != wsID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "document belongs to another workspace"})
			return
		}
		raw = json.RawMessage(doc.Content)
		if title == "" {
			title = doc.Title
		}
		if req.Icon == "" {
			req.Icon = doc.Icon
		}
	}
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	clean, preview, err := validateDocContent(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	tpl, err := h.q.CreateDocumentTemplate(c, db.CreateDocumentTemplateParams{
		WorkspaceID: wsID,
		AuthorID:    &uid,
		Title:       truncateRunes(title, maxDocTemplateTitle),
		Description: truncateRunes(req.Description, maxDocTemplateDesc),
		Icon:        req.Icon,
		Content:     clean,
		Preview:     preview,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, viewDocumentTemplate(tpl))
}

// GetDocumentTemplate returns one template with its body — what the gallery
// asks for when a template is previewed.
func (h *API) GetDocumentTemplate(c *gin.Context) {
	tpl, ok := h.loadDocumentTemplate(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, viewDocumentTemplate(tpl))
}

// UpdateDocumentTemplate edits the gallery card: title, description, icon. The
// body is not editable here — a template whose content needs work is opened as
// a document and saved again, which keeps one editing surface instead of two.
func (h *API) UpdateDocumentTemplate(c *gin.Context) {
	tpl, ok := h.loadDocumentTemplate(c)
	if !ok {
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Icon        *string `json:"icon"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	next := db.UpdateDocumentTemplateMetaParams{
		ID:          tpl.ID,
		Title:       tpl.Title,
		Description: tpl.Description,
		Icon:        tpl.Icon,
	}
	if req.Title != nil {
		if *req.Title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title must not be empty"})
			return
		}
		next.Title = truncateRunes(*req.Title, maxDocTemplateTitle)
	}
	if req.Description != nil {
		next.Description = truncateRunes(*req.Description, maxDocTemplateDesc)
	}
	if req.Icon != nil {
		next.Icon = *req.Icon
	}
	updated, err := h.q.UpdateDocumentTemplateMeta(c, next)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, viewDocumentTemplate(updated))
}

// DeleteDocumentTemplate removes a template. Documents already created from it
// are untouched — a template is a starting point, not a parent.
func (h *API) DeleteDocumentTemplate(c *gin.Context) {
	tpl, ok := h.loadDocumentTemplate(c)
	if !ok {
		return
	}
	if err := h.q.DeleteDocumentTemplate(c, tpl.ID); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// loadDocumentTemplate fetches a template (path param :id) and authorizes via
// its workspace.
func (h *API) loadDocumentTemplate(c *gin.Context) (db.DocumentTemplate, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.DocumentTemplate{}, false
	}
	tpl, err := h.q.GetDocumentTemplate(c, id)
	if notFound(c, err) {
		return db.DocumentTemplate{}, false
	}
	if err != nil {
		fail(c, err)
		return db.DocumentTemplate{}, false
	}
	if !h.requireMember(c, tpl.WorkspaceID) {
		return db.DocumentTemplate{}, false
	}
	return tpl, true
}
