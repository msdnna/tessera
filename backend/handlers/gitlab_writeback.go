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
//   state    {"state": "closed"|"opened"}
//   priority {"priority": <int>}
//   comment  {"comment_id": "<uuid>", "body": "<text>"}
//   labels   {} — worker reconciles the task's current tags vs. the issue's labels
//   due      {} — worker pushes the task's current due_date (empty clears it)
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
	if !wb.Allows(kind) || integ.OwnerUserID == nil {
		return
	}

	rules := parseRules(integ.LabelRules)
	// Label write-back needs tag names to round-trip to full label titles; skip
	// queuing doomed rows when the prefix is stripped.
	if kind == "labels" && !rules.TagsInvertible() {
		return
	}
	_, prioInvertible := rules.InversePriority()
	if !shouldPushWriteback(kind, payload, link.GlLastState, prioInvertible) {
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
	// Coalesce a burst of same-kind edits into one pending row (latest wins).
	// Comments are distinct events and must never be merged.
	if kind != "comment" {
		if n, cerr := h.q.CoalescePendingWriteback(ctx, db.CoalescePendingWritebackParams{
			TaskID: taskID, ChangeKind: kind, Payload: raw,
		}); cerr == nil && n > 0 {
			return
		}
	}
	if cerr := h.q.CreateGitlabWriteback(ctx, db.CreateGitlabWritebackParams{
		TaskID: taskID, IntegrationID: link.IntegrationID, ChangeKind: kind, Payload: raw,
	}); cerr != nil {
		log.Printf("gitlab writeback: enqueue %s for task %s failed: %v", kind, taskID, cerr)
	}
}

// shouldPushWriteback is the pure loop-guard: given a change kind, its payload,
// the link's last-known GitLab state, and whether the priority mapping inverts,
// it decides whether the change is worth pushing. Never push a value GitLab
// already has (state echo); skip priority when no single label can be formed.
func shouldPushWriteback(kind string, payload map[string]any, lastState string, prioInvertible bool) bool {
	switch kind {
	case "state":
		s, _ := payload["state"].(string)
		return s != "" && s != lastState
	case "priority":
		return prioInvertible
	case "comment", "labels", "due", "assignees", "estimate", "milestone", "title_desc":
		// Loop-safe: only user-side handlers enqueue these (the pull uses a Nil
		// actor); the worker reads the latest task state at push time. title_desc is
		// additionally three-way conflict-checked before the push.
		return true
	default:
		return false
	}
}

