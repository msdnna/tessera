package main

import (
	"net/http"
	"testing"
)

// TestTransferProjectSeversCrossWorkspaceDocLinks pins the third link of the
// class TestTransferProjectSeversCrossWorkspaceRefs covers — the document↔task
// link (#2932).
//
// CreateDocumentTaskLink refuses a task from another workspace and its comment
// says why: membership is per workspace, so such a row "would leak the task's
// title to people who cannot see the task". A project transfer moves that
// boundary under links that were legal when they were made — the project's
// documents travel with it while a task in a neighbouring project stays behind,
// and vice versa — and re-checks neither side. Both reads join across the
// relation with no workspace filter of their own: ListDocumentTaskLinks takes
// task_title out of tasks, ListTaskDocumentLinks takes title/slug out of
// documents. Left in place, the row hands each workspace the other's titles
// through exactly the door that refuses to hand them out.
func TestTransferProjectSeversCrossWorkspaceDocLinks(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// A second project in the source workspace. It stays behind when the first
	// one leaves, and that is what puts the two ends of a link apart.
	// The name carries t.Name() because a project slug is globally unique and
	// this file runs alongside the sibling transfer tests.
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся проект "+t.Name())
	stayBoard := mkBoard(t, owner, stayProject, "Доска остающегося")
	stayCol := owner.get("/boards/" + stayBoard + "/columns").listBody(t)[0]["id"].(string)

	moving := mkTask(t, owner, src.Board, src.col(t, 0), "переезжающая задача")
	staying := mkTask(t, owner, stayBoard, stayCol, "остающаяся задача")
	// A task in the moving project: what it shares with the moving document
	// crosses no boundary and must come out of the transfer untouched.
	travelling := mkTask(t, owner, src.Board, src.col(t, 0), "её попутчица")

	// Documents follow their project, so one of these travels and one does not.
	movingDoc := mkProjectDoc(t, owner, src.WS, src.Project, "Регламент переезжающего")
	stayDoc := mkProjectDoc(t, owner, src.WS, stayProject, "Регламент остающегося")

	// Both directions: the document leaves and the task stays, and the reverse.
	linkDocTask(t, owner, movingDoc, staying["id"].(string))
	linkDocTask(t, owner, stayDoc, moving["id"].(string))
	// The control pair, both ends inside the moving project.
	linkDocTask(t, owner, movingDoc, travelling["id"].(string))

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	t.Run("the transfer reports what it severed", func(t *testing.T) {
		if n, _ := res["unlinked_documents"].(float64); n != 2 {
			t.Fatalf("unlinked_documents = %v, want 2: %v", res["unlinked_documents"], res)
		}
	})

	t.Run("no link survives the boundary, read from the document", func(t *testing.T) {
		for doc, stranded := range map[string]string{
			movingDoc: staying["id"].(string),
			stayDoc:   moving["id"].(string),
		} {
			for _, l := range owner.get("/documents/" + doc + "/tasks").listBody(t) {
				if l["task_id"] == stranded {
					t.Fatalf("document %s still lists task %q across the boundary: %v",
						doc, l["task_title"], l)
				}
			}
		}
	})

	t.Run("no link survives the boundary, read from the task", func(t *testing.T) {
		for task, stranded := range map[string]string{
			staying["id"].(string): movingDoc,
			moving["id"].(string):  stayDoc,
		} {
			for _, l := range owner.get("/tasks/" + task + "/documents").listBody(t) {
				if l["document_id"] == stranded {
					t.Fatalf("task %s still lists document %q across the boundary: %v",
						task, l["document_title"], l)
				}
			}
		}
	})

	t.Run("a link whose both ends travelled is untouched", func(t *testing.T) {
		links := owner.get("/tasks/" + travelling["id"].(string) + "/documents").listBody(t)
		if len(links) != 1 || links[0]["document_id"] != movingDoc {
			t.Fatalf("the link between a task and a document that travelled together is gone: %v", links)
		}
		back := owner.get("/documents/" + movingDoc + "/tasks").listBody(t)
		if len(back) != 1 || back[0]["task_id"] != travelling["id"] {
			t.Fatalf("document side of the surviving link: %v", back)
		}
	})
}

// mkProjectDoc creates a document owned by a project, which is what makes it
// travel with that project on a transfer.
func mkProjectDoc(t *testing.T, c *client, wsID, projectID, title string) string {
	t.Helper()
	m := c.expect(t, c.post("/workspaces/"+wsID+"/documents",
		map[string]any{"title": title, "project_id": projectID}), http.StatusCreated)
	return m["id"].(string)
}

func linkDocTask(t *testing.T, c *client, docID, taskID string) {
	t.Helper()
	c.expect(t, c.post("/documents/"+docID+"/tasks", map[string]any{"task_id": taskID}), http.StatusCreated)
}
