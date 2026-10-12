package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
	Priority     int32  `json:"priority"`      // resolved priority level the GL labels imply
	// Labels are the issue's label titles as GitLab ordered them. The rule
	// counters and «Проверить лейбл» (#3087) count by them, and they are the
	// baseline of the labels conflict (#3089). Absent in a snapshot taken before
	// they were recorded — the next pull of the issue fills them in.
	Labels []string `json:"labels"`
	// Assignees are the issue's assignee logins, sorted — the baseline of the
	// assignees conflict (#3089).
	Assignees []string `json:"assignees"`
}

// buildGlSnapshot captures the conflict-relevant fields of a freshly fetched issue.
// rules resolve the issue's labels to a priority level (the comparable baseline).
func buildGlSnapshot(issue gitlab.Issue, rules gitlab.Rules) []byte {
	snap := glSnapshot{
		Title:        issue.Title,
		Description:  issue.Description,
		State:        issue.State,
		Due:          dateStr(issue.DueDate),
		TimeEstimate: issue.TimeEstimate / 60,
		Priority:     rules.Resolve(issue.Labels).Priority,
		Labels:       labelTitles(issue.Labels),
		Assignees:    assigneeLogins(issue.Assignees),
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// labelTitles lists label titles in GitLab's order (never nil, so the snapshot
// records "no labels" rather than "not recorded").
func labelTitles(labels []gitlab.Label) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.Title)
	}
	return out
}

// assigneeLogins lists assignee logins sorted (never nil).
func assigneeLogins(people []gitlab.Person) []string {
	out := make([]string, 0, len(people))
	for _, p := range people {
		out = append(out, p.Login)
	}
	sort.Strings(out)
	return out
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

// taskDateStr is dateStr for a date the *task* owns rather than one GitLab sent,
// which is why it needs a zone: a task due date is an instant, not a date-only
// value. The editors build it from local calendar fields (DueEditor.vue picks
// `new Date(y, m, d, 0, 0, 0, 0)`) and «/due сегодня» resolves to midnight in the
// caller's zone, so east of UTC the stored instant belongs to the previous UTC
// day. Read in UTC it named that previous day — the day written into the GitLab
// issue, and the day the three-way check compared against a snapshot GitLab had
// filled with the right one, so a date nobody had moved read as diverged (#2932).
//
// A pure UTC-midnight instant is left in UTC: that is a genuine date-only value —
// what a pull from GitLab stores — and it has no time of day to re-read, so a zone
// west of UTC would walk it back a day. The clients draw the same line for the
// same reason (isDateOnly in frontend/src/utils/format.js).
//
// A nil zone means "unknown" and keeps the old UTC reading rather than panicking:
// the callers resolve it from a user profile, which may simply not name one.
func taskDateStr(t *time.Time, loc *time.Location) string {
	if t == nil {
		return ""
	}
	if loc == nil || isDateOnly(*t) {
		return dateStr(t)
	}
	return t.In(loc).Format("2006-01-02")
}

// isDateOnly reports whether an instant carries no time of day at all — the
// convention this codebase stores a day-without-a-time under.
func isDateOnly(t time.Time) bool {
	u := t.UTC()
	return u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 && u.Nanosecond() == 0
}

// minutesStr renders an optional estimate (minutes) as an integer string, "" when nil.
func minutesStr(m *float64) string {
	if m == nil {
		return ""
	}
	return strconv.FormatInt(int64(*m), 10)
}

// prioStr renders a priority level as a string (the comparable form for the triple).
func prioStr(p int32) string {
	return strconv.FormatInt(int64(p), 10)
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
	case "due", "estimate", "title_desc", "completion", "state", "priority",
		// The assignee set and the tag labels (#3089), compared as sorted lists.
		"assignees", "labels":
		// "state" is the legacy alias for "completion" (in-flight rows across upgrade).
		return true
	default:
		return false
	}
}

