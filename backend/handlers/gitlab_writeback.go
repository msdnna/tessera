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

	_, prioInvertible := parseRules(integ.LabelRules).InversePriority()
	if !shouldPushWriteback(kind, payload, link.GlLastState, prioInvertible) {
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
	case "comment":
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
	for _, w := range rows {
		h.settleWriteback(ctx, w, h.performWriteback(ctx, w))
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
func (h *API) performWriteback(ctx context.Context, w db.GitlabWriteback) error {
	link, err := h.q.GetGitlabLinkByTask(ctx, w.TaskID)
	if err != nil {
		return notify.Permanent(fmt.Errorf("task no longer linked: %w", err))
	}
	integ, err := h.q.GetGitlabIntegrationByID(ctx, w.IntegrationID)
	if err != nil {
		return notify.Permanent(fmt.Errorf("integration gone: %w", err))
	}
	wb := parseWriteback(integ.Writeback)
	if !wb.Allows(w.ChangeKind) {
		return notify.Permanent(fmt.Errorf("write-back %q is disabled", w.ChangeKind))
	}
	if integ.OwnerUserID == nil {
		return notify.Permanent(errors.New("integration has no owner credential"))
	}
	cred, err := h.q.GetGitlabCredential(ctx, *integ.OwnerUserID)
	if err != nil {
		return notify.Permanent(fmt.Errorf("owner credential gone: %w", err))
	}
	token, err := h.sealer.Decrypt(cred.TokenEnc)
	if err != nil {
		return notify.Permanent(fmt.Errorf("decrypt token: %w", err))
	}
	client := gitlab.New(cred.BaseUrl, token)
	path, iid := link.GlProjectPath, link.GlIid

	var payload map[string]any
	_ = json.Unmarshal(w.Payload, &payload)

	switch w.ChangeKind {
	case "state":
		event := "reopen"
		if s, _ := payload["state"].(string); s == "closed" {
			event = "close"
		}
		if err := client.UpdateIssueState(ctx, path, iid, event); err != nil {
			return err
		}
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "priority":
		rules := parseRules(integ.LabelRules)
		inv, ok := rules.InversePriority()
		if !ok {
			return notify.Permanent(errors.New("priority label mapping is not invertible"))
		}
		var prio int32
		if f, fok := payload["priority"].(float64); fok {
			prio = int32(f)
		}
		add, ok := inv[prio]
		if !ok {
			return notify.Permanent(fmt.Errorf("no GitLab label for priority %d", prio))
		}
		var remove []string
		for _, lbl := range rules.AllPriorityLabels() {
			if lbl != add {
				remove = append(remove, lbl)
			}
		}
		if err := client.SetIssueLabels(ctx, path, iid, []string{add}, remove); err != nil {
			return err
		}
		h.refreshLinkSnapshot(ctx, client, integ, w.TaskID, path, iid)

	case "comment":
		body, _ := payload["body"].(string)
		if strings.TrimSpace(body) == "" {
			return nil
		}
		gid, err := client.CreateIssueNote(ctx, path, iid, body)
		if err != nil {
			return err
		}
		// Tag the originating comment with the new note id so the next pull updates
		// that row (ON CONFLICT gl_note_id) instead of inserting a duplicate.
		if gid != "" {
			if cidStr, _ := payload["comment_id"].(string); cidStr != "" {
				if cid, perr := uuid.Parse(cidStr); perr == nil {
					_ = h.q.SetCommentGlNoteID(ctx, db.SetCommentGlNoteIDParams{ID: cid, GlNoteID: &gid})
				}
			}
		}

	default:
		return notify.Permanent(fmt.Errorf("unknown change_kind %q", w.ChangeKind))
	}
	return nil
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
}
