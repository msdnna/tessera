package handlers

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/middleware"
)

// maxAttachmentBytes caps a single upload (25 MiB).
const maxAttachmentBytes = 25 << 20

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
		fail(c)
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
		fail(c)
		return
	}
	ext := filepath.Ext(fileHeader.Filename)
	storagePath := filepath.Join(dir, uuid.NewString()+ext)
	if err := saveUploaded(fileHeader, storagePath); err != nil {
		fail(c)
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
		fail(c)
		return
	}
	h.logEvent(c, id, "attachment", map[string]any{"filename": att.Filename})
	h.broadcast(wsID, "task.updated", gin.H{"id": id})
	_ = t
	c.JSON(http.StatusCreated, att)
}

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
		fail(c)
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
	c.File(att.StoragePath)
}

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
		fail(c)
		return
	}
	if _, _, ok := h.loadTask(c, att.TaskID); !ok {
		return
	}
	if err := h.q.DeleteAttachment(c, id); err != nil {
		fail(c)
		return
	}
	_ = os.Remove(att.StoragePath) // best-effort; the row is already gone
	c.Status(http.StatusNoContent)
}

// saveUploaded streams a multipart file to disk.
func saveUploaded(fh *multipart.FileHeader, dst string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}
