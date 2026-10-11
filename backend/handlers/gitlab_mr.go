package handlers

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	db "tessera/internal/db"
	"tessera/internal/gitlab"
)

// ── Merge requests and pipelines in «Разработка» (#2964) ───
//
// GitLab merge requests reach a task through the issue it mirrors: an MR that
// closes, mentions or was branched from issue #N is linked to the task linked to
// #N. The rows live in the provider-neutral task_code_links (migration 0084);
// everything GitLab-specific — reading MRs, deciding which issues they are about,
// translating states — is in internal/gitlab, and this file is the glue between
// the two.
//
// Two feeds keep the rows fresh:
//   - the merge_request and pipeline webhooks, applied straight from the payload
//     (they are self-contained, unlike an issue hook, which needs the whole pull);
//   - a poll at the end of every pull: MRs updated since the last poll, plus the
//     open MRs whose pipeline is still running — a pipeline finishing does not
//     touch the MR, so the delta alone would leave it «идёт» forever.

const (
	codeProviderGitlab = "gitlab"
	codeKindMR         = "merge_request"

	// mrBackfill is how far back the very first poll of a binding reaches. Older
	// MRs are history nobody opens a task for; a whole-project fetch on a big
	// repository would only make the first sync slower.
	mrBackfill = 30 * 24 * time.Hour
	// mrClosesBudget bounds the REST «closes issues» look-ups per poll: one round
	// trip per MR. The text and branch references cover what is past the budget.
	mrClosesBudget = 30
)

