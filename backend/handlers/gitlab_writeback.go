package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/internal/notify"
)

// GitLab write-back (phase B): push Tessera-side changes back to the linked issue.
// The flow is fully async — task handlers only enqueue; a background worker drains
// the gitlab_writebacks outbox (mirroring the notification delivery outbox), so a
// GitLab outage never blocks a mutation, and pushes retry with backoff.

const (
	writebackWorkerTick  = 10 * time.Second
	writebackBatchSize   = 20
	maxWritebackAttempts = 5
)

// enqueueWriteback queues a change for push to GitLab when the task is linked, the
// integration has write-back enabled for this kind, and the change isn't already
// reflected in GitLab (loop-guard). Best-effort: a failure here never breaks the
// user-facing mutation. actorID==Nil (system/sync) is skipped — the pull never
// touches the enqueue-ing handlers, so this is just belt-and-suspenders.
//
// payload is kind-specific:
//
//	state    {"state": "closed"|"opened"}
//	priority {"priority": <int>}
//	comment  {"comment_id": "<uuid>", "body": "<text>"}
//	labels   {} — worker reconciles the task's current tags vs. the issue's labels
//	due      {} — worker pushes the task's current due_date (empty clears it)
func (h *API) enqueueWriteback(ctx context.Context, taskID, actorID uuid.UUID, kind string, payload map[string]any) {
	if actorID == uuid.Nil {
		return
	}
	link, err := h.q.GetGitlabLinkByTask(ctx, taskID)
	if err != nil {
		return // not linked (or transient) — nothing to push
	}
	integ, err := h.q.GetGitlabIntegrationByID(ctx, link.IntegrationID)
	if err != nil {
		return
	}
	wb := parseWriteback(integ.Writeback)
	if integ.OwnerUserID == nil {
		return
	}

	rules := parseRules(integ.LabelRules)
	// Gate on the binding table: skip enqueuing a trigger no binding consumes.
	// This subsumes the old per-kind Allows() check and the priority-invertibility
	// guard (a non-invertible priority synthesizes no binding → resolves to nothing).
	if len(wb.ResolveActions(triggerFromKind(kind, payload), rules)) == 0 {
		return
	}
	// Label reconcile needs tag names to round-trip to full label titles; skip
	// queuing doomed rows when the prefix is stripped.
	if kind == gitlab.TrigLabels && !rules.TagsInvertible() {
		return
	}
	if !shouldPushWriteback(kind, payload, link.GlLastState) {
		return
	}
	// An open conflict for this (task, kind) already represents the pending intent;
	// resolution re-pushes the task's live value, so don't stack a second row.
	if _, cerr := h.q.GetOpenConflict(ctx, db.GetOpenConflictParams{TaskID: taskID, ChangeKind: kind}); cerr == nil {
		return
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	// Attribute the row to the acting user so the worker can push under their GitLab
	// identity (personal PAT, else admin sudo). actorID is never Nil here (guarded above).
	actor := actorID
	// Coalesce a burst of same-kind edits into one pending row (latest wins, incl. actor).
	// Comments are distinct events and must never be merged.
	if kind != "comment" {
		if n, cerr := h.q.CoalescePendingWriteback(ctx, db.CoalescePendingWritebackParams{
			TaskID: taskID, ChangeKind: kind, Payload: raw, ActorUserID: &actor,
		}); cerr == nil && n > 0 {
			return
		}
	}
	if cerr := h.q.CreateGitlabWriteback(ctx, db.CreateGitlabWritebackParams{
		TaskID: taskID, IntegrationID: link.IntegrationID, ChangeKind: kind, Payload: raw,
		ActorUserID: &actor,
	}); cerr != nil {
		log.Printf("gitlab writeback: enqueue %s for task %s failed: %v", kind, taskID, cerr)
	}
}

// shouldPushWriteback is the pure loop-guard: never push a value GitLab already
// has. The only echo-prone trigger is completion→set_state (open/closed): skip when
// the resulting state already equals the link's last-known GitLab state. Every other
// trigger is loop-safe (the pull uses a Nil actor and never enqueues; the worker
// reads the latest task state at push time; title_desc is additionally three-way
// conflict-checked).
func shouldPushWriteback(kind string, payload map[string]any, lastState string) bool {
	if kind == gitlab.TrigCompletion || kind == "state" {
		completed, _ := payload["completed"].(bool)
		if s, ok := payload["state"].(string); ok { // legacy "state" payload
			return s != "" && s != lastState
		}
		return issueState(completed) != lastState
	}
	return true
}

// triggerFromKind builds a BindTrigger from an enqueue kind + payload — the fast,
// best-effort gate used at enqueue time. performWriteback re-derives the trigger
// from live task state (resolveTrigger) as the authoritative source.
func triggerFromKind(kind string, payload map[string]any) gitlab.BindTrigger {
	if kind == "state" {
		kind = gitlab.TrigCompletion // legacy alias
	}
	t := gitlab.BindTrigger{Type: kind}
	switch kind {
	case gitlab.TrigColumn:
		t.ColumnID, _ = payload["column_id"].(string)
		t.ColumnName, _ = payload["column_name"].(string)
	case gitlab.TrigPriority:
		// The payload is an in-memory map, not decoded JSON, so priority may
		// arrive as any integer type (handlers enqueue int32).
		switch v := payload["priority"].(type) {
		case float64:
			p := int32(v)
			t.Priority = &p
		case int32:
			p := v
			t.Priority = &p
		case int:
			p := int32(v)
			t.Priority = &p
		}
	case gitlab.TrigCompletion:
		if b, ok := payload["completed"].(bool); ok {
			t.Completed = &b
		} else if s, ok := payload["state"].(string); ok { // legacy payload
			b := s == "closed"
			t.Completed = &b
		}
	case gitlab.TrigDue:
		if dk, _ := payload["date_kind"].(string); dk != "" {
			t.DateKind = dk
		} else {
			t.DateKind = "due"
		}
	}
	return t
}

// RunGitlabWriteBackWorker drains the write-back outbox on a timer: claims due
// pending rows, pushes each to GitLab, and marks it sent / retried / failed.
// Blocks until ctx is done; start it in a goroutine. Idle (a cheap claim query)
// until a user enables write-back.
func (h *API) RunGitlabWriteBackWorker(ctx context.Context) {
	ticker := time.NewTicker(writebackWorkerTick)
	defer ticker.Stop()
	h.tick(jobGitlabWriteback, "выгрузка изменений в GitLab")
	h.withAdvisoryLock(ctx, "gitlab_writeback", func() { h.drainWritebacks(ctx) }) // drain backlog at startup
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobGitlabWriteback, "выгрузка изменений в GitLab")
			h.withAdvisoryLock(ctx, "gitlab_writeback", func() { h.drainWritebacks(ctx) })
		}
	}
}

