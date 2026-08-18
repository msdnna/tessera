package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/internal/notify"
	"tessera/middleware"
)

// The child half of GitLab issue hierarchy (#2592): when a subtask appears under a
// grouped, linked parent, mirror it into GitLab as a child work item.
//
// This goes through the write-back outbox rather than doing the work inline, for the
// same reason every other push does: creating an issue takes two GitLab round trips
// (REST create, then the hierarchy mutation), and neither may hold up — or fail — the
// mutation that created the subtask. What is different from every other outbox kind is
// that the task has no link yet, so the parent's link carries the integration.

// enqueueChildAttach queues "this task is now a subtask of parentID" for GitLab: a
// child issue creation when the task has no issue yet, a re-parent when it has one.
// Best-effort — a failure here never breaks the mutation that caused it.
func (h *API) enqueueChildAttach(ctx context.Context, childID uuid.UUID, parentID uuid.UUID, actorID uuid.UUID) {
	h.enqueueChildWriteback(ctx, childID, &parentID, actorID, "")
}

// enqueueChildDetach queues "this task is no longer a subtask" for GitLab. prevParentID
// is the parent it just left — recorded in the payload for the journal; the push itself
// only needs the child's own work item.
func (h *API) enqueueChildDetach(ctx context.Context, childID uuid.UUID, prevParentID uuid.UUID, actorID uuid.UUID) {
	h.enqueueChildWriteback(ctx, childID, &prevParentID, actorID, gitlab.KindChildDetach)
}

