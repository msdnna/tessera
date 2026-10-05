package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
)

// Search looks up tasks (title/description/number), boards (own or project
// name), notes (title/body) and documents (title) within a workspace — the
// header search and the ⌘K palette both read it.
func (h *API) Search(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"tasks": []any{}, "boards": []any{}, "notes": []any{}, "documents": []any{}})
		return
	}
	tasks, err := h.q.SearchTasks(c, db.SearchTasksParams{WorkspaceID: wsID, Q: q})
	if err != nil {
		fail(c, err)
		return
	}
	boards, err := h.q.SearchBoards(c, db.SearchBoardsParams{WorkspaceID: wsID, Q: q})
	if err != nil {
		fail(c, err)
		return
	}
	notes, err := h.q.SearchNotes(c, db.SearchNotesParams{WorkspaceID: wsID, Column2: &q})
	if err != nil {
		fail(c, err)
		return
	}
	documents, err := h.q.SearchDocuments(c, db.SearchDocumentsParams{WorkspaceID: wsID, Column2: &q})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tasks":     orEmpty(tasks),
		"boards":    orEmpty(boards),
		"notes":     orEmpty(notes),
		"documents": orEmpty(documents),
	})
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
