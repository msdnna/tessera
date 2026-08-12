package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// journalRetention caps how many runs we keep per integration; older runs (and
// their actions, via ON DELETE CASCADE) are pruned after every flush.
const journalRetention = 200

// journalTextCap bounds the length of free-text (descriptions, comment bodies)
// stored in a journal detail so a single sync row can't bloat the table.
const journalTextCap = 400

// truncForJournal shortens free text for storage in a journal detail.
func truncForJournal(s string) string {
	r := []rune(s)
	if len(r) <= journalTextCap {
		return s
	}
	return string(r[:journalTextCap]) + "…"
}

// timePtrEq reports whether two optional timestamps represent the same instant.
func timePtrEq(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// journalAction is one accumulated action within a sync run, flushed to
// gitlab_sync_actions when the run completes.
type journalAction struct {
	Direction  string         // "pull" (GL→DB) | "push" (DB→GL)
	EntityType string         // task|subtask|tag|assignee|comment|state|priority
	Op         string         // create|update|delete|push
	TaskID     *uuid.UUID     // nil when no task is involved/known
	GlIid      *int64         // the GitLab issue iid, when known
	Summary    string         // one-line label for the left list
	Detail     map[string]any // before/after (pull) or payload/result (push)
	Status     string         // "ok" | "fail" (defaults to "ok")
	Err        string         // populated when Status == "fail"
}

// syncJournal accumulates one run's worth of actions in memory, then flushes the
// run header + its actions together in flushJournal. Provider-neutral: nothing
// here is GitLab-specific beyond the table names.
type syncJournal struct {
	integrationID uuid.UUID
	kind          string // "pull" | "push"
	trigger       string // "manual" | "auto"
	mode          string // "full" | "incremental" (pull only; push runs are always "full")
	actorID       *uuid.UUID
	startedAt     time.Time  // when the run actually began, stamped into the run row
	runID         *uuid.UUID // set once the run row exists (beginJournal or flushJournal)
	actions       []journalAction
	created       int
	updated       int
	deleted       int
	status        string // overall run status; "ok" unless an action fails or the run aborts
	errText       string // run-level error (a fetch/board failure that aborted the pull)
}

func (h *API) newJournal(integrationID uuid.UUID, kind, trigger string, actorID *uuid.UUID) *syncJournal {
	return &syncJournal{
		integrationID: integrationID, kind: kind, trigger: trigger, mode: "full", actorID: actorID,
		startedAt: time.Now(), status: "ok",
	}
}

// beginJournal opens the run row before the sync starts, so a long manual run is
// visible as "running" in the journal while it works instead of appearing only
// once it finishes. Manual runs only — an auto heartbeat with no changes must not
// litter the journal, and flushJournal still decides that at the end.
// Best-effort: a failure here just falls back to create-on-flush.
func (h *API) beginJournal(ctx context.Context, j *syncJournal) {
	if j == nil || j.trigger != "manual" || j.runID != nil {
		return
	}
	run, err := h.q.CreateGitlabSyncRun(ctx, db.CreateGitlabSyncRunParams{
		IntegrationID: j.integrationID, Kind: j.kind, Trigger: j.trigger, ActorID: j.actorID,
		Status: "running", StartedAt: j.startedAt, Mode: j.mode,
	})
	if err != nil {
		log.Printf("gitlab journal: begin run failed: %v", err)
		return
	}
	j.runID = &run.ID
}

// FailStaleSyncRuns closes runs still marked "running" from a previous process
// lifetime — a crash or restart mid-sync would otherwise leave them spinning in
// the journal forever. Safe at startup: nothing of ours is running yet.
func (h *API) FailStaleSyncRuns(ctx context.Context) {
	if err := h.q.FailStaleGitlabSyncRuns(ctx); err != nil {
		log.Printf("gitlab journal: fail stale runs: %v", err)
	}
}

// add appends an action, bumping the run's create/update/delete counts and
// downgrading the run status to "partial" on a per-action failure.
func (j *syncJournal) add(a journalAction) {
	if j == nil {
		return
	}
	if a.Status == "" {
		a.Status = "ok"
	}
	switch a.Op {
	case "create":
		j.created++
	case "update":
		j.updated++
	case "delete":
		j.deleted++
	}
	if a.Status == "fail" && j.status == "ok" {
		j.status = "partial"
	}
	j.actions = append(j.actions, a)
}

// abort records a run-level failure (e.g. the GitLab fetch failed before any
// per-item work), so the run shows as errored in the journal.
func (j *syncJournal) abort(err error) {
	if j == nil || err == nil {
		return
	}
	j.status = "error"
	j.errText = err.Error()
}

// flushJournal persists the run and its actions, then prunes old runs. When
// beginJournal already opened the run row, its actions are appended to it and the
// status stamped — otherwise the row is created here, still carrying the real
// start time. A successful auto run with no actions is skipped so the journal
// isn't filled with empty heartbeats (gitlab_integrations.last_synced_at already
// records that). Best-effort: a journal write failure must never break the sync.
func (h *API) flushJournal(ctx context.Context, j *syncJournal) {
	if j == nil {
		return
	}
	if j.runID == nil {
		if len(j.actions) == 0 && j.status == "ok" && j.trigger != "manual" {
			return
		}
		run, err := h.q.CreateGitlabSyncRun(ctx, db.CreateGitlabSyncRunParams{
			IntegrationID: j.integrationID, Kind: j.kind, Trigger: j.trigger, ActorID: j.actorID,
			Status: "running", StartedAt: j.startedAt,
		})
		if err != nil {
			log.Printf("gitlab journal: create run failed: %v", err)
			return
		}
		j.runID = &run.ID
	}
	runID := *j.runID
	for i, a := range j.actions {
		detail := []byte("{}")
		if a.Detail != nil {
			if b, derr := json.Marshal(a.Detail); derr == nil {
				detail = b
			}
		}
		if cerr := h.q.CreateGitlabSyncAction(ctx, db.CreateGitlabSyncActionParams{
			RunID: runID, Seq: int32(i), Direction: a.Direction, EntityType: a.EntityType,
			Op: a.Op, TaskID: a.TaskID, GlIid: a.GlIid, Summary: a.Summary,
			Detail: detail, Status: a.Status, Error: a.Err,
		}); cerr != nil {
			log.Printf("gitlab journal: create action failed: %v", cerr)
		}
	}
	if ferr := h.q.FinishGitlabSyncRun(ctx, db.FinishGitlabSyncRunParams{
		ID: runID, Status: j.status, CreatedCount: int32(j.created), UpdatedCount: int32(j.updated),
		DeletedCount: int32(j.deleted), ActionCount: int32(len(j.actions)), Error: j.errText,
	}); ferr != nil {
		log.Printf("gitlab journal: finish run failed: %v", ferr)
	}
	if perr := h.q.PruneGitlabSyncRuns(ctx, db.PruneGitlabSyncRunsParams{
		IntegrationID: j.integrationID, Limit: journalRetention,
	}); perr != nil {
		log.Printf("gitlab journal: prune failed: %v", perr)
	}
}

// ── HTTP: read the journal & retry failed pushes ───────────

// journalWorkspace resolves the :id workspace after a membership check, writing
// the appropriate error response on failure.
func (h *API) journalWorkspace(c *gin.Context) (uuid.UUID, bool) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return uuid.Nil, false
	}
	if !h.requireMember(c, wsID) {
		return uuid.Nil, false
	}
	return wsID, true
}

