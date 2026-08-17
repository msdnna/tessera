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
		Title      string     `json:"title" binding:"required"`
		Icon       string     `json:"icon"`
		ParentID   *uuid.UUID `json:"parent_id"`
		ProjectID  *uuid.UUID `json:"project_id"`
		TemplateID *uuid.UUID `json:"template_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ParentID != nil && !h.parentInWorkspace(c, *req.ParentID, wsID) {
		return
	}
	seed, ok := h.seedFromTemplate(c, wsID, req.TemplateID)
	if !ok {
		return
	}
	// An icon passed explicitly wins; otherwise the template's own icon carries
	// over, so a document made from "Протокол совещания" looks like one in the grid.
	if req.Icon == "" {
		req.Icon = seed.icon
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
		Content:     seed.content,
		Preview:     seed.preview,
	})
	if err != nil {
		fail(c, err)
		return
	}
	view := viewDocument(doc)
	h.broadcast(wsID, "document.created", documentMeta(doc))
	c.JSON(http.StatusCreated, view)
}

// docSeed is the body a new document starts life with: empty, or a copy of a
// template (D9).
type docSeed struct {
	content []byte
	preview string
	icon    string
}

// seedFromTemplate resolves the optional template_id of a create request.
//
// The template's body is copied, not referenced: a document made from a
// template is thereafter its own document, and later edits to the template must
// not rewrite documents somebody already filled in. Writes the response and
// returns false when the template is missing or belongs elsewhere.
func (h *API) seedFromTemplate(c *gin.Context, wsID uuid.UUID, templateID *uuid.UUID) (docSeed, bool) {
	empty := docSeed{content: []byte(`{"type":"doc","content":[]}`)}
	if templateID == nil {
		return empty, true
	}
	tpl, err := h.q.GetDocumentTemplate(c, *templateID)
	if notFound(c, err) {
		return empty, false
	}
	if err != nil {
		fail(c, err)
		return empty, false
	}
	if tpl.WorkspaceID != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template belongs to another workspace"})
		return empty, false
	}
	if len(tpl.Content) == 0 {
		return empty, true
	}
	return docSeed{content: tpl.Content, preview: tpl.Preview, icon: tpl.Icon}, true
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
	// Collect the subtree *before* deleting it: afterwards the rows are gone and
	// there is nothing left to walk, while a nested document's participants would
	// go on typing into a row that no longer exists (#2729).
	doomed := h.documentSubtreeIDs(c, doc)
	if err := h.q.DeleteDocumentSubtree(c, doc.ID); err != nil {
		fail(c, err)
		return
	}
	if h.docRooms != nil {
		for _, id := range doomed {
			h.docRooms.Drop(id)
		}
	}
	h.broadcast(doc.WorkspaceID, "document.deleted", gin.H{"id": doc.ID})
	c.Status(http.StatusNoContent)
}

// documentSubtreeIDs returns the document and every document nested below it.
//
// The delete itself uses a recursive CTE; this walks the workspace's flat list
// in Go instead, because the ids are wanted for the live rooms (#2729) rather
// than for the statement, and a second SQL round trip on a rare, already
// multi-query path buys nothing. A read error degrades to "just this document":
// the delete must not fail because a socket bookkeeping list came up short.
func (h *API) documentSubtreeIDs(c *gin.Context, doc db.Document) []uuid.UUID {
	ids := []uuid.UUID{doc.ID}
	docs, err := h.q.ListDocuments(c, doc.WorkspaceID)
	if err != nil {
		return ids
	}
	// Breadth-first over the parent links; the tree is a tree (reparentAllowed
	// rejects cycles), so this terminates.
	for frontier := ids; len(frontier) > 0; {
		var next []uuid.UUID
		for i := range docs {
			if docs[i].ParentID == nil {
				continue
			}
			for _, parent := range frontier {
				if *docs[i].ParentID == parent {
					next = append(next, docs[i].ID)
					break
				}
			}
		}
		ids = append(ids, next...)
		frontier = next
	}
	return ids
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
