package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
)

// Search looks up tasks (by title) and notes (title/body) within a workspace.
func (h *API) Search(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"tasks": []any{}, "notes": []any{}})
		return
	}
	tasks, err := h.q.SearchTasks(c, db.SearchTasksParams{WorkspaceID: wsID, Column2: &q})
	if err != nil {
		fail(c)
		return
	}
	notes, err := h.q.SearchNotes(c, db.SearchNotesParams{WorkspaceID: wsID, Column2: &q})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": orEmpty(tasks), "notes": orEmpty(notes)})
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
