package handlers

import (
	"context"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
)

// GitLab linked items → Tessera relations (task #2591).
//
// Two steps, on purpose. The fetched links are first written verbatim into
// gitlab_issue_links, and only then projected onto the core `task_relations` table
// as source='gitlab' rows. That split is what makes the sync idempotent (the raw
// row is the dedup key, not the projection) and what lets a link survive until its
// other endpoint is imported: an unresolvable link simply stays in the table and is
// retried by every later run.
//
// Nothing GitLab-shaped crosses into the core — link_type, iids and project paths
// stop at the projection boundary, where RelationKind translates them.

// relationsSyncEnabled reports whether the binding pulls linked items. "pull" is the
// default; "off" opts out entirely (and costs no GitLab round-trips).
func relationsSyncEnabled(integ db.GitlabIntegration) bool {
	return integ.RelationsSync != "off"
}

// relationStats is one run's worth of link bookkeeping, folded into a single
// journal entry so relations never disturb the per-task created/updated counters.
type relationStats struct {
	added    int // relations newly projected onto the core table
	removed  int // projections dropped because GitLab no longer reports the link
	deferred int // links kept unresolved — the other endpoint isn't imported (yet)
}

// quiet reports that nothing worth journalling happened. Deferred links alone don't
// qualify: a link whose other endpoint is never imported would otherwise write a row
// on every single auto run forever. The count still rides along whenever there IS a
// change to report.
func (s relationStats) quiet() bool { return s.added == 0 && s.removed == 0 }

// syncRelations mirrors the linked items of the issues synced this run. syncedIIDs
// are the source issues actually inspected — stale-link pruning is scoped to them,
// so an issue left out of this run's scope never loses its relations.
//
// Best-effort throughout: every failure is logged and skipped, because a relation
// is an extra on top of a task that has already been synced successfully.
func (h *API) syncRelations(ctx context.Context, integ db.GitlabIntegration, client *gitlab.Client, syncedIIDs []int64, j *syncJournal) {
	if !relationsSyncEnabled(integ) || len(syncedIIDs) == 0 {
		return
	}
	// Stamped before the first upsert: links written or refreshed after this instant
	// are "seen this run", everything older is stale.
	runStart := time.Now()
	sort.Slice(syncedIIDs, func(a, b int) bool { return syncedIIDs[a] < syncedIIDs[b] })

	byIID := h.fetchLinkedItems(ctx, integ, client, syncedIIDs)
	var stats relationStats
	for _, src := range syncedIIDs {
		for _, li := range byIID[src] {
			if _, ok := gitlab.RelationKind(li.LinkType); !ok {
				continue // a link type Tessera has no relation for — skip, don't guess
			}
			if _, err := h.q.UpsertGitlabIssueLink(ctx, db.UpsertGitlabIssueLinkParams{
				IntegrationID:  integ.ID,
				SrcProjectPath: integ.ProjectPath,
				SrcIid:         src,
				DstProjectPath: li.ProjectPath,
				DstIid:         li.IID,
				LinkType:       li.LinkType,
				GlLinkID:       li.LinkID,
				GlWebUrl:       li.WebURL,
			}); err != nil {
				log.Printf("gitlab relations: upsert link %d→%s#%d: %v", src, li.ProjectPath, li.IID, err)
			}
			// GitLab links are bidirectional. When both endpoints live in THIS project,
			// mirror the reverse edge so the relation shows on both tasks even if the
			// other endpoint fell outside this run's scope (e.g. an incremental sync
			// that only touched one issue). Its src_iid is this project's own, so the
			// existing per-src stale-pruning still owns it. Cross-project reverses are
			// left to the other integration's own sync — recording them here under the
			// wrong integration_id would break that pruning.
			if li.ProjectPath == integ.ProjectPath {
				if _, err := h.q.UpsertGitlabIssueLink(ctx, db.UpsertGitlabIssueLinkParams{
					IntegrationID:  integ.ID,
					SrcProjectPath: integ.ProjectPath,
					SrcIid:         li.IID,
					DstProjectPath: integ.ProjectPath,
					DstIid:         src,
					LinkType:       gitlab.InverseLinkType(li.LinkType),
					GlLinkID:       li.LinkID,
					GlWebUrl:       li.WebURL,
				}); err != nil {
					log.Printf("gitlab relations: upsert reverse link %s#%d→%d: %v", li.ProjectPath, li.IID, src, err)
				}
			}
		}
	}

	// Prune first, resolve second: a link GitLab dropped must not be re-projected by
	// the resolve pass in the same run.
	stats.removed = h.pruneStaleRelations(ctx, integ, syncedIIDs, runStart)
	added, deferred := h.resolvePendingRelations(ctx, integ)
	stats.added, stats.deferred = added, deferred

	if stats.quiet() {
		return
	}
	j.add(journalAction{
		Direction:  "pull",
		EntityType: "relation",
		// Deliberately not create/update/delete: the run's task counters must keep
		// counting tasks, so this aggregate row carries its numbers in the detail.
		Op:      "link",
		Summary: relationSummary(stats),
		Detail: map[string]any{"relations": map[string]any{
			"added": stats.added, "removed": stats.removed, "deferred": stats.deferred,
		}},
	})
}

