package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/docroom"
	"tessera/middleware"
)

// maxDocLinkQuote caps the snippet stored with an anchored link. Same role and
// same bound as a comment's quote: it says which paragraph was linked once that
// paragraph has been rewritten, and a link row is not a place to keep a page of
// text.
const maxDocLinkQuote = 500

// ListDocumentTaskLinks returns the tasks linked to a document, each carrying
// enough of the task to render the row and navigate to it.
func (h *API) ListDocumentTaskLinks(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	rows, err := h.q.ListDocumentTaskLinks(c, doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// CreateDocumentTaskLink links a task to this document, or to one block of it.
//
// The block id is the anchor from the document's own JSON (see the migration),
// so the link keeps pointing at the right clause while the text around it is
// edited. Passing none links the document as a whole.
func (h *API) CreateDocumentTaskLink(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		TaskID  uuid.UUID `json:"task_id" binding:"required"`
		BlockID string    `json:"block_id"`
		Quote   string    `json:"quote"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// The task has to live in the document's workspace. Membership is per
	// workspace, so a cross-workspace link would render as a row nobody on either
	// side can open — and it would leak the task's title to people who cannot see
	// the task.
	taskWs, err := h.q.WorkspaceIDForTask(c, req.TaskID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if taskWs != doc.WorkspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task belongs to another workspace"})
		return
	}
	uid := middleware.CurrentUser(c)
	link, err := h.q.CreateDocumentTaskLink(c, db.CreateDocumentTaskLinkParams{
		DocumentID: doc.ID,
		TaskID:     req.TaskID,
		BlockID:    req.BlockID,
		Quote:      truncateRunes(req.Quote, maxDocLinkQuote),
		CreatedBy:  &uid,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.notifyDocLinks(doc.ID)
	c.JSON(http.StatusCreated, link)
}

// DeleteDocumentTaskLink unlinks. Open to any member rather than to the person
// who linked: a link is a statement about two objects both sides can see, not
// someone's property, and the alternative is stale links nobody is allowed to
// clear.
func (h *API) DeleteDocumentTaskLink(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	link, err := h.q.GetDocumentTaskLink(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	doc, err := h.q.GetDocument(c, link.DocumentID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, doc.WorkspaceID) {
		return
	}
	if err := h.q.DeleteDocumentTaskLink(c, link.ID); err != nil {
		fail(c, err)
		return
	}
	h.notifyDocLinks(doc.ID)
	c.Status(http.StatusNoContent)
}

// ListTaskDocumentLinks reads the same relation from the task side — what the
// task modal shows under "Документы".
func (h *API) ListTaskDocumentLinks(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, _, ok := h.loadTask(c, id); !ok {
		return
	}
	rows, err := h.q.ListTaskDocumentLinks(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// notifyDocLinks nudges everyone with the document open to refetch its links and
// protocols. Payload-free for the same reason the comments nudge is: the
// receiver re-reads, so a nudge lost to a reconnect costs a stale panel until the
// next one rather than a phantom row.
func (h *API) notifyDocLinks(docID uuid.UUID) {
	if h.docRooms == nil {
		return
	}
	h.docRooms.Notify(docID, docroom.TypeLinks)
}
