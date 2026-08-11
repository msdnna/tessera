package handlers

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// maxAttachmentBytes caps a single upload (25 MiB).
const maxAttachmentBytes = 25 << 20

// maxMediaBytes caps an inline image embedded in a description/comment (8 MiB).
const maxMediaBytes = 8 << 20

// mediaExts maps allowed image content types to a file extension.
//
// SVG is deliberately absent: it is a script-bearing document, and /uploads is
// served publicly on the app's own origin, so an inline SVG would run with
// access to the session in localStorage.
var mediaExts = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/bmp":  ".bmp",
}

// inlineSafeExts is the set of extensions ServeUpload may hand out with their
// real content type. Derived from mediaExts, so dropping a type from uploads
// also stops files of that type already on disk from rendering.
var inlineSafeExts = func() map[string]bool {
	m := make(map[string]bool, len(mediaExts))
	for _, ext := range mediaExts {
		m[ext] = true
	}
	return m
}()

// mediaNameRe guards the public serve route against path traversal.
var mediaNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+\.[a-z0-9]+$`)

// UploadMedia stores an inline image and returns a public URL the editor embeds
// as Markdown. Unlike task attachments, these are served without auth (images
// can't send the bearer header) — unguessable by their UUID filename.
func (h *API) UploadMedia(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не передан"})
		return
	}
	if fileHeader.Size > maxMediaBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "изображение больше 8 МБ"})
		return
	}
	// Gate on the bytes, not on the declared Content-Type or the filename —
	// both are attacker-controlled, so either would let HTML through as .png.
	// The stored extension is derived from the sniff for the same reason.
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

	dir := filepath.Join(h.uploadDir, "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, err)
		return
	}
	name := uuid.NewString() + ext
	if err := saveUploaded(fileHeader, filepath.Join(dir, name)); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"url": "/api/uploads/" + name})
}

// ServeUpload serves an inline image by name. Who may ask is decided upstream by
// middleware.MediaAuth (media cookie or bearer token, and — unless
// MEDIA_REQUIRE_AUTH is on — anyone holding the UUID filename); see UploadMedia.
func (h *API) ServeUpload(c *gin.Context) {
	name := filepath.Base(c.Param("name"))
	if !mediaNameRe.MatchString(name) {
		c.Status(http.StatusNotFound)
		return
	}
	p := filepath.Join(h.uploadDir, "media", name)
	if _, err := os.Stat(p); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// private, not public: the name is the only thing guarding the file, so it
	// must not settle in shared proxy/CDN caches. immutable and the year stay —
	// the name is a UUID, so the bytes behind it never change.
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	// This route is public and same-origin, so anything it renders runs with the
	// app's session. nosniff pins the declared type; the CSP neuters any active
	// content that still slips through (both are inert for real images).
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	if !inlineSafeExts[strings.ToLower(filepath.Ext(name))] {
		// Legacy files from when SVG was accepted: hand them out as opaque
		// downloads instead of letting the browser execute them.
		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Disposition", "attachment")
	}
	c.File(p)
}

// ListAttachments returns the attachments on a task.
func (h *API) ListAttachments(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, _, ok := h.loadTask(c, id); !ok {
		return
	}
	items, err := h.q.ListTaskAttachments(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(items))
}

// UploadAttachment stores a multipart file on disk and records it.
func (h *API) UploadAttachment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не передан"})
		return
	}
	if fileHeader.Size > maxAttachmentBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "файл больше 25 МБ"})
		return
	}

	// Store under <uploadDir>/<taskID>/<uuid><ext> so paths never collide and
	// removing a task's files is a single directory.
	dir := filepath.Join(h.uploadDir, id.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, err)
		return
	}
	ext := filepath.Ext(fileHeader.Filename)
	storagePath := filepath.Join(dir, uuid.NewString()+ext)
	if err := saveUploaded(fileHeader, storagePath); err != nil {
		fail(c, err)
		return
	}

	uid := middleware.CurrentUser(c)
	att, err := h.q.CreateAttachment(c, db.CreateAttachmentParams{
		TaskID:      id,
		UploaderID:  &uid,
		Filename:    filepath.Base(fileHeader.Filename),
		ContentType: fileHeader.Header.Get("Content-Type"),
		Size:        fileHeader.Size,
		StoragePath: storagePath,
	})
	if err != nil {
		_ = os.Remove(storagePath)
		fail(c, err)
		return
	}
	h.logEvent(c, id, "attachment", map[string]any{"filename": att.Filename})
	h.broadcast(wsID, "task.updated", gin.H{"id": id})
	_ = t
	c.JSON(http.StatusCreated, att)
}

// DownloadAttachment streams an attachment's stored file to the client.
func (h *API) DownloadAttachment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	att, err := h.q.GetAttachment(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if _, _, ok := h.loadTask(c, att.TaskID); !ok {
		return
	}
	if _, err := os.Stat(att.StoragePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "файл недоступен"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
	// Attachments are arbitrary user files; without nosniff a browser may still
	// sniff one into HTML and render it on our origin.
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(att.StoragePath)
}

// DeleteAttachment removes an attachment row and best-effort deletes its file.
func (h *API) DeleteAttachment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	att, err := h.q.GetAttachment(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if _, _, ok := h.loadTask(c, att.TaskID); !ok {
		return
	}
	if err := h.q.DeleteAttachment(c, id); err != nil {
		fail(c, err)
		return
	}
	_ = os.Remove(att.StoragePath) // best-effort; the row is already gone
	c.Status(http.StatusNoContent)
}

// sniffContentType reports the type a browser would infer from an upload's
// leading bytes, which is what decides whether it renders as active content.
func sniffContentType(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
}

// saveUploaded streams a multipart file to disk.
func saveUploaded(fh *multipart.FileHeader, dst string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, src)
	return err
}