func (h *API) drainWritebacks(ctx context.Context) {
	rows, err := h.q.ClaimPendingWritebacks(ctx, writebackBatchSize)
	if err != nil || len(rows) == 0 {
		return
	}
	// One push run per integration touched in this drain cycle, recording every
	// delivery attempt (and its outcome) so failures are visible in the journal.
	journals := map[uuid.UUID]*syncJournal{}
	for _, w := range rows {
		j := journals[w.IntegrationID]
		if j == nil {
			j = h.newJournal(w.IntegrationID, "push", "auto", nil)
			journals[w.IntegrationID] = j
		}
		res, perr := h.performWriteback(ctx, w)
		if res.conflict {
			// Parked by performWriteback; don't settle (the row is now 'conflict').
			h.recordConflictAction(j, w, res, res.fields)
			if res.wsID != uuid.Nil {
				h.broadcast(res.wsID, "gitlab.conflict", map[string]any{
					"task_id": w.TaskID, "change_kind": w.ChangeKind, "fields": res.fields,
				})
			}
			continue
		}
		h.settleWriteback(ctx, w, perr)
		h.recordWritebackAction(j, w, res, perr)
	}
	for _, j := range journals {
		h.flushJournal(context.WithoutCancel(ctx), j)
	}
}

// writebackResult captures what a push delivery did, for the sync journal.
type writebackResult struct {
	glIid    int64           // the target GitLab issue iid (0 if not yet resolved)
	result   string          // human-readable outcome, e.g. "issue closed", "label set"
	wsID     uuid.UUID       // owning workspace (for realtime broadcast)
	conflict bool            // true when the push was parked as a conflict (don't settle)
	fields   []conflictField // diverged fields, when conflict
}

