package handlers

import (
	"context"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	db "tessera/internal/db"
	"tessera/internal/gitlab"
)

// ── Merge requests of other GitLab projects (#2964 rework) ─
//
// A binding mirrors one project's issues, and its poll and webhooks see that
// project's merge requests only. But the MRs working on an issue usually live in
// other repositories (a scrum project's issue, the service repositories' MRs), and
// those projects' hooks never reach us. So for them the direction is reversed: the
// issue is asked which MRs GitLab relates to it and which close it, from any
// project the token can read, and each such MR is read once for its pipeline.
//
// Rows land in the same task_code_links, keyed by their own repository (0095), so
// !12 of two projects are two rows. The binding's own MRs stay with the project
// poll (gitlab_mr.go) — this file skips them, the two never fight over a row.
//
// Two feeds, both best-effort (a project the token can't read is skipped quietly):
//   - the pull: the issues it mirrored this run, plus foreign MRs whose pipeline
//     was last seen running (a finishing pipeline touches neither MR nor issue);
//   - opening a task: GET /tasks/:id refreshes the task in the background, at
//     most once per codeLinksRefreshTTL, and the result arrives as task.code_links.

const (
	// foreignMRIssueBudget bounds the issues one pull asks about (two round trips
	// each, plus the MR reads). A first full sweep spends it on the newest issues;
	// the others refresh when their task is opened.
	foreignMRIssueBudget = 50
	// foreignMRReadBudget bounds the single-MR reads (for the pipeline) per issue.
	foreignMRReadBudget = 20
	// codeLinksRefreshTTL is how long an opened task's foreign MRs are trusted
	// before the next open asks GitLab again.
	codeLinksRefreshTTL = 2 * time.Minute
	// codeLinksRefreshTimeout bounds one background refresh.
	codeLinksRefreshTimeout = 30 * time.Second
)

// mrRefreshGate remembers when each task's foreign MRs were last asked for and
// which are being asked right now, so neither the open-a-task path nor a pull
// running alongside it asks twice. In memory: a restart only costs one early
// refresh per task.
type mrRefreshGate struct {
	mu   sync.Mutex
	at   map[uuid.UUID]time.Time
	busy map[uuid.UUID]bool
}

// claim marks a refresh of taskID as started, unless one is running or the last
// one is younger than ttl.
func (g *mrRefreshGate) claim(taskID uuid.UUID, now time.Time, ttl time.Duration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy[taskID] {
		return false
	}
	if t, ok := g.at[taskID]; ok && ttl > 0 && now.Sub(t) < ttl {
		return false
	}
	if g.busy == nil {
		g.busy = map[uuid.UUID]bool{}
	}
	g.busy[taskID] = true
	return true
}

// done ends a claimed refresh. ok=false (it never ran) leaves the last stamp.
func (g *mrRefreshGate) done(taskID uuid.UUID, now time.Time, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.busy, taskID)
	if !ok {
		return
	}
	if g.at == nil {
		g.at = map[uuid.UUID]time.Time{}
	}
	// Keep the map from growing with every task ever opened.
	if len(g.at) > 10000 {
		for id, t := range g.at {
			if now.Sub(t) >= codeLinksRefreshTTL {
				delete(g.at, id)
			}
		}
	}
	g.at[taskID] = now
}

// refreshCodeLinksSoon starts a background refresh of a GitLab-mirrored task's
// foreign MRs, unless a recent one makes it pointless. Never blocks the request.
func (h *API) refreshCodeLinksSoon(taskID uuid.UUID) {
	if !h.mrRefresh.claim(taskID, time.Now(), codeLinksRefreshTTL) {
		return
	}
	started := h.runDetached("gitlab_mr_refresh", func(ctx context.Context) {
		ran := h.refreshTaskCodeLinks(ctx, taskID)
		h.mrRefresh.done(taskID, time.Now(), ran)
	})
	if !started {
		h.mrRefresh.done(taskID, time.Now(), false)
	}
}

// refreshTaskCodeLinks asks GitLab for the foreign MRs of one task's issue and
// tells open windows when the list changed. Reports whether GitLab was asked.
func (h *API) refreshTaskCodeLinks(ctx context.Context, taskID uuid.UUID) bool {
	ctx, cancel := context.WithTimeout(ctx, codeLinksRefreshTimeout)
	defer cancel()
	link, err := h.q.GetGitlabLinkByTask(ctx, taskID)
	if err != nil {
		return false
	}
	integ, err := h.q.GetGitlabIntegrationByID(ctx, link.IntegrationID)
	if err != nil || !integ.Enabled {
		return false
	}
	if archived, aerr := h.q.ProjectArchivedForBoard(ctx, integ.BoardID); aerr == nil && archived {
		return false
	}
	cred, _, ok := h.syncCredential(ctx, integ)
	if !ok {
		return false
	}
	client, _, err := h.syncClient(ctx, cred)
	if err != nil {
		return false
	}
	issuePath := link.GlProjectPath
	if issuePath == "" {
		issuePath = integ.ProjectPath
	}
	touched, asked := h.refreshForeignMRs(ctx, integ, client, taskID, issuePath, link.GlIid)
	h.announceCodeLinks(ctx, integ.WorkspaceID, touched)
	return asked
}

