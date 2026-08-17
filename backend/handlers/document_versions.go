package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/docroom"
	"tessera/middleware"
)

const (
	// docVersionWindow is how long one editing session keeps writing into the
	// same version. Autosave (D2) fires every few seconds, so without coalescing
	// an afternoon of typing would leave hundreds of near-identical snapshots and
	// a journal nobody can read. The window is measured from the session's start,
	// not from its last edit: an idle-based rule lets continuous typing hold one
	// version open indefinitely, which is exactly the case where intermediate
	// restore points are wanted most.
	docVersionWindow = 10 * time.Minute

	// docVersionKeep caps a document's automatic history. Manual snapshots do not
	// count against it and are never pruned — see the query.
	docVersionKeep = 50

	// maxDocVersionLabel bounds the free-text name of a manual snapshot. The
	// journal renders it on one line; a pasted page there is a broken panel.
	maxDocVersionLabel = 120
)

// documentVersionView is a version whose content ships as JSON rather than the
// []byte sqlc scans jsonb into (encoding/json would base64 it), mirroring
// documentView.
type documentVersionView struct {
	db.DocumentVersion
	Content json.RawMessage `json:"content"`
}

func viewDocumentVersion(v db.DocumentVersion) documentVersionView {
	content := json.RawMessage(v.Content)
	if len(content) == 0 {
		content = json.RawMessage(`{"type":"doc","content":[]}`)
	}
	return documentVersionView{DocumentVersion: v, Content: content}
}

