package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// Marking a task as a "grouped task" in GitLab (#2592) — the parent half of the
// hierarchy feature. Deliberately a button and not a tag: the grouping label lives in
// a namespace the pull rules own (`M: `), and the frontend hides such prefixes from
// the tag pickers on purpose (KanbanBoard.vue metaTagPrefixes). Opening it up as an
// ordinary tag would let a user set a label that the rules then re-interpret on the
// next pull, so the tag and the field it maps to drift apart.
//
// The child half (creating a subtask as a child work item) goes through the write-back
// outbox instead, because it talks to GitLab on the task's behalf long after the
// request that caused it.

// SetGitlabTaskGroup marks the task's GitLab issue as a grouped parent: it adds the
// grouping label in GitLab and records the flag on the link. POST /tasks/:id/gitlab-group.
func (h *API) SetGitlabTaskGroup(c *gin.Context) {
	h.changeGitlabTaskGroup(c, true)
}

// ClearGitlabTaskGroup removes the grouping label again. Refuses while the task still
// has GitLab-linked subtasks. DELETE /tasks/:id/gitlab-group.
func (h *API) ClearGitlabTaskGroup(c *gin.Context) {
	h.changeGitlabTaskGroup(c, false)
}

// changeGitlabTaskGroup is the shared body of both directions: same task/integration
// resolution, same gates, opposite label operation.
func (h *API) changeGitlabTaskGroup(c *gin.Context, group bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	task, wsID, ok := h.loadTask(c, id)
	if !ok {
		return
	}

	// The binding is resolved by the task's own board, like every other GitLab action
	// on a task — a workspace can have several integrations.
	integ, err := h.q.GetGitlabIntegrationByBoard(c, task.BoardID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task is not on a GitLab integration board"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	link, lerr := h.q.GetGitlabLinkByTask(c, id)
	rules := parseRules(integ.LabelRules)
	wb := parseWriteback(integ.Writeback)
	label := wb.EffectiveGroupLabel()
	if status, msg := checkGroupGate(integ.Enabled, wb, rules, lerr == nil); status != 0 {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	// Ungrouping a parent that still has GitLab children would leave those issues
	// parented in GitLab while Tessera stops asking for them — the next pull would then
	// read "unclaimed" and scatter the subtasks. Refuse instead of half-doing it.
	if !group {
		if n, cerr := h.q.CountGitlabChildLinks(c, db.CountGitlabChildLinksParams{
			IntegrationID: integ.ID, ParentID: &id,
		}); cerr == nil && n > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "task still has GitLab-linked subtasks — detach them first"})
			return
		}
	}

	actor := middleware.CurrentUser(c)
	baseURL, token, sudoUser, ok := h.writeGitlabConn(c, &actor, integ.OwnerUserID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect a GitLab account first, or ask an admin to set a service token"})
		return
	}
	client := gitlab.New(baseURL, token).WithSudo(sudoUser)

	add, remove := []string{label}, []string(nil)
	if !group {
		add, remove = nil, []string{label}
	}
	if lerr := client.SetIssueLabels(c, link.GlProjectPath, link.GlIid, add, remove); lerr != nil {
		log.Printf("gitlab group label for task %s: %v", id, lerr)
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitLab rejected the label change: " + truncErr(lerr)})
		return
	}
	if serr := h.q.SetGitlabLinkGroup(c, db.SetGitlabLinkGroupParams{TaskID: id, GlIsGroup: group}); serr != nil {
		fail(c, serr)
		return
	}
	// Cache the parent's work-item gid while we are here, so pushing the first child
	// does not need an extra round trip — and so a later hierarchy failure is about the
	// hierarchy, not about resolving the parent. Best-effort by design: an instance
	// without the work-items API still gets a correctly labelled grouped parent, which
	// is all the PULL side needs.
	if group {
		if wi, supported, werr := client.WorkItemByIID(c, link.GlProjectPath, link.GlIid); werr == nil && supported {
			soft(c, "SetGitlabLinkHierarchy", h.q.SetGitlabLinkHierarchy(c, db.SetGitlabLinkHierarchyParams{
				TaskID: id, GlWorkItemID: wi.GID, GlParentGlobalID: wi.ParentGID,
			}))
		}
	}

	h.logEventActor(c, id, actor, "synced", map[string]any{
		"source": "gitlab", "iid": link.GlIid, "url": link.GlWebUrl, "grouped": group,
	})
	h.broadcast(wsID, "task.updated", gin.H{"task_id": id})
	c.JSON(http.StatusOK, h.gitlabLinkForTask(c, id))
}

// checkGroupGate is the pure half of the grouping endpoints: everything that can be
// decided from config alone. Split out because the handlers around it need a database
// and a GitLab server, and these rules are exactly the part worth pinning down in a
// test. Returns (0, "") when the request may proceed.
func checkGroupGate(integEnabled bool, wb gitlab.Writeback, rules gitlab.Rules, linked bool) (int, string) {
	switch {
	case !integEnabled:
		return http.StatusBadRequest, "GitLab integration is disabled"
	case !wb.PushChildren:
		return http.StatusBadRequest, "grouped tasks are disabled for this integration"
	case !linked:
		return http.StatusBadRequest, "task is not linked to a GitLab issue"
	case !rules.ResolvesToGroup(wb.EffectiveGroupLabel()):
		// The button would put on a label that this integration's own rules do not read
		// back as grouping: the pull would import it as an ordinary tag and the parent
		// would stay ungrouped. Failing loudly here beats that silent desync.
		return http.StatusBadRequest, "label " + wb.EffectiveGroupLabel() + " is not a grouping label under this integration's rules"
	}
	return 0, ""
}
