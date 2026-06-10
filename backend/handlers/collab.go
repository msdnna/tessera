package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/middleware"
)

// ── journal + notification helpers ─────────────────────────────

// logEvent appends an entry to a task's activity journal. Best-effort: a
// logging failure must never break the user-facing mutation, so errors are
// swallowed.
func (h *API) logEvent(c *gin.Context, taskID uuid.UUID, kind string, data map[string]any) {
	raw := []byte("{}")
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			raw = b
		}
	}
	actor := middleware.CurrentUser(c)
	_, _ = h.q.LogTaskEvent(c, db.LogTaskEventParams{
		TaskID: taskID, ActorID: &actor, Kind: kind, Data: raw,
	})
}

// notify creates a persistent notification for a user (skipping the actor
// themselves) and pushes it live over the workspace socket. Best-effort.
func (h *API) notify(c *gin.Context, userID, wsID uuid.UUID, taskID *uuid.UUID, kind, text string) {
	actor := middleware.CurrentUser(c)
	if userID == actor {
		return
	}
	n, err := h.q.CreateNotification(c, db.CreateNotificationParams{
		UserID: userID, WorkspaceID: wsID, TaskID: taskID, ActorID: &actor, Kind: kind, Text: text,
	})
	if err != nil {
		return
	}
	// Enrich the live payload with the task's board id + number so a freshly
	// pushed notification is clickable (the list endpoint joins these in, but
	// the raw inserted row doesn't carry them).
	obj := gin.H{
		"id": n.ID, "user_id": n.UserID, "workspace_id": n.WorkspaceID,
		"task_id": n.TaskID, "actor_id": n.ActorID, "kind": n.Kind,
		"text": n.Text, "read_at": n.ReadAt, "created_at": n.CreatedAt,
	}
	if taskID != nil {
		if t, terr := h.q.GetTask(c, *taskID); terr == nil {
			obj["task_board_id"] = t.BoardID
			obj["task_number"] = t.Number
		}
	}
	h.broadcast(wsID, "notification", gin.H{"user_id": userID, "notification": obj})
}

// notifyTaskParticipants notifies a task's assignees and creator (minus the
// actor) — used when something happens that watchers should hear about.
func (h *API) notifyTaskParticipants(c *gin.Context, t db.Task, wsID uuid.UUID, kind, text string) {
	h.notifyTaskParticipantsExcept(c, t, wsID, kind, text, nil)
}

// notifyTaskParticipantsExcept is notifyTaskParticipants with a set of user ids
// to skip (e.g. users already reached by a more specific mention notification).
func (h *API) notifyTaskParticipantsExcept(c *gin.Context, t db.Task, wsID uuid.UUID, kind, text string, skip map[uuid.UUID]bool) {
	seen := map[uuid.UUID]bool{}
	for id := range skip {
		seen[id] = true
	}
	if assignees, err := h.q.ListTaskAssignees(c, t.ID); err == nil {
		for _, a := range assignees {
			if !seen[a.ID] {
				seen[a.ID] = true
				h.notify(c, a.ID, wsID, &t.ID, kind, text)
			}
		}
	}
	if t.CreatedBy != nil && !seen[*t.CreatedBy] {
		h.notify(c, *t.CreatedBy, wsID, &t.ID, kind, text)
	}
}

// actorName returns the current user's display name for notification text.
func (h *API) actorName(c *gin.Context) string {
	u, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		return "Кто-то"
	}
	return u.Name
}

// errIsNoRows reports whether err is pgx's "no rows" sentinel (used to treat a
// no-op ON CONFLICT DO NOTHING as success rather than an error).
func errIsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// ── task activity journal ──────────────────────────────────────

// taskEventResp serialises the journal entry with `data` as raw JSON (the
// generated row carries it as []byte, which would otherwise be base64-encoded).
type taskEventResp struct {
	ID        uuid.UUID       `json:"id"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	ActorName *string         `json:"actor_name"`
}

func (h *API) ListTaskEvents(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, _, ok := h.loadTask(c, id); !ok {
		return
	}
	events, err := h.q.ListTaskEvents(c, id)
	if err != nil {
		fail(c)
		return
	}
	out := make([]taskEventResp, 0, len(events))
	for _, e := range events {
		data := json.RawMessage(e.Data)
		if len(data) == 0 {
			data = json.RawMessage("{}")
		}
		out = append(out, taskEventResp{
			ID: e.ID, Kind: e.Kind, Data: data, CreatedAt: e.CreatedAt, ActorName: e.ActorName,
		})
	}
	c.JSON(http.StatusOK, out)
}

// ── comments ───────────────────────────────────────────────────

func (h *API) ListComments(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, _, ok := h.loadTask(c, id); !ok {
		return
	}
	comments, err := h.q.ListTaskComments(c, id)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, orEmpty(comments))
}

func (h *API) CreateComment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		Body     string      `json:"body" binding:"required"`
		Mentions []uuid.UUID `json:"mentions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)
	cm, err := h.q.CreateComment(c, db.CreateCommentParams{TaskID: id, AuthorID: &uid, Body: req.Body})
	if err != nil {
		fail(c)
		return
	}
	h.logEvent(c, id, "comment", map[string]any{"comment_id": cm.ID})
	h.broadcast(wsID, "task.commented", gin.H{"task_id": id})

	// @-mentions: notify each mentioned workspace member explicitly, then fall
	// back to the generic "commented" notice for the remaining participants.
	mentioned := h.notifyMentions(c, t, wsID, req.Mentions)
	h.notifyTaskParticipantsExcept(c, t, wsID, "comment",
		fmt.Sprintf("%s прокомментировал #%s", h.actorName(c), taskRef(t.Number)), mentioned)
	c.JSON(http.StatusCreated, cm)
}

