package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// GitLab write-back conflict resolution (phase B+).
//
// The MVP loop-guard only avoided echoing a value GitLab already had — it never
// noticed GitLab changing the same field between syncs, so a push was effectively
// last-writer-wins. This adds three-way detection against a per-link baseline
// (gl_snapshot): at push time we compare GitLab's current value (theirs), the last
// synced value (base) and Tessera's desired value (ours). When both sides moved
// away from the baseline the push is parked as a conflict for an interactive
// ours/theirs/manual decision, instead of clobbering the other side.
//
// Conflict-checked kinds (this pass): "due", "estimate". Title/description join in
// the title_desc step. The remaining kinds (state/priority/labels/assignees/comment)
// keep their existing behaviour until extended — no regression.

// glSnapshot is the last-synced GitLab field state for a linked issue (stored as
// gitlab_links.gl_snapshot). It is the baseline ("base") for conflict detection.
// Fields are added as more kinds become conflict-checked.
type glSnapshot struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	State        string `json:"state"`         // opened | closed
	Due          string `json:"due"`           // YYYY-MM-DD, "" when unset
	TimeEstimate int64  `json:"time_estimate"` // minutes, 0 when unset
}

// buildGlSnapshot captures the conflict-relevant fields of a freshly fetched issue.
func buildGlSnapshot(issue gitlab.Issue) []byte {
	snap := glSnapshot{
		Title:        issue.Title,
		Description:  issue.Description,
		State:        issue.State,
		Due:          dateStr(issue.DueDate),
		TimeEstimate: issue.TimeEstimate / 60,
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// snapshotPresence parses the snapshot both as typed values and as a presence map,
// so detection can tell "field absent (no baseline yet → safe to push)" from
// "field present and empty".
func snapshotPresence(raw []byte) (glSnapshot, map[string]json.RawMessage) {
	var snap glSnapshot
	_ = json.Unmarshal(raw, &snap)
	present := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &present)
	return snap, present
}

// dateStr formats an optional date as YYYY-MM-DD (UTC), "" when nil — the
// representation GitLab uses for issue due dates.
func dateStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// minutesStr renders an optional estimate (minutes) as an integer string, "" when nil.
func minutesStr(m *float64) string {
	if m == nil {
		return ""
	}
	return strconv.FormatInt(int64(*m), 10)
}

// conflictField is one diverged field: the three-way base/ours/theirs values the
// resolver UI shows. basePresent is internal (not serialised) — false means the
// snapshot had no baseline for this field yet, so it isn't a real conflict.
type conflictField struct {
	Field       string `json:"field"`
	Base        string `json:"base"`
	Ours        string `json:"ours"`
	Theirs      string `json:"theirs"`
	basePresent bool
}

type conflictDecision int

const (
	conflictProceed conflictDecision = iota // push ours (baseline matches or absent)
	conflictNoop                            // GitLab already has ours — nothing to push
	conflictParked                          // both sides changed — parked for resolution
)

// conflictCheckedKind reports whether a change kind goes through three-way conflict
// detection. Other kinds push directly (current behaviour).
func conflictCheckedKind(kind string) bool {
	switch kind {
	case "due", "estimate", "title_desc":
		return true
	default:
		return false
	}
}

// conflictTriples builds the base/ours/theirs triple(s) for a conflict-checked kind
// from the snapshot, the freshly fetched issue and the current task. wsID is used to
// resolve GitLab attachment links in the issue body to the same proxy URLs Tessera
// stores, so a description comparison doesn't false-conflict on link rewriting.
func (h *API) conflictTriples(kind string, raw []byte, issue gitlab.Issue, task db.Task, wsID uuid.UUID) []conflictField {
	snap, present := snapshotPresence(raw)
	switch kind {
	case "due":
		_, ok := present["due"]
		return []conflictField{{
			Field: "due", Base: snap.Due, Ours: dateStr(task.DueDate), Theirs: dateStr(issue.DueDate),
			basePresent: ok,
		}}
	case "estimate":
		_, ok := present["time_estimate"]
		return []conflictField{{
			Field: "estimate", Base: minutesStr(ptrFloat(snap.TimeEstimate)), Ours: minutesStr(task.Estimate),
			Theirs: minutesStr(ptrFloat(issue.TimeEstimate / 60)), basePresent: ok,
		}}
	case "title_desc":
		_, titleOK := present["title"]
		_, descOK := present["description"]
		theirsDesc := h.rewriteAssets(issue.Description, wsID)
		return []conflictField{
			{Field: "title", Base: snap.Title, Ours: task.Title, Theirs: issue.Title, basePresent: titleOK},
			{Field: "description", Base: snap.Description, Ours: task.Description, Theirs: theirsDesc, basePresent: descOK},
		}
	default:
		return nil
	}
}

func ptrFloat(v int64) *float64 {
	f := float64(v)
	return &f
}

// evalConflict turns the triples into a decision: park (any field diverged on both
// sides), proceed (some field needs pushing, baseline clean/absent), or noop.
func evalConflict(triples []conflictField) (conflictDecision, []conflictField) {
	var conflicts []conflictField
	needPush := false
	for _, t := range triples {
		if t.Theirs == t.Ours {
			continue // GitLab already matches our value
		}
		if t.basePresent && t.Theirs != t.Base {
			conflicts = append(conflicts, t) // GitLab moved away from the baseline too
			continue
		}
		needPush = true // baseline matches (or none yet) → safe to overwrite
	}
	if len(conflicts) > 0 {
		return conflictParked, conflicts
	}
	if needPush {
		return conflictProceed, nil
	}
	return conflictNoop, nil
}

// evalWritebackConflict is the conflict gate called from performWriteback for
// conflict-checked kinds. It returns the decision and (when parked) the diverged
// fields. The caller has already fetched the issue for its own push.
func (h *API) evalWritebackConflict(ctx context.Context, w db.GitlabWriteback, link db.GitlabLink, issue gitlab.Issue, wsID uuid.UUID) (conflictDecision, []conflictField, error) {
	task, err := h.q.GetTask(ctx, w.TaskID)
	if err != nil {
		return conflictProceed, nil, err // transient: retry
	}
	triples := h.conflictTriples(w.ChangeKind, link.GlSnapshot, issue, task, wsID)
	if len(triples) == 0 {
		return conflictProceed, nil, nil
	}
	decision, fields := evalConflict(triples)
	return decision, fields, nil
}

// parkConflict marks a claimed outbox row as a conflict with the diverged fields.
func (h *API) parkConflict(ctx context.Context, w db.GitlabWriteback, fields []conflictField) error {
	raw, err := json.Marshal(map[string]any{"fields": fields, "detected_at": time.Now().UTC()})
	if err != nil {
		return err
	}
	return h.q.MarkWritebackConflict(ctx, db.MarkWritebackConflictParams{ID: w.ID, Conflict: raw})
}

// conflictDetail is the parsed conflict jsonb.
type conflictDetail struct {
	Fields     []conflictField `json:"fields"`
	DetectedAt time.Time       `json:"detected_at"`
}

func parseConflict(raw []byte) conflictDetail {
	var c conflictDetail
	_ = json.Unmarshal(raw, &c)
	return c
}

// recordConflictAction logs a parked conflict in the sync journal (a "conflict" op).
func (h *API) recordConflictAction(j *syncJournal, w db.GitlabWriteback, res writebackResult, fields []conflictField) {
	var iidPtr *int64
	if res.glIid != 0 {
		iid := res.glIid
		iidPtr = &iid
	}
	tid := w.TaskID
	j.add(journalAction{
		Direction: "push", EntityType: w.ChangeKind, Op: "conflict",
		TaskID: &tid, GlIid: iidPtr,
		Summary: pushSummary(w.ChangeKind, nil, iidPtr) + " — конфликт",
		Detail:  map[string]any{"change_kind": w.ChangeKind, "fields": fields, "writeback_id": w.ID},
		Status:  "ok",
	})
}

// ── HTTP: conflicts inbox + interactive resolution ──────────

// conflictDTO is one open conflict for the inbox/resolver.
type conflictDTO struct {
	ID         uuid.UUID       `json:"id"`
	TaskID     uuid.UUID       `json:"task_id"`
	TaskTitle  string          `json:"task_title"`
	TaskNumber *int64          `json:"task_number"`
	ChangeKind string          `json:"change_kind"`
	GlIid      int64           `json:"gl_iid"`
	Fields     []conflictField `json:"fields"`
	DetectedAt time.Time       `json:"detected_at"`
}

// ListGitlabConflicts returns every open write-back conflict for the workspace's
// integration, newest first — the conflicts inbox.
func (h *API) ListGitlabConflicts(c *gin.Context) {
	integ, ok := h.integrationForWorkspace(c)
	if !ok {
		return
	}
	rows, err := h.q.ListOpenConflicts(c, integ.ID)
	if err != nil {
		fail(c)
		return
	}
	out := make([]conflictDTO, 0, len(rows))
	for _, r := range rows {
		cd := parseConflict(r.Conflict)
		var iid int64
		if link, lerr := h.q.GetGitlabLinkByTask(c, r.TaskID); lerr == nil {
			iid = link.GlIid
		}
		out = append(out, conflictDTO{
			ID: r.ID, TaskID: r.TaskID, TaskTitle: r.TaskTitle, TaskNumber: r.TaskNumber,
			ChangeKind: r.ChangeKind, GlIid: iid, Fields: cd.Fields, DetectedAt: cd.DetectedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

// ResolveGitlabConflict applies the user's decision for one parked conflict:
//   - theirs: write GitLab's value into the task; nothing is pushed.
//   - ours:   acknowledge GitLab's value as the new baseline and re-queue the push
//     of the task's current value.
//   - manual: write the user-supplied merged value into the task, then push it.
//
// ours/manual re-arm the outbox row; the worker re-fetches GitLab and (if GitLab
// hasn't moved again) pushes cleanly — otherwise it re-parks, which is safe.
func (h *API) ResolveGitlabConflict(c *gin.Context) {
	taskID, ok := parseID(c, "id")
	if !ok {
		return
	}
	conflictID, ok := parseID(c, "conflictId")
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
	w, err := h.q.GetGitlabWriteback(c, conflictID)
	if err != nil {
		if notFound(c, err) {
			return
		}
		fail(c)
		return
	}
	if w.Status != "conflict" || w.TaskID != taskID {
		c.JSON(http.StatusNotFound, gin.H{"error": "conflict not found"})
		return
	}
	integ, err := h.q.GetGitlabIntegrationByID(c, w.IntegrationID)
	if err != nil {
		fail(c)
		return
	}
	if !h.requireMember(c, integ.WorkspaceID) {
		return
	}
	uid := middleware.CurrentUser(c)
	link, err := h.q.GetGitlabLinkByTask(c, taskID)
	if err != nil {
		fail(c)
		return
	}
	snap, _ := snapshotPresence(link.GlSnapshot)
	cd := parseConflict(w.Conflict)

	switch req.Resolution {
	case "theirs":
		for _, f := range cd.Fields {
			h.applyConflictValue(c, taskID, f.Field, f.Theirs)
			setSnapshotField(&snap, f.Field, f.Theirs)
		}
		_ = h.q.SetGitlabLinkSnapshot(c, db.SetGitlabLinkSnapshotParams{TaskID: taskID, GlSnapshot: marshalSnapshot(snap)})
		_ = h.q.ResolveConflictSettled(c, db.ResolveConflictSettledParams{ID: w.ID, Resolution: "theirs", ResolvedBy: &uid})
	case "ours":
		for _, f := range cd.Fields {
			setSnapshotField(&snap, f.Field, f.Theirs) // acknowledge GitLab's value as baseline
		}
		_ = h.q.SetGitlabLinkSnapshot(c, db.SetGitlabLinkSnapshotParams{TaskID: taskID, GlSnapshot: marshalSnapshot(snap)})
		_ = h.q.ReArmConflict(c, db.ReArmConflictParams{ID: w.ID, Resolution: "ours", ResolvedBy: &uid})
	case "manual":
		for _, f := range cd.Fields {
			h.applyConflictValue(c, taskID, f.Field, req.Value[f.Field])
			setSnapshotField(&snap, f.Field, f.Theirs)
		}
		_ = h.q.SetGitlabLinkSnapshot(c, db.SetGitlabLinkSnapshotParams{TaskID: taskID, GlSnapshot: marshalSnapshot(snap)})
		_ = h.q.ReArmConflict(c, db.ReArmConflictParams{ID: w.ID, Resolution: "manual", ResolvedBy: &uid})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "resolution must be ours|theirs|manual"})
		return
	}

	if t, terr := h.q.GetTask(c, taskID); terr == nil {
		h.broadcast(integ.WorkspaceID, "task.updated", t)
	}
	h.broadcast(integ.WorkspaceID, "gitlab.conflict", map[string]any{
		"task_id": taskID, "resolved": true, "resolution": req.Resolution,
	})
	c.JSON(http.StatusOK, gin.H{"status": "ok", "resolution": req.Resolution})
}

// applyConflictValue writes a resolved string value into the task's field (the
// inverse of conflictTriples' field extraction).
func (h *API) applyConflictValue(ctx context.Context, taskID uuid.UUID, field, value string) {
	switch field {
	case "due":
		var due *time.Time
		if value != "" {
			if t, err := time.Parse("2006-01-02", value); err == nil {
				t = t.UTC()
				due = &t
			}
		}
		_ = h.q.UpdateTaskDueDate(ctx, db.UpdateTaskDueDateParams{ID: taskID, DueDate: due})
	case "estimate":
		var est *float64
		if value != "" {
			if n, err := strconv.ParseFloat(value, 64); err == nil {
				est = &n
			}
		}
		_ = h.q.UpdateTaskEstimate(ctx, db.UpdateTaskEstimateParams{ID: taskID, Estimate: est})
	case "title":
		_ = h.q.SetTaskTitle(ctx, db.SetTaskTitleParams{ID: taskID, Title: value})
	case "description":
		_ = h.q.SetTaskDescription(ctx, db.SetTaskDescriptionParams{ID: taskID, Description: value})
	}
}

// setSnapshotField writes a resolved string back into the typed snapshot.
func setSnapshotField(snap *glSnapshot, field, value string) {
	switch field {
	case "due":
		snap.Due = value
	case "estimate":
		if value == "" {
			snap.TimeEstimate = 0
		} else if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			snap.TimeEstimate = n
		}
	case "title":
		snap.Title = value
	case "description":
		snap.Description = value
	case "state":
		snap.State = value
	}
}

func marshalSnapshot(snap glSnapshot) []byte {
	b, err := json.Marshal(snap)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// conflictFrozenKinds returns the set of change kinds with an open conflict for a
// task, so the pull can skip overwriting those fields (preserving the user's
// pending "ours" until they resolve). Best-effort; an error yields an empty set.
func (h *API) conflictFrozenKinds(ctx context.Context, taskID uuid.UUID) map[string]bool {
	frozen := map[string]bool{}
	if _, err := h.q.GetGitlabLinkByTask(ctx, taskID); errors.Is(err, pgx.ErrNoRows) {
		return frozen
	}
	rows, err := h.q.ListOpenConflictKinds(ctx, taskID)
	if err != nil {
		return frozen
	}
	for _, k := range rows {
		frozen[k] = true
	}
	return frozen
}
