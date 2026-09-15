package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"tessera/internal/db"
	"tessera/middleware"
)

// ListWorkspaceTasks backs the "My tasks" / "All tasks" views (feature #1).
// ?assignee=me limits to tasks assigned to the caller.
func (h *API) ListWorkspaceTasks(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListWorkspaceTasks(c, db.ListWorkspaceTasksParams{
		WorkspaceID:     wsID,
		IncludeSubtasks: c.Query("include_subtasks") == "1" || c.Query("include_subtasks") == "true",
	})
	if err != nil {
		fail(c, err)
		return
	}
	if c.Query("assignee") == "me" {
		me := middleware.CurrentUser(c)
		rows = filterAssignedTo(rows, me)
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// WorkspaceSummary returns headline counts for the home screen (feature #1).
func (h *API) WorkspaceSummary(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	// Day boundaries stay in Go: a due date is bucketed by its own calendar day, and
	// the comparison is between absolute instants, so "day(due) < today" is exactly
	// "due < today" once the midnights are passed in as parameters — no re-deriving
	// it with date_trunc under the server's session timezone.
	//
	// The calendar is the caller's, not the server's. The clients store a due date
	// as the local midnight the user picked (+03:00 "today" is 21:00 UTC the day
	// before), and they colour the row under the tile by their own calendar day —
	// counting against the server's midnight instead puts a task that is due today
	// in "Просрочено" for the whole of it, contradicting the list right below.
	//
	// A due stored as a pure UTC midnight is a date-only value, not an instant, and
	// is bucketed by its bare date instead — the same rule the clients apply and the
	// one taskDateStr already follows here (see the note in workspace_tasks.sql). So
	// the caller's day goes down as a date too, alongside the midnights.
	uid := middleware.CurrentUser(c)
	now := time.Now().In(h.callerLocation(c, uid))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	s, err := h.q.WorkspaceTaskSummary(c, db.WorkspaceTaskSummaryParams{
		WorkspaceID: wsID,
		UserID:      uid,
		DayStart:    today,
		NextDay:     today.AddDate(0, 0, 1),
		WeekEnd:     today.AddDate(0, 0, 7),
		TodayDay:    bareDate(today),
		WeekEndDay:  bareDate(today.AddDate(0, 0, 7)),
	})
	if err != nil {
		fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":      s.Total,
		"completed":  s.Completed,
		"active":     s.Total - s.Completed,
		"assigned":   s.Assigned,
		"overdue":    s.Overdue,
		"due_today":  s.DueToday,
		"due_week":   s.DueWeek,
		"unassigned": s.Unassigned,
	})
}

// bareDate drops the time of day and the zone: the calendar date t names, as a
// `date` the query can compare a date-only due against. The zone has to go — a
// pgtype.Date built from a +14:00 midnight would otherwise be read back as the
// day before by anything that normalised it to UTC first.
func bareDate(t time.Time) pgtype.Date {
	return pgtype.Date{
		Time:  time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
}

func filterAssignedTo(rows []db.ListWorkspaceTasksRow, user uuid.UUID) []db.ListWorkspaceTasksRow {
	out := make([]db.ListWorkspaceTasksRow, 0, len(rows))
	for _, t := range rows {
		if containsID(t.AssigneeIds, user) {
			out = append(out, t)
		}
	}
	return out
}

func containsID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
