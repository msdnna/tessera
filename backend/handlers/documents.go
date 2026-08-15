package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// documentView is a document whose content ships as raw JSON rather than the
// []byte sqlc scans a jsonb column into — encoding/json would otherwise emit it
// base64-encoded.
type documentView struct {
	db.Document
	Content json.RawMessage `json:"content"`
}

func viewDocument(d db.Document) documentView {
	content := json.RawMessage(d.Content)
	if len(content) == 0 {
		content = json.RawMessage(`{"type":"doc","content":[]}`)
	}
	return documentView{Document: d, Content: content}
}

// CreateDocument adds a document to a workspace, optionally nested under
// another document and/or scoped to a project.
func (h *API) CreateDocument(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Title     string     `json:"title" binding:"required"`
		Icon      string     `json:"icon"`
		ParentID  *uuid.UUID `json:"parent_id"`
		ProjectID *uuid.UUID `json:"project_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ParentID != nil && !h.parentInWorkspace(c, *req.ParentID, wsID) {
		return
	}
	uid := middleware.CurrentUser(c)
	doc, err := h.q.CreateDocument(c, db.CreateDocumentParams{
		WorkspaceID: wsID,
		ProjectID:   req.ProjectID,
		ParentID:    req.ParentID,
		AuthorID:    &uid,
		Title:       req.Title,
		Slug:        h.uniqueDocumentSlug(c, wsID, req.Title),
		Icon:        req.Icon,
		Position:    h.nextDocumentPosition(c, wsID, req.ParentID),
	})
	if err != nil {
		fail(c, err)
		return
	}
	view := viewDocument(doc)
	h.broadcast(wsID, "document.created", documentMeta(doc))
	c.JSON(http.StatusCreated, view)
}

// ListDocuments returns the workspace's documents as a flat list (the client
// assembles the tree). Content is not included — see the query comment.
func (h *API) ListDocuments(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	if raw := c.Query("project_id"); raw != "" {
		projID, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
			return
		}
		docs, err := h.q.ListDocumentsByProject(c, db.ListDocumentsByProjectParams{
			WorkspaceID: wsID, ProjectID: &projID,
		})
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(http.StatusOK, orEmpty(docs))
		return
	}
	docs, err := h.q.ListDocuments(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(docs))
}

// GetDocument returns a single document with its content.
func (h *API) GetDocument(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, viewDocument(doc))
}

// ResolveDocumentBySlug resolves a workspace-scoped slug. It answers with the
// workspace id so a deep link can point the app at the right workspace before
// mounting the view — slugs are unique per workspace, not globally, and a
// resolver that omits the scope reproduces #2721 (board opened by link kept the
// remembered workspace and silently broke realtime).
func (h *API) ResolveDocumentBySlug(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	doc, err := h.q.GetDocumentBySlug(c, db.GetDocumentBySlugParams{
		WorkspaceID: wsID, Slug: c.Param("slug"),
	})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, viewDocument(doc))
}

// UpdateDocument edits a document's metadata: title, icon, parent, project and
// position. Content is D2's business and is not touched here.
func (h *API) UpdateDocument(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		Title     *string    `json:"title"`
		Icon      *string    `json:"icon"`
		ParentID  *uuid.UUID `json:"parent_id"`
		ProjectID *uuid.UUID `json:"project_id"`
		Position  *float64   `json:"position"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	next := db.UpdateDocumentMetaParams{
		ID:        doc.ID,
		Title:     doc.Title,
		Icon:      doc.Icon,
		ParentID:  doc.ParentID,
		ProjectID: doc.ProjectID,
		Position:  doc.Position,
	}
	if req.Title != nil {
		if *req.Title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title must not be empty"})
			return
		}
		next.Title = *req.Title
	}
	if req.Icon != nil {
		next.Icon = *req.Icon
	}
	if req.ProjectID != nil {
		next.ProjectID = req.ProjectID
	}
	if req.Position != nil {
		next.Position = *req.Position
	}
	if req.ParentID != nil {
		if !h.parentInWorkspace(c, *req.ParentID, doc.WorkspaceID) {
			return
		}
		if !h.reparentAllowed(c, doc.ID, *req.ParentID) {
			return
		}
		next.ParentID = req.ParentID
	}
	updated, err := h.q.UpdateDocumentMeta(c, next)
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(doc.WorkspaceID, "document.updated", documentMeta(updated))
	c.JSON(http.StatusOK, viewDocument(updated))
}

