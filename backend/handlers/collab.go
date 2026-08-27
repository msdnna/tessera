package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/internal/quickact"
	"tessera/middleware"
)

// ── journal + notification helpers ─────────────────────────────

// logEvent appends an entry to a task's activity journal. Best-effort: a
// logging failure must never break the user-facing mutation, so errors are
// swallowed.
func (h *API) logEvent(c *gin.Context, taskID uuid.UUID, kind string, data map[string]any) {
	h.logEventActor(c, taskID, middleware.CurrentUser(c), kind, data)
}

// logEventActor is logEvent with an explicit actor, usable outside an HTTP
// request (e.g. the background sync worker). actorID == uuid.Nil logs a system
// event with no actor.
func (h *API) logEventActor(ctx context.Context, taskID, actorID uuid.UUID, kind string, data map[string]any) {
	raw := []byte("{}")
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			raw = b
		}
	}
	var actor *uuid.UUID
	if actorID != uuid.Nil {
		actor = &actorID
	}
	_, _ = h.q.LogTaskEvent(ctx, db.LogTaskEventParams{
		TaskID: taskID, ActorID: actor, Kind: kind, Data: raw,
	})
}

// notify creates a persistent notification for a user (skipping the actor
// themselves) and pushes it live over the workspace socket. Best-effort.
func (h *API) notify(c *gin.Context, userID, wsID uuid.UUID, taskID *uuid.UUID, kind string, msg notifyMsg) {
	actor := middleware.CurrentUser(c)
	if userID == actor {
		return
	}
	a := actor
	h.deliverNotification(c, userID, wsID, taskID, &a, kind, msg)
}

// notifyTaskParticipants notifies a task's assignees and creator (minus the
// actor) — used when something happens that watchers should hear about.
func (h *API) notifyTaskParticipants(c *gin.Context, t db.Task, wsID uuid.UUID, kind string, msg notifyMsg) {
	h.notifyTaskParticipantsExcept(c, t, wsID, kind, msg, nil)
}

// notifyTaskParticipantsExcept is notifyTaskParticipants with a set of user ids
// to skip (e.g. users already reached by a more specific mention notification).
func (h *API) notifyTaskParticipantsExcept(c *gin.Context, t db.Task, wsID uuid.UUID, kind string, msg notifyMsg, skip map[uuid.UUID]bool) {
	seen := map[uuid.UUID]bool{}
	for id := range skip {
		seen[id] = true
	}
	if assignees, err := h.q.ListTaskAssignees(c, t.ID); err == nil {
		for _, a := range assignees {
			if !seen[a.ID] {
				seen[a.ID] = true
				h.notify(c, a.ID, wsID, &t.ID, kind, msg)
			}
		}
	}
	if t.CreatedBy != nil && !seen[*t.CreatedBy] {
		h.notify(c, *t.CreatedBy, wsID, &t.ID, kind, msg)
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
	ActorID   *uuid.UUID      `json:"actor_id"`
	ActorName *string         `json:"actor_name"`
}

// ListTaskEvents returns the activity-log events for a task.
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
		fail(c, err)
		return
	}
	out := make([]taskEventResp, 0, len(events))
	for _, e := range events {
		data := json.RawMessage(e.Data)
		if len(data) == 0 {
			data = json.RawMessage("{}")
		}
		out = append(out, taskEventResp{
			ID: e.ID, Kind: e.Kind, Data: data, CreatedAt: e.CreatedAt, ActorID: e.ActorID, ActorName: e.ActorName,
		})
	}
	c.JSON(http.StatusOK, out)
}

// ── comments ───────────────────────────────────────────────────

// ListComments returns the comments on a task.
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
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(comments))
}