// conflictTriples builds the base/ours/theirs triple(s) for a conflict-checked kind
// from the snapshot, the freshly fetched issue and the current task. wsID is used to
// resolve GitLab attachment links in the issue body to the same proxy URLs Tessera
// stores, so a description comparison doesn't false-conflict on link rewriting.
// rules map the issue's labels to a priority level for the priority triple.
// loc is the calendar the task's own dates are read in — the same one the push
// would use, so the comparison and the value it guards cannot disagree.
func (h *API) conflictTriples(ctx context.Context, kind string, raw []byte, issue gitlab.Issue, task db.Task, wsID uuid.UUID, rules gitlab.Rules, loc *time.Location) []conflictField {
	snap, present := snapshotPresence(raw)
	switch kind {
	case "due":
		_, ok := present["due"]
		return []conflictField{{
			Field: "due", Base: snap.Due, Ours: taskDateStr(task.DueDate, loc), Theirs: dateStr(issue.DueDate),
			basePresent: ok,
		}}
	case "estimate":
		_, ok := present["time_estimate"]
		return []conflictField{{
			Field: "estimate", Base: minutesStr(ptrFloat(snap.TimeEstimate)), Ours: minutesStr(task.Estimate),
			Theirs: minutesStr(ptrFloat(issue.TimeEstimate / 60)), basePresent: ok,
		}}
	case "completion", "state":
		_, ok := present["state"]
		return []conflictField{{
			Field: "state", Base: snap.State, Ours: issueState(task.CompletedAt != nil),
			Theirs: issue.State, basePresent: ok,
		}}
	case "priority":
		// Only meaningful when the priority label mapping inverts (else the push
		// can't form a label anyway); skip detection otherwise.
		if _, inv := rules.InversePriority(); !inv {
			return nil
		}
		_, ok := present["priority"]
		return []conflictField{{
			Field: "priority", Base: prioStr(snap.Priority), Ours: prioStr(task.Priority),
			Theirs: prioStr(rules.Resolve(issue.Labels).Priority), basePresent: ok,
		}}
	case "assignees":
		return []conflictField{{
			Field: "assignees", Base: joinList(snap.Assignees), Ours: joinList(h.taskGitlabLogins(ctx, task.ID)),
			Theirs: joinList(assigneeLogins(issue.Assignees)), basePresent: recorded(present, "assignees"),
		}}
	case "labels":
		// Tag labels only (status/priority labels have pushes of their own), and
		// only when a tag name spells its label — the push needs the same.
		if !rules.TagsInvertible() {
			return nil
		}
		return []conflictField{{
			Field: "labels", Base: joinList(tagLabels(rules, snap.Labels)), Ours: joinList(tagLabels(rules, h.taskTagNames(ctx, task.ID))),
			Theirs: joinList(tagLabels(rules, labelTitles(issue.Labels))), basePresent: recorded(present, "labels"),
		}}
	case "title_desc":
		_, titleOK := present["title"]
		_, descOK := present["description"]
		theirsDesc := h.rewriteAssets(ctx, issue.Description, wsID)
		return []conflictField{
			{Field: "title", Base: snap.Title, Ours: task.Title, Theirs: issue.Title, basePresent: titleOK},
			{Field: "description", Base: snap.Description, Ours: task.Description, Theirs: theirsDesc, basePresent: descOK},
		}
	default:
		return nil
	}
}

// recorded reports whether a snapshot field holds a value — present and not null.
// Labels and assignees were added to the snapshot later (#3089); a snapshot taken
// before that, re-marshalled through the typed struct, carries them as null, which
// is "no baseline yet", not "empty".
func recorded(present map[string]json.RawMessage, key string) bool {
	v, ok := present[key]
	return ok && string(v) != "null"
}

// listSep joins the items of a list-valued conflict field. The resolver shows it
// as is and a manual value comes back in the same shape.
const listSep = ", "

// joinList renders a set as its sorted, deduplicated items — the comparable form
// of a list-valued conflict field.
func joinList(items []string) string {
	set := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || set[it] {
			continue
		}
		set[it] = true
		out = append(out, it)
	}
	sort.Strings(out)
	return strings.Join(out, listSep)
}

// splitList is the inverse of joinList.
func splitList(s string) []string {
	var out []string
	for _, it := range strings.Split(s, ",") {
		if it = strings.TrimSpace(it); it != "" {
			out = append(out, it)
		}
	}
	return out
}

// tagLabels keeps the titles that are tag labels under the rules.
func tagLabels(rules gitlab.Rules, titles []string) []string {
	out := make([]string, 0, len(titles))
	for _, t := range titles {
		if t = strings.TrimSpace(t); rules.TagLabelClass(t) {
			out = append(out, t)
		}
	}
	return out
}