// fetchLinkedItems reads the links of every synced issue, preferring the batched
// GraphQL widget and falling back to the per-issue REST endpoint when this GitLab's
// schema has no LinkedItems widget (it is not present on every self-hosted install).
func (h *API) fetchLinkedItems(ctx context.Context, integ db.GitlabIntegration, client *gitlab.Client, iids []int64) map[int64][]gitlab.LinkedItem {
	strs := make([]string, 0, len(iids))
	for _, id := range iids {
		strs = append(strs, strconv.FormatInt(id, 10))
	}
	byIID, supported, err := client.LinkedItems(ctx, integ.ProjectPath, strs)
	if err == nil && supported {
		return byIID
	}
	if err != nil {
		log.Printf("gitlab relations: linked-items widget unavailable (%v), falling back to REST", err)
	}
	out := make(map[int64][]gitlab.LinkedItem, len(iids))
	for _, iid := range iids {
		items, rerr := client.IssueLinksREST(ctx, integ.ProjectPath, iid)
		if rerr != nil {
			log.Printf("gitlab relations: REST links for #%d: %v", iid, rerr)
			continue
		}
		if len(items) > 0 {
			out[iid] = items
		}
	}
	return out
}

// pruneStaleRelations drops the links GitLab no longer reports and unprojects the
// relations they had created. Relations the user has since made their own
// (source='user') survive — the delete is source-guarded in SQL.
func (h *API) pruneStaleRelations(ctx context.Context, integ db.GitlabIntegration, iids []int64, runStart time.Time) int {
	orphans, err := h.q.DeleteStaleGitlabIssueLinks(ctx, db.DeleteStaleGitlabIssueLinksParams{
		IntegrationID: integ.ID, SrcIids: iids, SeenBefore: runStart,
	})
	if err != nil {
		log.Printf("gitlab relations: prune stale links: %v", err)
		return 0
	}
	removed := 0
	for _, relID := range orphans {
		n, derr := h.q.DeleteGitlabRelation(ctx, relID)
		if derr != nil {
			log.Printf("gitlab relations: unproject %s: %v", relID, derr)
			continue
		}
		removed += int(n)
	}
	return removed
}