// notifyMentions sends a "mention" notification to each id that is a member of
// the workspace (skipping duplicates and the actor, handled in notify). Returns
// the set of users actually notified so the caller can avoid double-notifying.
func (h *API) notifyMentions(c *gin.Context, t db.Task, wsID uuid.UUID, ids []uuid.UUID) map[uuid.UUID]bool {
	notified := map[uuid.UUID]bool{}
	if len(ids) == 0 {
		return notified
	}
	text := fmt.Sprintf("%s упомянул(а) вас в #%s", h.actorName(c), taskRef(t.Number))
	for _, uid := range ids {
		if notified[uid] {
			continue
		}
		if _, err := h.q.GetMembership(c, db.GetMembershipParams{WorkspaceID: wsID, UserID: uid}); err != nil {
			continue // not a member of this workspace — ignore
		}
		notified[uid] = true
		h.notify(c, uid, wsID, &t.ID, "mention", text)
	}
	return notified
}

func (h *API) UpdateComment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	cm, err := h.q.GetComment(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if _, _, ok := h.loadTask(c, cm.TaskID); !ok {
		return
	}
	if cm.AuthorID == nil || *cm.AuthorID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your comment"})
		return
	}
	var req struct {
		Body string `json:"body" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.q.UpdateComment(c, db.UpdateCommentParams{ID: id, Body: req.Body})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *API) DeleteComment(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	cm, err := h.q.GetComment(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if _, _, ok := h.loadTask(c, cm.TaskID); !ok {
		return
	}
	if cm.AuthorID == nil || *cm.AuthorID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your comment"})
		return
	}
	if err := h.q.DeleteComment(c, id); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── relations (referenced by #N) ───────────────────────────────

func (h *API) ListRelations(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if _, _, ok := h.loadTask(c, id); !ok {
		return
	}
	rels, err := h.q.ListTaskRelations(c, id)
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rels))
}

func (h *API) AddRelation(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	t, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	var req struct {
		Number int64  `json:"number" binding:"required"`
		Kind   string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kind := req.Kind
	if kind == "" {
		kind = "relates"
	}
	num := req.Number
	target, err := h.q.GetTaskByNumber(c, db.GetTaskByNumberParams{WorkspaceID: wsID, Number: &num})
	if notFound(c, err) {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("задача #%d не найдена", req.Number)})
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if target.ID == t.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "нельзя связать задачу с собой"})
		return
	}
	if _, err := h.q.AddTaskRelation(c, db.AddTaskRelationParams{
		TaskID: id, RelatedTaskID: target.ID, Kind: kind,
	}); err != nil && !errIsNoRows(err) {
		fail(c)
		return
	}
	h.logEvent(c, id, "relation", map[string]any{"related": req.Number, "kind": kind})
	// Record the link on the referenced task too, so it shows in #target's
	// history ("добавил связь с #<source>") — relations are otherwise one-way.
	if t.Number != nil {
		h.logEvent(c, target.ID, "relation", map[string]any{"related": *t.Number, "kind": inverseRelationKind(kind)})
		h.broadcast(wsID, "task.updated", gin.H{"id": target.ID})
	}
	h.broadcast(wsID, "task.updated", gin.H{"id": id})
	c.Status(http.StatusCreated)
}

// inverseRelationKind flips a directed relation kind for the referenced task's
// side (relates/duplicates are symmetric; blocks ⇄ blocked_by).
func inverseRelationKind(kind string) string {
	switch kind {
	case "blocks":
		return "blocked_by"
	case "blocked_by":
		return "blocks"
	default:
		return kind
	}
}

func (h *API) DeleteRelation(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	rel, err := h.q.GetTaskRelation(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c)
		return
	}
	if _, _, ok := h.loadTask(c, rel.TaskID); !ok {
		return
	}
	if err := h.q.DeleteTaskRelation(c, id); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── notifications ───────────────────────────────────────────────

func (h *API) ListNotifications(c *gin.Context) {
	items, err := h.q.ListNotifications(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, orEmpty(items))
}

func (h *API) UnreadNotificationCount(c *gin.Context) {
	n, err := h.q.CountUnreadNotifications(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": n})
}

func (h *API) MarkNotificationRead(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.q.MarkNotificationRead(c, db.MarkNotificationReadParams{
		ID: id, UserID: middleware.CurrentUser(c),
	}); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *API) MarkAllNotificationsRead(c *gin.Context) {
	if err := h.q.MarkAllNotificationsRead(c, middleware.CurrentUser(c)); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// taskRef renders a task number for notification/journal text ("12" or "?").
func taskRef(n *int64) string {
	if n == nil {
		return "?"
	}
	return fmt.Sprintf("%d", *n)
}
