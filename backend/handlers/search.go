package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
)

// likeEscaper quotes the characters LIKE reserves, so a query means itself. The
// search queries splice the term into an ILIKE pattern (`'%' || $2 || '%'`),
// which makes a typed `_` stand for "any one character" and a typed `%` for "any
// run of them" — and a tracker for a codebase is exactly where people search for
// `created_at` or `task_assignees`. The backslash is in the set because it is
// LIKE's own escape character: left alone it swallows the next character, so
// `C:\build` finds `C:build` and misses the row that is actually named that.
//
// One Replacer rather than three ReplaceAll calls, and not for tidiness: it
// makes a single left-to-right pass and never re-scans what it emitted, so the
// backslashes it adds are not themselves escaped again. A sequential rewrite
// would have to do the backslash first to stay correct.
//
// Escaping here rather than in SQL is deliberate: the statement text stays
// byte-for-byte what the trigram indexes of migration 0052 were built for.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Search looks up tasks (title/description), notes (title/body) and documents
// (title) within a workspace.
func (h *API) Search(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"tasks": []any{}, "notes": []any{}, "documents": []any{}})
		return
	}
	term := likeEscaper.Replace(q)
	tasks, err := h.q.SearchTasks(c, db.SearchTasksParams{WorkspaceID: wsID, Column2: &term})
	if err != nil {
		fail(c, err)
		return
	}
	notes, err := h.q.SearchNotes(c, db.SearchNotesParams{WorkspaceID: wsID, Column2: &term})
	if err != nil {
		fail(c, err)
		return
	}
	documents, err := h.q.SearchDocuments(c, db.SearchDocumentsParams{WorkspaceID: wsID, Column2: &term})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": orEmpty(tasks), "notes": orEmpty(notes), "documents": orEmpty(documents)})
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
