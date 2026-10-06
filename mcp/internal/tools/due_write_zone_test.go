package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// The other half of the due-date zone rule (due_zone_test.go pins the reading
// half): a bare "2026-09-21" is a *calendar day*, and the day it lands on has to
// be the profile's, not the one the MCP subprocess happens to run in.
//
// A due date is stored as midnight in the writer's own calendar and read back by
// re-expressing that instant in the reader's zone (dueDayOf in
// frontend/src/utils/taskFilter.js, callerLocation in handlers/api.go). So a
// midnight taken in a zone *ahead* of the profile's is an instant that still
// belongs to the previous day over there: written from this host (MSK, +03) for
// a profile on New York time, "2026-09-21" is filed as the 20th — a day early on
// the board, and overdue in my_tasks on the very day it was asked for.

// writeZoneAPI is newMux plus the /auth/me the zone comes from.
func writeZoneAPI(t *testing.T, tz string, get map[string]any) (*client.Client, *muxAPI) {
	t.Helper()
	routes := map[string]any{
		"/api/auth/me": map[string]any{
			"user":        map[string]any{"id": "u1"},
			"preferences": map[string]any{"timezone": tz},
		},
	}
	for path, body := range get {
		routes[path] = body
	}
	return newMux(t, routes)
}

// pinWriteZone puts the process well ahead of the profile zones under test, the
// way this host (MSK) sits ahead of a reader in the Americas.
func pinWriteZone(t *testing.T) {
	t.Helper()
	saved := time.Local
	time.Local = time.FixedZone("TEST+0300", 3*60*60)
	t.Cleanup(func() { time.Local = saved })
}

// dayIn is the calendar day an instant falls on for a reader in loc — the same
// reduction every surface of the product makes before comparing due dates.
func dayIn(t *testing.T, raw string, loc *time.Location) string {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("due date %q is not RFC3339: %v", raw, err)
	}
	return ts.In(loc).Format("2006-01-02")
}

func zoneOrSkip(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("%s unavailable: %v", name, err)
	}
	return loc
}

func createBoardRoutes() map[string]any {
	return map[string]any{
		"/api/boards/b1":         model.Board{ID: "b1", ProjectID: "p1"},
		"/api/boards/b1/columns": []model.Column{{ID: "c1", Name: "К работе", Position: 1}},
		"/api/projects/p1":       model.Project{ID: "p1", WorkspaceID: "w1", Slug: "tessera"},
		"/api/projects/p1/tags":  []model.Tag{},
	}
}

