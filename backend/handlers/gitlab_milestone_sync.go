package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// GitLab stages both ways (#3010: #3023 edit, #3024 close/reopen, #3025 conflict).
//
// A stage linked to a GitLab milestone used to be read-only in Tessera: the pull
// overwrote it with GitLab's values. Now it is edited in Tessera too, and an edit
// is written to GitLab right away when the binding allows writing (binding on and
// write-back on — milestoneWritable). Both directions compare three versions per
// field, the way the issue write-back does (gitlab_conflict.go): the baseline
// (gitlab_milestone_links.gl_snapshot — GitLab's value at the last successful
// sync), ours (the stage now) and theirs (GitLab now).
//
//	ours == theirs           → in sync; the baseline catches up
//	theirs == base           → only Tessera moved: push ours
//	ours == base             → only GitLab moved: take theirs
//	all three differ         → a conflict, parked on the link for the window
//
// The write is synchronous (a stage is edited rarely and the editor wants the
// answer), with no outbox: a failed write leaves the stage changed locally and
// marked «not synced» (push_error) — the pull keeps the local value because
// theirs still equals the baseline, the stage strip offers «retry», and the sync
// retries by itself after its next pull.

// milestoneSyncFields are the stage fields mirrored with GitLab, in the order the
// conflicts window lists them.
var milestoneSyncFields = []string{"title", "description", "start", "due", "state"}

// milestoneSnap is one version of a stage's mirrored fields. Dates are
// YYYY-MM-DD ("" when unset), state is active | closed.
type milestoneSnap struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Start       string `json:"start"`
	Due         string `json:"due"`
	State       string `json:"state"`
}

func (s milestoneSnap) get(field string) string {
	switch field {
	case "title":
		return s.Title
	case "description":
		return s.Description
	case "start":
		return s.Start
	case "due":
		return s.Due
	case "state":
		return s.State
	}
	return ""
}

func (s *milestoneSnap) set(field, v string) {
	switch field {
	case "title":
		s.Title = v
	case "description":
		s.Description = v
	case "start":
		s.Start = v
	case "due":
		s.Due = v
	case "state":
		s.State = v
	}
}

func normMilestoneState(s string) string {
	if s == "closed" {
		return "closed"
	}
	return "active"
}

// snapOfMilestone is ours: the stage as Tessera holds it.
func snapOfMilestone(m db.Milestone) milestoneSnap {
	return milestoneSnap{
		Title: m.Title, Description: m.Description,
		Start: dateStr(m.StartDate), Due: dateStr(m.DueDate), State: normMilestoneState(m.State),
	}
}

// snapOfGitlab is theirs as REST reports it (the write path).
func snapOfGitlab(gm gitlab.Milestone) milestoneSnap {
	return milestoneSnap{
		Title: gm.Title, Description: gm.Description,
		Start: gm.StartDate, Due: gm.DueDate, State: normMilestoneState(gm.State),
	}
}

// snapOfIssue is theirs as the pull sees it, through an issue that carries it.
func snapOfIssue(issue gitlab.Issue) milestoneSnap {
	return milestoneSnap{
		Title: issue.MilestoneTitle, Description: issue.MilestoneDescription,
		Start: dateStr(issue.MilestoneStart), Due: dateStr(issue.MilestoneDue),
		State: normMilestoneState(issue.MilestoneState),
	}
}

// parseMilestoneSnap reads the baseline; ok is false while the link has none
// (a link older than the baseline, before the backfill could reach it).
func parseMilestoneSnap(raw []byte) (milestoneSnap, bool) {
	present := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &present); err != nil || len(present) == 0 {
		return milestoneSnap{}, false
	}
	var s milestoneSnap
	_ = json.Unmarshal(raw, &s)
	return s, true
}

