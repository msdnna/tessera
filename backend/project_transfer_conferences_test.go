package main

import (
	"net/http"
	"testing"
)

// TestTransferProjectUnpinsCrossWorkspaceConferences pins the sixth reference a
// project transfer used to leave hanging across the workspace boundary it moves
// (#2932): the conference pinned to a task.
//
// CreateConference refuses a task from another workspace and says why — the link
// "is what later files the protocol back into the task, and a cross-workspace
// link would put it in front of a team that was never in the call". A conference
// carries its own workspace_id and does not belong to a project, so it never
// travels: transferring the task's project leaves the call in the source
// workspace pointing at a task that now lives in the target one.
//
// GET /tasks/:id/conferences authorizes on the *task's* workspace — the target,
// after the transfer — and then lists by task_id alone, so the source's call
// (title, description, schedule) is read back by a team that was never in it.
func TestTransferProjectUnpinsCrossWorkspaceConferences(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// A second project in the source workspace: its task stays behind, and the
	// conference pinned to it is the control — that link crosses no boundary.
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся проект")
	stayBoard := mkBoard(t, owner, stayProject, "Доска остающегося")
	stayCol := owner.get("/boards/" + stayBoard + "/columns").listBody(t)[0]["id"].(string)

	moving := mkTask(t, owner, src.Board, src.col(t, 0), "переезжающая задача")
	staying := mkTask(t, owner, stayBoard, stayCol, "остающаяся задача")

	movingConf := owner.expect(t, owner.post("/workspaces/"+src.WS+"/conferences", map[string]any{
		"title":       "Разбор переезжающей",
		"description": "что обсудили про сроки",
		"task_id":     moving["id"],
	}), http.StatusCreated)
	stayConf := owner.expect(t, owner.post("/workspaces/"+src.WS+"/conferences", map[string]any{
		"title":   "Разбор остающейся",
		"task_id": staying["id"],
	}), http.StatusCreated)

	// Someone who belongs to the target workspace only — the team that was never
	// in the call, and the only reader whose view of the link matters.
	newcomer := addMember(t, owner, dst.WS)

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	t.Run("the transfer reports what it unpinned", func(t *testing.T) {
		if n, _ := res["unpinned_conferences"].(float64); n != 1 {
			t.Fatalf("unpinned_conferences = %v, want 1: %v", res["unpinned_conferences"], res)
		}
	})

	t.Run("the target workspace cannot read the source's call", func(t *testing.T) {
		r := newcomer.get("/tasks/" + moving["id"].(string) + "/conferences")
		if r.Status != http.StatusOK {
			t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
		}
		if got := r.listBody(t); len(got) != 0 {
			t.Fatalf("a member of the target workspace reads a call of the source one: %v", got)
		}
	})

	t.Run("the conference survives in its own workspace, only unpinned", func(t *testing.T) {
		got := owner.expect(t, owner.get("/conferences/"+movingConf["id"].(string)), http.StatusOK)
		conf, _ := got["conference"].(map[string]any)
		if conf == nil {
			t.Fatalf("the cleanup deleted the conference instead of its link: %v", got)
		}
		if conf["task_id"] != nil {
			t.Fatalf("conference still pinned to a task in another workspace: %v", conf["task_id"])
		}
		if conf["workspace_id"] != src.WS {
			t.Fatalf("conference moved workspace: %v, want %s", conf["workspace_id"], src.WS)
		}
	})

	t.Run("a conference whose task stayed keeps its link", func(t *testing.T) {
		got := owner.expect(t, owner.get("/conferences/"+stayConf["id"].(string)), http.StatusOK)
		conf, _ := got["conference"].(map[string]any)
		if conf == nil || conf["task_id"] != staying["id"] {
			t.Fatalf("the cleanup unpinned a call that crossed no boundary: %v", got)
		}
	})

	// Which side of the boundary the call is on is not fixed: once the project
	// has arrived, pinning a conference of the *target* workspace to one of its
	// tasks is legal, and transferring the project back strands that one instead.
	// A cleanup written around "the source's conferences" passes everything above
	// and still leaves this row behind.
	t.Run("the same holds when the project goes back", func(t *testing.T) {
		back := owner.expect(t, owner.post("/workspaces/"+dst.WS+"/conferences", map[string]any{
			"title":   "Разбор на новом месте",
			"task_id": moving["id"],
		}), http.StatusCreated)
		res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
			map[string]any{"workspace_id": src.WS}), http.StatusOK)
		if n, _ := res["unpinned_conferences"].(float64); n != 1 {
			t.Fatalf("unpinned_conferences on the way back = %v, want 1: %v", res["unpinned_conferences"], res)
		}
		got := owner.expect(t, owner.get("/conferences/"+back["id"].(string)), http.StatusOK)
		if conf, _ := got["conference"].(map[string]any); conf == nil || conf["task_id"] != nil {
			t.Fatalf("a call of the workspace the task left is still pinned to it: %v", got)
		}
	})
}