// codeLinkView is a linked code change as the task window reads it. Ref is how
// the change is named to a reader: "!418" for a change of the binding's own
// repository, "group/proj!12" for one of another (#2964 rework).
type codeLinkView struct {
	Provider     string     `json:"provider"`
	Kind         string     `json:"kind"`
	Number       int64      `json:"number"`
	Repo         string     `json:"repo"`
	Ref          string     `json:"ref"`
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	State        string     `json:"state"`
	Draft        bool       `json:"draft"`
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	CIStatus     string     `json:"ci_status"`
	CIURL        string     `json:"ci_url"`
	Via          string     `json:"via"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

func codeLinkViewOf(l db.TaskCodeLink, ownRepo string) codeLinkView {
	return codeLinkView{
		Provider: l.Provider, Kind: l.Kind, Number: l.Number, Repo: l.Repo, URL: l.Url,
		Ref:   changeRef(l.Repo, ownRepo, l.Number),
		Title: l.Title, State: l.State, Draft: l.Draft,
		SourceBranch: l.SourceBranch, TargetBranch: l.TargetBranch,
		CIStatus: l.CiStatus, CIURL: l.CiUrl, Via: l.Via, UpdatedAt: l.RemoteUpdatedAt,
	}
}

// changeRef names a change for a reader: the bare "!12" inside the binding's own
// repository, the qualified "group/proj!12" for any other.
func changeRef(repo, ownRepo string, number int64) string {
	short := "!" + strconv.FormatInt(number, 10)
	if repo == "" || strings.EqualFold(repo, ownRepo) {
		return short
	}
	return repo + short
}

// codeLinksForTask lists a task's linked changes, never nil (the client reads an
// empty list as «no MR yet» and keeps the issue link / «Создать issue»).
func (h *API) codeLinksForTask(ctx context.Context, taskID uuid.UUID) []codeLinkView {
	rows, err := h.q.ListTaskCodeLinks(ctx, taskID)
	out := make([]codeLinkView, 0, len(rows))
	if err != nil {
		return out
	}
	own := map[uuid.UUID]string{}
	for _, r := range rows {
		repo, ok := own[r.SourceID]
		if !ok && r.Provider == codeProviderGitlab {
			if integ, ierr := h.q.GetGitlabIntegrationByID(ctx, r.SourceID); ierr == nil {
				repo = integ.ProjectPath
			}
			own[r.SourceID] = repo
		}
		out = append(out, codeLinkViewOf(r, repo))
	}
	return out
}

// mrApply is one MR to write: what GitLab said, plus what we know about which
// issues it closes. closes == nil means "not asked" — then the links can only
// grow (a missing reference may just be one the closes list would have held).
type mrApply struct {
	mr     gitlab.MergeRequest
	closes []int64
	known  bool // closes was answered: stale links may be pruned
}

// applyMergeRequest writes one MR onto the tasks it is about and returns the
// tasks whose «Разработка» changed. Journals a line when an MR first appears on a
// task and when it is merged or closed there.
func (h *API) applyMergeRequest(ctx context.Context, integ db.GitlabIntegration, a mrApply) map[uuid.UUID]bool {
	touched := map[uuid.UUID]bool{}
	mr := a.mr
	targets := gitlab.IssueTargets(integ.ProjectPath, mr, a.closes)
	iids := make([]int64, 0, len(targets))
	for iid := range targets {
		iids = append(iids, iid)
	}
	taskVia := map[uuid.UUID]string{}
	if len(iids) > 0 {
		rows, err := h.q.GitlabTasksForIids(ctx, db.GitlabTasksForIidsParams{IntegrationID: integ.ID, Iids: iids})
		if err != nil {
			log.Printf("gitlab mr !%d: resolve tasks: %v", mr.IID, err)
			return touched
		}
		for _, r := range rows {
			taskVia[r.TaskID] = targets[r.GlIid]
		}
	}

	existing, err := h.q.ListCodeLinksForChange(ctx, db.ListCodeLinksForChangeParams{
		Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR, Repo: integ.ProjectPath, Number: mr.IID,
	})
	if err != nil {
		log.Printf("gitlab mr !%d: load links: %v", mr.IID, err)
		return touched
	}
	before := make(map[uuid.UUID]db.TaskCodeLink, len(existing))
	for _, e := range existing {
		before[e.TaskID] = e
		if !a.known {
			// Without the closes list a link we hold may rest on it: keep it, and
			// keep the stronger reason it was made for.
			if v, ok := taskVia[e.TaskID]; !ok || gitlab.ViaRank(e.Via) > gitlab.ViaRank(v) {
				taskVia[e.TaskID] = e.Via
			}
		}
	}

	// A task newly linked inherits the CI state its siblings already carry —
	// the hook that would tell it arrived before the link existed.
	var ciStatus, ciURL string
	var ciRun int64
	for _, e := range existing {
		if e.CiRunID >= ciRun {
			ciStatus, ciURL, ciRun = e.CiStatus, e.CiUrl, e.CiRunID
		}
	}
	if p := mr.Pipeline; p != nil && p.ID >= ciRun {
		ciStatus, ciURL, ciRun = gitlab.CIState(p.Status), p.WebURL, p.ID
	}

	state := gitlab.ChangeState(mr.State)
	for taskID, via := range taskVia {
		changed, err := h.q.UpsertTaskCodeLink(ctx, db.UpsertTaskCodeLinkParams{
			TaskID: taskID, Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR,
			Number: mr.IID, ExternalID: mr.GlobalID, Repo: integ.ProjectPath, Url: mr.WebURL,
			Title: mr.Title, State: state, Draft: mr.Draft,
			SourceBranch: mr.SourceBranch, TargetBranch: mr.TargetBranch, Via: via,
			RemoteUpdatedAt: mr.UpdatedAt, CiStatus: ciStatus, CiUrl: ciURL, CiRunID: ciRun,
		})
		if err != nil {
			log.Printf("gitlab mr !%d: upsert task %s: %v", mr.IID, taskID, err)
			continue
		}
		if len(changed) == 0 {
			continue
		}
		touched[taskID] = true
		prev, had := before[taskID]
		if !had || (prev.State != state && state != gitlab.ChangeOpen) {
			h.logEventActor(ctx, taskID, uuid.Nil, "code_change", map[string]any{
				"source": codeProviderGitlab, "number": mr.IID, "state": state,
				"branch": mr.SourceBranch, "title": mr.Title,
			})
		}
	}

	if a.known {
		keep := make([]uuid.UUID, 0, len(taskVia))
		for id := range taskVia {
			keep = append(keep, id)
		}
		gone, err := h.q.DeleteStaleCodeLinks(ctx, db.DeleteStaleCodeLinksParams{
			Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR, Repo: integ.ProjectPath, Number: mr.IID, Keep: keep,
		})
		if err != nil {
			log.Printf("gitlab mr !%d: prune links: %v", mr.IID, err)
		}
		for _, id := range gone {
			touched[id] = true
		}
	}

	if mr.Pipeline != nil {
		for id := range h.applyCI(ctx, integ, integ.ProjectPath, []int64{mr.IID}, *mr.Pipeline) {
			touched[id] = true
		}
	}
	return touched
}

// applyCI records a pipeline run on some MRs of one repository a binding sees.
func (h *API) applyCI(ctx context.Context, integ db.GitlabIntegration, repo string, numbers []int64, p gitlab.Pipeline) map[uuid.UUID]bool {
	touched := map[uuid.UUID]bool{}
	if len(numbers) == 0 {
		return touched
	}
	ids, err := h.q.SetCodeChangeCI(ctx, db.SetCodeChangeCIParams{
		CiStatus: gitlab.CIState(p.Status), CiUrl: p.WebURL, CiRunID: p.ID,
		Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR, Repo: repo, Numbers: numbers,
	})
	if err != nil {
		log.Printf("gitlab pipeline %d: %v", p.ID, err)
		return touched
	}
	for _, id := range ids {
		touched[id] = true
	}
	return touched
}

// applyPipelineEvent lands a pipeline hook: on its MR when it ran for one, else on
// every open MR from the branch it ran on.
func (h *API) applyPipelineEvent(ctx context.Context, integ db.GitlabIntegration, ev gitlab.PipelineEvent) map[uuid.UUID]bool {
	numbers := []int64{}
	if ev.MRIID > 0 {
		numbers = append(numbers, ev.MRIID)
	} else if ev.Ref != "" {
		ns, err := h.q.OpenCodeChangesOnBranch(ctx, db.OpenCodeChangesOnBranchParams{
			Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR, Repo: integ.ProjectPath, SourceBranch: ev.Ref,
		})
		if err != nil {
			log.Printf("gitlab pipeline %d: branch MRs: %v", ev.Pipeline.ID, err)
		}
		numbers = append(numbers, ns...)
	}
	return h.applyCI(ctx, integ, integ.ProjectPath, numbers, ev.Pipeline)
}

// announceCodeLinks tells open task windows that their «Разработка» changed. The
// payload carries the fresh list, so the window repaints without a request. The
// change came from GitLab, not from a member — no actor.
func (h *API) announceCodeLinks(ctx context.Context, wsID uuid.UUID, touched map[uuid.UUID]bool) {
	for taskID := range touched {
		h.broadcastByActor(wsID, "task.code_links", uuid.Nil, map[string]any{
			"task_id": taskID, "code_links": h.codeLinksForTask(ctx, taskID),
		})
	}
}

// syncMergeRequests is the poll half: run at the end of a pull, best-effort — a
// GitLab without the MR connection, or a failing query, must never fail the issue
// sync it rides along with. The cursor only moves on success.
func (h *API) syncMergeRequests(ctx context.Context, integ db.GitlabIntegration, client *gitlab.Client) {
	started := time.Now()
	since := started.Add(-mrBackfill)
	if integ.MrSyncedAt != nil {
		since = integ.MrSyncedAt.Add(-syncOverlap)
	}
	mrs, err := client.MergeRequestsSince(ctx, integ.ProjectPath, &since)
	if err != nil {
		log.Printf("gitlab mr poll ws=%s: %v", integ.WorkspaceID, err)
		return
	}
	// Re-read the MRs whose pipeline we last saw running and the delta didn't bring.
	inDelta := make(map[int64]bool, len(mrs))
	for _, mr := range mrs {
		inDelta[mr.IID] = true
	}
	running, _ := h.q.CodeChangesWithRunningCI(ctx, db.CodeChangesWithRunningCIParams{
		Provider: codeProviderGitlab, SourceID: integ.ID, Kind: codeKindMR, Repo: integ.ProjectPath,
	})
	var stale []int64
	for _, n := range running {
		if !inDelta[n] {
			stale = append(stale, n)
		}
	}
	if len(stale) > 0 {
		more, rerr := client.MergeRequestsByIIDs(ctx, integ.ProjectPath, stale)
		if rerr != nil {
			log.Printf("gitlab mr poll ws=%s: running pipelines: %v", integ.WorkspaceID, rerr)
		}
		mrs = append(mrs, more...)
	}

	touched := map[uuid.UUID]bool{}
	budget := mrClosesBudget
	for _, mr := range mrs {
		a := mrApply{mr: mr}
		// GitLab's own closes list only matters while the MR can still close
		// something, and it costs a round trip — spend it on the open ones.
		if budget > 0 && gitlab.ChangeState(mr.State) == gitlab.ChangeOpen {
			budget--
			if closes, cerr := client.ClosesIssues(ctx, integ.ProjectPath, mr.IID); cerr == nil {
				a.closes, a.known = closes, true
			}
		}
		for id := range h.applyMergeRequest(ctx, integ, a) {
			touched[id] = true
		}
	}
	h.announceCodeLinks(ctx, integ.WorkspaceID, touched)
	if err := h.q.MarkGitlabMRSynced(ctx, db.MarkGitlabMRSyncedParams{ID: integ.ID, MrSyncedAt: &started}); err != nil {
		log.Printf("gitlab mr poll ws=%s: cursor: %v", integ.WorkspaceID, err)
	}
}