// recordWritebackAction appends a journal action for one push delivery, capturing
// the payload, the outcome and any error (so retry-from-UI can re-enqueue it).
func (h *API) recordWritebackAction(j *syncJournal, w db.GitlabWriteback, res writebackResult, err error) {
	var payload map[string]any
	_ = json.Unmarshal(w.Payload, &payload)
	var iidPtr *int64
	if res.glIid != 0 {
		iid := res.glIid
		iidPtr = &iid
	}
	detail := map[string]any{
		"change_kind":    w.ChangeKind,
		"payload":        payload,
		"writeback_id":   w.ID,
		"integration_id": w.IntegrationID,
	}
	tid := w.TaskID
	action := journalAction{
		Direction: "push", EntityType: w.ChangeKind, Op: "push",
		TaskID: &tid, GlIid: iidPtr, Detail: detail,
	}
	if err != nil {
		action.Status = "fail"
		action.Err = truncErr(err)
		detail["error"] = action.Err
		action.Summary = pushSummary(w.ChangeKind, payload, iidPtr) + " — ошибка"
	} else {
		detail["result"] = res.result
		action.Summary = pushSummary(w.ChangeKind, payload, iidPtr)
	}
	j.add(action)
}

// pushSummary builds the one-line label for a push action.
func pushSummary(kind string, payload map[string]any, iidPtr *int64) string {
	prefix := "Issue"
	if iidPtr != nil {
		prefix = "Issue #" + strconv.FormatInt(*iidPtr, 10)
	}
	switch kind {
	case "column":
		return prefix + ": метка статуса"
	case "completion", "state":
		state := "открыто"
		if c, _ := payload["completed"].(bool); c {
			state = "закрыто"
		} else if s, _ := payload["state"].(string); s == "closed" { // legacy payload
			state = "закрыто"
		}
		return prefix + ": состояние → " + state
	case "priority":
		return prefix + ": приоритет"
	case "comment":
		return prefix + ": комментарий"
	case "labels":
		return prefix + ": метки"
	case "due":
		return prefix + ": срок"
	case "assignees":
		return prefix + ": исполнители"
	case "estimate":
		return prefix + ": оценка"
	case "milestone":
		return prefix + ": этап"
	case "title_desc":
		return prefix + ": заголовок/описание"
	case gitlab.KindChildCreate:
		return prefix + ": подзадача в GitLab"
	case gitlab.KindChildAttach:
		return prefix + ": привязка к родителю"
	case gitlab.KindChildDetach:
		return prefix + ": открепление от родителя"
	default:
		return prefix + ": " + kind
	}
}

// settleWriteback marks a row sent, retried (quadratic backoff) or failed. Claim
// already bumped attempts, so w.Attempts is this attempt's number.
func (h *API) settleWriteback(ctx context.Context, w db.GitlabWriteback, err error) {
	if err == nil {
		soft(ctx, "MarkWritebackSent", h.q.MarkWritebackSent(ctx, w.ID))
		return
	}
	if isPermanentWriteback(err) || int(w.Attempts) >= maxWritebackAttempts {
		soft(ctx, "MarkWritebackFailed", h.q.MarkWritebackFailed(ctx, db.MarkWritebackFailedParams{ID: w.ID, LastError: truncErr(err)}))
		log.Printf("gitlab writeback: %s for task %s gave up after %d attempt(s): %v", w.ChangeKind, w.TaskID, w.Attempts, err)
		return
	}
	next := time.Now().Add(time.Duration(w.Attempts*w.Attempts) * time.Minute) // 1, 4, 9, 16 min
	soft(ctx, "MarkWritebackRetry", h.q.MarkWritebackRetry(ctx, db.MarkWritebackRetryParams{ID: w.ID, LastError: truncErr(err), NextAttemptAt: next}))
}

