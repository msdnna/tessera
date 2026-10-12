package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func fp(v float64) *float64 { return &v }
func tp(t time.Time) *time.Time {
	return &t
}

// The stage page reads its whole history back from the journal (#2992): which
// tasks were in the stage on which day, what joined after the start and what
// left, and how much of it was done by each evening.
func TestBuildMilestoneMetrics(t *testing.T) {
	ms := db.Milestone{
		ID: uuid.New(), Title: "Спринт 42", State: "active",
		StartDate: tp(day("2026-10-12")), DueDate: tp(day("2026-10-26")),
		CreatedAt: day("2026-10-01"),
	}
	col := func(name string) (string, string) { return name, "#7c5cff" }
	a, b, c, d, gone := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	user := uuid.New()
	todo, todoColor := col("К работе")
	doneCol, doneColor := col("Готово")
	tasks := []db.MilestoneTasksRow{
		// a: pinned before the journal knew stages — in from creation; done on the 14th.
		{ID: a, Title: "a", ColumnName: doneCol, ColumnColor: doneColor, Estimate: fp(60),
			CompletedAt: tp(day("2026-10-14").Add(10 * time.Hour)), CreatedAt: day("2026-10-02")},
		// b: open, overdue, no estimate, no assignee.
		{ID: b, Title: "b", ColumnName: todo, ColumnColor: todoColor,
			DueDate: tp(day("2026-10-15")), CreatedAt: day("2026-10-02")},
		// c: joined on the 16th — scope added after the start.
		{ID: c, Title: "c", ColumnName: todo, ColumnColor: todoColor, Estimate: fp(120),
			CreatedAt: day("2026-10-03")},
		// d: pinned on the start day itself — planning, not scope change.
		{ID: d, Title: "d", ColumnName: todo, ColumnColor: todoColor, Estimate: fp(30),
			CreatedAt: day("2026-10-03")},
	}
	in := metricsInput{
		Milestone: ms,
		Tasks:     tasks,
		Assignees: []db.MilestoneTaskAssigneesRow{
			{TaskID: a, UserID: user, Name: "Мария"},
			{TaskID: c, UserID: user, Name: "Мария"},
			{TaskID: d, UserID: user, Name: "Мария"},
		},
		Events: []membershipEvent{
			{TaskID: c, At: day("2026-10-16").Add(9 * time.Hour), Delta: 1},
			{TaskID: d, At: day("2026-10-12").Add(9 * time.Hour), Delta: 1},
			// gone: in from the 10th, left on the 18th.
			{TaskID: gone, At: day("2026-10-10"), Delta: 1},
			{TaskID: gone, At: day("2026-10-18").Add(12 * time.Hour), Delta: -1},
		},
		Others: []metricsTask{{ID: gone, Estimate: fp(45), CreatedAt: day("2026-10-01")}},
		Now:    day("2026-10-21").Add(15 * time.Hour),
	}
	out := buildMilestoneMetrics(in)

	if out.Totals.Tasks != 4 || out.Totals.Done != 1 || out.Totals.Open != 3 {
		t.Fatalf("totals = %+v", out.Totals)
	}
	if out.Totals.Estimate != 210 || out.Totals.DoneEstimate != 60 {
		t.Fatalf("estimate totals = %+v", out.Totals)
	}
	if out.Signals != (msSignals{Overdue: 1, NoEstimate: 1, NoAssignee: 1}) {
		t.Fatalf("signals = %+v", out.Signals)
	}
	if len(out.Statuses) != 2 || out.Statuses[0].Name != "Готово" || out.Statuses[1].Count != 3 {
		t.Fatalf("statuses = %+v", out.Statuses)
	}
	if len(out.People) != 2 || out.People[0].Total != 3 || out.People[0].Done != 1 || out.People[1].UserID != nil {
		t.Fatalf("people = %+v", out.People)
	}
	// At the start: a, b, d, gone (4). After: c joined, gone left.
	if out.Scope != (msScope{AtStart: 4, Added: 1, Removed: 1, Now: 4}) {
		t.Fatalf("scope = %+v", out.Scope)
	}
	if out.Start != "2026-10-12" || out.End != "2026-10-26" || out.Today != "2026-10-21" {
		t.Fatalf("range = %s..%s today %s", out.Start, out.End, out.Today)
	}
	if len(out.Series) != 10 { // 12th … 21st
		t.Fatalf("series length = %d", len(out.Series))
	}
	at := func(date string) msPoint {
		for _, p := range out.Series {
			if p.Date == date {
				return p
			}
		}
		t.Fatalf("no point for %s", date)
		return msPoint{}
	}
	if p := at("2026-10-12"); p.Scope != 4 || p.Done != 0 || p.ScopeEst != 135 {
		t.Fatalf("12th = %+v", p)
	}
	if p := at("2026-10-14"); p.Done != 1 || p.DoneEst != 60 {
		t.Fatalf("14th = %+v", p)
	}
	if p := at("2026-10-16"); p.Scope != 5 {
		t.Fatalf("16th = %+v", p)
	}
	if p := at("2026-10-21"); p.Scope != 4 || p.Done != 1 {
		t.Fatalf("21st = %+v", p)
	}
	if len(out.Pace) != 1 || !out.Pace[0].Current || out.Pace[0].Done != 1 {
		t.Fatalf("pace = %+v", out.Pace)
	}
	for _, tk := range out.Tasks {
		if tk.ID == b && !tk.Overdue {
			t.Fatalf("b should be overdue")
		}
	}
}

// A stage that has not started yet has no series — the page draws «График
// появится после старта».
func TestBuildMilestoneMetricsNotStarted(t *testing.T) {
	ms := db.Milestone{ID: uuid.New(), Title: "Спринт 43", State: "active",
		StartDate: tp(day("2026-10-27")), CreatedAt: day("2026-10-01")}
	out := buildMilestoneMetrics(metricsInput{Milestone: ms, Now: day("2026-10-21")})
	if len(out.Series) != 0 {
		t.Fatalf("series = %v, want none before the start", out.Series)
	}
}

func TestParseMembershipEvent(t *testing.T) {
	m := db.Milestone{ID: uuid.New(), Title: "Спринт"}
	other := uuid.New().String()
	cases := []struct {
		kind, data string
		delta      int
		ok         bool
	}{
		{"milestone", `{"title":"Спринт","milestone_id":"` + m.ID.String() + `"}`, 1, true},
		{"milestone", `{"title":"Другой","milestone_id":"` + other + `","from_id":"` + m.ID.String() + `"}`, -1, true},
		{"milestone", `{"title":"Другой","milestone_id":"` + other + `"}`, 0, false},
		{"milestone_removed", `{"title":"Спринт","milestone_id":"` + m.ID.String() + `"}`, -1, true},
		// Legacy entries: title only.
		{"milestone", `{"title":"Спринт"}`, 1, true},
		{"milestone_removed", `{"title":"Спринт"}`, -1, true},
		{"milestone_removed", `{"title":"Другой"}`, 0, false},
	}
	for _, tc := range cases {
		d, ok := parseMembershipEvent(m, tc.kind, []byte(tc.data))
		if d != tc.delta || ok != tc.ok {
			t.Errorf("%s %s → %d %v, want %d %v", tc.kind, tc.data, d, ok, tc.delta, tc.ok)
		}
	}
}