// RunGitlabWriteBackWorker drains the write-back outbox on a timer: claims due
// pending rows, pushes each to GitLab, and marks it sent / retried / failed.
// Blocks until ctx is done; start it in a goroutine. Idle (a cheap claim query)
// until a user enables write-back.
func (h *API) RunGitlabWriteBackWorker(ctx context.Context) {
	ticker := time.NewTicker(writebackWorkerTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.drainWritebacks(ctx)
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
	case "state":
		state := "открыто"
		if s, _ := payload["state"].(string); s == "closed" {
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
	default:
		return prefix + ": " + kind
	}
}

// settleWriteback marks a row sent, retried (quadratic backoff) or failed. Claim
// already bumped attempts, so w.Attempts is this attempt's number.
func (h *API) settleWriteback(ctx context.Context, w db.GitlabWriteback, err error) {
	if err == nil {
		_ = h.q.MarkWritebackSent(ctx, w.ID)
		return
	}
	if isPermanentWriteback(err) || int(w.Attempts) >= maxWritebackAttempts {
		_ = h.q.MarkWritebackFailed(ctx, db.MarkWritebackFailedParams{ID: w.ID, LastError: truncErr(err)})
		log.Printf("gitlab writeback: %s for task %s gave up after %d attempt(s): %v", w.ChangeKind, w.TaskID, w.Attempts, err)
		return
	}
	next := time.Now().Add(time.Duration(w.Attempts*w.Attempts) * time.Minute) // 1, 4, 9, 16 min
	_ = h.q.MarkWritebackRetry(ctx, db.MarkWritebackRetryParams{ID: w.ID, LastError: truncErr(err), NextAttemptAt: next})
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
	if !wb.Allows(w.ChangeKind) {
		return res, notify.Permanent(fmt.Errorf("write-back %q is disabled", w.ChangeKind))
	}
	if integ.OwnerUserID == nil {
		return res, notify.Permanent(errors.New("integration has no owner credential"))
	}
	cred, err := h.q.GetGitlabCredential(ctx, *integ.OwnerUserID)
	if err != nil {
		return res, notify.Permanent(fmt.Errorf("owner credential gone: %w", err))
	}
	token, err := h.sealer.Decrypt(cred.TokenEnc)
	if err != nil {
		return res, notify.Permanent(fmt.Errorf("decrypt token: %w", err))
	}
	client := gitlab.New(cred.BaseUrl, token)
	path, iid := link.GlProjectPath, link.GlIid
	res.wsID = integ.WorkspaceID

	var payload map[string]any
	_ = json.Unmarshal(w.Payload, &payload)

	// Conflict gate: for three-way-checked kinds, fetch the current issue and decide
	// whether to push (baseline clean), no-op (already in sync), or park as a conflict.
	if conflictCheckedKind(w.ChangeKind) {
		issues, ferr := client.IssuesByIIDs(ctx, path, []string{strconv.FormatInt(iid, 10)})
		if ferr != nil {
			return res, ferr
		}
		if len(issues) == 0 {
			return res, notify.Permanent(errors.New("issue not found"))
		}
		decision, fields, derr := h.evalWritebackConflict(ctx, w, link, issues[0], integ.WorkspaceID, parseRules(integ.LabelRules))
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

	switch w.ChangeKind {
	case "state":
		// Read the live task state (not the enqueue payload) so a conflict resolved
		// in our favour pushes the task's current open/closed, never a stale value.
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return res, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		event, outcome := "reopen", "issue reopened"
		if task.CompletedAt != nil {
			event, outcome = "close", "issue closed"
		}
		if err := client.UpdateIssueState(ctx, path, iid, event); err != nil {
			return res, err
		}
		res.result = outcome
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "priority":
		rules := parseRules(integ.LabelRules)
		inv, ok := rules.InversePriority()
		if !ok {
			return res, notify.Permanent(errors.New("priority label mapping is not invertible"))
		}
		// Live task priority (not the enqueue payload), for the same reason as state.
		task, terr := h.q.GetTask(ctx, w.TaskID)
		if terr != nil {
			return res, notify.Permanent(fmt.Errorf("task gone: %w", terr))
		}
		prio := task.Priority
		add, ok := inv[prio]
		if !ok {
			return res, notify.Permanent(fmt.Errorf("no GitLab label for priority %d", prio))
		}
		var remove []string
		for _, lbl := range rules.AllPriorityLabels() {
			if lbl != add {
				remove = append(remove, lbl)
			}
		}
		if err := client.SetIssueLabels(ctx, path, iid, []string{add}, remove); err != nil {
			return res, err
		}
		res.result = "label «" + add + "» set"
		if len(remove) > 0 {
			res.result += ", removed " + strings.Join(remove, ", ")
		}
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "labels":
		rules := parseRules(integ.LabelRules)
		if !rules.TagsInvertible() {
			return res, notify.Permanent(errors.New("tag labels are not invertible (prefix stripped)"))
		}
		// Diff the issue's current tag-namespace labels against the task's tags;
		// status/priority labels are excluded (owned by their own write-back path).
		issues, err := client.IssuesByIIDs(ctx, path, []string{strconv.FormatInt(iid, 10)})
		if err != nil {
			return res, err
		}
		if len(issues) == 0 {
			return res, notify.Permanent(errors.New("issue not found"))
		}
		current := map[string]bool{}
		for _, l := range issues[0].Labels {
			if t := strings.TrimSpace(l.Title); rules.TagLabelClass(t) {
				current[t] = true
			}
		}
		tags, err := h.q.ListTaskTags(ctx, w.TaskID)
		if err != nil {
			return res, err // transient: retry
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
			res.result = "labels already in sync"
			return res, nil
		}
		if err := client.SetIssueLabels(ctx, path, iid, add, remove); err != nil {
			return res, err
		}
		res.result = "labels reconciled"
		if len(add) > 0 {
			res.result += " +[" + strings.Join(add, ", ") + "]"
		}
		if len(remove) > 0 {
			res.result += " -[" + strings.Join(remove, ", ") + "]"
		}
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "assignees":
		// Replace the issue's assignees with the resolved Tessera-side set:
		// Tessera-user assignees that have a connected GitLab account (their numeric
		// gl_user_id) + user-pinned GitLab members (resolved via the members table).
		// GitLab's assignee_ids is replace-all, so unresolvable assignees drop out.
		ids := map[int64]bool{}
		if tas, err := h.q.ListTaskAssignees(ctx, w.TaskID); err == nil {
			for _, a := range tas {
				if cred, cerr := h.q.GetGitlabCredential(ctx, a.ID); cerr == nil && cred.GlUserID != 0 {
					ids[cred.GlUserID] = true
				}
			}
		} else {
			return res, err // transient
		}
		if gas, err := h.q.ListTaskGitlabAssignees(ctx, w.TaskID); err == nil {
			for _, g := range gas {
				if uid, gerr := h.q.GetGitlabMemberIDByUsername(ctx, db.GetGitlabMemberIDByUsernameParams{
					IntegrationID: integ.ID, GlUsername: g.GlUsername,
				}); gerr == nil && uid != 0 {
					ids[uid] = true
				}
			}
		} else {
			return res, err // transient
		}
		list := make([]int64, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		if err := client.SetIssueAssignees(ctx, path, iid, list); err != nil {
			return res, err
		}
		res.result = fmt.Sprintf("assignees set (%d)", len(list))
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "estimate":
		// Only meaningful when the board's estimation unit is time; GitLab's
		// timeEstimate is seconds, our canon is minutes.
		if h.integrationEstimationUnit(ctx, integ) != "time" {
			return res, notify.Permanent(errors.New("estimate write-back needs a time estimation unit"))
		}
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return res, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		var minutes int64
		if task.Estimate != nil {
			minutes = int64(*task.Estimate)
		}
		if err := client.SetIssueTimeEstimate(ctx, path, iid, minutes); err != nil {
			return res, err
		}
		if minutes <= 0 {
			res.result = "estimate cleared"
		} else {
			res.result = fmt.Sprintf("estimate → %dm", minutes)
		}
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "due":
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return res, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		date := ""
		if task.DueDate != nil {
			date = task.DueDate.UTC().Format("2006-01-02")
		}
		if err := client.UpdateIssueDueDate(ctx, path, iid, date); err != nil {
			return res, err
		}
		if date == "" {
			res.result = "due date cleared"
		} else {
			res.result = "due date → " + date
		}
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "comment":
		body, _ := payload["body"].(string)
		if strings.TrimSpace(body) == "" {
			res.result = "empty comment skipped"
			return res, nil
		}
		gid, err := client.CreateIssueNote(ctx, path, iid, body)
		if err != nil {
			return res, err
		}
		res.result = "note posted"
		// Tag the originating comment with the new note id so the next pull updates
		// that row (ON CONFLICT gl_note_id) instead of inserting a duplicate.
		if gid != "" {
			if cidStr, _ := payload["comment_id"].(string); cidStr != "" {
				if cid, perr := uuid.Parse(cidStr); perr == nil {
					_ = h.q.SetCommentGlNoteID(ctx, db.SetCommentGlNoteIDParams{ID: cid, GlNoteID: &gid})
				}
			}
		}

	case "milestone":
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return res, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		var milestoneID int64 // 0 clears the issue's milestone
		if task.MilestoneID != nil {
			ml, lerr := h.q.GetGitlabMilestoneLink(ctx, *task.MilestoneID)
			if lerr != nil {
				// Native (non-GitLab) milestone — nothing to push, not an error.
				res.result = "native milestone, not pushed"
				return res, nil
			}
			milestoneID = ml.GlNumericID
		}
		if err := client.SetIssueMilestone(ctx, path, iid, milestoneID); err != nil {
			return res, err
		}
		if milestoneID == 0 {
			res.result = "milestone cleared"
		} else {
			res.result = "milestone set"
		}
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "title_desc":
		task, err := h.q.GetTask(ctx, w.TaskID)
		if err != nil {
			return res, notify.Permanent(fmt.Errorf("task gone: %w", err))
		}
		if err := client.UpdateIssueTitleDescription(ctx, path, iid, task.Title, task.Description); err != nil {
			return res, err
		}
		res.result = "title/description updated"
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	default:
		return res, notify.Permanent(fmt.Errorf("unknown change_kind %q", w.ChangeKind))
	}
	return res, nil
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
		TitleHash: hashStr(issue.Title), DescHash: hashStr(issue.Description),
		LabelsHash:        hashStr(labelsKey(issue.Labels)),
		GlAuthor:          issue.AuthorLogin, GlAuthorName: issue.AuthorName,
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
