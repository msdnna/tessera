package main

import (
	"net/http"
	"testing"
	"time"
)

// Meetings v2 (#3034): duration and repeat (#3052), the stage (#3053).

func mkMilestone(t *testing.T, c *client, projectID, title string) map[string]any {
	t.Helper()
	return c.expect(t, c.post("/projects/"+projectID+"/milestones",
		map[string]any{"title": title}), http.StatusCreated)
}

func findConf(t *testing.T, list []map[string]any, id string) map[string]any {
	t.Helper()
	for _, r := range list {
		if r["id"] == id {
			return r
		}
	}
	t.Fatalf("conference %s not in list %#v", id, list)
	return nil
}

func TestConferencePlanRoundTrip(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	ms := mkMilestone(t, owner, s.Project, "Спринт 43")
	at := time.Now().Add(26 * time.Hour).UTC().Truncate(time.Minute)

	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title":            "Планирование Спринта 43",
		"scheduled_at":     at.Format(time.RFC3339),
		"duration_minutes": 45,
		"recurrence":       "biweekly",
		"timezone":         "Europe/Moscow",
		"milestone_id":     ms["id"],
	}), http.StatusCreated)
	id := conf["id"].(string)
	if conf["duration_minutes"] != float64(45) || conf["recurrence"] != "biweekly" ||
		conf["timezone"] != "Europe/Moscow" || conf["milestone_id"] != ms["id"] {
		t.Fatalf("plan not stored: %#v", conf)
	}
	if conf["milestone_title"] != "Спринт 43" {
		t.Fatalf("create answers the stage title: %#v", conf)
	}

	row := findConf(t, owner.get("/workspaces/"+s.WS+"/conferences").listBody(t), id)
	if row["milestone_title"] != "Спринт 43" || row["milestone_slug"] == nil || row["milestone_project_slug"] == nil {
		t.Fatalf("list row lacks the stage: %#v", row)
	}
	if row["next_at"] == nil {
		t.Fatalf("list row lacks next_at: %#v", row)
	}

	got := owner.expect(t, owner.get("/conferences/"+id), http.StatusOK)["conference"].(map[string]any)
	if got["milestone_title"] != "Спринт 43" {
		t.Fatalf("detail lacks the stage: %#v", got)
	}
	// No linked task: a task typed into the chat goes to the stage's project.
	if got["default_board_id"] != s.Board {
		t.Fatalf("default_board_id = %v, want the stage project's board %s", got["default_board_id"], s.Board)
	}

	t.Run("a PATCH without the new fields keeps them (old clients)", func(t *testing.T) {
		upd := owner.expect(t, owner.patch("/conferences/"+id, map[string]any{
			"title":        "Планирование (переименовано)",
			"scheduled_at": at.Format(time.RFC3339),
		}), http.StatusOK)
		if upd["recurrence"] != "biweekly" || upd["duration_minutes"] != float64(45) || upd["milestone_id"] != ms["id"] {
			t.Fatalf("absent fields were cleared: %#v", upd)
		}
	})

	t.Run("explicit null and empty clear them", func(t *testing.T) {
		upd := owner.expect(t, owner.patch("/conferences/"+id, map[string]any{
			"title":            "Планирование",
			"scheduled_at":     at.Format(time.RFC3339),
			"milestone_id":     nil,
			"duration_minutes": nil,
			"recurrence":       "",
		}), http.StatusOK)
		if upd["milestone_id"] != nil || upd["duration_minutes"] != nil || upd["recurrence"] != "" {
			t.Fatalf("not cleared: %#v", upd)
		}
		if upd["milestone_title"] != nil {
			t.Fatalf("stage title still answered: %#v", upd)
		}
	})
}