// taskGitlabLogins is the task's assignee set as GitLab would hold it: the GitLab
// login of every Tessera assignee that has one, plus the GitLab-only assignees.
// Someone without a GitLab identity is left out — the push drops them too.
func (h *API) taskGitlabLogins(ctx context.Context, taskID uuid.UUID) []string {
	var out []string
	if tas, err := h.q.ListTaskAssignees(ctx, taskID); err == nil {
		for _, a := range tas {
			if login := h.actorGitlabUsername(ctx, a.ID); login != "" {
				out = append(out, login)
			}
		}
	}
	if gas, err := h.q.ListTaskGitlabAssignees(ctx, taskID); err == nil {
		for _, g := range gas {
			out = append(out, g.GlUsername)
		}
	}
	return out
}

// taskTagNames lists the names of the task's tags.
func (h *API) taskTagNames(ctx context.Context, taskID uuid.UUID) []string {
	tags, err := h.q.ListTaskTags(ctx, taskID)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.Name)
	}
	return out
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
func (h *API) evalWritebackConflict(ctx context.Context, w db.GitlabWriteback, link db.GitlabLink, issue gitlab.Issue, wsID uuid.UUID, rules gitlab.Rules) (conflictDecision, []conflictField, error) {
	task, err := h.q.GetTask(ctx, w.TaskID)
	if err != nil {
		return conflictProceed, nil, err // transient: retry
	}
	triples := h.conflictTriples(ctx, w.ChangeKind, link.GlSnapshot, issue, task, wsID, rules, h.actorLocation(ctx, w.ActorUserID))
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

// conflictDTO is one open conflict for the inbox/resolver. Entity tells a task's
// conflict from a stage's (#3025): a stage conflict carries the stage instead of
// the task (TaskID null, change_kind "milestone"), and its ID is the stage's id.
type conflictDTO struct {
	ID         uuid.UUID       `json:"id"`
	Entity     string          `json:"entity"` // task | milestone
	TaskID     *uuid.UUID      `json:"task_id"`
	TaskTitle  string          `json:"task_title"`
	TaskNumber *int64          `json:"task_number"`
	ChangeKind string          `json:"change_kind"`
	GlIid      int64           `json:"gl_iid"`
	Fields     []conflictField `json:"fields"`
	DetectedAt time.Time       `json:"detected_at"`

	MilestoneID    *uuid.UUID `json:"milestone_id,omitempty"`
	MilestoneTitle string     `json:"milestone_title,omitempty"`
	MilestoneSlug  string     `json:"milestone_slug,omitempty"`
	ProjectSlug    string     `json:"project_slug,omitempty"`
}

// ListGitlabConflicts returns every open write-back conflict for the workspace's
// integration, newest first — the conflicts inbox. Returns an empty list (not 400)
// when the workspace has no GitLab integration, so clients can poll any workspace
// for a conflict count without special-casing.
//
// ?include=milestones appends the stages' conflicts (#3025) after the tasks'. Opt
// in, so a client that only knows task conflicts (Android) never receives an
// entry it would try to resolve as a task.
func (h *API) ListGitlabConflicts(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListOpenConflictsByWorkspace(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]conflictDTO, 0, len(rows))
	for _, r := range rows {
		cd := parseConflict(r.Conflict)
		var iid int64
		if link, lerr := h.q.GetGitlabLinkByTask(c, r.TaskID); lerr == nil {
			iid = link.GlIid
		}
		tid := r.TaskID
		out = append(out, conflictDTO{
			ID: r.ID, Entity: "task", TaskID: &tid, TaskTitle: r.TaskTitle, TaskNumber: r.TaskNumber,
			ChangeKind: r.ChangeKind, GlIid: iid, Fields: cd.Fields, DetectedAt: cd.DetectedAt,
		})
	}
	if c.Query("include") == "milestones" {
		ms, merr := h.q.ListOpenMilestoneConflictsByWorkspace(c, wsID)
		if merr != nil {
			fail(c, merr)
			return
		}
		for _, r := range ms {
			cd := parseMilestoneConflict(r.Conflict)
			if len(cd.Fields) == 0 {
				continue
			}
			var iid int64
			if r.GlIid != nil {
				iid = *r.GlIid
			}
			mid := r.MilestoneID
			out = append(out, conflictDTO{
				ID: mid, Entity: "milestone", ChangeKind: "milestone", GlIid: iid,
				Fields: cd.Fields, DetectedAt: cd.DetectedAt,
				MilestoneID: &mid, MilestoneTitle: r.MilestoneTitle,
				MilestoneSlug: r.MilestoneSlug, ProjectSlug: r.ProjectSlug,
			})
		}
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
	var req conflictDecisionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !validResolution(req.Resolution) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resolution must be ours|theirs|manual"})
		return
	}
	w, integ, ok := h.loadConflict(c, taskID, conflictID)
	if !ok {
		return
	}
	h.applyConflictDecision(c, w, integ, req.Resolution, req.Value)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "resolution": req.Resolution})
}

