package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"tessera/internal/db"
	"tessera/internal/quickact"
	"tessera/middleware"
)

// ── Учёт времени (#2965) ────────────────────────────────────
//
// «Потрачено» beside the estimate: entries of who spent how many minutes on a
// task, on which day, and (optionally) on what. Anyone who may edit the task
// logs time; an entry is removed by its author or by the workspace's owner or
// admin. The sums — the task's own and its subtree's — travel with the task
// detail and with the task.time event every change sends.

const maxTimeEntryMinutes = 100000

// timeSpent is the pair of sums the clients show.
func (h *API) timeSpent(c *gin.Context, taskID uuid.UUID) gin.H {
	s, err := h.q.TaskSpentMinutes(c, taskID)
	if err != nil {
		return gin.H{"spent": 0, "spent_total": 0}
	}
	return gin.H{"spent": s.Own, "spent_total": s.Total}
}

func (h *API) announceTime(c *gin.Context, wsID, taskID uuid.UUID) {
	payload := h.timeSpent(c, taskID)
	payload["task_id"] = taskID
	h.broadcastAs(c, wsID, "task.time", payload)
}

// ListTimeEntries — GET /tasks/:id/time: the entries, newest day first, and the sums.
func (h *API) ListTimeEntries(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, _, ok := h.loadTask(c, id); !ok {
		return
	}
	rows, err := h.q.ListTimeEntries(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	out := h.timeSpent(c, id)
	out["entries"] = orEmpty(rows)
	c.JSON(http.StatusOK, out)
}

// AddTimeEntry — POST /tasks/:id/time {minutes, note?, spent_on?: "YYYY-MM-DD"}.
func (h *API) AddTimeEntry(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		Minutes int    `json:"minutes"`
		Note    string `json:"note"`
		SpentOn string `json:"spent_on"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entry, err := h.addTime(c, t.ID, req.Minutes, req.Note, req.SpentOn)
	if err != nil {
		if ue, isUser := err.(opError); isUser {
			c.JSON(http.StatusBadRequest, gin.H{"error": ue.msg})
			return
		}
		fail(c, err)
		return
	}
	h.announceTime(c, wsID, t.ID)
	c.JSON(http.StatusCreated, entry)
}

// addTime validates and stores one entry, journaling it; the quick action
// /spend goes through here too. A bad input is an opError (a 400 / a refused
// command), anything else a failure.
func (h *API) addTime(c *gin.Context, taskID uuid.UUID, minutes int, note, spentOn string) (db.TaskTimeEntry, error) {
	if minutes <= 0 || minutes > maxTimeEntryMinutes {
		return db.TaskTimeEntry{}, opError{msg: "minutes out of range"}
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 500 {
		return db.TaskTimeEntry{}, opError{msg: "note too long"}
	}
	var day pgtype.Date
	if spentOn != "" {
		d, err := time.Parse("2006-01-02", spentOn)
		if err != nil {
			return db.TaskTimeEntry{}, opError{msg: "spent_on must be YYYY-MM-DD"}
		}
		day = pgtype.Date{Time: d, Valid: true}
	}
	uid := middleware.CurrentUser(c)
	entry, err := h.q.AddTimeEntry(c, db.AddTimeEntryParams{
		TaskID: taskID, UserID: &uid, Minutes: int32(minutes), Note: note, SpentOn: day,
	})
	if err != nil {
		return db.TaskTimeEntry{}, err
	}
	h.logEvent(c, taskID, "time_spent", map[string]any{"minutes": minutes})
	return entry, nil
}

// qaSpend — «/spend 1h30m» in a comment (#2965): a time entry by the comment's
// author, read like an estimate in time units of the project's day/week.
func (h *API) qaSpend(c *gin.Context, t db.Task, wsID uuid.UUID, cmd quickact.Cmd) (string, error) {
	minutes, err := h.spendMinutes(c, t, wsID, cmd.Arg)
	if err != nil {
		return "", err
	}
	if _, err := h.addTime(c, t.ID, minutes, "", ""); err != nil {
		return "", err
	}
	h.announceTime(c, wsID, t.ID)
	return "потрачено " + strings.TrimSpace(cmd.Arg), nil
}

func (h *API) spendMinutes(c *gin.Context, t db.Task, wsID uuid.UUID, arg string) (int, error) {
	cfg := h.estimateConfig(c, t, wsID)
	cfg.Unit = "time" // spent time is time, whatever the project estimates in
	v, err := quickact.ParseEstimate(arg, cfg)
	if err != nil {
		return 0, opError{msg: fmt.Sprintf("не понимаю время %q", strings.TrimSpace(arg))}
	}
	return int(v + 0.5), nil
}

// DeleteTimeEntry — DELETE /tasks/:id/time/:entryId: the author's own entry, or
// any entry for the workspace's owner/admin.
func (h *API) DeleteTimeEntry(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	entryID, ok := parseID(c, "entryId")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	e, err := h.q.GetTimeEntry(c, db.GetTimeEntryParams{ID: entryID, TaskID: t.ID})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	mine := e.UserID != nil && *e.UserID == middleware.CurrentUser(c)
	if !mine && !h.isManager(c, wsID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the author or a workspace admin removes an entry"})
		return
	}
	if _, err := h.q.DeleteTimeEntry(c, db.DeleteTimeEntryParams{ID: entryID, TaskID: t.ID}); err != nil {
		fail(c, err)
		return
	}
	h.announceTime(c, wsID, t.ID)
	c.Status(http.StatusNoContent)
}