// resolvePendingRelations walks every link with no core relation yet — the ones just
// upserted plus older deferred ones — and projects those whose endpoints are now
// both imported. Returns how many relations were created and how many links stayed
// deferred.
func (h *API) resolvePendingRelations(ctx context.Context, integ db.GitlabIntegration) (added, deferred int) {
	pending, err := h.q.ListUnresolvedGitlabIssueLinks(ctx, integ.ID)
	if err != nil {
		log.Printf("gitlab relations: list unresolved: %v", err)
		return 0, 0
	}
	r := &relationResolver{h: h, integ: integ, tasks: map[string]*uuid.UUID{}, integs: map[string]*uuid.UUID{}}
	for _, link := range pending {
		kind, ok := gitlab.RelationKind(link.LinkType)
		if !ok {
			continue
		}
		src, sok := r.taskFor(ctx, link.SrcProjectPath, link.SrcIid)
		dst, dok := r.taskFor(ctx, link.DstProjectPath, link.DstIid)
		// Either endpoint missing → leave the row unresolved; a later run retries it
		// once the other issue has been imported.
		if !sok || !dok {
			deferred++
			continue
		}
		if src == dst {
			continue // GitLab shouldn't produce these, and task_relations forbids them
		}
		rel, aerr := h.q.AddTaskRelationSourced(ctx, db.AddTaskRelationSourcedParams{
			TaskID: src, RelatedTaskID: dst, Kind: kind, Source: "gitlab",
		})
		if aerr != nil {
			log.Printf("gitlab relations: project %s: %v", link.ID, aerr)
			continue
		}
		if serr := h.q.SetGitlabIssueLinkResolved(ctx, db.SetGitlabIssueLinkResolvedParams{
			ID: link.ID, ResolvedRelationID: &rel.ID,
		}); serr != nil {
			log.Printf("gitlab relations: mark resolved %s: %v", link.ID, serr)
		}
		added++
	}
	return added, deferred
}

// relationResolver maps a (project path, iid) pair to the Tessera task mirroring it,
// memoising both hops. A path other than the binding's own is resolved through a
// second binding *in the same workspace*, which is what keeps a relation from ever
// spanning two workspaces.
type relationResolver struct {
	h      *API
	integ  db.GitlabIntegration
	tasks  map[string]*uuid.UUID // "path#iid" → task (nil = no such task here)
	integs map[string]*uuid.UUID // project path → binding (nil = none in this workspace)
}

func (r *relationResolver) taskFor(ctx context.Context, path string, iid int64) (uuid.UUID, bool) {
	key := path + "#" + strconv.FormatInt(iid, 10)
	if cached, seen := r.tasks[key]; seen {
		if cached == nil {
			return uuid.Nil, false
		}
		return *cached, true
	}
	var out *uuid.UUID
	if integID, ok := r.integrationFor(ctx, path); ok {
		if link, err := r.h.q.GetGitlabLinkByIID(ctx, db.GetGitlabLinkByIIDParams{
			IntegrationID: integID, GlIid: iid,
		}); err == nil {
			id := link.TaskID
			out = &id
		}
	}
	r.tasks[key] = out
	if out == nil {
		return uuid.Nil, false
	}
	return *out, true
}

func (r *relationResolver) integrationFor(ctx context.Context, path string) (uuid.UUID, bool) {
	if path == r.integ.ProjectPath {
		return r.integ.ID, true
	}
	if cached, seen := r.integs[path]; seen {
		if cached == nil {
			return uuid.Nil, false
		}
		return *cached, true
	}
	var out *uuid.UUID
	other, err := r.h.q.GetGitlabIntegrationByWorkspaceProject(ctx, db.GetGitlabIntegrationByWorkspaceProjectParams{
		WorkspaceID: r.integ.WorkspaceID, ProjectPath: path,
	})
	if err == nil {
		id := other.ID
		out = &id
	}
	r.integs[path] = out
	if out == nil {
		return uuid.Nil, false
	}
	return *out, true
}

// relationSummary renders the aggregate journal line, e.g. "Связи: +2, −1, отложено 3".
func relationSummary(s relationStats) string {
	out := "Связи:"
	sep := " "
	if s.added > 0 {
		out += sep + "+" + strconv.Itoa(s.added)
		sep = ", "
	}
	if s.removed > 0 {
		out += sep + "−" + strconv.Itoa(s.removed)
		sep = ", "
	}
	if s.deferred > 0 {
		out += sep + "отложено " + strconv.Itoa(s.deferred)
	}
	return out
}
