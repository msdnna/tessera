package handlers

import (
	"net/http"
	"strconv"
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

// queryNumber reads a task number out of a search term: `2932` and `#2932` both
// mean the task numbered 2932, which is how the number is written on a card, in a
// URL (`?task=2932`) and in the board's own search box. Anything else — a term
// with letters in it, a bare `#`, a number too large to be one — is 0, the value
// SearchTasks treats as "no number was typed" (numbers start at 1).
//
// Only the whole term counts. `2932` inside `отчёт 2932` is a word in a sentence,
// not an address, and the text match already covers it; treating it as a number
// would pull an unrelated task to the top of every such search.
func queryNumber(q string) int64 {
	n, err := strconv.ParseInt(strings.TrimPrefix(q, "#"), 10, 64)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// searchLimit is how many hits of each kind one search answers with. The
// queries ask for one more than this (db/queries/search.sql) and page() throws
// the extra away, so the two constants have to stay in step.
//
// The number is a dropdown's worth, not a judgement about how much matched: the
// endpoint fires from a debounced input on every keystroke, and a term like "а"
// matches most of a real tracker. What the cap must not do is pass itself off as
// the whole answer, which is what has_more is for (#2932).
const searchLimit = 25

// page trims a query's probe row and reports whether it was there. A result set
// that came back short is complete by definition — the database had nothing
// further to give — so only a full-plus-one page counts as truncated.
func page[T any](rows []T) ([]T, bool) {
	if len(rows) > searchLimit {
		return rows[:searchLimit], true
	}
	return orEmpty(rows), false
}

// Search looks up tasks (title/description/number), notes (title/body) and
// documents (title) within a workspace, up to searchLimit of each.
//
// The response carries has_more per kind rather than a total. The clients use it
// to say "the first 25 — narrow the search", which is the honest thing to show
// and the thing that actually helps: a count of 412 tells the reader nothing
// they can act on, and costs a full scan to produce.
func (h *API) Search(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		// The empty search is not a truncated one. Spelling the flags out here
		// rather than omitting the map keeps the shape identical to every other
		// answer, so a client can read it without a special case.
		c.JSON(http.StatusOK, gin.H{
			"tasks": []any{}, "notes": []any{}, "documents": []any{},
			"has_more": gin.H{"tasks": false, "notes": false, "documents": false},
		})
		return
	}
	term := likeEscaper.Replace(q)
	tasks, err := h.q.SearchTasks(c, db.SearchTasksParams{
		WorkspaceID: wsID,
		Term:        &term,
		Number:      queryNumber(q),
	})
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
	taskPage, taskMore := page(tasks)
	notePage, noteMore := page(notes)
	docPage, docMore := page(documents)
	c.JSON(http.StatusOK, gin.H{
		"tasks": taskPage, "notes": notePage, "documents": docPage,
		"has_more": gin.H{"tasks": taskMore, "notes": noteMore, "documents": docMore},
	})
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