// enqueueChildWriteback is the shared body. kind may be empty for the attach direction,
// in which case it is decided from the child's link state (no link → create).
func (h *API) enqueueChildWriteback(ctx context.Context, childID uuid.UUID, parentID *uuid.UUID, actorID uuid.UUID, kind string) {
	if actorID == uuid.Nil {
		return // system/sync actor — the pull never pushes back
	}
	integ, parentLink, childLink, ok := h.resolveChildContext(ctx, childID, parentID)
	if !ok {
		return
	}
	if kind == "" {
		kind = childKind(childLink != nil)
	}
	wb := parseWriteback(integ.Writeback)
	if status, _ := checkChildGate(kind, integ.Enabled, wb, parentLink, childLink != nil); status != 0 {
		return
	}

	payload := map[string]any{}
	if parentID != nil {
		payload["parent_task_id"] = parentID.String()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	actor := actorID
	if kind == gitlab.KindChildCreate {
		// Idempotent insert: the partial unique index allows one in-flight creation per
		// task, and a second enqueue for the same subtask must collapse into it rather
		// than open a second issue (or surface a constraint error here).
		if cerr := h.q.CreateGitlabChildWriteback(ctx, db.CreateGitlabChildWritebackParams{
			TaskID: childID, IntegrationID: integ.ID, ChangeKind: kind, Payload: raw, ActorUserID: &actor,
		}); cerr != nil {
			log.Printf("gitlab writeback: enqueue %s for task %s failed: %v", kind, childID, cerr)
		}
		return
	}
	// Re-parenting is idempotent, so a burst collapses into the pending row.
	if n, cerr := h.q.CoalescePendingWriteback(ctx, db.CoalescePendingWritebackParams{
		TaskID: childID, ChangeKind: kind, Payload: raw, ActorUserID: &actor,
	}); cerr == nil && n > 0 {
		return
	}
	if cerr := h.q.CreateGitlabWriteback(ctx, db.CreateGitlabWritebackParams{
		TaskID: childID, IntegrationID: integ.ID, ChangeKind: kind, Payload: raw, ActorUserID: &actor,
	}); cerr != nil {
		log.Printf("gitlab writeback: enqueue %s for task %s failed: %v", kind, childID, cerr)
	}
}

// resolveChildContext finds the integration a child push belongs to, plus whichever
// links exist. The integration comes from the parent for the attach directions (the
// child has no link yet) and from the child itself for a detach (by then the parent may
// already be gone). A nil link means "not linked", which is a legitimate state here,
// not an error — the gate decides what it means for each kind.
func (h *API) resolveChildContext(ctx context.Context, childID uuid.UUID, parentID *uuid.UUID) (db.GitlabIntegration, *db.GitlabLink, *db.GitlabLink, bool) {
	var integ db.GitlabIntegration
	var parentLink, childLink *db.GitlabLink
	if parentID != nil {
		if l, err := h.q.GetGitlabLinkByTask(ctx, *parentID); err == nil {
			parentLink = &l
		}
	}
	if l, err := h.q.GetGitlabLinkByTask(ctx, childID); err == nil {
		childLink = &l
	}
	var integID uuid.UUID
	switch {
	case parentLink != nil:
		integID = parentLink.IntegrationID
	case childLink != nil:
		integID = childLink.IntegrationID
	default:
		return integ, nil, nil, false // neither side is mirrored — nothing to push
	}
	integ, err := h.q.GetGitlabIntegrationByID(ctx, integID)
	if err != nil || integ.OwnerUserID == nil {
		return integ, nil, nil, false
	}
	return integ, parentLink, childLink, true
}

// childKind picks the attach direction's kind from the child's link state: an existing
// issue is re-parented, a task without one gets an issue opened for it.
func childKind(childLinked bool) string {
	if childLinked {
		return gitlab.KindChildAttach
	}
	return gitlab.KindChildCreate
}

// checkChildGate is the pure gate for pushing a subtask into GitLab's hierarchy —
// everything decidable without talking to GitLab. Returns (0, "") when the push may be
// queued; otherwise an HTTP status and a message, so the manual endpoint and the
// automatic hook agree on the rules and only differ in whether anyone is told.
//
// The parent-is-grouped requirement is the one worth spelling out: a child work item
// only stays visible under an ungrouped parent by accident, and the pull reads an
// ungrouped parent as having no children — so pushing there would create an issue that
// the very next sync detaches again.
func checkChildGate(kind string, integEnabled bool, wb gitlab.Writeback, parentLink *db.GitlabLink, childLinked bool) (int, string) {
	switch {
	case !integEnabled:
		return http.StatusBadRequest, "GitLab integration is disabled"
	case !wb.PushChildren:
		return http.StatusBadRequest, "pushing subtasks to GitLab is disabled for this integration"
	}
	if kind == gitlab.KindChildDetach {
		// Detach needs the child's own issue and nothing else: the parent may already be
		// unlinked, ungrouped or archived by the time this runs.
		if !childLinked {
			return http.StatusBadRequest, "subtask is not linked to a GitLab issue"
		}
		return 0, ""
	}
	switch {
	case parentLink == nil:
		return http.StatusBadRequest, "parent task is not linked to a GitLab issue"
	case !parentLink.GlIsGroup && !wb.AutoGroupOnChild:
		return http.StatusBadRequest, "parent is not marked as a grouped task in GitLab"
	case kind == gitlab.KindChildCreate && childLinked:
		return http.StatusConflict, "subtask is already linked to a GitLab issue"
	case kind == gitlab.KindChildAttach && !childLinked:
		return http.StatusBadRequest, "subtask is not linked to a GitLab issue"
	}
	return 0, ""
}

// PushGitlabChild queues the task's subtask push by hand: the retry behind "not in the
// GitLab hierarchy" in the UI, and the way to backfill subtasks that were created
// before the parent was grouped (or before this feature existed). Unlike the automatic
// hook it reports why it refused. POST /tasks/:id/gitlab-child.
func (h *API) PushGitlabChild(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	task, _, ok := h.loadTask(c, id)
	if !ok {
		return
	}
	if task.ParentID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task is not a subtask"})
		return
	}
	integ, parentLink, childLink, resolved := h.resolveChildContext(c, id, task.ParentID)
	if !resolved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parent task is not linked to a GitLab issue"})
		return
	}
	kind := childKind(childLink != nil)
	wb := parseWriteback(integ.Writeback)
	if status, msg := checkChildGate(kind, integ.Enabled, wb, parentLink, childLink != nil); status != 0 {
		c.JSON(status, gin.H{"error": msg})
		return
	}
	actor := middleware.CurrentUser(c)
	h.enqueueChildWriteback(c, id, task.ParentID, actor, kind)
	// Queued, not done: the worker owns the GitLab round trips, and the task's GitLab
	// state arrives over the realtime channel when it lands.
	c.JSON(http.StatusAccepted, gin.H{"queued": kind})
}