// ListGitlabSyncRuns returns recent sync runs across every binding of the
// workspace, newest first.
func (h *API) ListGitlabSyncRuns(c *gin.Context) {
	wsID, ok := h.journalWorkspace(c)
	if !ok {
		return
	}
	limit := int32(50)
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= journalRetention {
			limit = int32(n)
		}
	}
	runs, err := h.q.ListGitlabSyncRunsByWorkspace(c, db.ListGitlabSyncRunsByWorkspaceParams{WorkspaceID: wsID, Limit: limit})
	if err != nil {
		fail(c, err)
		return
	}
	if runs == nil {
		runs = []db.GitlabSyncRun{}
	}
	c.JSON(http.StatusOK, runs)
}

// journalPageSize / journalPageMax bound how many actions one page returns. A run
// can hold thousands of actions; the list ships them in keyset pages (by seq)
// instead of one multi-MB blob, and never carries the heavy `detail` diff — that
// is fetched per row on demand via GetGitlabSyncActionDetail.
const (
	journalPageSize = 500
	journalPageMax  = 2000
)

// ListGitlabSyncActions returns a page of one run's actions (scoped to the
// workspace's bindings), in sequence order, without their detail diffs.
// ?limit=<n> caps the page; ?after_seq=<n> is the keyset cursor (pass the last
// seq of the previous page). The reply is {items, has_more, next_after_seq}.
func (h *API) ListGitlabSyncActions(c *gin.Context) {
	wsID, ok := h.journalWorkspace(c)
	if !ok {
		return
	}
	runID, ok := parseID(c, "runId")
	if !ok {
		return
	}
	if _, err := h.q.GetGitlabSyncRunInWorkspace(c, db.GetGitlabSyncRunInWorkspaceParams{ID: runID, WorkspaceID: wsID}); err != nil {
		if notFound(c, err) {
			return
		}
		fail(c, err)
		return
	}
	limit := int32(journalPageSize)
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= journalPageMax {
			limit = int32(n)
		}
	}
	// seq is 0-based, so -1 is "before the first row" (includes seq 0).
	afterSeq := int32(-1)
	if v := c.Query("after_seq"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			afterSeq = int32(n)
		}
	}
	rows, err := h.q.ListGitlabSyncActionsPage(c, db.ListGitlabSyncActionsPageParams{
		RunID: runID, AfterSeq: afterSeq, LimitN: limit,
	})
	if err != nil {
		fail(c, err)
		return
	}
	if rows == nil {
		rows = []db.ListGitlabSyncActionsPageRow{}
	}
	hasMore := int32(len(rows)) == limit
	var nextAfterSeq *int32
	if hasMore {
		s := rows[len(rows)-1].Seq
		nextAfterSeq = &s
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "has_more": hasMore, "next_after_seq": nextAfterSeq})
}

