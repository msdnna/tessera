package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// GitLab milestone pull-mapping (B/M1): map a GitLab issue's milestone onto a native
// Tessera milestone + a gitlab_milestone_links row, and point the task at it. The
// link's presence marks the milestone GitLab-sourced (read-only in Tessera).

// gidNumericID extracts the trailing numeric id from a GraphQL global id
// ("gid://gitlab/Milestone/42" → 42), for the REST milestone_id parameter.
func gidNumericID(gid string) int64 {
	i := strings.LastIndex(gid, "/")
	if i < 0 || i+1 >= len(gid) {
		return 0
	}
	n, _ := strconv.ParseInt(gid[i+1:], 10, 64)
	return n
}

// reconcileTaskMilestone points a synced task at the milestone its GitLab issue
// carries (creating/refreshing the native milestone + link), or clears it when the
// issue has none. Skipped when the user overrode the task's milestone locally.
func (h *API) reconcileTaskMilestone(ctx context.Context, integ db.GitlabIntegration, projectID, taskID uuid.UUID, issue gitlab.Issue, overridden bool) {
	if overridden {
		return
	}
	if issue.MilestoneGID == "" {
		// GitLab removed the milestone — clear the (GitLab-sourced) one on the task.
		soft(ctx, "SetTaskMilestone", h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: nil}))
		return
	}
	mID, err := h.ensureGitlabMilestone(ctx, integ, projectID, issue)
	if err != nil {
		return
	}
	soft(ctx, "SetTaskMilestone", h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: &mID}))
}

// ensureGitlabMilestone upserts the native milestone + link for a GitLab milestone,
// returning the native milestone id. GitLab is the source of truth for the synced
// fields (title/dates/state).
func (h *API) ensureGitlabMilestone(ctx context.Context, integ db.GitlabIntegration, projectID uuid.UUID, issue gitlab.Issue) (uuid.UUID, error) {
	state := issue.MilestoneState
	if state != "closed" {
		state = "active"
	}
	numeric := gidNumericID(issue.MilestoneGID)
	titleHash := hashStr(issue.MilestoneTitle)
	var iid *int64
	if issue.MilestoneIID != 0 {
		v := issue.MilestoneIID
		iid = &v
	}

	link, err := h.q.GetGitlabMilestoneLinkByGID(ctx, db.GetGitlabMilestoneLinkByGIDParams{
		IntegrationID: integ.ID, GlGlobalID: issue.MilestoneGID,
	})
	switch {
	case err == nil:
		// Refresh the native milestone from GitLab + the link.
		if _, uerr := h.q.UpdateMilestone(ctx, db.UpdateMilestoneParams{
			ID: link.MilestoneID, Title: issue.MilestoneTitle, Description: "",
			StartDate: issue.MilestoneStart, DueDate: issue.MilestoneDue, State: state,
		}); uerr != nil {
			return uuid.Nil, uerr
		}
		soft(ctx, "UpdateGitlabMilestoneLink", h.q.UpdateGitlabMilestoneLink(ctx, db.UpdateGitlabMilestoneLinkParams{
			MilestoneID: link.MilestoneID, GlIid: iid, GlNumericID: numeric,
			GlWebUrl: issue.MilestoneURL, GlState: state, TitleHash: titleHash,
		}))
		return link.MilestoneID, nil
	case errors.Is(err, pgx.ErrNoRows):
		st := state
		slug := h.uniqueMilestoneSlug(ctx, projectID, issue.MilestoneTitle)
		// Transactional: the native milestone and its GitLab link must be created
		// together — a milestone without its link makes the next sync create a
		// duplicate instead of recognising the existing one.
		var mID uuid.UUID
		if terr := h.inTx(ctx, func(q *db.Queries) error {
			m, cerr := q.CreateMilestone(ctx, db.CreateMilestoneParams{
				ProjectID: projectID, Title: issue.MilestoneTitle, Description: "",
				StartDate: issue.MilestoneStart, DueDate: issue.MilestoneDue, State: &st,
				Slug: slug,
			})
			if cerr != nil {
				return cerr
			}
			mID = m.ID
			return q.CreateGitlabMilestoneLink(ctx, db.CreateGitlabMilestoneLinkParams{
				MilestoneID: m.ID, IntegrationID: integ.ID, GlGlobalID: issue.MilestoneGID,
				GlIid: iid, GlNumericID: numeric, GlWebUrl: issue.MilestoneURL, GlState: state, TitleHash: titleHash,
			})
		}); terr != nil {
			return uuid.Nil, terr
		}
		return mID, nil
	default:
		return uuid.Nil, err
	}
}

// PushMilestoneToGitlab creates a GitLab project milestone from a NATIVE Tessera
// milestone and links them — an explicit, selective action. Creating a native
// milestone never auto-pushes to GitLab; the user opts in per milestone here.
// POST /milestones/:id/gitlab.
func (h *API) PushMilestoneToGitlab(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	m, wsID, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	// Resolve the binding whose target board lives in the milestone's project
	// (GitLab milestones are project-scoped).
	integ, err := h.q.GetGitlabIntegrationByProject(c, m.ProjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this project has no GitLab integration"})
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
	if _, lerr := h.q.GetGitlabMilestoneLink(c, id); lerr == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "milestone is already linked to GitLab"})
		return
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

	created, err := client.CreateProjectMilestone(c, integ.ProjectPath, m.Title, m.Description, dateStr(m.StartDate), dateStr(m.DueDate))
	if err != nil {
		log.Printf("gitlab create milestone %s: %v", id, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "GitLab rejected the milestone: " + truncErr(err)})
		return
	}
	state := created.State
	if state != "closed" {
		state = "active"
	}
	var iid *int64
	if created.IID != 0 {
		v := created.IID
		iid = &v
	}
	if cerr := h.q.CreateGitlabMilestoneLink(c, db.CreateGitlabMilestoneLinkParams{
		MilestoneID: id, IntegrationID: integ.ID, GlGlobalID: created.GlobalID(),
		GlIid: iid, GlNumericID: created.ID, GlWebUrl: created.WebURL, GlState: state,
		TitleHash: hashStr(m.Title),
	}); cerr != nil {
		fail(c, err)
		return
	}
	h.broadcast(wsID, "milestone.updated", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"id": id, "gl_url": created.WebURL, "gl_linked": true})
}
