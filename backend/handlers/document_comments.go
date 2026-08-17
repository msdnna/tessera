package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/docroom"
	"tessera/middleware"
)

// maxDocCommentQuote caps the annotated snippet stored with a thread. The quote
// exists to say *what* the discussion was about when the block has since been
// rewritten — a sentence does that, a pasted page does not, and the column would
// otherwise grow without any bound the UI can render.
const maxDocCommentQuote = 500

// ListDocumentComments returns every comment on a document, roots and replies
// together. Threading and grouping by block happen on the client: it already
// walks the tree to place the annotations next to their text, and it is the only
// side that knows which block ids are still in the document it has open.
func (h *API) ListDocumentComments(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	rows, err := h.q.ListDocumentComments(c, doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// CreateDocumentComment starts a thread on a block, or replies to one.
//
// The anchor is the block id from the document's own JSON, so the annotation
// keeps pointing at the right paragraph while the text around it is edited — see
// the migration for why this is not a mark inside the content.
func (h *API) CreateDocumentComment(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		Body     string     `json:"body" binding:"required"`
		BlockID  string     `json:"block_id"`
		ParentID *uuid.UUID `json:"parent_id"`
		Quote    string     `json:"quote"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	arg := db.CreateDocumentCommentParams{
		DocumentID: doc.ID,
		BlockID:    req.BlockID,
		Body:       req.Body,
		Quote:      truncateRunes(req.Quote, maxDocCommentQuote),
	}
	if req.ParentID != nil {
		parent, err := h.q.GetDocumentComment(c, *req.ParentID)
		if notFound(c, err) {
			return
		}
		if err != nil {
			fail(c, err)
			return
		}
		// A reply belongs to the thread it answers, whatever the client said: it
		// must not land on another document, and threads are one level deep, so a
		// reply to a reply joins the same root instead of nesting.
		if parent.DocumentID != doc.ID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent comment belongs to another document"})
			return
		}
		root := parent.ID
		if parent.ParentID != nil {
			root = *parent.ParentID
		}
		arg.ParentID = &root
		arg.BlockID = parent.BlockID
		arg.Quote = ""
	}
	uid := middleware.CurrentUser(c)
	arg.AuthorID = &uid
	cm, err := h.q.CreateDocumentComment(c, arg)
	if err != nil {
		fail(c, err)
		return
	}
	h.notifyDocComments(doc.ID)
	c.JSON(http.StatusCreated, cm)
}

// UpdateDocumentComment edits the body of one's own comment (same rule as task
// comments: membership lets you read and reply, not rewrite what someone said).
func (h *API) UpdateDocumentComment(c *gin.Context) {
	cm, _, ok := h.loadDocumentComment(c)
	if !ok {
		return
	}
	if cm.AuthorID == nil || *cm.AuthorID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your comment"})
		return
	}
	var req struct {
		Body string `json:"body" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateDocumentCommentBody(c, db.UpdateDocumentCommentBodyParams{
		ID: cm.ID, Body: req.Body,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.notifyDocComments(cm.DocumentID)
	c.JSON(http.StatusOK, updated)
}

// ResolveDocumentComment closes a thread, or reopens it.
//
// Unlike editing, this is open to any member: marking a remark as handled is the
// point of a review, and requiring the author to come back and do it is how
// threads accumulate forever. Only a root can be resolved — the state belongs to
// the thread, and the table's CHECK enforces the same thing one layer down.
func (h *API) ResolveDocumentComment(c *gin.Context) {
	cm, _, ok := h.loadDocumentComment(c)
	if !ok {
		return
	}
	if cm.ParentID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only a thread root can be resolved"})
		return
	}
	var req struct {
		Resolved *bool `json:"resolved"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Absent means "resolve": the button that sends nothing is the one in the
	// thread header, and reopening always passes the flag explicitly.
	arg := db.SetDocumentCommentResolvedParams{ID: cm.ID}
	if req.Resolved == nil || *req.Resolved {
		now := time.Now()
		uid := middleware.CurrentUser(c)
		arg.ResolvedAt = &now
		arg.ResolvedBy = &uid
	}
	updated, err := h.q.SetDocumentCommentResolved(c, arg)
	if err != nil {
		fail(c, err)
		return
	}
	h.notifyDocComments(cm.DocumentID)
	c.JSON(http.StatusOK, updated)
}

// DeleteDocumentComment removes one's own comment; a root takes its replies with
// it (ON DELETE CASCADE), which is why the count goes out in the response — a
// client that deletes a thread of ten needs to know it deleted ten.
func (h *API) DeleteDocumentComment(c *gin.Context) {
	cm, _, ok := h.loadDocumentComment(c)
	if !ok {
		return
	}
	if cm.AuthorID == nil || *cm.AuthorID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your comment"})
		return
	}
	if err := h.q.DeleteDocumentComment(c, cm.ID); err != nil {
		fail(c, err)
		return
	}
	h.notifyDocComments(cm.DocumentID)
	c.Status(http.StatusNoContent)
}

// loadDocumentComment fetches a comment (path param :id) and authorizes through
// the document it hangs on, which is where the workspace — and therefore the
// membership check — comes from.
func (h *API) loadDocumentComment(c *gin.Context) (db.DocumentComment, db.Document, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.DocumentComment{}, db.Document{}, false
	}
	cm, err := h.q.GetDocumentComment(c, id)
	if notFound(c, err) {
		return db.DocumentComment{}, db.Document{}, false
	}
	if err != nil {
		fail(c, err)
		return db.DocumentComment{}, db.Document{}, false
	}
	doc, err := h.q.GetDocument(c, cm.DocumentID)
	if notFound(c, err) {
		return db.DocumentComment{}, db.Document{}, false
	}
	if err != nil {
		fail(c, err)
		return db.DocumentComment{}, db.Document{}, false
	}
	if !h.requireMember(c, doc.WorkspaceID) {
		return db.DocumentComment{}, db.Document{}, false
	}
	return cm, doc, true
}

// notifyDocComments nudges everyone with this document open to refetch the
// threads.
//
// The nudge goes over the per-document socket rather than the workspace hub for
// the reason the hub exists to avoid: a comment concerns the handful of people
// reading that document, not every member of the workspace. It carries no
// payload on purpose — the receiver re-reads the list, so a nudge lost to a
// reconnect costs one stale panel until the next one, not a phantom comment.
func (h *API) notifyDocComments(docID uuid.UUID) {
	if h.docRooms == nil {
		return
	}
	h.docRooms.Notify(docID, docroom.TypeComments)
}

// truncateRunes cuts a string to at most n runes (not bytes — the quote is
// Russian text as often as not, and a byte cut would split a character).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