// isPermanentWriteback classifies a push error as non-retryable: an explicitly
// permanent error, or a GitLab 4xx that won't fix itself (except 429 rate-limit).
func isPermanentWriteback(err error) bool {
	if notify.IsPermanent(err) {
		return true
	}
	var ae *gitlab.APIError
	if errors.As(err, &ae) {
		return ae.Status >= 400 && ae.Status < 500 && ae.Status != http.StatusTooManyRequests
	}
	return false
}

// performWriteback executes one outbox row against GitLab using the integration
// owner's credential. Returns nil on success, a permanent error for unrecoverable
// states (unlinked task, disabled config, bad credential), or a transient error
// (network / GitLab 5xx) to retry.
func (h *API) performWriteback(ctx context.Context, w db.GitlabWriteback) (writebackResult, error) {
	var res writebackResult
	// Structural child pushes branch out before the link lookup below: a child_create
	// row exists precisely because its task has NO link yet, so falling through would
	// give up on it permanently ("task no longer linked") on the very first attempt.
	if gitlab.IsChildKind(w.ChangeKind) {
		return h.performChildWriteback(ctx, w)
	}
	link, err := h.q.GetGitlabLinkByTask(ctx, w.TaskID)
	if err != nil {
		return res, notify.Permanent(fmt.Errorf("task no longer linked: %w", err))
	}
	res.glIid = link.GlIid
	integ, err := h.q.GetGitlabIntegrationByID(ctx, w.IntegrationID)
	if err != nil {
		return res, notify.Permanent(fmt.Errorf("integration gone: %w", err))
	}
	wb := parseWriteback(integ.Writeback)
	// Resolve the write connection attributed to the acting user (task #2690): the
	// actor's own PAT, else the service token with a `Sudo:` header (admin sudo), else
	// the plain service token / owner PAT. The pull stays on the service token — only
	// writes carry the actor's identity.
	baseURL, token, sudoUser, ok := h.writeGitlabConn(ctx, w.ActorUserID, integ.OwnerUserID)
	if !ok {
		return res, notify.Permanent(errors.New("no GitLab credential available (personal PAT or service token)"))
	}
	client := gitlab.New(baseURL, token).WithSudo(sudoUser)
	path, iid := link.GlProjectPath, link.GlIid
	res.wsID = integ.WorkspaceID

	var payload map[string]any
	_ = json.Unmarshal(w.Payload, &payload)

	// Resolve the trigger from LIVE task state (not the stale enqueue payload) so a
	// conflict resolved in our favour, or a coalesced burst of moves, pushes the
	// task's current value. Then map the trigger to 0..N bound actions.
	rules := parseRules(integ.LabelRules)
	trigger, terr := h.resolveTrigger(ctx, w, payload)
	if terr != nil {
		return res, terr
	}
	actions := wb.ResolveActions(trigger, rules)
	if len(actions) == 0 {
		// Config changed since enqueue (binding removed/disabled) — nothing to do.
		res.result = "нет подходящего биндинга"
		return res, nil
	}

	// Conflict gate: for three-way-checked triggers, fetch the current issue and decide
	// whether to push (baseline clean), no-op (already in sync), or park as a conflict.
	if conflictCheckedKind(w.ChangeKind) {
		issues, ferr := client.IssuesByIIDs(ctx, path, []string{strconv.FormatInt(iid, 10)})
		if ferr != nil {
			return res, ferr
		}
		if len(issues) == 0 {
			return res, notify.Permanent(errors.New("issue not found"))
		}
		decision, fields, derr := h.evalWritebackConflict(ctx, w, link, issues[0], integ.WorkspaceID, rules)
		if derr != nil {
			return res, derr
		}
		switch decision {
		case conflictNoop:
			res.result = "уже синхронно с GitLab"
			return res, nil
		case conflictParked:
			if perr := h.parkConflict(ctx, w, fields); perr != nil {
				return res, perr
			}
			res.conflict, res.fields = true, fields
			res.result = "конфликт — ожидает решения"
			return res, nil
		}
		// conflictProceed: fall through to the push.
	}

	// Fan out: one trigger occurrence may bind to several actions (e.g. a column
	// move that sets multiple labels). Any error aborts the row for retry.
	var results []string
	for _, act := range actions {
		out, aerr := h.performAction(ctx, client, integ, w, path, iid, act, wb, rules, payload)
		if aerr != nil {
			return res, aerr
		}
		if out != "" {
			results = append(results, out)
		}
	}
	res.result = strings.Join(results, "; ")
	return res, nil
}

