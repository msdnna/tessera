package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// Stage page metrics (#2992, page #2993): one read that answers everything the
// stage page draws — progress by count and by estimate, the signals (overdue,
// unestimated, unassigned), the split by status and by person, how the scope
// moved after the start, a daily burn-up series and the pace of the previous
// stages.
//
// The population is the tasks the stage holds now (MilestoneTasks). History —
// the burn-up's scope line and «Изменение объёма» — is read back from the task
// journal: every pin, move and unpin of a stage is a milestone /
// milestone_removed entry (journalMilestone), carrying the stage's id since this
// change. A task with no entry at all was pinned before the journal knew about
// stages; it is taken to have been in the stage from the day it was created —
// the only reading that history allows.
//
// Days are UTC calendar days, the way stage dates are stored (date-only).

// maxSeriesDays caps the burn-up series: a stage spanning years still answers
// with its most recent stretch rather than an array nobody can draw.
const maxSeriesDays = 366

// paceStages is how many earlier stages «Темп» compares against.
const paceStages = 3

type metricsTask struct {
	ID          uuid.UUID
	Estimate    *float64
	CompletedAt *time.Time
	CreatedAt   time.Time
	Current     bool // in the stage right now
}

// membershipEvent is one journal entry that moved a task into (+1) or out of
// (-1) the stage.
type membershipEvent struct {
	TaskID uuid.UUID
	At     time.Time
	Delta  int
}

type metricsInput struct {
	Milestone db.Milestone
	Tasks     []db.MilestoneTasksRow
	Assignees []db.MilestoneTaskAssigneesRow
	Events    []membershipEvent
	// Others are tasks the journal names that are not in the stage any more.
	Others   []metricsTask
	Previous []db.PreviousMilestonesRow
	Now      time.Time
}

type msTotals struct {
	Tasks        int     `json:"tasks"`
	Done         int     `json:"done"`
	Open         int     `json:"open"`
	Estimate     float64 `json:"estimate"`
	DoneEstimate float64 `json:"done_estimate"`
}

type msSignals struct {
	Overdue    int `json:"overdue"`
	NoEstimate int `json:"no_estimate"`
	NoAssignee int `json:"no_assignee"`
}

type msStatus struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"count"`
	Done  int    `json:"done"`
}

type msPerson struct {
	UserID       *uuid.UUID `json:"user_id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Total        int        `json:"total"`
	Done         int        `json:"done"`
	Estimate     float64    `json:"estimate"`
	DoneEstimate float64    `json:"done_estimate"`
}

type msScope struct {
	AtStart int `json:"at_start"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Now     int `json:"now"`
}

type msPoint struct {
	Date     string  `json:"date"`
	Scope    int     `json:"scope"`
	Done     int     `json:"done"`
	ScopeEst float64 `json:"scope_est"`
	DoneEst  float64 `json:"done_est"`
}

type msPace struct {
	ID      uuid.UUID `json:"id"`
	Title   string    `json:"title"`
	Slug    string    `json:"slug"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	Current bool      `json:"current"`
}

type msTask struct {
	ID          uuid.UUID   `json:"id"`
	Number      *int64      `json:"number"`
	Title       string      `json:"title"`
	ParentID    *uuid.UUID  `json:"parent_id"`
	BoardID     uuid.UUID   `json:"board_id"`
	BoardSlug   string      `json:"board_slug"`
	ColumnID    uuid.UUID   `json:"column_id"`
	ColumnName  string      `json:"column_name"`
	ColumnColor string      `json:"column_color"`
	Priority    int32       `json:"priority"`
	DueDate     *time.Time  `json:"due_date"`
	Estimate    *float64    `json:"estimate"`
	CompletedAt *time.Time  `json:"completed_at"`
	Overdue     bool        `json:"overdue"`
	Assignees   []uuid.UUID `json:"assignees"`
}