// DeleteDocument removes a document. A container with children answers 409 with
// the count unless ?recursive=true is passed: unlike a comment, a document is
// someone's work, and one stray click must not take a whole subtree with it.
func (h *API) DeleteDocument(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	children, err := h.q.CountDocumentChildren(c, &doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if children > 0 && c.Query("recursive") != "true" {
		c.JSON(http.StatusConflict, gin.H{
			"error":    "document has nested documents",
			"children": children,
		})
		return
	}
	if err := h.q.DeleteDocumentSubtree(c, doc.ID); err != nil {
		fail(c, err)
		return
	}
	h.broadcast(doc.WorkspaceID, "document.deleted", gin.H{"id": doc.ID})
	c.Status(http.StatusNoContent)
}

// loadDocument fetches a document (path param :id) and authorizes via its
// workspace.
func (h *API) loadDocument(c *gin.Context) (db.Document, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.Document{}, false
	}
	doc, err := h.q.GetDocument(c, id)
	if notFound(c, err) {
		return db.Document{}, false
	}
	if err != nil {
		fail(c, err)
		return db.Document{}, false
	}
	if !h.requireMember(c, doc.WorkspaceID) {
		return db.Document{}, false
	}
	return doc, true
}

// parentInWorkspace checks the proposed parent exists in the same workspace.
// Writes the response and returns false when it does not.
func (h *API) parentInWorkspace(c *gin.Context, parentID, wsID uuid.UUID) bool {
	parent, err := h.q.GetDocument(c, parentID)
	if notFound(c, err) {
		return false
	}
	if err != nil {
		fail(c, err)
		return false
	}
	if parent.WorkspaceID != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parent belongs to another workspace"})
		return false
	}
	return true
}

// reparentAllowed rejects moving a document into itself or into its own
// subtree, which would detach the branch from the tree and leave it orphaned in
// the table. The check walks up from the proposed parent: if we meet the moved
// document on the way to the root, the parent sits below it.
func (h *API) reparentAllowed(c *gin.Context, docID, parentID uuid.UUID) bool {
	ancestors, err := h.q.DocumentAncestorIDs(c, parentID)
	if err != nil {
		fail(c, err)
		return false
	}
	for _, id := range ancestors {
		if id == docID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot move a document into its own subtree"})
			return false
		}
	}
	return true
}

// nextDocumentPosition puts a new document after its current siblings.
func (h *API) nextDocumentPosition(c *gin.Context, wsID uuid.UUID, parentID *uuid.UUID) float64 {
	docs, err := h.q.ListDocuments(c, wsID)
	if err != nil {
		return positionBetween(nil, nil)
	}
	var maxPos *float64
	for i := range docs {
		if !sameParent(docs[i].ParentID, parentID) {
			continue
		}
		if maxPos == nil || docs[i].Position > *maxPos {
			p := docs[i].Position
			maxPos = &p
		}
	}
	return positionBetween(maxPos, nil)
}

func sameParent(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// documentMeta is the realtime payload for list-level events. Content is
// deliberately absent: broadcasting a full document to every workspace
// subscriber on each save is not something the shared hub should carry.
func documentMeta(d db.Document) gin.H {
	return gin.H{
		"id":           d.ID,
		"workspace_id": d.WorkspaceID,
		"parent_id":    d.ParentID,
		"project_id":   d.ProjectID,
		"title":        d.Title,
		"slug":         d.Slug,
		"icon":         d.Icon,
		"position":     d.Position,
		"updated_at":   d.UpdatedAt,
	}
}