func (s milestoneSnap) marshal() []byte {
	b, err := json.Marshal(s)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// parseSnapDate turns a YYYY-MM-DD back into the UTC-midnight instant stages
// store a date under; "" (or garbage) is no date.
func parseSnapDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

// milestoneConflict is the parked conflict jsonb on the link.
type milestoneConflict struct {
	Fields     []conflictField `json:"fields"`
	DetectedAt time.Time       `json:"detected_at"`
}

func parseMilestoneConflict(raw []byte) milestoneConflict {
	var c milestoneConflict
	_ = json.Unmarshal(raw, &c)
	return c
}

// threeWay merges one field: what the stage should hold, what the new baseline
// is, and whether the field is pushed or in conflict.
type fieldMerge struct {
	local    string
	base     string
	push     bool
	conflict bool
}

func mergeMilestoneField(ours, base, theirs string) fieldMerge {
	switch {
	case ours == theirs:
		return fieldMerge{local: ours, base: theirs}
	case theirs == base:
		return fieldMerge{local: ours, base: base, push: true}
	case ours == base:
		return fieldMerge{local: theirs, base: theirs}
	default:
		return fieldMerge{local: ours, base: base, conflict: true}
	}
}

// milestoneWritable reports whether a stage of this binding may be written to
// GitLab: the binding is on and its write-back is on. Off — the stage stays
// read-only in Tessera, as before #3010.
func milestoneWritable(integ db.GitlabIntegration) bool {
	return integ.Enabled && parseWriteback(integ.Writeback).Enabled
}

// glSyncResult is what a write to GitLab came to, for the editor.
type glSyncResult struct {
	Status string   `json:"status"` // ok | failed | conflict | readonly
	Error  string   `json:"error,omitempty"`
	Fields []string `json:"fields,omitempty"` // conflicting or unwritten fields
}

// applyMilestoneSnap writes the mirrored fields into the stage. UpdateMilestone
// keeps closed_at following the state.
func (h *API) applyMilestoneSnap(ctx context.Context, id uuid.UUID, s milestoneSnap) (db.Milestone, error) {
	return h.q.UpdateMilestone(ctx, db.UpdateMilestoneParams{
		ID: id, Title: s.Title, Description: s.Description,
		StartDate: parseSnapDate(s.Start), DueDate: parseSnapDate(s.Due), State: normMilestoneState(s.State),
	})
}

// setMilestoneConflict parks the given fields on the link (or clears the
// conflict when there are none) and tells the workspace when that changed.
func (h *API) setMilestoneConflict(ctx context.Context, link db.GitlabMilestoneLink, wsID uuid.UUID, fields []conflictField) {
	had := len(parseMilestoneConflict(link.Conflict).Fields) > 0
	raw := []byte("{}")
	if len(fields) > 0 {
		if b, err := json.Marshal(milestoneConflict{Fields: fields, DetectedAt: time.Now().UTC()}); err == nil {
			raw = b
		}
	}
	soft(ctx, "SetGitlabMilestoneConflict", h.q.SetGitlabMilestoneConflict(ctx, db.SetGitlabMilestoneConflictParams{
		MilestoneID: link.MilestoneID, Conflict: raw,
	}))
	if had || len(fields) > 0 {
		h.broadcast(wsID, "gitlab.conflict", gin.H{"milestone_id": link.MilestoneID, "resolved": len(fields) == 0})
	}
}

// syncMilestoneToGitlab writes a linked stage's local changes to GitLab, three
// ways: what only Tessera changed is pushed, what only GitLab changed is taken,
// what both changed is parked as a conflict. actorID is who the write goes out
// as (their PAT, else the service token with sudo, else the owner's PAT); nil —
// the binding's owner.
func (h *API) syncMilestoneToGitlab(ctx context.Context, milestoneID uuid.UUID, actorID *uuid.UUID) glSyncResult {
	link, err := h.q.GetGitlabMilestoneLink(ctx, milestoneID)
	if err != nil {
		return glSyncResult{Status: "failed", Error: "stage is not linked to GitLab"}
	}
	integ, err := h.q.GetGitlabIntegrationByID(ctx, link.IntegrationID)
	if err != nil {
		return glSyncResult{Status: "failed", Error: truncErr(err)}
	}
	if !milestoneWritable(integ) {
		return glSyncResult{Status: "readonly"}
	}
	m, err := h.q.GetMilestone(ctx, milestoneID)
	if err != nil {
		return glSyncResult{Status: "failed", Error: truncErr(err)}
	}
	ours := snapOfMilestone(m)

	// From here a failure is remembered on the link: the stage stays changed
	// locally and is marked «not synced» until a retry gets through.
	markFailed := func(msg string, fields []string) glSyncResult {
		soft(ctx, "SetGitlabMilestonePushResult", h.q.SetGitlabMilestonePushResult(ctx, db.SetGitlabMilestonePushResultParams{
			MilestoneID: milestoneID, PushError: msg, PushFields: fields, PushActorID: actorID,
		}))
		return glSyncResult{Status: "failed", Error: msg, Fields: fields}
	}
	// Every field that differs from the baseline is what a failure leaves unwritten.
	base, hasBase := parseMilestoneSnap(link.GlSnapshot)
	var pending []string
	for _, f := range milestoneSyncFields {
		if !hasBase || ours.get(f) != base.get(f) {
			pending = append(pending, f)
		}
	}

	baseURL, token, sudo, ok := h.writeGitlabConn(ctx, actorID, integ.OwnerUserID)
	if !ok {
		return markFailed("no GitLab account to write as: connect GitLab or ask an admin for a service token", pending)
	}
	client := gitlab.New(baseURL, token).WithSudo(sudo)
	current, err := client.GetProjectMilestone(ctx, integ.ProjectPath, link.GlNumericID)
	if err != nil {
		log.Printf("gitlab milestone %s: read: %v", milestoneID, err)
		return markFailed(truncErr(err), pending)
	}
	theirs := snapOfGitlab(current)
	if !hasBase {
		// Nothing to compare against: Tessera's edit is the newest intent.
		base = theirs
	}

	var (
		local     = ours
		newBase   = base
		push      gitlab.MilestoneUpdate
		pushed    []string
		conflicts []conflictField
	)
	for _, f := range milestoneSyncFields {
		r := mergeMilestoneField(ours.get(f), base.get(f), theirs.get(f))
		local.set(f, r.local)
		newBase.set(f, r.base)
		switch {
		case r.conflict:
			conflicts = append(conflicts, conflictField{
				Field: f, Base: base.get(f), Ours: ours.get(f), Theirs: theirs.get(f), basePresent: true,
			})
		case r.push:
			pushed = append(pushed, f)
			v := ours.get(f)
			switch f {
			case "title":
				push.Title = &v
			case "description":
				push.Description = &v
			case "start":
				push.StartDate = &v
			case "due":
				push.DueDate = &v
			case "state":
				push.StateEvent = "activate"
				if v == "closed" {
					push.StateEvent = "close"
				}
			}
		}
	}

	if !push.Empty() {
		if _, perr := client.UpdateProjectMilestone(ctx, integ.ProjectPath, link.GlNumericID, push); perr != nil {
			log.Printf("gitlab milestone %s: write %v: %v", milestoneID, pushed, perr)
			return markFailed(truncErr(perr), pushed)
		}
		for _, f := range pushed {
			newBase.set(f, ours.get(f))
		}
	}
	// What GitLab changed alone comes in with the same round trip.
	if local != ours {
		if _, uerr := h.applyMilestoneSnap(ctx, milestoneID, local); uerr != nil {
			return markFailed(truncErr(uerr), pending)
		}
	}
	soft(ctx, "SetGitlabMilestoneSnapshot", h.q.SetGitlabMilestoneSnapshot(ctx, db.SetGitlabMilestoneSnapshotParams{
		MilestoneID: milestoneID, GlSnapshot: newBase.marshal(),
	}))
	soft(ctx, "UpdateGitlabMilestoneLink", h.q.UpdateGitlabMilestoneLink(ctx, db.UpdateGitlabMilestoneLinkParams{
		MilestoneID: milestoneID, GlIid: link.GlIid, GlNumericID: link.GlNumericID,
		GlWebUrl: firstNonEmpty(current.WebURL, link.GlWebUrl), GlState: newBase.State,
		TitleHash: gitlab.HashStr(newBase.Title),
	}))
	soft(ctx, "SetGitlabMilestonePushResult", h.q.SetGitlabMilestonePushResult(ctx, db.SetGitlabMilestonePushResultParams{
		MilestoneID: milestoneID, PushError: "", PushFields: []string{}, PushActorID: nil,
	}))
	if wsID, werr := h.q.WorkspaceIDForProject(ctx, m.ProjectID); werr == nil {
		h.setMilestoneConflict(ctx, link, wsID, conflicts)
	}
	if len(conflicts) > 0 {
		names := make([]string, len(conflicts))
		for i, c := range conflicts {
			names[i] = c.Field
		}
		return glSyncResult{Status: "conflict", Fields: names}
	}
	return glSyncResult{Status: "ok"}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// pullGitlabMilestone merges GitLab's version of a linked stage (seen through an
// issue during the pull) into the stage, three ways. A field only GitLab changed
// is taken; one only Tessera changed (a write that has not gone through yet) is
// kept; one both changed is parked as a conflict. Without a baseline — or when
// the binding no longer writes to GitLab (writable false), so nothing local could
// ever go out — GitLab wins on every field, the behaviour from before #3010, and
// a leftover «not synced» mark is dropped with the edit it stood for.
func (h *API) pullGitlabMilestone(ctx context.Context, link db.GitlabMilestoneLink, wsID uuid.UUID, theirs milestoneSnap, writable bool) error {
	m, err := h.q.GetMilestone(ctx, link.MilestoneID)
	if err != nil {
		return err
	}
	ours := snapOfMilestone(m)
	base, hasBase := parseMilestoneSnap(link.GlSnapshot)
	local, newBase := theirs, theirs
	var conflicts []conflictField
	if !writable && link.PushError != "" {
		soft(ctx, "SetGitlabMilestonePushResult", h.q.SetGitlabMilestonePushResult(ctx, db.SetGitlabMilestonePushResultParams{
			MilestoneID: link.MilestoneID, PushError: "", PushFields: []string{}, PushActorID: nil,
		}))
	}
	if hasBase && writable {
		local, newBase = ours, base
		for _, f := range milestoneSyncFields {
			r := mergeMilestoneField(ours.get(f), base.get(f), theirs.get(f))
			if r.push {
				continue // ours is waiting to be written; keep it and its baseline
			}
			local.set(f, r.local)
			newBase.set(f, r.base)
			if r.conflict {
				conflicts = append(conflicts, conflictField{
					Field: f, Base: base.get(f), Ours: ours.get(f), Theirs: theirs.get(f), basePresent: true,
				})
			}
		}
	}
	if local != ours {
		if _, uerr := h.applyMilestoneSnap(ctx, link.MilestoneID, local); uerr != nil {
			return uerr
		}
	}
	soft(ctx, "SetGitlabMilestoneSnapshot", h.q.SetGitlabMilestoneSnapshot(ctx, db.SetGitlabMilestoneSnapshotParams{
		MilestoneID: link.MilestoneID, GlSnapshot: newBase.marshal(),
	}))
	// Re-park on every pull: a field is in conflict exactly while all three
	// versions differ, so GitLab changing back (or catching up) clears it.
	prev := parseMilestoneConflict(link.Conflict).Fields
	if len(prev) > 0 || len(conflicts) > 0 {
		if !sameConflict(prev, conflicts) {
			h.setMilestoneConflict(ctx, link, wsID, conflicts)
		}
	}
	return nil
}

// sameConflict compares two conflict field lists by content.
func sameConflict(a, b []conflictField) bool {
	return slices.EqualFunc(a, b, func(x, y conflictField) bool {
		return x.Field == y.Field && x.Base == y.Base && x.Ours == y.Ours && x.Theirs == y.Theirs
	})
}

// retryMilestonePushes re-sends the binding's stage writes that failed, after a
// pull (the sync's own heartbeat stands in for a retry worker).
func (h *API) retryMilestonePushes(ctx context.Context, integ db.GitlabIntegration) {
	if !milestoneWritable(integ) {
		return
	}
	links, err := h.q.ListPendingGitlabMilestonePushes(ctx, integ.ID)
	if err != nil {
		return
	}
	for _, l := range links {
		res := h.syncMilestoneToGitlab(ctx, l.MilestoneID, l.PushActorID)
		if res.Status != "failed" {
			h.broadcast(integ.WorkspaceID, "milestone.updated", gin.H{"id": l.MilestoneID})
		}
	}
}

// milestoneWithSync is a stage plus how its write to GitLab went.
type milestoneWithSync struct {
	db.Milestone
	GlSync *glSyncResult `json:"gl_sync,omitempty"`
}

// RetryMilestoneGitlab re-sends a linked stage's local changes to GitLab — the
// «retry» of the «not synced» mark. POST /milestones/:id/gitlab/sync.
func (h *API) RetryMilestoneGitlab(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	_, wsID, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	link, err := h.q.GetGitlabMilestoneLink(c, id)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stage is not linked to GitLab"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireWritableMilestone(c, link) {
		return
	}
	actor := middleware.CurrentUser(c)
	ctx, cancel := gitlabWriteCtx(c)
	defer cancel()
	res := h.syncMilestoneToGitlab(ctx, id, &actor)
	m, err := h.q.GetMilestone(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "milestone.updated", m)
	c.JSON(http.StatusOK, milestoneWithSync{Milestone: m, GlSync: &res})
}

// gitlabWriteCtx bounds a synchronous GitLab write and detaches it from the
// client: a closed tab must not leave the stage half-written.
func gitlabWriteCtx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.Request.Context()), 20*time.Second)
}