// ── worker side ────────────────────────────────────────────────

// performChildWriteback executes one structural child row. Called from
// performWriteback before it resolves the task's own link, because child_create runs
// for a task that has none yet.
//
// The invariant across every branch: the subtask is never lost. A GitLab instance that
// declines the hierarchy still gets the issue, linked — the subtask is then simply
// "linked but not in the hierarchy", which the link records (empty gl_parent_global_id)
// and the UI offers to retry.
func (h *API) performChildWriteback(ctx context.Context, w db.GitlabWriteback) (writebackResult, error) {
	var res writebackResult
	integ, err := h.q.GetGitlabIntegrationByID(ctx, w.IntegrationID)
	if err != nil {
		return res, notify.Permanent(fmt.Errorf("integration gone: %w", err))
	}
	res.wsID = integ.WorkspaceID
	wb := parseWriteback(integ.Writeback)
	if !integ.Enabled || !wb.PushChildren {
		// Config changed since enqueue — same treatment as a binding removed under a
		// queued row: drop it quietly rather than fail it.
		res.result = "выгрузка подзадач выключена"
		return res, nil
	}
	baseURL, token, sudoUser, ok := h.writeGitlabConn(ctx, w.ActorUserID, integ.OwnerUserID)
	if !ok {
		return res, notify.Permanent(errors.New("no GitLab credential available (personal PAT or service token)"))
	}
	client := gitlab.New(baseURL, token).WithSudo(sudoUser)

	if w.ChangeKind == gitlab.KindChildDetach {
		return h.performChildDetach(ctx, client, w, res)
	}

	// The parent comes from LIVE task state, not the enqueue payload: by the time this
	// runs the subtask may have been moved under a different parent, and the current one
	// is what GitLab should end up reflecting.
	task, err := h.q.GetTask(ctx, w.TaskID)
	if err != nil {
		return res, notify.Permanent(fmt.Errorf("task gone: %w", err))
	}
	if task.ParentID == nil {
		res.result = "подзадача уже откреплена — пропущено"
		return res, nil
	}
	parentLink, err := h.q.GetGitlabLinkByTask(ctx, *task.ParentID)
	if err != nil {
		return res, notify.Permanent(errors.New("parent task is not linked to a GitLab issue"))
	}
	if !parentLink.GlIsGroup {
		out, gerr := h.groupParentForChild(ctx, client, integ, wb, parentLink)
		if gerr != nil {
			return res, gerr
		}
		if out != "" {
			res.result = out
			return res, nil // parent not grouped and auto-grouping is off — nothing to push
		}
	}

	var payload map[string]any
	_ = json.Unmarshal(w.Payload, &payload)

	childLink, cerr := h.q.GetGitlabLinkByTask(ctx, w.TaskID)
	if cerr != nil {
		if w.ChangeKind != gitlab.KindChildCreate {
			return res, notify.Permanent(errors.New("subtask is not linked to a GitLab issue"))
		}
		childLink, err = h.createChildIssue(ctx, client, integ, wb, w, task, payload)
		if err != nil {
			return res, err
		}
	}
	res.glIid = childLink.GlIid

	out, aerr := h.attachChildInGitlab(ctx, client, childLink, parentLink)
	if aerr != nil {
		return res, aerr
	}
	res.result = out
	h.broadcast(integ.WorkspaceID, "task.updated", map[string]any{"task_id": w.TaskID})
	return res, nil
}

