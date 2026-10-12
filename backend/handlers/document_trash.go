package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
	"tessera/middleware"
)

// The documents' trash (migration 0086, #3006).
//
// DELETE /documents/:id is a soft delete: the document and the documents nested
// below it (?recursive=true, as before) get one deleted_at stamp and drop out of
// the tree, search, task links and their live rooms. «Корзина» in the documents
// tree lists them (GET /workspaces/:id/documents/trash); POST …/restore brings
// one back, DELETE …/purge («Удалить навсегда») or trash_purge after
// taskTrashRetention removes it for good together with its images. Links to
// tasks survive the soft delete untouched — they are hidden with the document
// and come back with it.

// ListDeletedDocuments is the trash list.
func (h *API) ListDeletedDocuments(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	docs, err := h.q.ListDeletedDocuments(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(docs))
}

// loadDeletedDocument resolves a document in the trash and authorizes the
// caller against its workspace; a live one is a 404 here.
func (h *API) loadDeletedDocument(c *gin.Context) (db.Document, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.Document{}, false
	}
	doc, err := h.q.GetDocumentAny(c, id)
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
	if doc.DeletedAt == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "документ не в корзине"})
		return db.Document{}, false
	}
	return doc, true
}

// softDeleteDocument is DeleteDocument's write: the subtree goes to the trash
// and its live rooms are closed (a participant typing into a document nobody
// can open any more would be writing into the void, #2729).
func (h *API) softDeleteDocument(c *gin.Context, doc db.Document) bool {
	uid := middleware.CurrentUser(c)
	taken, err := h.q.SoftDeleteDocumentSubtree(c, db.SoftDeleteDocumentSubtreeParams{ID: doc.ID, DeletedBy: &uid})
	if err != nil {
		fail(c, err)
		return false
	}
	h.dropDocRooms(taken)
	return true
}

// RestoreDeletedDocument takes a document out of the trash
// (POST /documents/:id/restore). If its parent is still in the trash it comes
// back at the top level of the tree.
func (h *API) RestoreDeletedDocument(c *gin.Context) {
	doc, ok := h.loadDeletedDocument(c)
	if !ok {
		return
	}
	detach := false
	if doc.ParentID != nil {
		p, err := h.q.GetDocumentAny(c, *doc.ParentID)
		detach = err != nil || p.DeletedAt != nil
	}
	if _, err := h.q.RestoreDocumentSubtree(c, db.RestoreDocumentSubtreeParams{ID: doc.ID, Detach: detach}); err != nil {
		fail(c, err)
		return
	}
	restored, err := h.q.GetDocument(c, doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	// The tree re-reads its list on any document event it cannot place.
	h.broadcastAs(c, doc.WorkspaceID, "document.restored", gin.H{"id": doc.ID})
	c.JSON(http.StatusOK, viewDocument(restored))
}

// PurgeDeletedDocument removes a document from the trash for good
// (DELETE /documents/:id/purge). Only a document already in the trash can be
// purged, so no single gesture loses a page.
func (h *API) PurgeDeletedDocument(c *gin.Context) {
	doc, ok := h.loadDeletedDocument(c)
	if !ok {
		return
	}
	removed, err := h.q.DeleteDocumentSubtree(c, doc.ID)
	if err != nil {
		fail(c, err)
		return
	}
	h.purgeDocAssets(c, removed)
	h.broadcastAs(c, doc.WorkspaceID, "document.purged", gin.H{"id": doc.ID})
	c.Status(http.StatusNoContent)
}

// purgeDeletedDocuments is trash_purge's pass over the documents' trash.
func (h *API) purgeDeletedDocuments(ctx context.Context, cutoff time.Time) {
	removed, err := h.q.PurgeDeletedDocuments(ctx, cutoff)
	if err != nil {
		slog.Warn("trash purge: documents", "err", err)
		return
	}
	h.purgeDocAssets(ctx, removed)
}