// tesseraCommentMarker is the optional footer appended to a pushed comment when the
// binding sets AddMarker, so a GitLab reader can tell the note originated in Tessera.
const tesseraCommentMarker = "\n\n---\n_Отправлено через Tessera_"

// resolveTrigger rebuilds the BindTrigger from the live task state (authoritative at
// push time), falling back to the payload for fields with no live source (comment
// body, due kind). "state" is the legacy alias for the "completion" trigger.
func (h *API) resolveTrigger(ctx context.Context, w db.GitlabWriteback, payload map[string]any) (gitlab.BindTrigger, error) {
	kind := w.ChangeKind
	if kind == "state" {
		kind = gitlab.TrigCompletion
	}
	t := gitlab.BindTrigger{Type: kind}
	switch kind {
	case gitlab.TrigColumn:
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return t, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		t.ColumnID = task.ColumnID.String()
		if col, cerr := h.q.GetColumn(ctx, task.ColumnID); cerr == nil {
			t.ColumnName = col.Name
		}
	case gitlab.TrigPriority:
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return t, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		p := task.Priority
		t.Priority = &p
	case gitlab.TrigCompletion:
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return t, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		b := task.CompletedAt != nil
		t.Completed = &b
	case gitlab.TrigDue:
		if dk, _ := payload["date_kind"].(string); dk != "" {
			t.DateKind = dk
		} else {
			t.DateKind = "due"
		}
	}
	return t, nil
}

// performAction dispatches one resolved action to its GitLab-side executor. Each
// executor reuses the existing client calls + refreshLinkSnapshot; extracting the
// old switch cases verbatim keeps behaviour identical under the default synthesis.
func (h *API) performAction(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, w db.GitlabWriteback, path string, iid int64, act gitlab.BindAction, wb gitlab.Writeback, rules gitlab.Rules, payload map[string]any) (string, error) {
	switch act.Type {
	case gitlab.ActSetState:
		return h.performSetState(ctx, client, integ, w.TaskID, path, iid, act)
	case gitlab.ActSetLabel:
		return h.performSetLabel(ctx, client, integ, w.TaskID, path, iid, act, wb, rules)
	case gitlab.ActReconcileLabels:
		return h.performReconcileLabels(ctx, client, integ, w.TaskID, path, iid, rules)
	case gitlab.ActSetDue:
		return h.performSetDue(ctx, client, integ, w.TaskID, path, iid)
	case gitlab.ActSetAssignees:
		return h.performSetAssignees(ctx, client, integ, w.TaskID, path, iid)
	case gitlab.ActSetEstimate:
		return h.performSetEstimate(ctx, client, integ, w.TaskID, path, iid)
	case gitlab.ActSetMilestone:
		return h.performSetMilestone(ctx, client, integ, w.TaskID, path, iid)
	case gitlab.ActPostComment:
		return h.performPostComment(ctx, client, path, iid, act, payload)
	case gitlab.ActSetTitleDesc:
		return h.performSetTitleDesc(ctx, client, integ, w.TaskID, path, iid)
	default:
		return "", notify.Permanent(fmt.Errorf("unknown action %q", act.Type))
	}
}