// CreateComment adds a comment to a task.
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
		ParentID *uuid.UUID  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := middleware.CurrentUser(c)

	// Resolve the thread this comment belongs to. Threads are two levels deep, as
	// in GitLab: replying to a reply lands in the same thread rather than nesting
	// further. Normalising here rather than on the client is deliberate — Android
	// and the MCP server hit the same endpoint and must not be able to build a
	// depth the readers cannot render.
	parentID, ok := h.resolveCommentParent(c, id, req.ParentID)
	if !ok {
		return
	}

	// Quick actions ("/close", "/assign @msdnna") execute here and their lines are
	// dropped from the body; custom dictionary commands ("/approve") are left in
	// the text, because those are a note to whoever reads the comment.
	parsed := quickact.Parse(req.Body, h.customCommandKeys(c, wsID))
	summary := h.runQuickActions(c, t, wsID, parsed.Cmds)
	if !summary.empty() {
		applied := make([]string, 0, len(summary.Applied))
		for _, o := range summary.Applied {
			applied = append(applied, o.Key)
		}
		failed := make([]string, 0, len(summary.Errors))
		for _, o := range summary.Errors {
			failed = append(failed, o.Key)
		}
		// One aggregated entry, not one per command: a five-command comment must
		// not bury the rest of the task's history.
		h.logEvent(c, id, "commands", map[string]any{"applied": applied, "failed": failed})
		// The commands may have moved or renamed the task; later notification text
		// should quote what it looks like now.
		if fresh, ferr := h.q.GetTask(c, id); ferr == nil {
			t = fresh
		}
	}

	// Nothing left to say: the comment was pure commands, so we do not store an
	// empty one (GitLab behaves the same). 200 instead of 201 — a client that
	// sends commands is by definition new enough to expect it.
	if strings.TrimSpace(parsed.Rest) == "" && len(parsed.Cmds) > 0 {
		if len(summary.Applied) == 0 {
			// Every command failed and there is no text to fall back on. Saying so
			// beats silently dropping what the user wrote.
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "ни одна команда не применилась", "command_summary": summary,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"comment": nil, "command_summary": summary})
		return
	}

	body := parsed.Rest
	if len(parsed.Cmds) == 0 {
		body = req.Body // no commands ran: keep the body byte-for-byte
	}
	cm, err := h.q.CreateComment(c, db.CreateCommentParams{TaskID: id, AuthorID: &uid, Body: body, ParentID: parentID})
	if err != nil {
		fail(c, err)
		return
	}
	h.logEvent(c, id, "comment", map[string]any{"comment_id": cm.ID})
	h.broadcast(wsID, "task.commented", gin.H{"task_id": id})
	// Mirror the comment to a linked GitLab issue as a note (opt-in per integration).
	wb := map[string]any{"comment_id": cm.ID.String(), "body": body}
	if parentID != nil {
		wb["parent_comment_id"] = parentID.String()
	}
	h.enqueueWriteback(c, id, uid, gitlab.TrigComment, wb)

	// @-mentions: notify each mentioned workspace member explicitly, then fall
	// back to the generic "commented" notice for the remaining participants. A
	// short comment is inlined for context; a long one shows only the #N (the
	// message builders apply that cut-off).
	mentioned := h.notifyMentions(c, t, wsID, req.Mentions, body)
	if parentID != nil {
		// A reply has an audience the task's participant list does not cover: the
		// author of the root comment is frequently neither assignee nor reporter,
		// and would never learn a branch was started under their text.
		mentioned = h.notifyThread(c, t, wsID, *parentID, body, mentioned)
	}
	h.notifyTaskParticipantsExcept(c, t, wsID, "comment",
		msgComment(h.actorName(c), t.Number, body), mentioned)
	if summary.empty() {
		c.JSON(http.StatusCreated, cm)
		return
	}
	// Older clients read the comment fields off the top level, so keep them there
	// and hang the summary alongside.
	c.JSON(http.StatusCreated, commentWithCommands{TaskComment: cm, CommandSummary: &summary})
}

// commentWithCommands is a created comment plus what the quick actions in it
// did. The comment's own fields stay inlined at the top level so clients that
// predate quick actions keep working unchanged.
type commentWithCommands struct {
	db.TaskComment
	CommandSummary *commandSummary `json:"command_summary,omitempty"`
}