// syncForeignMergeRequests is the pull's half: the issues mirrored this run (the
// newest first, up to the budget), then the foreign MRs whose pipeline was last
// seen running.
func (h *API) syncForeignMergeRequests(ctx context.Context, integ db.GitlabIntegration, client *gitlab.Client, syncedIIDs []int64) {
	iids := slices.Clone(syncedIIDs)
	slices.Sort(iids)
	slices.Reverse(iids)
	iids = slices.Compact(iids)
	if len(iids) > foreignMRIssueBudget {
		iids = iids[:foreignMRIssueBudget]
	}
	touched := map[uuid.UUID]bool{}
	if len(iids) > 0 {
		rows, err := h.q.GitlabTasksForIids(ctx, db.GitlabTasksForIidsParams{IntegrationID: integ.ID, Iids: iids})
		if err != nil {
			log.Printf("gitlab foreign mr ws=%s: resolve tasks: %v", integ.WorkspaceID, err)
		}
		for _, r := range rows {
			if !h.mrRefresh.claim(r.TaskID, time.Now(), 0) {
				continue // the task's window is refreshing it right now
			}
			got, _ := h.refreshForeignMRs(ctx, integ, client, r.TaskID, integ.ProjectPath, r.GlIid)
			// No stamp: the pull saw the issue change, not the pipelines of its
			// MRs — opening the task still looks again.
			h.mrRefresh.done(r.TaskID, time.Now(), false)
			for id := range got {
				touched[id] = true
			}
		}
	}

	running, err := h.q.ForeignCodeChangesWithRunningCI(ctx, db.ForeignCodeChangesWithRunningCIParams{
		Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR, OwnRepo: integ.ProjectPath,
	})
	if err != nil {
		log.Printf("gitlab foreign mr ws=%s: running pipelines: %v", integ.WorkspaceID, err)
	}
	for _, r := range running {
		mr, rerr := client.MergeRequestByPath(ctx, r.Repo, r.Number)
		if rerr != nil {
			if !gitlab.IsNoAccess(rerr) {
				log.Printf("gitlab foreign mr %s!%d: %v", r.Repo, r.Number, rerr)
			}
			continue
		}
		mr.ProjectPath = r.Repo // the stored spelling, so the row is found again
		links, lerr := h.q.ListCodeLinksForChange(ctx, db.ListCodeLinksForChangeParams{
			Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR, Repo: r.Repo, Number: r.Number,
		})
		if lerr != nil {
			continue
		}
		for _, l := range links {
			for id := range h.upsertForeignMR(ctx, integ, l.TaskID, mr, l.Via, l, true) {
				touched[id] = true
			}
		}
	}
	h.announceCodeLinks(ctx, integ.WorkspaceID, touched)
}