func TestConferencePlanValidation(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	other := mkStack(t, owner)
	foreign := mkMilestone(t, owner, other.Project, "Чужой этап")
	at := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	path := "/workspaces/" + s.WS + "/conferences"

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"zero duration", map[string]any{"title": "a", "duration_minutes": 0}, http.StatusBadRequest},
		{"too long", map[string]any{"title": "a", "duration_minutes": 1441}, http.StatusBadRequest},
		{"unknown repeat", map[string]any{"title": "a", "scheduled_at": at, "recurrence": "yearly"}, http.StatusBadRequest},
		{"repeat without a start", map[string]any{"title": "a", "recurrence": "daily"}, http.StatusBadRequest},
		{"bad zone", map[string]any{"title": "a", "timezone": "Mars/Olympus"}, http.StatusBadRequest},
		{"foreign stage", map[string]any{"title": "a", "milestone_id": foreign["id"]}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner.expect(t, owner.post(path, tc.body), tc.want)
		})
	}
}

// A repeating meeting planned in the past shows its next occurrence, not the
// first one — and a one-off one keeps its date.
func TestConferenceNextOccurrence(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	past := time.Now().Add(-15 * 24 * time.Hour).UTC().Truncate(time.Minute)
	daily := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title": "Стендап", "scheduled_at": past.Format(time.RFC3339),
		"duration_minutes": 15, "recurrence": "daily", "timezone": "UTC",
	}), http.StatusCreated)
	once := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title": "Разовая", "scheduled_at": past.Format(time.RFC3339),
	}), http.StatusCreated)

	list := owner.get("/workspaces/" + s.WS + "/conferences").listBody(t)
	next, err := time.Parse(time.RFC3339, findConf(t, list, daily["id"].(string))["next_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if next.Add(15 * time.Minute).Before(time.Now()) {
		t.Fatalf("daily next_at %v is already over", next)
	}
	if time.Until(next) > 24*time.Hour {
		t.Fatalf("daily next_at %v is more than a day ahead", next)
	}
	if next.Hour() != past.Hour() || next.Minute() != past.Minute() {
		t.Fatalf("daily next_at %v lost the wall clock of %v", next, past)
	}
	onceNext, _ := time.Parse(time.RFC3339, findConf(t, list, once["id"].(string))["next_at"].(string))
	if !onceNext.Equal(past) {
		t.Fatalf("one-off next_at moved: %v, want %v", onceNext, past)
	}
}

// The linked task's board wins over the stage's project for the chat's «/».
func TestConferenceDefaultBoardFollowsTask(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	second := mkBoard(t, owner, s.Project, "Вторая доска")
	col := owner.get("/boards/" + second + "/columns").listBody(t)[0]["id"].(string)
	task := mkTask(t, owner, second, col, "обсуждаемая")
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences", map[string]any{
		"title": "Разбор", "task_id": task["id"],
	}), http.StatusCreated)
	got := owner.expect(t, owner.get("/conferences/"+conf["id"].(string)), http.StatusOK)["conference"].(map[string]any)
	if got["default_board_id"] != second {
		t.Fatalf("default_board_id = %v, want the task's board %s", got["default_board_id"], second)
	}
}

// A project transfer takes its stages along; a meeting left behind in the
// source workspace loses the link instead of naming a stage nobody there can
// open (#3053).
func TestTransferProjectUnstagesConferences(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)
	ms := mkMilestone(t, owner, src.Project, "Уезжающий этап")
	conf := owner.expect(t, owner.post("/workspaces/"+src.WS+"/conferences", map[string]any{
		"title": "Ретро", "milestone_id": ms["id"],
	}), http.StatusCreated)

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)
	if n, _ := res["unstaged_conferences"].(float64); n != 1 {
		t.Fatalf("unstaged_conferences = %v, want 1", res["unstaged_conferences"])
	}
	got := owner.expect(t, owner.get("/conferences/"+conf["id"].(string)), http.StatusOK)["conference"].(map[string]any)
	if got["milestone_id"] != nil || got["milestone_title"] != nil {
		t.Fatalf("the stage link survived the transfer: %#v", got)
	}
}
