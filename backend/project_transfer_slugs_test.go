package main

import (
	"net/http"
	"testing"
)

// A note's and a document's address is unique per *workspace*
// (notes_ws_slug_key, idx_documents_ws_slug), and a project transfer re-stamps
// workspace_id on both without touching their slug (#2932). So the transfer
// only succeeds while no title in the arriving project matches one already in
// the target: «Регламент» on both sides and the unique index aborts the
// transaction mid-way. The user sees an internal error with nothing naming the
// document that caused it, and every retry fails the same way — the project is
// stuck in the workspace it is leaving until someone guesses which title to
// rename.
//
// Tags are already immune (uniqueness is per project since 0023), and the task
// numbers were given their own resolution earlier in this task. Notes and
// documents are the two that still carry a workspace-scoped address across.
func TestTransferProjectResolvesSlugCollisions(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// The target workspace already lives at those two addresses.
	residentDoc := owner.expect(t, owner.post("/workspaces/"+dst.WS+"/documents",
		map[string]any{"title": "Регламент", "project_id": dst.Project}), http.StatusCreated)
	residentNote := owner.expect(t, owner.post("/workspaces/"+dst.WS+"/notes",
		map[string]any{"title": "Идеи", "project_id": dst.Project}), http.StatusCreated)

	// The moving project carries documents with the very same titles — plus a
	// second one already sitting on the "-2" variant, so a naive rename that
	// only appends -2 collides with a sibling instead of the resident.
	movingDoc := owner.expect(t, owner.post("/workspaces/"+src.WS+"/documents",
		map[string]any{"title": "Регламент", "project_id": src.Project}), http.StatusCreated)
	movingDoc2 := owner.expect(t, owner.post("/workspaces/"+src.WS+"/documents",
		map[string]any{"title": "Регламент", "project_id": src.Project}), http.StatusCreated)
	movingNote := owner.expect(t, owner.post("/workspaces/"+src.WS+"/notes",
		map[string]any{"title": "Идеи", "project_id": src.Project}), http.StatusCreated)

	if movingDoc2["slug"] != "reglament-2" {
		t.Fatalf("second source document slug = %v, want reglament-2", movingDoc2["slug"])
	}

	owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	t.Run("documents keep distinct addresses", func(t *testing.T) {
		seen := map[string]string{}
		for _, id := range []string{
			residentDoc["id"].(string), movingDoc["id"].(string), movingDoc2["id"].(string),
		} {
			m := owner.expect(t, owner.get("/documents/"+id), http.StatusOK)
			slug, _ := m["slug"].(string)
			if slug == "" {
				t.Fatalf("document %s has no slug: %v", id, m)
			}
			if other, dup := seen[slug]; dup {
				t.Fatalf("documents %s and %s both live at %q", other, id, slug)
			}
			seen[slug] = id
		}
	})

	t.Run("notes keep distinct addresses", func(t *testing.T) {
		seen := map[string]string{}
		for _, id := range []string{residentNote["id"].(string), movingNote["id"].(string)} {
			m := owner.expect(t, owner.get("/notes/"+id), http.StatusOK)
			slug, _ := m["slug"].(string)
			if slug == "" {
				t.Fatalf("note %s has no slug: %v", id, m)
			}
			if other, dup := seen[slug]; dup {
				t.Fatalf("notes %s and %s both live at %q", other, id, slug)
			}
			seen[slug] = id
		}
	})

	t.Run("the resident keeps the address it had", func(t *testing.T) {
		m := owner.expect(t, owner.get("/documents/"+residentDoc["id"].(string)), http.StatusOK)
		if m["slug"] != residentDoc["slug"] {
			t.Fatalf("resident document moved from %v to %v", residentDoc["slug"], m["slug"])
		}
		n := owner.expect(t, owner.get("/notes/"+residentNote["id"].(string)), http.StatusOK)
		if n["slug"] != residentNote["slug"] {
			t.Fatalf("resident note moved from %v to %v", residentNote["slug"], n["slug"])
		}
	})

	t.Run("everything arrives in the target workspace", func(t *testing.T) {
		docs := owner.get("/workspaces/" + dst.WS + "/documents").listBody(t)
		if len(docs) != 3 {
			t.Fatalf("target workspace has %d documents, want 3", len(docs))
		}
		notes := owner.get("/workspaces/" + dst.WS + "/notes").listBody(t)
		if len(notes) != 2 {
			t.Fatalf("target workspace has %d notes, want 2", len(notes))
		}
	})
}