// performSetState closes or reopens the issue. act.State forces a state; the empty
// default derives open/closed from the live completion flag (legacy behaviour).
func (h *API) performSetState(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64, act gitlab.BindAction) (string, error) {
	var closed bool
	switch act.State {
	case "closed":
		closed = true
	case "opened":
		closed = false
	default:
		task, err := h.q.GetTask(ctx, taskID)
		if err != nil {
			return "", notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		closed = task.CompletedAt != nil
	}
	event, outcome := "reopen", "issue переоткрыт"
	if closed {
		event, outcome = "close", "issue закрыт"
	}
	if err := client.UpdateIssueState(ctx, path, iid, event); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	return outcome, nil
}

// performSetLabel sets one label and (when ClearPrefix) removes its same-namespace
// siblings, so status/priority labels stay mutually exclusive. This is the flagship
// column→label path; it also serves synthesized per-value priority labels.
func (h *API) performSetLabel(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64, act gitlab.BindAction, wb gitlab.Writeback, rules gitlab.Rules) (string, error) {
	label := strings.TrimSpace(act.Label)
	if label == "" {
		return "пустая метка пропущена", nil
	}
	var remove []string
	if act.ClearPrefix {
		remove = wb.SiblingLabels(label, rules)
	}
	if err := client.SetIssueLabels(ctx, path, iid, []string{label}, remove); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	out := "метка «" + label + "» проставлена"
	if len(remove) > 0 {
		out += ", снято: " + strings.Join(remove, ", ")
	}
	return out, nil
}

// performReconcileLabels diffs the issue's tag-namespace labels against the task's
// tags (status/priority labels excluded — owned by their own paths).
func (h *API) performReconcileLabels(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64, rules gitlab.Rules) (string, error) {
	if !rules.TagsInvertible() {
		return "", notify.Permanent(errors.New("tag labels are not invertible (prefix stripped)"))
	}
	issues, err := client.IssuesByIIDs(ctx, path, []string{strconv.FormatInt(iid, 10)})
	if err != nil {
		return "", err
	}
	if len(issues) == 0 {
		return "", notify.Permanent(errors.New("issue not found"))
	}
	current := map[string]bool{}
	for _, l := range issues[0].Labels {
		if t := strings.TrimSpace(l.Title); rules.TagLabelClass(t) {
			current[t] = true
		}
	}
	tags, err := h.q.ListTaskTags(ctx, taskID)
	if err != nil {
		return "", err // transient: retry
	}
	desired := map[string]bool{}
	for _, tg := range tags {
		if t := strings.TrimSpace(tg.Name); t != "" && rules.TagLabelClass(t) {
			desired[t] = true
		}
	}
	var add, remove []string
	for t := range desired {
		if !current[t] {
			add = append(add, t)
		}
	}
	for t := range current {
		if !desired[t] {
			remove = append(remove, t)
		}
	}
	if len(add) == 0 && len(remove) == 0 {
		return "метки уже синхронны", nil
	}
	if err := client.SetIssueLabels(ctx, path, iid, add, remove); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	out := "метки согласованы"
	if len(add) > 0 {
		out += " +[" + strings.Join(add, ", ") + "]"
	}
	if len(remove) > 0 {
		out += " -[" + strings.Join(remove, ", ") + "]"
	}
	return out, nil
}

// performSetAssignees replaces the issue's assignees with the resolved Tessera-side
// set (connected Tessera users' gl_user_id + pinned GitLab members). Replace-all, so
// unresolvable assignees drop out.
func (h *API) performSetAssignees(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64) (string, error) {
	ids := map[int64]bool{}
	if tas, err := h.q.ListTaskAssignees(ctx, taskID); err == nil {
		for _, a := range tas {
			if gid, ok := h.assigneeGlUserID(ctx, a.ID); ok {
				ids[gid] = true
			}
		}
	} else {
		return "", err // transient
	}
	if gas, err := h.q.ListTaskGitlabAssignees(ctx, taskID); err == nil {
		for _, g := range gas {
			if uid, gerr := h.q.GetGitlabMemberIDByUsername(ctx, db.GetGitlabMemberIDByUsernameParams{
				IntegrationID: integ.ID, GlUsername: g.GlUsername,
			}); gerr == nil && uid != 0 {
				ids[uid] = true
			}
		}
	} else {
		return "", err // transient
	}
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	if err := client.SetIssueAssignees(ctx, path, iid, list); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	return fmt.Sprintf("исполнители проставлены (%d)", len(list)), nil
}

// performSetEstimate pushes the task estimate as the issue's timeEstimate (minutes;
// only when the board's estimation unit is time).
func (h *API) performSetEstimate(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64) (string, error) {
	if h.integrationEstimationUnit(ctx, integ) != "time" {
		return "", notify.Permanent(errors.New("estimate write-back needs a time estimation unit"))
	}
	task, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return "", notify.Permanent(fmt.Errorf("task gone: %w", err))
	}
	var minutes int64
	if task.Estimate != nil {
		minutes = int64(*task.Estimate)
	}
	if err := client.SetIssueTimeEstimate(ctx, path, iid, minutes); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	if minutes <= 0 {
		return "оценка очищена", nil
	}
	return fmt.Sprintf("оценка → %dм", minutes), nil
}