// conflictDecisionReq is one decision: the resolution and, for manual, the merged
// value per field.
type conflictDecisionReq struct {
	Resolution string            `json:"resolution"`
	Value      map[string]string `json:"value"`
}

func validResolution(r string) bool {
	return r == "ours" || r == "theirs" || r == "manual"
}

// ResolveGitlabConflicts applies a batch of decisions at once (#3089) — the
// resolver's «Применить ⌘↵» over every conflict on screen. All of them are checked
// before any is applied, so a stale one (resolved elsewhere a moment ago) fails
// the batch with nothing half-done; the re-armed pushes then leave in the worker's
// next drain together.
func (h *API) ResolveGitlabConflicts(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Items []struct {
			TaskID     uuid.UUID `json:"task_id"`
			ConflictID uuid.UUID `json:"conflict_id"`
			conflictDecisionReq
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	type loaded struct {
		w     db.GitlabWriteback
		integ db.GitlabIntegration
		d     conflictDecisionReq
	}
	batch := make([]loaded, 0, len(req.Items))
	for _, it := range req.Items {
		if !validResolution(it.Resolution) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "resolution must be ours|theirs|manual"})
			return
		}
		w, integ, ok := h.loadConflict(c, it.TaskID, it.ConflictID)
		if !ok {
			return
		}
		if integ.WorkspaceID != wsID {
			c.JSON(http.StatusNotFound, gin.H{"error": "conflict not found"})
			return
		}
		batch = append(batch, loaded{w: w, integ: integ, d: it.conflictDecisionReq})
	}
	for _, b := range batch {
		h.applyConflictDecision(c, b.w, b.integ, b.d.Resolution, b.d.Value)
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "resolved": len(batch)})
}

// loadConflict resolves one open conflict of a task, authorising the caller
// through its binding's workspace and the task itself. Writes the error response
// and returns ok=false when it can't.
func (h *API) loadConflict(c *gin.Context, taskID, conflictID uuid.UUID) (db.GitlabWriteback, db.GitlabIntegration, bool) {
	w, err := h.q.GetGitlabWriteback(c, conflictID)
	if err != nil {
		if !notFound(c, err) {
			fail(c, err)
		}
		return w, db.GitlabIntegration{}, false
	}
	if w.Status != "conflict" || w.TaskID != taskID {
		c.JSON(http.StatusNotFound, gin.H{"error": "conflict not found"})
		return w, db.GitlabIntegration{}, false
	}
	integ, err := h.q.GetGitlabIntegrationByID(c, w.IntegrationID)
	if err != nil {
		fail(c, err)
		return w, integ, false
	}
	if !h.requireMember(c, integ.WorkspaceID) {
		return w, integ, false
	}
	// The conflict is authorized through its binding, so the task's own board is
	// asked separately: a task on a soft-deleted board (0078) is gone, and
	// resolving its conflict would push it to GitLab from the trash.
	if _, _, ok := h.loadTask(c, taskID); !ok {
		return w, integ, false
	}
	return w, integ, true
}

