package handlers

import (
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PDF is read, not converted (#2733, D8 of #2718 — item 1 of the parent asks
// for "PDF (чтение)").
//
// Every other import format goes through the LibreOffice sidecar and comes back
// as blocks. A PDF deliberately does not: LibreOffice opens one as a page of
// absolutely positioned text frames, so the "blocks" it yields are a grid of
// fragments with no paragraphs, no headings and no reading order — worse for
// every downstream feature (annotations, versions diff, export) than keeping
// the file. So the bytes are stored as a document asset and the document body
// holds one `pdfEmbed` node pointing at them; the browser renders it with
// pdf.js.
//
// Consequences worth stating, because they are chosen rather than incidental:
//   - the import needs no sidecar, so it works on an install without one;
//   - the document is not editable *as text*, but it is a normal document in
//     every other respect — it can be renamed, nested, linked to a task (D7),
//     versioned (D6) and annotated (D5), because the block carries an id like
//     any other.

// docNativeImportExts are the formats handled without the sidecar. Reported by
// ConverterStatus so the client can offer them even when conversion is down.
var docNativeImportExts = []string{".pdf"}

// pdfMagic is what a PDF starts with. http.DetectContentType knows this
// signature, and the check is on the bytes rather than the extension or the
// declared Content-Type for the same reason UploadDocumentAsset does it: both
// of those are attacker-controlled, and the file is later served from our own
// origin.
const pdfContentType = "application/pdf"

// importPdfDocument stores an uploaded PDF and creates the document that shows
// it. The caller has already checked membership and the size cap.
//
// Order is sniff → create → store, which means a write that fails after the row
// exists leaves an empty document behind. That is the same trade the converted
// path documents and makes deliberately: the alternative is deleting a document
// the user has just seen appear.
func (h *API) importPdfDocument(c *gin.Context, wsID uuid.UUID, fileHeader *multipart.FileHeader, name string) {
	ct, err := sniffContentType(fileHeader)
	if err != nil {
		fail(c, err)
		return
	}
	if ct != pdfContentType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не является PDF"})
		return
	}

	doc, ok := h.createImportedDocument(c, wsID, name)
	if !ok {
		return
	}

	dir := filepath.Join(h.uploadDir, "documents", doc.ID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, err)
		return
	}
	stored := uuid.NewString() + ".pdf"
	if err := saveUploaded(fileHeader, filepath.Join(dir, stored)); err != nil {
		fail(c, err)
		return
	}

	h.broadcast(wsID, "document.created", documentMeta(doc))
	// The body is written by the client through the ordinary content endpoint,
	// exactly as the converted import does — so the one path that validates a
	// document body stays the one path that validates a document body.
	c.JSON(http.StatusCreated, gin.H{
		"document": viewDocument(doc),
		"pdf": gin.H{
			"src":  h.docAssetURL(doc.WorkspaceID, doc.ID, stored),
			"name": name,
			"size": fileHeader.Size,
		},
		"source_file_name": name,
	})
}

// UploadDocumentPdf attaches a PDF to an existing document, for the case the
// file is dropped into a page that already has text rather than imported as a
// document of its own.
//
// Separate from UploadDocumentAsset instead of relaxing its type check: that
// route's contract is "images only", it is what the editor's paste and drop
// handlers call, and widening it would mean every one of those paths silently
// starts accepting PDFs too.
func (h *API) UploadDocumentPdf(c *gin.Context) {
	doc, ok := h.loadDocument(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не передан"})
		return
	}
	if fileHeader.Size > maxDocImportBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "файл больше 20 МБ"})
		return
	}
	ct, err := sniffContentType(fileHeader)
	if err != nil {
		fail(c, err)
		return
	}
	if ct != pdfContentType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не является PDF"})
		return
	}
	dir := filepath.Join(h.uploadDir, "documents", doc.ID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, err)
		return
	}
	stored := uuid.NewString() + ".pdf"
	if err := saveUploaded(fileHeader, filepath.Join(dir, stored)); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"src":  h.docAssetURL(doc.WorkspaceID, doc.ID, stored),
		"name": strings.TrimSpace(filepath.Base(fileHeader.Filename)),
		"size": fileHeader.Size,
	})
}