// promoteThreadSuccessor prepares a thread for the deletion of its root: the
// oldest reply becomes the new root and the remaining replies are re-hung under
// it. All-or-nothing — a half-applied promotion would leave replies pointing at
// a row that is about to disappear (the FK would null them out and scatter the
// thread across the timeline).
func (h *API) promoteThreadSuccessor(c *gin.Context, rootID uuid.UUID) error {
	replies, err := h.q.ListThreadReplies(c, &rootID)
	if err != nil || len(replies) == 0 {
		return err
	}
	successor := replies[0].ID // oldest first, per the query's ORDER BY
	return h.inTx(c, func(q *db.Queries) error {
		if err := q.ReparentReplies(c, db.ReparentRepliesParams{ToRoot: &successor, FromRoot: &rootID}); err != nil {
			return err
		}
		return q.PromoteReplyToRoot(c, successor)
	})
}

// resolveCommentParent validates a requested parent and returns the id the reply
// should actually hang off: nil for a root comment, the parent for a first-level
// reply, and the parent's own parent when replying to a reply (threads are two
// levels deep, as in GitLab). A parent on a different task is rejected — that is
// a branch moved between tasks, not a typo worth silently absorbing.
func (h *API) resolveCommentParent(c *gin.Context, taskID uuid.UUID, want *uuid.UUID) (*uuid.UUID, bool) {
	if want == nil {
		return nil, true
	}
	parent, err := h.q.GetComment(c, *want)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "родительский комментарий не найден"})
		return nil, false
	}
	if parent.TaskID != taskID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "родительский комментарий из другой задачи"})
		return nil, false
	}
	if parent.ParentID != nil {
		return parent.ParentID, true // collapse reply-to-reply into the thread root
	}
	return &parent.ID, true
}

// notifyThread notifies the people already in a thread (its root author and
// everyone who replied) that someone answered in it, skipping whoever the
// mention pass already reached. Returns the updated notified set.
func (h *API) notifyThread(c *gin.Context, t db.Task, wsID, rootID uuid.UUID, body string, notified map[uuid.UUID]bool) map[uuid.UUID]bool {
	ids, err := h.q.ListThreadParticipants(c, rootID)
	if err != nil {
		return notified
	}
	msg := msgThreadReply(h.actorName(c), t.Number, body)
	for _, id := range ids {
		if id == nil || notified[*id] {
			continue
		}
		if _, err := h.q.GetMembership(c, db.GetMembershipParams{WorkspaceID: wsID, UserID: *id}); err != nil {
			continue // no longer a member of this workspace
		}
		notified[*id] = true
		h.notify(c, *id, wsID, &t.ID, "comment", msg)
	}
	return notified
}

// notifyMentions sends a "mention" notification to each id that is a member of
// the workspace (skipping duplicates and the actor, handled in notify). Returns
// the set of users actually notified so the caller can avoid double-notifying.
func (h *API) notifyMentions(c *gin.Context, t db.Task, wsID uuid.UUID, ids []uuid.UUID, body string) map[uuid.UUID]bool {
	notified := map[uuid.UUID]bool{}
	if len(ids) == 0 {
		return notified
	}
	msg := msgMention(h.actorName(c), t.Number, body)
	for _, uid := range ids {
		if notified[uid] {
			continue
		}
		if _, err := h.q.GetMembership(c, db.GetMembershipParams{WorkspaceID: wsID, UserID: uid}); err != nil {
			continue // not a member of this workspace — ignore
		}
		notified[uid] = true
		h.notify(c, uid, wsID, &t.ID, "mention", msg)
	}
	return notified
}

// UpdateComment edits a comment's body.
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
		fail(c, err)
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
		fail(c, err)
		return
	}
	// Mirror the edit to the linked GitLab note (only when this comment already has
	// one). Without this the next pull overwrites the local body back to GitLab's,
	// silently dropping the edit.
	if cm.GlNoteID != nil && *cm.GlNoteID != "" {
		h.enqueueWriteback(c, cm.TaskID, middleware.CurrentUser(c), gitlab.TrigComment,
			map[string]any{"op": "edit", "comment_id": id.String(), "gl_note_id": *cm.GlNoteID, "body": req.Body})
	}
	c.JSON(http.StatusOK, updated)
}