// applyConflictDecision carries out one validated decision (see
// ResolveGitlabConflict) and tells the workspace.
func (h *API) applyConflictDecision(c *gin.Context, w db.GitlabWriteback, integ db.GitlabIntegration, resolution string, value map[string]string) {
	taskID := w.TaskID
	uid := middleware.CurrentUser(c)
	link, err := h.q.GetGitlabLinkByTask(c, taskID)
	if err != nil {
		fail(c, err)
		return
	}
	rules := parseRules(integ.LabelRules)
	snap, _ := snapshotPresence(link.GlSnapshot)
	cd := parseConflict(w.Conflict)

	switch resolution {
	case "theirs":
		for _, f := range cd.Fields {
			h.applyConflictValue(c, taskID, f.Field, f.Theirs)
			setSnapshotField(&snap, f.Field, f.Theirs, rules)
		}
		soft(c, "SetGitlabLinkSnapshot", h.q.SetGitlabLinkSnapshot(c, db.SetGitlabLinkSnapshotParams{TaskID: taskID, GlSnapshot: marshalSnapshot(snap)}))
		soft(c, "ResolveConflictSettled", h.q.ResolveConflictSettled(c, db.ResolveConflictSettledParams{ID: w.ID, Resolution: "theirs", ResolvedBy: &uid}))
	case "ours":
		for _, f := range cd.Fields {
			setSnapshotField(&snap, f.Field, f.Theirs, rules) // acknowledge GitLab's value as baseline
		}
		soft(c, "SetGitlabLinkSnapshot", h.q.SetGitlabLinkSnapshot(c, db.SetGitlabLinkSnapshotParams{TaskID: taskID, GlSnapshot: marshalSnapshot(snap)}))
		soft(c, "ReArmConflict", h.q.ReArmConflict(c, db.ReArmConflictParams{ID: w.ID, Resolution: "ours", ResolvedBy: &uid}))
	case "manual":
		for _, f := range cd.Fields {
			h.applyConflictValue(c, taskID, f.Field, value[f.Field])
			setSnapshotField(&snap, f.Field, f.Theirs, rules)
		}
		soft(c, "SetGitlabLinkSnapshot", h.q.SetGitlabLinkSnapshot(c, db.SetGitlabLinkSnapshotParams{TaskID: taskID, GlSnapshot: marshalSnapshot(snap)}))
		soft(c, "ReArmConflict", h.q.ReArmConflict(c, db.ReArmConflictParams{ID: w.ID, Resolution: "manual", ResolvedBy: &uid}))
	}

	if t, terr := h.q.GetTask(c, taskID); terr == nil {
		h.broadcast(integ.WorkspaceID, "task.updated", t)
	}
	h.broadcast(integ.WorkspaceID, "gitlab.conflict", map[string]any{
		"task_id": taskID, "resolved": true, "resolution": resolution,
	})
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
		soft(ctx, "UpdateTaskDueDate", h.q.UpdateTaskDueDate(ctx, db.UpdateTaskDueDateParams{ID: taskID, DueDate: due}))
	case "estimate":
		var est *float64
		if value != "" {
			if n, err := strconv.ParseFloat(value, 64); err == nil {
				est = &n
			}
		}
		soft(ctx, "UpdateTaskEstimate", h.q.UpdateTaskEstimate(ctx, db.UpdateTaskEstimateParams{ID: taskID, Estimate: est}))
	case "title":
		soft(ctx, "SetTaskTitle", h.q.SetTaskTitle(ctx, db.SetTaskTitleParams{ID: taskID, Title: value}))
	case "description":
		soft(ctx, "SetTaskDescription", h.q.SetTaskDescription(ctx, db.SetTaskDescriptionParams{ID: taskID, Description: value}))
	case "priority":
		if n, err := strconv.ParseInt(value, 10, 32); err == nil {
			soft(ctx, "SetTaskPriority", h.q.SetTaskPriority(ctx, db.SetTaskPriorityParams{ID: taskID, Priority: int32(n)}))
		}
	case "state":
		h.applyTaskState(ctx, taskID, value)
	case "assignees":
		h.applyTaskAssignees(ctx, taskID, splitList(value))
	case "labels":
		h.applyTaskTagLabels(ctx, taskID, splitList(value))
	}
}