// ListDocumentVersions returns the document's journal, newest first and without
// bodies — see the query for why the content stays behind.
func (h *API) ListDocumentVersions(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	rows, err := h.q.ListDocumentVersions(c, doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// CreateDocumentVersion takes a named snapshot of the document as it stands.
//
// The automatic history already covers "what did this look like an hour ago";
// this endpoint exists for the other question — "keep the state we agreed on".
// Hence manual = true: retention never prunes it, and the next autosave starts a
// fresh session instead of typing over the milestone.
func (h *API) CreateDocumentVersion(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	var req struct {
		Label string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	v, err := h.q.CreateDocumentVersion(c, db.CreateDocumentVersionParams{
		DocumentID: doc.ID,
		AuthorID:   &uid,
		Title:      doc.Title,
		Content:    doc.Content,
		Preview:    doc.Preview,
		Label:      truncateRunes(req.Label, maxDocVersionLabel),
		Manual:     true,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, viewDocumentVersion(v))
}

// GetDocumentVersion returns one version with its content — what the journal
// asks for when a version is previewed or compared.
func (h *API) GetDocumentVersion(c *gin.Context) {
	v, _, ok := h.loadDocumentVersion(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, viewDocumentVersion(v))
}

// RestoreDocumentVersion rolls the document back to a version.
//
// Two things happen before the write, and both are the point of the endpoint
// rather than caution: the state being replaced is snapshotted (a rollback the
// user cannot undo is a delete with extra steps), and the restored state is
// written as a new version instead of rewinding the counter — the journal is a
// log of what happened to the document, and "reverted to 3" is one of the things
// that happened.
func (h *API) RestoreDocumentVersion(c *gin.Context) {
	src, doc, ok := h.loadDocumentVersion(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)
	// The current state is normally already the newest version (every content
	// save writes one), so snapshot it only when it is not — otherwise every
	// rollback would leave a duplicate pair in the journal.
	if err := h.snapshotCurrentIfUnsaved(c, doc, uid); err != nil {
		fail(c, err)
		return
	}
	updated, err := h.q.SetDocumentContent(c, db.SetDocumentContentParams{
		ID:      doc.ID,
		Content: src.Content,
		Preview: src.Preview,
	})
	if err != nil {
		fail(c, err)
		return
	}
	if _, err := h.q.CreateDocumentVersion(c, db.CreateDocumentVersionParams{
		DocumentID: doc.ID,
		AuthorID:   &uid,
		Title:      updated.Title,
		Content:    updated.Content,
		Preview:    updated.Preview,
		Label:      fmt.Sprintf("Откат к версии %d", src.Revision),
		Manual:     true,
	}); err != nil {
		fail(c, err)
		return
	}
	h.broadcast(doc.WorkspaceID, "document.updated", documentMeta(updated))
	// Everyone with the document open is holding the pre-rollback tree; nudging
	// the room is what turns a rollback into something they see rather than
	// something they overwrite with their next keystroke.
	h.notifyDocContent(doc.ID)
	c.JSON(http.StatusOK, viewDocument(updated))
}

// snapshotDocument records a content save in the journal.
//
// It is called after the write succeeded, so the newest version always holds
// what the document holds — the property the compare and restore paths rely on.
// Failures are reported to the caller but must not fail the save itself: losing
// a journal entry is a worse outcome only than losing the user's text.
func (h *API) snapshotDocument(c *gin.Context, before, after db.Document, uid uuid.UUID) error {
	latest, err := h.q.LatestDocumentVersion(c, after.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// First tracked save. Record what the document looked like *before* it,
		// or the state the user is about to leave would be the one state the
		// journal never held. A document that was still empty has nothing worth
		// keeping, so it gets no baseline.
		if !isEmptyDocContent(before.Content) {
			if _, err := h.q.CreateDocumentVersion(c, db.CreateDocumentVersionParams{
				DocumentID: before.ID,
				AuthorID:   before.AuthorID,
				Title:      before.Title,
				Content:    before.Content,
				Preview:    before.Preview,
				Label:      "Исходная версия",
			}); err != nil {
				return err
			}
		}
	case err != nil:
		return err
	default:
		if canExtendDocVersion(latest, uid) {
			_, err := h.q.ExtendDocumentVersion(c, db.ExtendDocumentVersionParams{
				ID:      latest.ID,
				Content: after.Content,
				Preview: after.Preview,
				Title:   after.Title,
			})
			return err
		}
	}
	if _, err := h.q.CreateDocumentVersion(c, db.CreateDocumentVersionParams{
		DocumentID: after.ID,
		AuthorID:   &uid,
		Title:      after.Title,
		Content:    after.Content,
		Preview:    after.Preview,
	}); err != nil {
		return err
	}
	return h.q.PruneDocumentVersions(c, db.PruneDocumentVersionsParams{
		DocumentID: after.ID,
		Keep:       docVersionKeep,
	})
}

// canExtendDocVersion reports whether this save belongs to the session that
// opened the newest version.
//
// Three conditions, each of which is a place the journal would otherwise lie: a
// manual snapshot is a milestone and never absorbs later edits; a different
// author means the entry would be attributed to whoever started, not to whoever
// wrote; and past the window the session is simply over.
func canExtendDocVersion(v db.DocumentVersion, uid uuid.UUID) bool {
	if v.Manual || v.AuthorID == nil || *v.AuthorID != uid {
		return false
	}
	return time.Since(v.CreatedAt) < docVersionWindow
}

// snapshotCurrentIfUnsaved preserves the document's present content before a
// rollback overwrites it, unless the newest version already holds exactly that.
func (h *API) snapshotCurrentIfUnsaved(c *gin.Context, doc db.Document, uid uuid.UUID) error {
	latest, err := h.q.LatestDocumentVersion(c, doc.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// Both sides were written by validateDocContent, so equal documents are equal
	// bytes; a false "differs" costs one extra journal entry, never a lost state.
	if err == nil && bytes.Equal(latest.Content, doc.Content) {
		return nil
	}
	_, err = h.q.CreateDocumentVersion(c, db.CreateDocumentVersionParams{
		DocumentID: doc.ID,
		AuthorID:   &uid,
		Title:      doc.Title,
		Content:    doc.Content,
		Preview:    doc.Preview,
		Label:      "Перед откатом",
		Manual:     true,
	})
	return err
}

// notifyDocContent tells everyone with the document open that its body was
// replaced from outside the room, so they reload instead of typing on top of a
// tree the server no longer has.
func (h *API) notifyDocContent(docID uuid.UUID) {
	if h.docRooms == nil {
		return
	}
	h.docRooms.Notify(docID, docroom.TypeContent)
}

// loadDocumentVersion fetches a version (path param :id) and authorizes through
// the document it belongs to — versions have no workspace of their own.
func (h *API) loadDocumentVersion(c *gin.Context) (db.DocumentVersion, db.Document, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.DocumentVersion{}, db.Document{}, false
	}
	v, err := h.q.GetDocumentVersion(c, id)
	if notFound(c, err) {
		return db.DocumentVersion{}, db.Document{}, false
	}
	if err != nil {
		fail(c, err)
		return db.DocumentVersion{}, db.Document{}, false
	}
	doc, err := h.q.GetDocument(c, v.DocumentID)
	if notFound(c, err) {
		return db.DocumentVersion{}, db.Document{}, false
	}
	if err != nil {
		fail(c, err)
		return db.DocumentVersion{}, db.Document{}, false
	}
	if !h.requireMember(c, doc.WorkspaceID) {
		return db.DocumentVersion{}, db.Document{}, false
	}
	return v, doc, true
}