// DeleteComment removes a comment.
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
		fail(c, err)
		return
	}
	if _, _, ok := h.loadTask(c, cm.TaskID); !ok {
		return
	}
	if cm.AuthorID == nil || *cm.AuthorID != middleware.CurrentUser(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not your comment"})
		return
	}
	// Enqueue the GitLab-note deletion BEFORE dropping the local row (we need its
	// gl_note_id). Only when the comment was actually mirrored to GitLab.
	if cm.GlNoteID != nil && *cm.GlNoteID != "" {
		h.enqueueWriteback(c, cm.TaskID, middleware.CurrentUser(c), gitlab.TrigComment,
			map[string]any{"op": "delete", "comment_id": id.String(), "gl_note_id": *cm.GlNoteID})
	}
	// Deleting a thread root must not take the replies with it: promote the oldest
	// reply to root and re-hang the rest under it. GitLab behaves the same (the
	// thread survives as long as it still holds a note), and one "✕" never removes
	// someone else's text.
	if cm.ParentID == nil {
		if err := h.promoteThreadSuccessor(c, id); err != nil {
			fail(c, err)
			return
		}
	}
	if err := h.q.DeleteComment(c, id); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── relations (referenced by #N) ───────────────────────────────

// ListRelations returns a task's relations (blocks / blocked-by / relates / duplicates).
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
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rels))
}

// AddRelation links two tasks with a relation.
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
		fail(c, err)
		return
	}
	if err := h.applyRelation(c, t, wsID, target, kind); err != nil {
		respondOpError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// BoardDependencies returns every blocking edge among a board's tasks — the
// Gantt view fetches the whole graph at once (the per-task /relations endpoint
// would need one round-trip per bar). Accepts a board UUID or slug like GetBoard.
func (h *API) BoardDependencies(c *gin.Context) {
	param := c.Param("id")
	var b db.Board
	var err error
	if id, perr := uuid.Parse(param); perr == nil {
		b, err = h.q.GetBoard(c, id)
	} else {
		b, err = h.q.GetBoardBySlug(c, param)
	}
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(c, b.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListBoardDependencies(c, b.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
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

// DeleteRelation removes a task relation.
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
		fail(c, err)
		return
	}
	if _, _, ok := h.loadTask(c, rel.TaskID); !ok {
		return
	}
	if err := h.q.DeleteTaskRelation(c, id); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ── notifications ───────────────────────────────────────────────

// ListNotifications returns the current user's notifications.
func (h *API) ListNotifications(c *gin.Context) {
	items, err := h.q.ListNotifications(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(items))
}

// UnreadNotificationCount returns the current user's unread notification count.
func (h *API) UnreadNotificationCount(c *gin.Context) {
	n, err := h.q.CountUnreadNotifications(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": n})
}

// MarkNotificationRead marks a single notification read.
func (h *API) MarkNotificationRead(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.q.MarkNotificationRead(c, db.MarkNotificationReadParams{
		ID: id, UserID: middleware.CurrentUser(c),
	}); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// MarkAllNotificationsRead marks all of the current user's notifications read.
func (h *API) MarkAllNotificationsRead(c *gin.Context) {
	if err := h.q.MarkAllNotificationsRead(c, middleware.CurrentUser(c)); err != nil {
		fail(c, err)
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

// shortContextLimit is the max length (runes) of content (a task title, a comment)
// inlined into a notification; longer content is omitted so only the "#N + what"
// summary shows.
const shortContextLimit = 16

// shortCtx returns a « quoted » suffix for s when it's short enough to inline,
// else "" (the notification then shows only the #N reference + what changed).
func shortCtx(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > shortContextLimit {
		return ""
	}
	return " «" + s + "»"
}
