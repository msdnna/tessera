package tools

import (
	"context"
	"testing"
	"time"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// The queue's "today" has to be the reader's, not the MCP process's.
//
// A due date is a calendar day stored as midnight in the writer's zone and
// returned carrying that offset. Every other surface buckets it in the profile's
// zone — the backend resolves it for its own day counts (callerLocation in
// handlers/api.go) and the web lets the `timezone` preference win over the
// browser's (utils/taskFilter.js). The MCP never asked for it: /auth/me was read
// for the user id alone, and `overdue` was decided against midnight in whatever
// zone the subprocess happened to start in. Launched from a container on UTC
// while the profile says Europe/Moscow, that filed every task due today as late,
// for the whole of the day it was actually due on.

// profileZoneCase pins the boundary from both sides at once, which is what makes
// the check independent of the wall clock: the two midnights can never coincide
// (the offsets differ by hours, not days), so whichever way they fall, one of
// these two tasks is on the wrong side of the process's midnight.
type profileZoneCase struct {
	name    string
	due     time.Time
	overdue bool
}

func profileZoneCases(now time.Time) []profileZoneCase {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return []profileZoneCase{
		{name: "due today", due: midnight, overdue: false},
		{name: "due yesterday", due: midnight.Add(-time.Nanosecond), overdue: true},
	}
}

// pinProcessZone puts the process in a zone far from the profile's, the way an
// MCP server started inside a container sits in a different one from the person
// reading its answers.
func pinProcessZone(t *testing.T) {
	t.Helper()
	saved := time.Local
	time.Local = time.FixedZone("TEST-0800", -8*60*60)
	t.Cleanup(func() { time.Local = saved })
}

func profileZoneAPI(t *testing.T, tz string, tasks []model.Task) *client.Client {
	t.Helper()
	return routeAPI(t, map[string]any{
		"/api/auth/me": map[string]any{
			"user":        map[string]any{"id": "u1"},
			"preferences": map[string]any{"timezone": tz},
		},
		"/api/workspaces/w1/tags":  []model.Tag{},
		"/api/workspaces/w1/tasks": tasks,
	})
}

func TestMyTasksBucketsDueDayInTheProfileZone(t *testing.T) {
	pinProcessZone(t)

	// Build the two tasks against the profile's calendar, then ask the tool.
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("Europe/Moscow unavailable: %v", err)
	}
	cases := profileZoneCases(time.Now().In(loc))
	tasks := make([]model.Task, 0, len(cases))
	for i := range cases {
		due := cases[i].due
		tasks = append(tasks, model.Task{ID: cases[i].name, Title: cases[i].name, DueDate: &due})
	}

	c := profileZoneAPI(t, "Europe/Moscow", tasks)
	_, out, err := myTasks(c)(context.Background(), nil, workspaceInput{WorkspaceID: "w1"})
	if err != nil {
		t.Fatalf("myTasks: %v", err)
	}
	if len(out.Tasks) != len(cases) {
		t.Fatalf("got %d tasks, want %d", len(out.Tasks), len(cases))
	}
	got := map[string]bool{}
	for _, s := range out.Tasks {
		got[s.ID] = s.Overdue
	}
	for _, tc := range cases {
		if got[tc.name] != tc.overdue {
			t.Errorf("%s: overdue = %v, want %v (process zone must not decide the day)",
				tc.name, got[tc.name], tc.overdue)
		}
	}
}

// The ranking follows the same boundary: overdue sorts ahead of merely-due, so a
// misplaced midnight does not just mislabel a task, it puts it at the head of the
// queue that next_task answers from.
func TestNextTaskDoesNotPromoteATaskDueToday(t *testing.T) {
	pinProcessZone(t)

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("Europe/Moscow unavailable: %v", err)
	}
	now := time.Now().In(loc)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dueToday := midnight
	dueLate := midnight.Add(-48 * time.Hour)

	// Equal priority, so the overdue flag is the only thing that can order them.
	c := profileZoneAPI(t, "Europe/Moscow", []model.Task{
		{ID: "today", Title: "today", Priority: 2, DueDate: &dueToday, Position: 1},
		{ID: "late", Title: "late", Priority: 2, DueDate: &dueLate, Position: 2},
	})
	_, out, err := nextTask(c)(context.Background(), nil, nextTaskInput{WorkspaceID: "w1"})
	if err != nil || !out.Found {
		t.Fatalf("nextTask: %+v %v", out, err)
	}
	if out.Task.ID != "late" {
		t.Fatalf("next task = %q, want the genuinely late one", out.Task.ID)
	}
}