type milestoneMetrics struct {
	Milestone map[string]any `json:"milestone"`
	Today     string         `json:"today"`
	Start     string         `json:"start"`
	End       string         `json:"end"`
	Totals    msTotals       `json:"totals"`
	Signals   msSignals      `json:"signals"`
	Statuses  []msStatus     `json:"statuses"`
	People    []msPerson     `json:"people"`
	Scope     msScope        `json:"scope"`
	Series    []msPoint      `json:"series"`
	Pace      []msPace       `json:"pace"`
	Tasks     []msTask       `json:"tasks"`
}

func utcDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

const dayFmt = "2006-01-02"

// parseMembershipEvent turns a journal row into a ±1 for this stage, or ok=false
// when the row does not move a task across this stage's border.
func parseMembershipEvent(m db.Milestone, kind string, raw []byte) (int, bool) {
	var d struct {
		MilestoneID *string `json:"milestone_id"`
		FromID      string  `json:"from_id"`
		Title       string  `json:"title"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return 0, false
	}
	id := m.ID.String()
	switch kind {
	case "milestone":
		if d.MilestoneID != nil {
			switch {
			case *d.MilestoneID == id:
				return 1, true
			case d.FromID == id:
				return -1, true
			}
			return 0, false
		}
		// Journalled before the ids were: the title is all there is.
		if d.Title == m.Title {
			return 1, true
		}
	case "milestone_removed":
		if d.MilestoneID != nil {
			if *d.MilestoneID == id {
				return -1, true
			}
			return 0, false
		}
		if d.Title == m.Title {
			return -1, true
		}
	}
	return 0, false
}

// membership answers "was task T in the stage at instant x" from its journal.
type membership struct {
	created time.Time
	current bool
	events  []membershipEvent // ascending
}

func (ms membership) at(x time.Time) bool {
	if x.Before(ms.created) {
		return false
	}
	state := ms.current
	if len(ms.events) > 0 {
		// Before its first entry the task was in the state that entry left.
		state = ms.events[0].Delta < 0
		for _, e := range ms.events {
			if e.At.After(x) {
				break
			}
			state = e.Delta > 0
		}
	}
	return state
}

// changePoints are the instants the task's membership can flip at.
func (ms membership) changePoints() []time.Time {
	pts := []time.Time{ms.created}
	for _, e := range ms.events {
		pts = append(pts, e.At)
	}
	return pts
}

func buildMilestoneMetrics(in metricsInput) milestoneMetrics {
	m := in.Milestone
	today := utcDay(in.Now)
	out := milestoneMetrics{
		Today:    today.Format(dayFmt),
		Statuses: []msStatus{},
		People:   []msPerson{},
		Series:   []msPoint{},
		Pace:     []msPace{},
		Tasks:    []msTask{},
	}

	start := utcDay(m.CreatedAt)
	if m.StartDate != nil {
		start = utcDay(*m.StartDate)
	}
	out.Start = start.Format(dayFmt)
	if m.DueDate != nil {
		out.End = utcDay(*m.DueDate).Format(dayFmt)
	}

	// ── who carries what ──
	byTask := map[uuid.UUID][]uuid.UUID{}
	people := map[uuid.UUID]*msPerson{}
	var personOrder []uuid.UUID
	for _, a := range in.Assignees {
		byTask[a.TaskID] = append(byTask[a.TaskID], a.UserID)
		if _, ok := people[a.UserID]; !ok {
			uid := a.UserID
			people[a.UserID] = &msPerson{UserID: &uid, Name: a.Name, Email: a.Email}
			personOrder = append(personOrder, a.UserID)
		}
	}
	unassigned := &msPerson{}

	// ── totals, signals, statuses, people, task list ──
	statusIdx := map[string]int{}
	for _, t := range in.Tasks {
		done := t.CompletedAt != nil
		est := 0.0
		if t.Estimate != nil {
			est = *t.Estimate
		}
		out.Totals.Tasks++
		out.Totals.Estimate += est
		if done {
			out.Totals.Done++
			out.Totals.DoneEstimate += est
		}
		overdue := !done && t.DueDate != nil && utcDay(*t.DueDate).Before(today)
		if !done {
			if overdue {
				out.Signals.Overdue++
			}
			if t.Estimate == nil {
				out.Signals.NoEstimate++
			}
			if len(byTask[t.ID]) == 0 {
				out.Signals.NoAssignee++
			}
		}

		key := strings.ToLower(strings.TrimSpace(t.ColumnName))
		i, ok := statusIdx[key]
		if !ok {
			i = len(out.Statuses)
			statusIdx[key] = i
			out.Statuses = append(out.Statuses, msStatus{Name: t.ColumnName, Color: t.ColumnColor})
		}
		out.Statuses[i].Count++
		if done {
			out.Statuses[i].Done++
		}

		owners := byTask[t.ID]
		if len(owners) == 0 {
			addToPerson(unassigned, done, est)
		}
		for _, uid := range owners {
			addToPerson(people[uid], done, est)
		}

		as := owners
		if as == nil {
			as = []uuid.UUID{}
		}
		out.Tasks = append(out.Tasks, msTask{
			ID: t.ID, Number: t.Number, Title: t.Title, ParentID: t.ParentID,
			BoardID: t.BoardID, BoardSlug: t.BoardSlug, ColumnID: t.ColumnID,
			ColumnName: t.ColumnName, ColumnColor: t.ColumnColor, Priority: t.Priority,
			DueDate: t.DueDate, Estimate: t.Estimate, CompletedAt: t.CompletedAt,
			Overdue: overdue, Assignees: as,
		})
	}
	out.Totals.Open = out.Totals.Tasks - out.Totals.Done
	for _, uid := range personOrder {
		out.People = append(out.People, *people[uid])
	}
	// Most loaded first; the unassigned bucket always last.
	sort.SliceStable(out.People, func(a, b int) bool { return out.People[a].Total > out.People[b].Total })
	if unassigned.Total > 0 {
		out.People = append(out.People, *unassigned)
	}

	// ── history: membership per task ──
	members := map[uuid.UUID]*membership{}
	info := map[uuid.UUID]metricsTask{}
	for _, t := range in.Tasks {
		members[t.ID] = &membership{created: t.CreatedAt, current: true}
		info[t.ID] = metricsTask{ID: t.ID, Estimate: t.Estimate, CompletedAt: t.CompletedAt, CreatedAt: t.CreatedAt, Current: true}
	}
	for _, t := range in.Others {
		if _, ok := members[t.ID]; ok {
			continue
		}
		members[t.ID] = &membership{created: t.CreatedAt, current: false}
		info[t.ID] = t
	}
	for _, e := range in.Events {
		if ms, ok := members[e.TaskID]; ok {
			ms.events = append(ms.events, e)
		}
	}
	for _, ms := range members {
		sort.SliceStable(ms.events, func(a, b int) bool { return ms.events[a].At.Before(ms.events[b].At) })
		// A task that left with no entry saying so (a journal older than this
		// page) must not read as present today.
		if !ms.current && len(ms.events) > 0 && ms.events[len(ms.events)-1].Delta > 0 {
			ms.events = append(ms.events, membershipEvent{At: ms.events[len(ms.events)-1].At, Delta: -1})
		}
	}

	// Changes during the start day are planning, not scope change.
	cut := start.AddDate(0, 0, 1)
	for _, ms := range members {
		if ms.at(cut) {
			out.Scope.AtStart++
		}
		for _, p := range ms.changePoints() {
			if p.Before(cut) {
				continue
			}
			was := ms.at(p.Add(-time.Nanosecond))
			now := ms.at(p)
			switch {
			case !was && now:
				out.Scope.Added++
			case was && !now:
				out.Scope.Removed++
			}
		}
	}
	out.Scope.Now = out.Totals.Tasks

	// ── daily series, start → today (or the day it closed) ──
	last := today
	if m.State == "closed" && m.ClosedAt != nil {
		last = utcDay(*m.ClosedAt)
	}
	if !last.Before(start) {
		first := start
		if days := int(last.Sub(start).Hours()/24) + 1; days > maxSeriesDays {
			first = last.AddDate(0, 0, -(maxSeriesDays - 1))
		}
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			end := d.AddDate(0, 0, 1)
			p := msPoint{Date: d.Format(dayFmt)}
			for id, ms := range members {
				if !ms.at(end.Add(-time.Nanosecond)) {
					continue
				}
				t := info[id]
				est := 0.0
				if t.Estimate != nil {
					est = *t.Estimate
				}
				p.Scope++
				p.ScopeEst += est
				if t.CompletedAt != nil && t.CompletedAt.Before(end) {
					p.Done++
					p.DoneEst += est
				}
			}
			out.Series = append(out.Series, p)
		}
	}

	// ── pace: earlier stages oldest first, then this one ──
	for i := len(in.Previous) - 1; i >= 0; i-- {
		p := in.Previous[i]
		out.Pace = append(out.Pace, msPace{ID: p.ID, Title: p.Title, Slug: p.Slug, Done: int(p.DoneCount), Total: int(p.TaskCount)})
	}
	out.Pace = append(out.Pace, msPace{ID: m.ID, Title: m.Title, Slug: m.Slug, Done: out.Totals.Done, Total: out.Totals.Tasks, Current: true})
	return out
}

func addToPerson(p *msPerson, done bool, est float64) {
	p.Total++
	p.Estimate += est
	if done {
		p.Done++
		p.DoneEstimate += est
	}
}

// MilestoneMetrics answers GET /milestones/:id/metrics for the stage page.
func (h *API) MilestoneMetrics(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	m, _, ok := h.milestoneWorkspace(c, id)
	if !ok {
		return
	}
	tasks, err := h.q.MilestoneTasks(c, &id)
	if err != nil {
		fail(c, err)
		return
	}
	assignees, err := h.q.MilestoneTaskAssignees(c, &id)
	if err != nil {
		fail(c, err)
		return
	}
	rows, err := h.q.MilestoneMembershipEvents(c, db.MilestoneMembershipEventsParams{
		MilestoneID: id.String(), Title: m.Title, ProjectID: m.ProjectID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	current := make(map[uuid.UUID]bool, len(tasks))
	for _, t := range tasks {
		current[t.ID] = true
	}
	var events []membershipEvent
	var otherIDs []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, r := range rows {
		delta, ok := parseMembershipEvent(m, r.Kind, r.Data)
		if !ok {
			continue
		}
		events = append(events, membershipEvent{TaskID: r.TaskID, At: r.CreatedAt, Delta: delta})
		if !current[r.TaskID] && !seen[r.TaskID] {
			seen[r.TaskID] = true
			otherIDs = append(otherIDs, r.TaskID)
		}
	}
	var others []metricsTask
	if len(otherIDs) > 0 {
		brief, err := h.q.MilestoneEventTasks(c, otherIDs)
		if err != nil {
			fail(c, err)
			return
		}
		for _, b := range brief {
			if b.Gone {
				continue
			}
			others = append(others, metricsTask{ID: b.ID, Estimate: b.Estimate, CompletedAt: b.CompletedAt, CreatedAt: b.CreatedAt})
		}
	}
	now := time.Now()
	before := now
	if m.State == "closed" && m.ClosedAt != nil {
		before = *m.ClosedAt
	}
	prev, err := h.q.PreviousMilestones(c, db.PreviousMilestonesParams{
		ProjectID: m.ProjectID, MilestoneID: m.ID, Before: before, Lim: paceStages,
	})
	if err != nil {
		fail(c, err)
		return
	}
	out := buildMilestoneMetrics(metricsInput{
		Milestone: m, Tasks: tasks, Assignees: assignees, Events: events,
		Others: others, Previous: prev, Now: now,
	})
	head := map[string]any{
		"id": m.ID, "project_id": m.ProjectID, "title": m.Title, "slug": m.Slug,
		"description": m.Description, "start_date": m.StartDate, "due_date": m.DueDate,
		"state": m.State, "closed_at": m.ClosedAt, "created_at": m.CreatedAt,
		"updated_at": m.UpdatedAt,
		"gl_url":     nil, "gl_iid": nil, "gl_synced_at": nil,
	}
	if link, lerr := h.q.GetGitlabMilestoneLink(c, m.ID); lerr == nil {
		head["gl_url"] = link.GlWebUrl
		head["gl_iid"] = link.GlIid
		head["gl_synced_at"] = link.LastSyncedAt
	}
	out.Milestone = head
	c.JSON(http.StatusOK, out)
}
