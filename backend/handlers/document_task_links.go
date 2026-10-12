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

// maxDocLinkBlockID bounds the anchor. The ids the editor draws are sixteen hex
// characters (newBlockId in docExtensions/blockId.js), with a `-<n>` suffix in the
// degraded-randomness branch, so this leaves room to spare for a value that is
// not free text at all.
//
// Refused rather than truncated, unlike the quote next to it: the quote is a
// label and a shortened one still reads, while the anchor is an identity —
// trimming it would store a link pointing at a block that does not exist and
// report success, which is the one outcome worse than saying no.
const maxDocLinkBlockID = 64

// maxDocTaskLinks caps how many tasks one document may name, for the reason
// maxBoardViews caps a board's saved layouts: nothing else bounded this table.
// The pair is an upsert, so re-linking the same task to the same block only
// refreshes the quote — but block_id is the third column of that key and it
// arrives from the client, so a *fresh* id always inserts, and there is no
// retention pass anywhere. A member could therefore keep POSTing distinct
// anchors, each row carrying up to maxDocLinkQuote of text, and grow the database
// until the disk ran out (#2932).
//
// Generously above anything a document does by hand — a spec that names five
// hundred separate tasks is not a document anybody reads — because the bound is
// here to stop unbounded growth, not to ration links.
const maxDocTaskLinks = 500

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
	if len([]rune(req.BlockID)) > maxDocLinkBlockID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "block id is too long"})
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
	// Counted before the upsert, and only a call that would add a row is refused.
	// A document sitting at the cap has to keep being able to refresh the links it
	// already has — the panel re-links to update a quote after the paragraph was
	// edited, and a cap that froze those would break the feature instead of
	// bounding the table.
	if ok := h.docLinkRoomLeft(c, doc.ID, req.TaskID, req.BlockID); !ok {
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

// docLinkRoomLeft reports whether this document may take the asked-for link,
// writing the refusal itself when it may not. Separate from the handler because
// it is three statements and one rule, and the handler above already reads as a
// list of the checks a link has to pass.
//
// A read that fails is not a refusal: the cap is a guard against a member
// hammering the endpoint, and letting a hiccup in count(*) block an ordinary link
// would trade a bound nobody reaches for a button that sometimes does not work.
func (h *API) docLinkRoomLeft(c *gin.Context, docID, taskID uuid.UUID, blockID string) bool {
	count, err := h.q.CountDocumentTaskLinks(c, docID)
	if err != nil || count < maxDocTaskLinks {
		return true
	}
	exists, err := h.q.DocumentTaskLinkExists(c, db.DocumentTaskLinkExistsParams{
		DocumentID: docID, TaskID: taskID, BlockID: blockID,
	})
	if err != nil || exists {
		return true
	}
	c.JSON(http.StatusConflict, gin.H{"error": "link limit reached"})
	return false
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