// groupParentForChild labels the parent as grouped when auto-grouping is on. Returns a
// non-empty outcome string when the push must stop instead (auto-grouping off) — the
// deliberate default, because silently editing labels in GitLab is not something to opt
// users into; the UI offers the button next to the warning instead.
func (h *API) groupParentForChild(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, wb gitlab.Writeback, parentLink db.GitlabLink) (string, error) {
	if !wb.AutoGroupOnChild {
		return "родитель не помечен сгруппированной задачей — пропущено", nil
	}
	label := wb.EffectiveGroupLabel()
	if !parseRules(integ.LabelRules).ResolvesToGroup(label) {
		// Same guard as the button: a label this integration's rules do not read back as
		// grouping would leave the parent ungrouped after the next pull.
		return "", notify.Permanent(errors.New("group label " + label + " is not a grouping label under this integration's rules"))
	}
	if err := client.SetIssueLabels(ctx, parentLink.GlProjectPath, parentLink.GlIid, []string{label}, nil); err != nil {
		return "", err
	}
	soft(ctx, "SetGitlabLinkGroup", h.q.SetGitlabLinkGroup(ctx, db.SetGitlabLinkGroupParams{
		TaskID: parentLink.TaskID, GlIsGroup: true,
	}))
	return "", nil
}

// createChildIssue opens the GitLab issue for a subtask and links it.
//
// The issue is created with issue_type=task (see gitlab.ChildIssueType) over the plain
// REST path, reusing the same field mapping as "create issue from task" — labels,
// due date, assignees. Its iid is written back into the outbox row BEFORE the link is
// stored, so a failure between the two costs a retry, not a duplicate issue: the retry
// finds the reservation and adopts it.
func (h *API) createChildIssue(ctx context.Context, client *gitlab.Client, integ db.GitlabIntegration, wb gitlab.Writeback, w db.GitlabWriteback, task db.Task, payload map[string]any) (db.GitlabLink, error) {
	iid, globalID, webURL := reservedIssue(payload)
	if iid == 0 {
		labels := h.buildCreateLabels(ctx, task, parseRules(integ.LabelRules), wb)
		due := ""
		if task.DueDate != nil {
			due = task.DueDate.UTC().Format("2006-01-02")
		}
		assignees := h.resolveTaskGitlabAssigneeIDs(ctx, integ, task.ID)
		created, err := client.CreateIssue(ctx, integ.ProjectPath, task.Title, task.Description,
			labels, due, assignees, gitlab.ChildIssueType)
		if err != nil {
			return db.GitlabLink{}, err
		}
		iid, globalID, webURL = created.IID, created.GlobalID(), created.WebURL
		if payload == nil {
			payload = map[string]any{}
		}
		payload["gl_iid"], payload["gl_global_id"], payload["gl_web_url"] = iid, globalID, webURL
		if raw, merr := json.Marshal(payload); merr == nil {
			soft(ctx, "SetGitlabWritebackPayload", h.q.SetGitlabWritebackPayload(ctx, db.SetGitlabWritebackPayloadParams{
				ID: w.ID, Payload: raw,
			}))
		}
	}
	if _, err := h.q.CreateGitlabLink(ctx, db.CreateGitlabLinkParams{
		TaskID: task.ID, IntegrationID: integ.ID, GlGlobalID: globalID,
		GlIid: iid, GlProjectPath: integ.ProjectPath, GlWebUrl: webURL,
		TitleHash:   gitlab.HashStr(task.Title),
		DescHash:    gitlab.HashStr(task.Description),
		GlLastState: "opened",
	}); err != nil {
		return db.GitlabLink{}, err // transient: the reservation makes the retry safe
	}
	// True up hashes/author/state from the issue itself, so the next pull sees nothing
	// remote-changed for a task we just pushed.
	h.refreshLinkSnapshot(ctx, client, integ, task.ID, integ.ProjectPath, iid)
	return h.q.GetGitlabLinkByTask(ctx, task.ID)
}

// reservedIssue reads the issue a previous attempt already opened for this row.
// Values come back through JSONB, so the iid arrives as a float64.
func reservedIssue(payload map[string]any) (int64, string, string) {
	iid, _ := payload["gl_iid"].(float64)
	if iid <= 0 {
		return 0, "", ""
	}
	globalID, _ := payload["gl_global_id"].(string)
	webURL, _ := payload["gl_web_url"].(string)
	return int64(iid), globalID, webURL
}