// performSetDue pushes the task's due date (empty clears it).
func (h *API) performSetDue(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64) (string, error) {
	task, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return "", notify.Permanent(fmt.Errorf("task gone: %w", err))
	}
	date := ""
	if task.DueDate != nil {
		date = task.DueDate.UTC().Format("2006-01-02")
	}
	if err := client.UpdateIssueDueDate(ctx, path, iid, date); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	if date == "" {
		return "срок очищен", nil
	}
	return "срок → " + date, nil
}

// performPostComment mirrors a Tessera comment to the issue's notes. The payload's
// "op" selects the effect: "edit"/"delete" act on the existing note (identified by
// the stored "gl_note_id" gid); the default posts a new note and tags the originating
// comment with the returned note id so the next pull dedups it.
func (h *API) performPostComment(ctx context.Context, client *gitlab.Client, path string, iid int64, act gitlab.BindAction, payload map[string]any) (string, error) {
	op, _ := payload["op"].(string)
	body, _ := payload["body"].(string)

	if op == "edit" || op == "delete" {
		noteID, ok := parseNoteGID(payload["gl_note_id"])
		if !ok {
			// No GitLab note yet (e.g. the create push hasn't run) — nothing to touch.
			return "нет комментария в GitLab — пропущено", nil
		}
		if op == "delete" {
			if err := client.DeleteIssueNote(ctx, path, iid, noteID); err != nil {
				return "", err
			}
			return "комментарий удалён", nil
		}
		if strings.TrimSpace(body) == "" {
			return "пустой комментарий пропущен", nil
		}
		if act.AddMarker {
			body += tesseraCommentMarker
		}
		if err := client.UpdateIssueNote(ctx, path, iid, noteID, body); err != nil {
			return "", err
		}
		return "комментарий отредактирован", nil
	}

	if strings.TrimSpace(body) == "" {
		return "пустой комментарий пропущен", nil
	}
	if act.AddMarker {
		body += tesseraCommentMarker
	}
	gid, err := client.CreateIssueNote(ctx, path, iid, body)
	if err != nil {
		return "", err
	}
	if gid != "" {
		if cidStr, _ := payload["comment_id"].(string); cidStr != "" {
			if cid, perr := uuid.Parse(cidStr); perr == nil {
				soft(ctx, "SetCommentGlNoteID", h.q.SetCommentGlNoteID(ctx, db.SetCommentGlNoteIDParams{ID: cid, GlNoteID: &gid}))
			}
		}
	}
	return "комментарий опубликован", nil
}

