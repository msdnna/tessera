package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// A document's stage (#3044) and its meeting (#3046) — the two chips of the
// document head in the v2 mockup (DocsPage «Спринт 42», TemplatesWin «Повестка
// встречи — с привязкой к встрече»).
//
// Both are a single nullable column on the document rather than a link table:
// a document has one stage the way a task does, and one meeting it is the
// agenda (or the protocol) of. Both ends must sit in the document's workspace —
// a pin across the boundary would show one team the title of another's stage or
// meeting — and TransferProject unpins what a move carries across it.
//
// The chips are drawn from the lists the client already loads for the pickers
// (GET /workspaces/:id/milestones, GET /workspaces/:id/conferences), so the
// document itself only carries the two ids.

// milestoneInWorkspace checks a stage exists and belongs to a project of wsID.
// Writes the response and returns false otherwise — 400 for another workspace,
// the same answer parentInWorkspace gives, so the call does not confirm that a
// stage the caller cannot see exists.
func (h *API) milestoneInWorkspace(c *gin.Context, id, wsID uuid.UUID) bool {
	ref, err := h.q.DocumentMilestoneRef(c, id)
	if notFound(c, err) {
		return false
	}
	if err != nil {
		fail(c, err)
		return false
	}
	if ref.WorkspaceID != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stage belongs to another workspace"})
		return false
	}
	return true
}

// conferenceInWorkspace is milestoneInWorkspace for a meeting.
func (h *API) conferenceInWorkspace(c *gin.Context, id, wsID uuid.UUID) bool {
	conf, err := h.q.GetConference(c, id)
	if notFound(c, err) {
		return false
	}
	if err != nil {
		fail(c, err)
		return false
	}
	if conf.WorkspaceID != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "meeting belongs to another workspace"})
		return false
	}
	return true
}

// SetDocumentMilestone pins the document to a stage, or unpins it with
// {"milestone_id": null}. PUT /documents/:id/milestone.
func (h *API) SetDocumentMilestone(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		MilestoneID *uuid.UUID `json:"milestone_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MilestoneID != nil && !h.milestoneInWorkspace(c, *req.MilestoneID, doc.WorkspaceID) {
		return
	}
	updated, err := h.q.SetDocumentMilestone(c, db.SetDocumentMilestoneParams{
		ID: doc.ID, MilestoneID: req.MilestoneID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, doc.WorkspaceID, "document.updated", documentMeta(updated))
	c.JSON(http.StatusOK, viewDocument(updated))
}

// SetDocumentConference makes the document the agenda of a meeting, or drops the
// link with {"conference_id": null}. PUT /documents/:id/conference.
func (h *API) SetDocumentConference(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		ConferenceID *uuid.UUID `json:"conference_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ConferenceID != nil && !h.conferenceInWorkspace(c, *req.ConferenceID, doc.WorkspaceID) {
		return
	}
	updated, err := h.q.SetDocumentConference(c, db.SetDocumentConferenceParams{
		ID: doc.ID, ConferenceID: req.ConferenceID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, doc.WorkspaceID, "document.updated", documentMeta(updated))
	c.JSON(http.StatusOK, viewDocument(updated))
}

// ListMilestoneDocuments — the documents pinned to a stage, for the stage page.
// GET /milestones/:id/documents.
func (h *API) ListMilestoneDocuments(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	m, wsID, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	rows, err := h.q.ListMilestoneDocuments(c, db.ListMilestoneDocumentsParams{
		MilestoneID: &m.ID, WorkspaceID: wsID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// ListConferenceDocuments — the documents pinned to the workspace's meetings, in
// one read for the meetings list. GET /workspaces/:id/conference-documents.
func (h *API) ListConferenceDocuments(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListWorkspaceConferenceDocuments(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}