// requireWritableMilestone answers 409 when the stage's binding does not allow
// writing to GitLab (binding or its write-back off): such a stage stays
// read-only in Tessera, otherwise the next pull would quietly undo the edit.
func (h *API) requireWritableMilestone(c *gin.Context, link db.GitlabMilestoneLink) bool {
	integ, err := h.q.GetGitlabIntegrationByID(c, link.IntegrationID)
	if err != nil {
		fail(c, err)
		return false
	}
	if !milestoneWritable(integ) {
		c.JSON(http.StatusConflict, gin.H{
			"error": "this stage comes from GitLab and its binding does not write to GitLab",
			"code":  "gitlab_readonly",
		})
		return false
	}
	return true
}

// ResolveMilestoneConflict applies the user's decision on a stage's parked
// conflict — the stage counterpart of ResolveGitlabConflict:
//   - theirs: GitLab's values go into the stage; nothing is pushed.
//   - ours:   GitLab's values become the baseline and the stage is pushed.
//   - manual: the given values go into the stage, then it is pushed.
//
// The push re-reads GitLab, so a field GitLab moved yet again is parked anew.
// POST /milestones/:id/gitlab/conflict/resolve {resolution, value}.
func (h *API) ResolveMilestoneConflict(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Resolution string            `json:"resolution"`
		Value      map[string]string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.Resolution != "theirs" && req.Resolution != "ours" && req.Resolution != "manual" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resolution must be ours|theirs|manual"})
		return
	}
	m, wsID, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	link, err := h.q.GetGitlabMilestoneLink(c, id)
	if err != nil {
		if notFound(c, err) {
			return
		}
		fail(c, err)
		return
	}
	cd := parseMilestoneConflict(link.Conflict)
	if len(cd.Fields) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "conflict not found"})
		return
	}
	if req.Resolution != "theirs" && !h.requireWritableMilestone(c, link) {
		return
	}
	local := snapOfMilestone(m)
	base, _ := parseMilestoneSnap(link.GlSnapshot)
	for _, f := range cd.Fields {
		switch req.Resolution {
		case "theirs":
			local.set(f.Field, f.Theirs)
		case "manual":
			v := req.Value[f.Field]
			if f.Field == "state" {
				v = normMilestoneState(v)
			}
			local.set(f.Field, v)
		}
		base.set(f.Field, f.Theirs) // GitLab's value is acknowledged either way
	}
	if local != snapOfMilestone(m) {
		if _, err := h.applyMilestoneSnap(c, id, local); err != nil {
			fail(c, err)
			return
		}
	}
	soft(c, "SetGitlabMilestoneSnapshot", h.q.SetGitlabMilestoneSnapshot(c, db.SetGitlabMilestoneSnapshotParams{
		MilestoneID: id, GlSnapshot: base.marshal(),
	}))
	h.setMilestoneConflict(c, link, wsID, nil)

	var res *glSyncResult
	if req.Resolution != "theirs" {
		actor := middleware.CurrentUser(c)
		ctx, cancel := gitlabWriteCtx(c)
		r := h.syncMilestoneToGitlab(ctx, id, &actor)
		cancel()
		res = &r
	}
	out, err := h.q.GetMilestone(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "milestone.updated", out)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "resolution": req.Resolution, "milestone": out, "gl_sync": res})
}