// parseNoteGID extracts the numeric REST note id from a stored GitLab note global id
// ("gid://gitlab/Note/<id>"). Returns false when the value is absent or malformed.
func parseNoteGID(v any) (int64, bool) {
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// performSetMilestone pushes the task's GitLab-linked milestone (0 clears it; a
// native milestone is a no-op, not an error).
func (h *API) performSetMilestone(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64) (string, error) {
	task, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return "", notify.Permanent(fmt.Errorf("task gone: %w", err))
	}
	var milestoneID int64 // 0 clears the issue's milestone
	if task.MilestoneID != nil {
		ml, lerr := h.q.GetGitlabMilestoneLink(ctx, *task.MilestoneID)
		if lerr != nil {
			return "нативный этап — не отправлено", nil
		}
		milestoneID = ml.GlNumericID
	}
	if err := client.SetIssueMilestone(ctx, path, iid, milestoneID); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	if milestoneID == 0 {
		return "этап очищен", nil
	}
	return "этап проставлен", nil
}

// performSetTitleDesc pushes the task title/description to the issue (conflict-checked
// upstream in performWriteback).
func (h *API) performSetTitleDesc(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64) (string, error) {
	task, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return "", notify.Permanent(fmt.Errorf("task gone: %w", err))
	}
	if err := client.UpdateIssueTitleDescription(ctx, path, iid, task.Title, task.Description); err != nil {
		return "", err
	}
	h.refreshLinkSnapshot(ctx, client, integ, taskID, path, iid)
	return "заголовок/описание обновлены", nil
}

// refreshLinkSnapshot re-fetches the one issue we just pushed to and rewrites the
// link's content snapshot (hashes + gl_last_state), so the next scheduled pull
// sees nothing remote-changed and doesn't echo our own change back. Best-effort:
// the push already succeeded, so a refresh failure isn't fatal.
func (h *API) refreshLinkSnapshot(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, taskID uuid.UUID, path string, iid int64) {
	issues, err := client.IssuesByIIDs(ctx, path, []string{strconv.FormatInt(iid, 10)})
	if err != nil || len(issues) == 0 {
		return
	}
	issue := issues[0]
	if _, uerr := h.q.UpdateGitlabLink(ctx, db.UpdateGitlabLinkParams{
		TaskID: taskID, GlIid: issue.IID, GlWebUrl: issue.WebURL, GlUpdatedAt: issue.UpdatedAt,
		TitleHash: gitlab.HashStr(issue.Title), DescHash: gitlab.HashStr(issue.Description),
		LabelsHash: gitlab.HashStr(gitlab.LabelsKey(issue.Labels)),
		GlAuthor:   issue.AuthorLogin, GlAuthorName: issue.AuthorName,
		GlAuthorAvatarUrl: h.avatarProxyURL(integ.WorkspaceID, issue.AuthorAvatar),
		GlLastState:       issue.State,
	}); uerr != nil {
		log.Printf("gitlab writeback: refresh link snapshot for task %s failed: %v", taskID, uerr)
	}
	// Refresh the conflict baseline so the next push sees nothing remote-changed.
	// Rewrite attachment links the same way the pull does, so the description
	// baseline matches what the task stores (avoids a false title_desc conflict).
	issue.Description = h.rewriteAssets(issue.Description, integ.WorkspaceID)
	if serr := h.q.SetGitlabLinkSnapshot(ctx, db.SetGitlabLinkSnapshotParams{
		TaskID: taskID, GlSnapshot: buildGlSnapshot(issue, parseRules(integ.LabelRules)),
	}); serr != nil {
		log.Printf("gitlab writeback: refresh conflict snapshot for task %s failed: %v", taskID, serr)
	}
}
