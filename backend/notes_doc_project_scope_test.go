package main

import (
	"net/http"
	"testing"
)

// TestNoteAndDocumentProjectMustBeInSameWorkspace pins the authorization of the
// project_id a caller sends in the *body* when creating a note or a document,
// and when re-pinning a document to another project.
//
// The route vouches for the workspace (requireMember on :id), not for the
// project. Every neighbouring body-supplied reference is already checked —
// parent_id through parentInWorkspace one `if` below the project_id assignment,
// template_id through seedFromTemplate, a reminder's task_id, a document↔task
// link's task_id ("it would leak the task's title to people who cannot see the
// task") — and project_id was the one that was not.
//
// Unchecked it is not a cosmetic mismatch, because two code paths read the
// relation by project_id alone and neither compares workspaces:
//
//   - notes.project_id is ON DELETE CASCADE (0002_domain.up.sql). Deleting a
//     project deletes every note pinned to it, so a stranger's DELETE in their
//     own workspace silently takes a note out of ours.
//   - TransferProject re-stamps workspace_id on the project's notes and
//     documents (ReassignProjectNotesWorkspace / …DocumentsWorkspace, both
//     `WHERE project_id = $1`). A note or document pinned across the boundary
//     rides along into a third workspace — out of the one that wrote it and in
//     front of a team that never saw it.
func TestNoteAndDocumentProjectMustBeInSameWorkspace(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	vs := mkStack(t, victim)

	author := signup(t) // member of nothing the victim owns
	as := mkStack(t, author)

	t.Run("a note cannot be pinned to a project of another workspace", func(t *testing.T) {
		r := author.post("/workspaces/"+as.WS+"/notes", map[string]any{
			"title": "своя заметка", "project_id": vs.Project,
		})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("note with foreign project: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("an unknown project_id is refused too", func(t *testing.T) {
		r := author.post("/workspaces/"+as.WS+"/notes", map[string]any{
			"title": "заметка в никуда", "project_id": "00000000-0000-0000-0000-000000000001",
		})
		if r.Status != http.StatusNotFound {
			t.Fatalf("note with unknown project: got %d %s, want 404", r.Status, r.Body)
		}
	})

	t.Run("a note in the caller's own project still saves", func(t *testing.T) {
		note := author.expect(t, author.post("/workspaces/"+as.WS+"/notes", map[string]any{
			"title": "своя заметка в своём проекте", "project_id": as.Project,
		}), http.StatusCreated)
		if got, _ := note["project_id"].(string); got != as.Project {
			t.Fatalf("own project_id did not stick: %v", note["project_id"])
		}
	})

	t.Run("a document cannot be created in a project of another workspace", func(t *testing.T) {
		r := author.post("/workspaces/"+as.WS+"/documents", map[string]any{
			"title": "свой документ", "project_id": vs.Project,
		})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("document with foreign project: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("a document cannot be re-pinned to a project of another workspace", func(t *testing.T) {
		doc := author.expect(t, author.post("/workspaces/"+as.WS+"/documents", map[string]any{
			"title": "документ без проекта",
		}), http.StatusCreated)
		docID := doc["id"].(string)

		r := author.patch("/documents/"+docID, map[string]any{"project_id": vs.Project})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("re-pin to foreign project: got %d %s, want 400", r.Status, r.Body)
		}
		// The refusal has to leave the document where it was, not half-move it.
		got := author.expect(t, author.get("/documents/"+docID), http.StatusOK)
		if got["project_id"] != nil {
			t.Fatalf("document gained a foreign project anyway: %v", got["project_id"])
		}

		// Positive control: the same PATCH into the caller's own project works, so
		// the guard refuses the boundary rather than the operation.
		author.expect(t, author.patch("/documents/"+docID,
			map[string]any{"project_id": as.Project}), http.StatusOK)
		moved := author.expect(t, author.get("/documents/"+docID), http.StatusOK)
		if pid, _ := moved["project_id"].(string); pid != as.Project {
			t.Fatalf("own project_id did not stick: %v", moved["project_id"])
		}
	})

	// The harm the cascade does, pinned from the other side: a note the author
	// pinned to their own project must not be reachable by a delete in the
	// victim's workspace, and after the guard above there is no way to pin it
	// there in the first place. Deleting the victim's project has to leave the
	// author's notes alone.
	t.Run("a stranger's project delete does not take our notes", func(t *testing.T) {
		note := author.expect(t, author.post("/workspaces/"+as.WS+"/notes", map[string]any{
			"title": "заметка, которую нельзя потерять", "project_id": as.Project,
		}), http.StatusCreated)
		noteID := note["id"].(string)

		victim.expect(t, victim.del("/projects/"+vs.Project), http.StatusNoContent)

		author.expect(t, author.get("/notes/"+noteID), http.StatusOK)
	})
}
