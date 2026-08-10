package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// GitLab issue creation (phase B+ step 4): open a brand-new GitLab issue mirroring
// a Tessera task and link the two. Unlike the edit write-back outbox (which pushes
// incidental changes to an already-linked issue asynchronously), this is a
// synchronous, explicit user action — the caller waits for, and gets back, the new
// issue's number/url. Gated on the integration's push_create flag; the task must
// live on the integration board and not already be linked.

// CreateGitlabIssueFromTask creates a GitLab issue from the task and returns the
// resulting link view. POST /tasks/:id/gitlab-issue.
func (h *API) CreateGitlabIssueFromTask(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	task, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}

	// Resolve the binding by the task's own board (multi-binding).
	integ, err := h.q.GetGitlabIntegrationByBoard(c, task.BoardID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task is not on a GitLab integration board"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !integ.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "GitLab integration is disabled"})
		return
	}
	wb := parseWriteback(integ.Writeback)
	if !wb.PushCreate {
		c.JSON(http.StatusBadRequest, gin.H{"error": "issue creation from tasks is disabled for this integration"})
		return
	}
	if _, lerr := h.q.GetGitlabLinkByTask(c, id); lerr == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "task is already linked to a GitLab issue"})
		return
	}
	// Optional description override (e.g. an issue-template-prefilled body the user
	// edited in the modal); falls back to the task's own description.
	var req struct {
		Description *string `json:"description"`
	}
	_ = c.ShouldBindJSON(&req)
	description := task.Description
	if req.Description != nil {
		description = *req.Description
	}

	// Connection: instance service token first, else the acting user's PAT, else the
	// binding owner's PAT.
	actor := middleware.CurrentUser(c)
	baseURL, token, ok := h.effectiveGitlabConn(c, &actor)
	if !ok {
		baseURL, token, ok = h.effectiveGitlabConn(c, integ.OwnerUserID)
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect a GitLab account first, or ask an admin to set a service token"})
		return
	}
	client := gitlab.New(baseURL, token)
	// Attribute the issue link to the acting user's GitLab identity when known.
	authorLogin := h.actorGitlabUsername(c, actor)
	rules := parseRules(integ.LabelRules)

	labels := h.buildCreateLabels(c, task, rules, wb)
	due := ""
	if task.DueDate != nil {
		due = task.DueDate.UTC().Format("2006-01-02")
	}
	assignees := h.resolveTaskGitlabAssigneeIDs(c, integ, id)

	created, err := client.CreateIssue(c, integ.ProjectPath, task.Title, description, labels, due, assignees)
	if err != nil {
		log.Printf("gitlab create issue for task %s: %v", id, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitLab rejected the issue: " + truncErr(err)})
		return
	}

	// Best-effort follow-up: time estimate is set via its own endpoint (time unit only).
	if wb.PushEstimate && task.Estimate != nil && *task.Estimate > 0 && h.integrationEstimationUnit(c, integ) == "time" {
		_ = client.SetIssueTimeEstimate(c, integ.ProjectPath, created.IID, int64(*task.Estimate))
	}

	state := created.State
	if state == "" {
		state = "opened"
	}
	if _, cerr := h.q.CreateGitlabLink(c, db.CreateGitlabLinkParams{
		TaskID: id, IntegrationID: integ.ID, GlGlobalID: created.GlobalID(),
		GlIid: created.IID, GlProjectPath: integ.ProjectPath, GlWebUrl: created.WebURL,
		GlUpdatedAt: nil,
		TitleHash:   hashStr(task.Title), DescHash: hashStr(description),
		LabelsHash:  hashStr(strings.Join(labels, "\n")),
		GlAuthor:    authorLogin,
		GlLastState: state,
	}); cerr != nil {
		log.Printf("gitlab create issue: link task %s failed: %v", id, cerr)
		fail(c, cerr)
		return
	}
	// True up the link snapshot (accurate hashes, author name/avatar, state) from the
	// freshly-created issue so the next scheduled pull sees nothing remote-changed.
	h.refreshLinkSnapshot(c, client, integ, id, integ.ProjectPath, created.IID)

	h.logEventActor(c, id, actor, "synced", map[string]any{
		"source": "gitlab", "iid": created.IID, "url": created.WebURL, "created": true,
	})
	h.broadcast(wsID, "task.updated", gin.H{"task_id": id})

	link, lerr := h.q.GetGitlabLinkByTask(c, id)
	if lerr != nil {
		// Link is in place; just return the creation basics.
		c.JSON(http.StatusOK, &gitlabLinkView{IID: created.IID, WebURL: created.WebURL, ProjectPath: integ.ProjectPath})
		return
	}
	c.JSON(http.StatusOK, &gitlabLinkView{
		IID: link.GlIid, WebURL: link.GlWebUrl, Author: link.GlAuthor,
		AuthorName: link.GlAuthorName, AuthorAvatarURL: link.GlAuthorAvatarUrl,
		ProjectPath: link.GlProjectPath,
	})
}

