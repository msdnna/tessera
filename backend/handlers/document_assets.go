package handlers

import (
	"context"
	"crypto/hmac"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Images inside a document do not go to /api/uploads. That route is public by
// design (an <img> cannot send a bearer token) and guarded only by an
// unguessable filename — acceptable for an avatar or a screenshot pasted into a
// comment, not for the contents of a document, which #2718 explicitly calls out:
// document content must not leak via a guessable link.
//
// So document images live under their document and are served through an
// HMAC-signed URL, the same capability model as the GitLab asset proxy: the
// signature covers (workspace, path), only Tessera can mint one, and the file
// is only reachable through a link the server produced for a member.

// docAssetName guards the serve route against path traversal.
var docAssetNameRe = mediaNameRe

// docAssetDir is where one document's images live. Named by the document alone:
// there is no row per image, so this directory *is* the bookkeeping, and it is
// only ever reachable through the src attributes inside a body.
func (h *API) docAssetDir(docID uuid.UUID) string {
	return filepath.Join(h.uploadDir, "documents", docID.String())
}

// docAssetPath is the on-disk location of a document image.
func (h *API) docAssetPath(docID uuid.UUID, name string) string {
	return filepath.Join(h.docAssetDir(docID), name)
}

// docAssetRel is the signed payload: stable, and specific enough that a
// signature for one document cannot be replayed for another.
func docAssetRel(docID uuid.UUID, name string) string {
	return "/documents/" + docID.String() + "/" + name
}

func (h *API) docAssetURL(wsID, docID uuid.UUID, name string) string {
	rel := docAssetRel(docID, name)
	return "/api/documents/asset?doc=" + docID.String() +
		"&ws=" + wsID.String() +
		"&n=" + name +
		"&sig=" + h.signAsset(wsID, rel)
}

// purgeDocAssets clears the image directories of documents that have just been
// deleted — every one of them whose images nothing else still embeds.
//
// The check is what separates this from the conference sweeps, which own their
// files outright. A template is a verbatim copy of a document's body, signed
// image URLs and all, and so is a document created from that template, so the
// source's directory can be the backing store for a whole family of bodies. A
// bare RemoveAll would leave those rendering broken images — a loud, permanent
// failure, and the wrong trade against a leak.
//
// Best-effort by design: the rows are gone either way, and a delete that
// answered 500 because a file was already missing would be worse than a
// directory left behind.
func (h *API) purgeDocAssets(ctx context.Context, ids []uuid.UUID) {
	for _, id := range ids {
		refs, err := h.q.CountDocumentAssetRefs(ctx, "%doc="+id.String()+"%")
		if err != nil || refs > 0 {
			continue
		}
		_ = os.RemoveAll(h.docAssetDir(id))
	}
}

// UploadDocumentAsset stores an image for a document and returns the signed URL
// the editor embeds.
func (h *API) UploadDocumentAsset(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не передан"})
		return
	}
	if fileHeader.Size > maxMediaBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "файл слишком большой"})
		return
	}
	// Gate on the bytes, not on the declared Content-Type or the filename — both
	// are attacker-controlled, so either would let HTML through as .png. Same
	// rule as UploadMedia; the signed URL does not make it safe to relax, since
	// the file is still served from our own origin.
	ct, err := sniffContentType(fileHeader)
	if err != nil {
		fail(c, err)
		return
	}
	ext, ok := mediaExts[ct]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "поддерживаются только изображения (PNG, JPEG, GIF, WebP, BMP)"})
		return
	}
	dir := h.docAssetDir(doc.ID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fail(c, err)
		return
	}
	name := uuid.NewString() + ext
	if err := saveUploaded(fileHeader, filepath.Join(dir, name)); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"url": h.docAssetURL(doc.WorkspaceID, doc.ID, name)})
}

// DocumentAsset serves a document image against a valid signature. Public,
// because an <img> cannot authenticate — the signature is the capability.
func (h *API) DocumentAsset(c *gin.Context) {
	docID, err := uuid.Parse(c.Query("doc"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	wsID, err := uuid.Parse(c.Query("ws"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	name := filepath.Base(c.Query("n"))
	if !docAssetNameRe.MatchString(name) {
		c.Status(http.StatusNotFound)
		return
	}
	want := h.signAsset(wsID, docAssetRel(docID, name))
	if !hmac.Equal([]byte(c.Query("sig")), []byte(want)) {
		c.Status(http.StatusForbidden)
		return
	}
	p := h.docAssetPath(docID, name)
	if _, err := os.Stat(p); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// Same hardening as ServeUpload: the file is rendered on our own origin, so
	// the declared type is pinned and active content is neutered.
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	if !inlineSafeExts[strings.ToLower(filepath.Ext(name))] {
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Disposition", "attachment")
	}
	c.File(p)
}