// attachChildInGitlab puts the child issue under its parent in GitLab's hierarchy.
//
// Both work-item gids are READ from GitLab (cached on the links once known) and never
// constructed from the issue numbers. An instance without the hierarchy widget, or one
// that refuses this parent/child type pair, is reported as a degraded outcome rather
// than an error: the subtask keeps its issue, the link records that it is not in the
// hierarchy, and the UI can offer a retry.
func (h *API) attachChildInGitlab(ctx context.Context, client *gitlab.Client, childLink, parentLink db.GitlabLink) (string, error) {
	parentGID := parentLink.GlWorkItemID
	if parentGID == "" {
		wi, supported, err := client.WorkItemByIID(ctx, parentLink.GlProjectPath, parentLink.GlIid)
		if err != nil {
			return "", err
		}
		if !supported {
			return "иерархия GitLab недоступна — подзадача создана без неё", nil
		}
		parentGID = wi.GID
		soft(ctx, "SetGitlabLinkHierarchy", h.q.SetGitlabLinkHierarchy(ctx, db.SetGitlabLinkHierarchyParams{
			TaskID: parentLink.TaskID, GlWorkItemID: wi.GID, GlParentGlobalID: wi.ParentGID,
		}))
	}
	childGID := childLink.GlWorkItemID
	if childGID == "" {
		wi, supported, err := client.WorkItemByIID(ctx, childLink.GlProjectPath, childLink.GlIid)
		if err != nil {
			return "", err
		}
		if !supported || wi.GID == "" {
			return "иерархия GitLab недоступна — подзадача создана без неё", nil
		}
		childGID = wi.GID
	}
	attached, err := client.SetWorkItemParent(ctx, childGID, parentGID)
	if err != nil {
		return "", err
	}
	if !attached {
		// Record the gid we did resolve, with no parent: "linked, not in the hierarchy".
		soft(ctx, "SetGitlabLinkHierarchy", h.q.SetGitlabLinkHierarchy(ctx, db.SetGitlabLinkHierarchyParams{
			TaskID: childLink.TaskID, GlWorkItemID: childGID, GlParentGlobalID: "",
		}))
		return "GitLab отклонил иерархию — подзадача без родителя", nil
	}
	soft(ctx, "SetGitlabLinkHierarchy", h.q.SetGitlabLinkHierarchy(ctx, db.SetGitlabLinkHierarchyParams{
		TaskID: childLink.TaskID, GlWorkItemID: childGID, GlParentGlobalID: parentGID,
	}))
	return "подзадача привязана к родителю", nil
}

// performChildDetach drops a linked subtask back to top-level in GitLab. It needs only
// the child's own work item — the former parent may already be unlinked or archived.
func (h *API) performChildDetach(ctx context.Context, client *gitlab.Client, w db.GitlabWriteback, res writebackResult) (writebackResult, error) {
	childLink, err := h.q.GetGitlabLinkByTask(ctx, w.TaskID)
	if err != nil {
		return res, notify.Permanent(errors.New("subtask is not linked to a GitLab issue"))
	}
	res.glIid = childLink.GlIid
	childGID := childLink.GlWorkItemID
	if childGID == "" {
		wi, supported, werr := client.WorkItemByIID(ctx, childLink.GlProjectPath, childLink.GlIid)
		if werr != nil {
			return res, werr
		}
		if !supported || wi.GID == "" {
			res.result = "иерархия GitLab недоступна — пропущено"
			return res, nil
		}
		childGID = wi.GID
	}
	detached, err := client.SetWorkItemParent(ctx, childGID, "")
	if err != nil {
		return res, err
	}
	if !detached {
		res.result = "иерархия GitLab недоступна — пропущено"
		return res, nil
	}
	soft(ctx, "SetGitlabLinkHierarchy", h.q.SetGitlabLinkHierarchy(ctx, db.SetGitlabLinkHierarchyParams{
		TaskID: childLink.TaskID, GlWorkItemID: childGID, GlParentGlobalID: "",
	}))
	res.result = "подзадача откреплена от родителя"
	return res, nil
}