// GetGitlabSyncActionDetail returns one action's before/after diff JSONB, scoped
// to the workspace's bindings. Backs the journal's lazy "expand this row" fetch,
// so the list stays light and only an inspected action pays for its diff.
func (h *API) GetGitlabSyncActionDetail(c *gin.Context) {
	wsID, ok := h.journalWorkspace(c)
	if !ok {
		return
	}
	actionID, ok := parseID(c, "actionId")
	if !ok {
		return
	}
	detail, err := h.q.GetGitlabSyncActionDetail(c, db.GetGitlabSyncActionDetailParams{
		ActionID: actionID, WorkspaceID: wsID,
	})
	if err != nil {
		if notFound(c, err) {
			return
		}
		fail(c, err)
		return
	}
	raw := json.RawMessage(detail)
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	c.JSON(http.StatusOK, gin.H{"detail": raw})
}

// RetryGitlabWriteback re-enqueues a failed push action by re-creating its outbox
// row from the recorded payload, so the worker delivers it again.
func (h *API) RetryGitlabWriteback(c *gin.Context) {
	wsID, ok := h.journalWorkspace(c)
	if !ok {
		return
	}
	actionID, ok := parseID(c, "actionId")
	if !ok {
		return
	}
	action, err := h.q.GetGitlabSyncActionInWorkspace(c, db.GetGitlabSyncActionInWorkspaceParams{ID: actionID, WorkspaceID: wsID})
	if err != nil {
		if notFound(c, err) {
			return
		}
		fail(c, err)
		return
	}
	if action.Direction != "push" || action.Status != "fail" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only failed push actions can be retried"})
		return
	}
	var detail struct {
		ChangeKind string          `json:"change_kind"`
		Payload    json.RawMessage `json:"payload"`
	}
	_ = json.Unmarshal(action.Detail, &detail)
	if action.TaskID == nil || detail.ChangeKind == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action is not replayable"})
		return
	}
	payload := detail.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	if err := h.q.CreateGitlabWriteback(c, db.CreateGitlabWritebackParams{
		TaskID: *action.TaskID, IntegrationID: action.RunIntegrationID, ChangeKind: detail.ChangeKind, Payload: payload,
	}); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "queued"})
}
