package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
		fail(c)
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
	rows, err := h.q.ListWorkspaceTasks(c, db.ListWorkspaceTasksParams{WorkspaceID: wsID})
	if err != nil {
		fail(c)
		return
	}
	me := middleware.CurrentUser(c)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekEnd := today.AddDate(0, 0, 7)

	var total, completed, mine, overdue, dueToday, dueWeek, noStatus int
	for _, t := range rows {
		total++
		if t.CompletedAt != nil {
			completed++
		}
		assignedToMe := containsID(t.AssigneeIds, me)
		if assignedToMe {
			mine++
		}
		if t.DueDate != nil && t.CompletedAt == nil {
			d := *t.DueDate
			day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
			switch {
			case day.Before(today):
				overdue++
			case day.Equal(today):
				dueToday++
				dueWeek++
			case day.Before(weekEnd):
				dueWeek++
			}
		}
		if len(t.AssigneeIds) == 0 {
			noStatus++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":      total,
		"completed":  completed,
		"active":     total - completed,
		"assigned":   mine,
		"overdue":    overdue,
		"due_today":  dueToday,
		"due_week":   dueWeek,
		"unassigned": noStatus,
	})
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