// buildCreateLabels assembles the GitLab label titles to set on a newly-created
// issue from the task's tags, priority and (if an explicit column→label binding is
// configured) its column — mirroring the inverse rules used by the edit write-back.
func (h *API) buildCreateLabels(ctx context.Context, task db.Task, rules gitlab.Rules, wb gitlab.Writeback) []string {
	var labels []string
	seen := map[string]bool{}
	add := func(l string) {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			return
		}
		seen[l] = true
		labels = append(labels, l)
	}
	// Tag-namespace labels, only when tag names round-trip to full label titles.
	if rules.TagsInvertible() {
		if tags, err := h.q.ListTaskTags(ctx, task.ID); err == nil {
			for _, tg := range tags {
				if name := strings.TrimSpace(tg.Name); name != "" && rules.TagLabelClass(name) {
					add(name)
				}
			}
		}
	}
	// Priority label (P:) when the mapping inverts.
	if task.Priority > 0 {
		if inv, ok := rules.InversePriority(); ok {
			if lbl, found := inv[task.Priority]; found {
				add(lbl)
			}
		}
	}
	// Status label (S:) from an explicit column→label binding, if one is configured.
	if len(wb.ColumnLabelBindings) > 0 {
		if col, err := h.q.GetColumn(ctx, task.ColumnID); err == nil {
			if lbl, found := wb.ColumnLabelBindings[col.Name]; found {
				add(lbl)
			}
		}
	}
	return labels
}

// resolveTaskGitlabAssigneeIDs resolves the task's assignees to numeric GitLab user
// ids: Tessera assignees with a connected GitLab account, plus pinned GitLab members
// (via the members roster). Unresolvable assignees are simply dropped.
func (h *API) resolveTaskGitlabAssigneeIDs(ctx context.Context, integ db.GitlabIntegration, taskID uuid.UUID) []int64 {
	ids := map[int64]bool{}
	if tas, err := h.q.ListTaskAssignees(ctx, taskID); err == nil {
		for _, a := range tas {
			if cred, cerr := h.q.GetGitlabCredential(ctx, a.ID); cerr == nil && cred.GlUserID != 0 {
				ids[cred.GlUserID] = true
			}
		}
	}
	if gas, err := h.q.ListTaskGitlabAssignees(ctx, taskID); err == nil {
		for _, g := range gas {
			if uid, gerr := h.q.GetGitlabMemberIDByUsername(ctx, db.GetGitlabMemberIDByUsernameParams{
				IntegrationID: integ.ID, GlUsername: g.GlUsername,
			}); gerr == nil && uid != 0 {
				ids[uid] = true
			}
		}
	}
	out := make([]int64, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

// ListGitlabIssueTemplates returns the project's issue templates
// (.gitlab/issue_templates/*.md) so the create-issue UI can prefill a description.
// Soft-fails to an empty list (manual entry remains the fallback).
// GET /workspaces/:id/gitlab/issue-templates.
func (h *API) ListGitlabIssueTemplates(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	// Pick the binding by ?integration_id (create-issue modal knows the task's
	// board's binding); fall back to the workspace's first binding.
	rows, err := h.q.ListGitlabIntegrationsByWorkspace(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusOK, []gitlab.IssueTemplate{})
		return
	}
	integ := rows[0]
	if want := c.Query("integration_id"); want != "" {
		if wid, perr := uuid.Parse(want); perr == nil {
			for _, r := range rows {
				if r.ID == wid {
					integ = r
					break
				}
			}
		}
	}
	uid := middleware.CurrentUser(c)
	// Service token first, else the caller's PAT, else the binding owner's PAT.
	baseURL, token, ok := h.effectiveGitlabConn(c, &uid)
	if !ok {
		baseURL, token, ok = h.effectiveGitlabConn(c, integ.OwnerUserID)
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect your GitLab account first"})
		return
	}
	client := gitlab.New(baseURL, token)
	tpls, err := client.IssueTemplates(c, integ.ProjectPath)
	if err != nil {
		log.Printf("gitlab issue templates ws=%s: %v", wsID, err)
		c.JSON(http.StatusOK, []gitlab.IssueTemplate{}) // soft-fail
		return
	}
	if tpls == nil {
		tpls = []gitlab.IssueTemplate{}
	}
	c.JSON(http.StatusOK, tpls)
}