// TestCreateTaskDatesLandOnTheProfileDay is the finding itself: the day asked
// for must be the day the reader sees, for both dates a task carries.
func TestCreateTaskDatesLandOnTheProfileDay(t *testing.T) {
	pinWriteZone(t)
	loc := zoneOrSkip(t, "America/New_York")

	c, mux := writeZoneAPI(t, "America/New_York", createBoardRoutes())
	_, _, err := createTask(c)(context.Background(), nil, createTaskInput{
		BoardID: "b1",
		taskFieldsInput: taskFieldsInput{
			Title: "Fix it", DueDate: "2026-09-21", StartDate: "2026-09-18",
		},
	})
	if err != nil {
		t.Fatalf("createTask: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(mux.writes["POST /api/boards/b1/tasks"], &body); err != nil {
		t.Fatalf("create body: %v", err)
	}
	for _, tc := range []struct{ field, want string }{
		{"due_date", "2026-09-21"},
		{"start_date", "2026-09-18"},
	} {
		raw, ok := body[tc.field].(string)
		if !ok {
			t.Fatalf("%s missing from the create body: %v", tc.field, body)
		}
		if got := dayIn(t, raw, loc); got != tc.want {
			t.Errorf("%s = %s → day %s for the reader, want %s (process zone must not decide the day)",
				tc.field, raw, got, tc.want)
		}
	}
}

// The patch path is a second, separate parser call site — and the one the agent
// uses far more often than creation.
func TestUpdateTaskDatesLandOnTheProfileDay(t *testing.T) {
	pinWriteZone(t)
	loc := zoneOrSkip(t, "America/New_York")

	c, mux := writeZoneAPI(t, "America/New_York", map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
	})
	due, start := "2026-09-21", "2026-09-18"
	_, _, err := updateTask(c)(context.Background(), nil, updateTaskInput{
		TaskID: "t1", DueDate: &due, StartDate: &start,
	})
	if err != nil {
		t.Fatalf("updateTask: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(mux.writes["PATCH /api/tasks/t1"], &body); err != nil {
		t.Fatalf("patch body: %v", err)
	}
	for _, tc := range []struct{ field, want string }{
		{"due_date", "2026-09-21"},
		{"start_date", "2026-09-18"},
	} {
		raw, ok := body[tc.field].(string)
		if !ok {
			t.Fatalf("%s missing from the patch: %v", tc.field, body)
		}
		if got := dayIn(t, raw, loc); got != tc.want {
			t.Errorf("%s = %s → day %s for the reader, want %s", tc.field, raw, got, tc.want)
		}
	}
}

// The consequence the agent actually sees: it sets a task due today, then reads
// its own queue back and is told the task is already late. Both halves now use
// the same zone, so the round trip has to agree with itself.
func TestATaskDueTodayIsNotWrittenOverdue(t *testing.T) {
	pinWriteZone(t)
	loc := zoneOrSkip(t, "America/New_York")
	today := time.Now().In(loc).Format("2006-01-02")

	c, mux := writeZoneAPI(t, "America/New_York", createBoardRoutes())
	if _, _, err := createTask(c)(context.Background(), nil, createTaskInput{
		BoardID:         "b1",
		taskFieldsInput: taskFieldsInput{Title: "Due today", DueDate: today},
	}); err != nil {
		t.Fatalf("createTask: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(mux.writes["POST /api/boards/b1/tasks"], &body); err != nil {
		t.Fatalf("create body: %v", err)
	}
	raw, _ := body["due_date"].(string)
	stored, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("due date %q is not RFC3339: %v", raw, err)
	}
	if isOverdue(model.Task{DueDate: &stored}, time.Now().In(loc)) {
		t.Fatalf("a task written as due today (%s) comes back overdue: stored %s", today, raw)
	}
}

// Control: a full RFC3339 timestamp carries its own offset and must be passed
// through untouched — the zone default only fills in what the caller left out.
func TestExplicitOffsetIsNotRezoned(t *testing.T) {
	pinWriteZone(t)
	zoneOrSkip(t, "America/New_York")

	c, mux := writeZoneAPI(t, "America/New_York", map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
	})
	due := "2026-09-21T12:30:00+09:00"
	if _, _, err := updateTask(c)(context.Background(), nil, updateTaskInput{
		TaskID: "t1", DueDate: &due,
	}); err != nil {
		t.Fatalf("updateTask: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(mux.writes["PATCH /api/tasks/t1"], &body); err != nil {
		t.Fatalf("patch body: %v", err)
	}
	got, _ := body["due_date"].(string)
	want, err := time.Parse(time.RFC3339, due)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	sent, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("sent due date %q is not RFC3339: %v", got, err)
	}
	if !sent.Equal(want) {
		t.Fatalf("due_date = %s, want the instant the caller gave (%s)", got, due)
	}
}

// Control: with no timezone preference the process zone is still the only hint
// there is, so nothing changes for a profile that never set one.
func TestNoProfileZoneKeepsTheProcessZone(t *testing.T) {
	pinWriteZone(t)

	c, mux := writeZoneAPI(t, "", map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
	})
	due := "2026-09-21"
	if _, _, err := updateTask(c)(context.Background(), nil, updateTaskInput{
		TaskID: "t1", DueDate: &due,
	}); err != nil {
		t.Fatalf("updateTask: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(mux.writes["PATCH /api/tasks/t1"], &body); err != nil {
		t.Fatalf("patch body: %v", err)
	}
	raw, _ := body["due_date"].(string)
	if got := dayIn(t, raw, time.Local); got != "2026-09-21" {
		t.Fatalf("due_date = %s → day %s in the process zone, want 2026-09-21", raw, got)
	}
}

// Control: an empty date is still "nothing to set", and a malformed one still
// fails loudly — the zone must not turn either into a silent value.
func TestZoneAwareParseKeepsTheEmptyAndBrokenCases(t *testing.T) {
	loc := zoneOrSkip(t, "America/New_York")

	got, err := parseDateIn("  ", loc)
	if err != nil || got != nil {
		t.Fatalf("empty date: got %v, %v — want nil, nil", got, err)
	}
	if _, err := parseDateIn("21.09.2026", loc); err == nil {
		t.Fatal("expected an error for an unparsable date")
	}
	ts, err := parseDateIn("2026-09-21", loc)
	if err != nil || ts == nil {
		t.Fatalf("parseDateIn: %v, %v", ts, err)
	}
	if name, _ := ts.Zone(); ts.Location() != loc {
		t.Fatalf("parsed in %s, want the location it was given", name)
	}
}