// applyTaskAssignees makes the task's assignee set the given GitLab logins
// (#3089): a Tessera assignee whose GitLab login is not among them is taken off,
// and the logins go through the sync's own reconciliation — a member of the
// workspace by their account, anybody else as a GitLab-only assignee.
func (h *API) applyTaskAssignees(ctx context.Context, taskID uuid.UUID, logins []string) {
	task, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	wsID, err := h.q.WorkspaceIDForBoard(ctx, task.BoardID)
	if err != nil {
		return
	}
	want := map[string]bool{}
	people := make([]gitlab.Person, 0, len(logins))
	for _, l := range logins {
		want[l] = true
		people = append(people, gitlab.Person{Login: l})
	}
	if tas, lerr := h.q.ListTaskAssignees(ctx, taskID); lerr == nil {
		for _, a := range tas {
			if login := h.actorGitlabUsername(ctx, a.ID); login != "" && !want[login] {
				_, rerr := h.q.RemoveTaskAssignee(ctx, db.RemoveTaskAssigneeParams{TaskID: taskID, UserID: a.ID})
				soft(ctx, "RemoveTaskAssignee", rerr)
			}
		}
	}
	h.reconcileAssignees(ctx, wsID, taskID, people)
}

// applyTaskTagLabels makes the task's tag labels exactly the given titles
// (#3089): the titles become the GitLab-sourced tags, and a hand-made tag that is
// a tag label under the binding's rules but not among them comes off. Tags no rule
// reads as a label (the user's own vocabulary) are left alone.
func (h *API) applyTaskTagLabels(ctx context.Context, taskID uuid.UUID, titles []string) {
	task, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	link, err := h.q.GetGitlabLinkByTask(ctx, taskID)
	if err != nil {
		return
	}
	integ, err := h.q.GetGitlabIntegrationByID(ctx, link.IntegrationID)
	if err != nil {
		return
	}
	projectID, err := h.q.ProjectIDForBoard(ctx, task.BoardID)
	if err != nil {
		return
	}
	rules := parseRules(integ.LabelRules)
	want := map[string]bool{}
	tags := make([]gitlab.Tag, 0, len(titles))
	for _, t := range titles {
		want[t] = true
		tags = append(tags, gitlab.Tag{Name: t})
	}
	if current, lerr := h.q.ListTaskTags(ctx, taskID); lerr == nil {
		for _, tg := range current {
			if rules.TagLabelClass(tg.Name) && !want[tg.Name] {
				_, rerr := h.q.RemoveTaskTag(ctx, db.RemoveTaskTagParams{TaskID: taskID, TagID: tg.ID})
				soft(ctx, "RemoveTaskTag", rerr)
			}
		}
	}
	h.reconcileTags(ctx, taskID, integ.WorkspaceID, projectID, tags)
}

// applyTaskState makes the task match a GitLab open/closed state: "closed" moves it
// to the board's done column (and marks it completed); "opened" clears completion and
// moves it out of the done column (to the first non-done column) if it sat there.
func (h *API) applyTaskState(ctx context.Context, taskID uuid.UUID, state string) {
	task, err := h.q.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	board, err := h.q.GetBoard(ctx, task.BoardID)
	if err != nil {
		return
	}
	done := doneColumnID(board)
	col := task.ColumnID
	var completed *time.Time
	if state == "closed" {
		if done != nil {
			col = *done
		}
		now := time.Now()
		completed = &now
	} else if done != nil && task.ColumnID == *done {
		if cols, cerr := h.q.ListColumns(ctx, board.ID); cerr == nil {
			for _, c := range cols {
				if c.ID != *done {
					col = c.ID
					break
				}
			}
		}
	}
	soft(ctx, "SetTaskColumnCompleted", h.q.SetTaskColumnCompleted(ctx, db.SetTaskColumnCompletedParams{ID: taskID, ColumnID: col, CompletedAt: completed}))
}

// setSnapshotField writes a resolved string back into the typed snapshot. rules
// tell the tag labels from the rest: a resolved "labels" value replaces only the
// tag labels of the recorded set, keeping the issue's status and priority labels.
func setSnapshotField(snap *glSnapshot, field, value string, rules gitlab.Rules) {
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
	case "priority":
		if n, err := strconv.ParseInt(value, 10, 32); err == nil {
			snap.Priority = int32(n)
		}
	case "assignees":
		snap.Assignees = append([]string{}, splitList(value)...)
	case "labels":
		kept := []string{}
		for _, l := range snap.Labels {
			if !rules.TagLabelClass(l) {
				kept = append(kept, l)
			}
		}
		kept = append(kept, splitList(value)...)
		snap.Labels = kept
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
