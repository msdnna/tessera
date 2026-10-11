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
// link's presence marks the milestone GitLab-sourced; since #3010 it is editable in
// Tessera and written back (gitlab_milestone_sync.go), so the pull merges three
// ways instead of overwriting.

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
// carries (creating/refreshing the native milestone + link), or — when it has none —
// at the stage a «лейбл → этап» rule named (labelStage, #3086), or clears it when
// neither says anything. Skipped when the user overrode the task's milestone locally.
//
// A change on a task the sync already knew is journalled (#2992) with the sync's
// actor, like every other field the sync moves: the stage page counts its scope
// change from these entries, and a task GitLab moved into the sprint mid-way is
// exactly the scope change it must show. A task the sync is creating right now
// (`fresh`) is not: its first import says nothing about when it joined the stage,
// and journalling it would book every task of a sprint imported mid-way as
// "added after the start".
func (h *API) reconcileTaskMilestone(ctx context.Context, integ db.GitlabIntegration, projectID, taskID uuid.UUID, issue gitlab.Issue, labelStage string, overridden, fresh bool, actorID uuid.UUID) {
	if overridden {
		return
	}
	var before *uuid.UUID
	if !fresh {
		if t, err := h.q.GetTask(ctx, taskID); err == nil {
			before = t.MilestoneID
		}
	}
	if issue.MilestoneGID == "" && labelStage != "" {
		// No native milestone, but a «лейбл → этап» rule names one (#3086): the
		// stage of that title in the project, created on first sight. A native
		// GitLab milestone, when the issue has one, wins over the label.
		mID, err := h.ensureLabelStage(ctx, projectID, labelStage)
		if err != nil {
			return
		}
		if before != nil && *before == mID {
			return
		}
		soft(ctx, "SetTaskMilestone", h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: &mID}))
		if !fresh {
			h.journalMilestoneActor(ctx, taskID, actorID, before, &mID, labelStage)
		}
		return
	}
	if issue.MilestoneGID == "" {
		// GitLab removed the milestone — clear the (GitLab-sourced) one on the task.
		soft(ctx, "SetTaskMilestone", h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: nil}))
		if !fresh {
			h.journalMilestoneActor(ctx, taskID, actorID, before, nil, "")
		}
		return
	}
	mID, err := h.ensureGitlabMilestone(ctx, integ, projectID, issue)
	if err != nil {
		return
	}
	soft(ctx, "SetTaskMilestone", h.q.SetTaskMilestone(ctx, db.SetTaskMilestoneParams{ID: taskID, MilestoneID: &mID}))
	if !fresh {
		h.journalMilestoneActor(ctx, taskID, actorID, before, &mID, issue.MilestoneTitle)
	}
}

// ensureLabelStage returns the project's stage of the given title, creating it when
// the project has none (#3086). Such a stage is Tessera's own — no GitLab link: the
// label named it, no GitLab milestone stands behind it, so a milestone write-back
// leaves it alone (performSetMilestone treats it as native).
func (h *API) ensureLabelStage(ctx context.Context, projectID uuid.UUID, title string) (uuid.UUID, error) {
	m, err := h.q.GetMilestoneInProjectByTitle(ctx, db.GetMilestoneInProjectByTitleParams{ProjectID: projectID, Title: title})
	if err == nil {
		return m.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	created, cerr := h.q.CreateMilestone(ctx, db.CreateMilestoneParams{
		ProjectID: projectID, Title: title, Slug: h.uniqueMilestoneSlug(ctx, projectID, title),
	})
	if cerr != nil {
		return uuid.Nil, cerr
	}
	return created.ID, nil
}

// ensureGitlabMilestone upserts the native milestone + link for a GitLab milestone,
// returning the native milestone id. An existing stage is merged three ways
// against the link's baseline (pullGitlabMilestone): a field only GitLab changed is
// taken — an empty GitLab value clears ours too — while a local edit not yet
// written to GitLab is kept and one changed on both sides becomes a conflict.
func (h *API) ensureGitlabMilestone(ctx context.Context, integ db.GitlabIntegration, projectID uuid.UUID, issue gitlab.Issue) (uuid.UUID, error) {
	state := issue.MilestoneState
	if state != "closed" {
		state = "active"
	}
	numeric := gidNumericID(issue.MilestoneGID)
	titleHash := gitlab.HashStr(issue.MilestoneTitle)
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
		// Merge GitLab's version into the native milestone, then refresh the link.
		if uerr := h.pullGitlabMilestone(ctx, link, integ.WorkspaceID, snapOfIssue(issue), milestoneWritable(integ)); uerr != nil {
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
				ProjectID: projectID, Title: issue.MilestoneTitle, Description: issue.MilestoneDescription,
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
				GlSnapshot: snapOfIssue(issue).marshal(),
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
	// Write connection attributed to the acting user (task #2690): their own PAT, else
	// the service token with a `Sudo:` header, else the plain service / owner PAT.
	actor := middleware.CurrentUser(c)
	baseURL, token, sudoUser, ok := h.writeGitlabConn(c, &actor, integ.OwnerUserID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect a GitLab account first, or ask an admin to set a service token"})
		return
	}
	client := gitlab.New(baseURL, token).WithSudo(sudoUser)

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
	// The milestone now exists in GitLab and this row is the only record Tessera
	// keeps of it: if the insert loses, the user is told "internal error" and their
	// next press of the button creates a SECOND remote milestone, because the
	// already-linked check above still finds nothing. Log cerr — the insert's own
	// error, not the create call's (nil by now) — the way the sibling
	// create-issue-from-task path does, so the one moment worth diagnosing is not
	// the one moment with no cause recorded.
	if cerr := h.q.CreateGitlabMilestoneLink(c, db.CreateGitlabMilestoneLinkParams{
		MilestoneID: id, IntegrationID: integ.ID, GlGlobalID: created.GlobalID(),
		GlIid: iid, GlNumericID: created.ID, GlWebUrl: created.WebURL, GlState: state,
		TitleHash: gitlab.HashStr(m.Title),
		// GitLab now holds exactly what was sent: the baseline of the next sync.
		GlSnapshot: milestoneSnap{
			Title: m.Title, Description: m.Description,
			Start: dateStr(m.StartDate), Due: dateStr(m.DueDate), State: normMilestoneState(state),
		}.marshal(),
	}); cerr != nil {
		log.Printf("gitlab push milestone: link milestone %s to %s failed: %v", id, created.WebURL, cerr)
		fail(c, cerr)
		return
	}
	h.broadcast(wsID, "milestone.updated", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"id": id, "gl_url": created.WebURL, "gl_linked": true})
}
