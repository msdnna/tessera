package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
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
		_ = h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: nil})
		return
	}
	mID, err := h.ensureGitlabMilestone(ctx, integ, projectID, issue)
	if err != nil {
		return
	}
	_ = h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: &mID})
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
		_ = h.q.UpdateGitlabMilestoneLink(ctx, db.UpdateGitlabMilestoneLinkParams{
			MilestoneID: link.MilestoneID, GlIid: iid, GlNumericID: numeric,
			GlWebUrl: issue.MilestoneURL, GlState: state, TitleHash: titleHash,
		})
		return link.MilestoneID, nil
	case errors.Is(err, pgx.ErrNoRows):
		st := state
		m, cerr := h.q.CreateMilestone(ctx, db.CreateMilestoneParams{
			ProjectID: projectID, Title: issue.MilestoneTitle, Description: "",
			StartDate: issue.MilestoneStart, DueDate: issue.MilestoneDue, State: &st,
		})
		if cerr != nil {
			return uuid.Nil, cerr
		}
		if cerr := h.q.CreateGitlabMilestoneLink(ctx, db.CreateGitlabMilestoneLinkParams{
			MilestoneID: m.ID, IntegrationID: integ.ID, GlGlobalID: issue.MilestoneGID,
			GlIid: iid, GlNumericID: numeric, GlWebUrl: issue.MilestoneURL, GlState: state, TitleHash: titleHash,
		}); cerr != nil {
			return uuid.Nil, cerr
		}
		return m.ID, nil
	default:
		return uuid.Nil, err
	}
}