// refreshForeignMRs brings one task's MRs from other projects in line with what
// GitLab relates to its issue. Returns the tasks whose «Разработка» changed and
// whether GitLab answered (only then are links it no longer names dropped).
func (h *API) refreshForeignMRs(ctx context.Context, integ db.GitlabIntegration, client *gitlab.Client, taskID uuid.UUID, issuePath string, issueIID int64) (map[uuid.UUID]bool, bool) {
	touched := map[uuid.UUID]bool{}
	mrs, closes, err := client.IssueMergeRequests(ctx, issuePath, issueIID)
	if err != nil {
		if !gitlab.IsNoAccess(err) {
			log.Printf("gitlab foreign mr %s#%d: %v", issuePath, issueIID, err)
		}
		return touched, false
	}
	rows, err := h.q.ListTaskCodeLinks(ctx, taskID)
	if err != nil {
		return touched, false
	}
	existing := map[string]db.TaskCodeLink{}
	for _, r := range rows {
		if r.Provider == codeProviderGitlab && r.SourceID == integ.ID && r.Kind == codeKindMR &&
			!strings.EqualFold(r.Repo, integ.ProjectPath) {
			existing[gitlab.MRKey(r.Repo, r.Number)] = r
		}
	}

	keep := []string{}
	reads := foreignMRReadBudget
	for _, mr := range mrs {
		if strings.EqualFold(mr.ProjectPath, integ.ProjectPath) {
			continue // the binding's own MRs are the project poll's
		}
		key := gitlab.MRKey(mr.ProjectPath, mr.IID)
		keep = append(keep, key)
		prev, had := existing[key]
		if had {
			mr.ProjectPath = prev.Repo // keep the stored spelling: one row per MR
		}
		if mr.Pipeline == nil && reads > 0 && needsMRRead(mr, prev, had) {
			reads--
			full, rerr := client.MergeRequestByPath(ctx, mr.ProjectPath, mr.IID)
			switch {
			case rerr == nil:
				mr.Pipeline = full.Pipeline
			case !gitlab.IsNoAccess(rerr):
				log.Printf("gitlab foreign mr %s: %v", key, rerr)
			}
		}
		via := gitlab.ViaMention
		if closes[key] {
			via = gitlab.ViaCloses
		}
		for id := range h.upsertForeignMR(ctx, integ, taskID, mr, via, prev, had) {
			touched[id] = true
		}
	}

	gone, err := h.q.DeleteStaleForeignCodeLinks(ctx, db.DeleteStaleForeignCodeLinksParams{
		TaskID: taskID, Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR,
		OwnRepo: integ.ProjectPath, Keep: keep,
	})
	if err != nil {
		log.Printf("gitlab foreign mr %s#%d: prune: %v", issuePath, issueIID, err)
	}
	for _, id := range gone {
		touched[id] = true
	}
	return touched, true
}

// needsMRRead tells whether an MR from a list is worth one more round trip for
// its pipeline: new to the task, still open (its pipeline may have moved without
// the MR changing), its pipeline last seen running, or changed since.
func needsMRRead(mr gitlab.MergeRequest, prev db.TaskCodeLink, had bool) bool {
	if !had || gitlab.ChangeState(mr.State) == gitlab.ChangeOpen || prev.CiStatus == gitlab.CIRunning {
		return true
	}
	if mr.UpdatedAt == nil || prev.RemoteUpdatedAt == nil {
		return true
	}
	return !mr.UpdatedAt.Equal(*prev.RemoteUpdatedAt)
}

// upsertForeignMR writes one foreign MR onto one task and journals its arrival
// and its merge / close there, like the project poll does for its own MRs.
func (h *API) upsertForeignMR(ctx context.Context, integ db.GitlabIntegration, taskID uuid.UUID, mr gitlab.MergeRequest, via string, prev db.TaskCodeLink, had bool) map[uuid.UUID]bool {
	touched := map[uuid.UUID]bool{}
	var ciStatus, ciURL string
	var ciRun int64
	if had {
		ciStatus, ciURL, ciRun = prev.CiStatus, prev.CiUrl, prev.CiRunID
	}
	if p := mr.Pipeline; p != nil && p.ID >= ciRun {
		ciStatus, ciURL, ciRun = gitlab.CIState(p.Status), p.WebURL, p.ID
	}
	state := gitlab.ChangeState(mr.State)
	ref := mr.ProjectPath + "!" + strconv.FormatInt(mr.IID, 10)
	changed, err := h.q.UpsertTaskCodeLink(ctx, db.UpsertTaskCodeLinkParams{
		TaskID: taskID, Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR,
		Number: mr.IID, ExternalID: mr.GlobalID, Repo: mr.ProjectPath, Url: mr.WebURL,
		Title: mr.Title, State: state, Draft: mr.Draft,
		SourceBranch: mr.SourceBranch, TargetBranch: mr.TargetBranch, Via: via,
		RemoteUpdatedAt: mr.UpdatedAt, CiStatus: ciStatus, CiUrl: ciURL, CiRunID: ciRun,
	})
	if err != nil {
		log.Printf("gitlab foreign mr %s: upsert task %s: %v", ref, taskID, err)
		return touched
	}
	if len(changed) > 0 {
		touched[taskID] = true
		if !had || (prev.State != state && state != gitlab.ChangeOpen) {
			h.logEventActor(ctx, taskID, uuid.Nil, "code_change", map[string]any{
				"source": codeProviderGitlab, "number": mr.IID, "ref": ref, "state": state,
				"branch": mr.SourceBranch, "title": mr.Title,
			})
		}
	}
	// A pipeline lands on every task the MR is linked to under this binding.
	if mr.Pipeline != nil {
		for id := range h.applyCI(ctx, integ, mr.ProjectPath, []int64{mr.IID}, *mr.Pipeline) {
			touched[id] = true
		}
	}
	return touched
}
