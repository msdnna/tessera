package main

import (
	"net/http"
	"testing"
)

// TestDocumentCRUD walks the basic lifecycle and checks the two things the list
// endpoint promises: a flat shape and no content column.
func TestDocumentCRUD(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Регламент"}), http.StatusCreated)
	id := doc["id"].(string)
	if doc["slug"].(string) != "reglament" {
		t.Fatalf("slug = %q, want reglament", doc["slug"])
	}
	// The empty document must arrive as a JSON object, not a base64 []byte.
	content, ok := doc["content"].(map[string]any)
	if !ok || content["type"] != "doc" {
		t.Fatalf("content = %#v, want an empty ProseMirror doc", doc["content"])
	}

	list := c.get("/workspaces/" + ws + "/documents").listBody(t)
	if len(list) != 1 {
		t.Fatalf("list has %d documents, want 1", len(list))
	}
	if _, present := list[0]["content"]; present {
		t.Fatal("ListDocuments returned content — with D2 that is the whole workspace on every open")
	}

	updated := c.expect(t, c.patch("/documents/"+id,
		map[string]any{"title": "Регламент v2", "icon": "📄"}), http.StatusOK)
	if updated["title"] != "Регламент v2" || updated["icon"] != "📄" {
		t.Fatalf("update did not stick: %#v", updated)
	}
	// Renaming keeps the slug: the id-addressed route is the permanent one.
	if updated["slug"] != "reglament" {
		t.Fatalf("slug changed on rename: %v", updated["slug"])
	}

	if r := c.del("/documents/" + id); r.Status != http.StatusNoContent {
		t.Fatalf("delete status %d\n%s", r.Status, r.Body)
	}
	if r := c.get("/documents/" + id); r.Status != http.StatusNotFound {
		t.Fatalf("get after delete: status %d", r.Status)
	}
}

// TestDocumentSlugUniqueAndResolve covers the deep-link path: same titles get
// distinct slugs, and resolution answers with the workspace id so the client can
// switch scope before mounting (the #2721 lesson).
func TestDocumentSlugUniqueAndResolve(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	first := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Протокол"}), http.StatusCreated)
	second := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Протокол"}), http.StatusCreated)
	if first["slug"] == second["slug"] {
		t.Fatalf("both documents got slug %v", first["slug"])
	}
	if second["slug"] != "protokol-2" {
		t.Fatalf("second slug = %v, want protokol-2", second["slug"])
	}

	resolved := c.expect(t, c.get("/workspaces/"+ws+"/documents/by-slug/protokol-2"), http.StatusOK)
	if resolved["id"] != second["id"] {
		t.Fatalf("resolved %v, want %v", resolved["id"], second["id"])
	}
	if resolved["workspace_id"] != ws {
		t.Fatalf("resolve omitted the workspace scope: %#v", resolved["workspace_id"])
	}
	if r := c.get("/workspaces/" + ws + "/documents/by-slug/net-takogo"); r.Status != http.StatusNotFound {
		t.Fatalf("unknown slug: status %d", r.Status)
	}
}

// TestDocumentDeleteWithChildren is the guard against one click taking a whole
// subtree: a container with children is a 409 until the caller says recursive.
func TestDocumentDeleteWithChildren(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	parent := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Раздел"}), http.StatusCreated)
	parentID := parent["id"].(string)
	child := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Подстраница", "parent_id": parentID}), http.StatusCreated)
	childID := child["id"].(string)
	if child["parent_id"] != parentID {
		t.Fatalf("parent_id = %v, want %v", child["parent_id"], parentID)
	}

	r := c.del("/documents/" + parentID)
	if r.Status != http.StatusConflict {
		t.Fatalf("delete with children: status %d, want 409\n%s", r.Status, r.Body)
	}
	if n := r.mapBody(t)["children"]; n != float64(1) {
		t.Fatalf("children count = %v, want 1", n)
	}

	if r := c.del("/documents/" + parentID + "?recursive=true"); r.Status != http.StatusNoContent {
		t.Fatalf("recursive delete: status %d\n%s", r.Status, r.Body)
	}
	if r := c.get("/documents/" + childID); r.Status != http.StatusNotFound {
		t.Fatalf("child survived the recursive delete: status %d", r.Status)
	}
}

// TestDocumentReparentCycle rejects moving a document under its own descendant,
// which would detach the branch and leave it orphaned in the table.
func TestDocumentReparentCycle(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	root := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Корень"}), http.StatusCreated)
	rootID := root["id"].(string)
	mid := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Середина", "parent_id": rootID}), http.StatusCreated)
	midID := mid["id"].(string)
	leaf := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Лист", "parent_id": midID}), http.StatusCreated)
	leafID := leaf["id"].(string)

	// Root under its own grandchild.
	if r := c.patch("/documents/"+rootID, map[string]any{"parent_id": leafID}); r.Status != http.StatusBadRequest {
		t.Fatalf("cycle via grandchild: status %d, want 400\n%s", r.Status, r.Body)
	}
	// A document under itself.
	if r := c.patch("/documents/"+rootID, map[string]any{"parent_id": rootID}); r.Status != http.StatusBadRequest {
		t.Fatalf("self-parent: status %d, want 400\n%s", r.Status, r.Body)
	}
	// Moving downward is still allowed: leaf up to the root.
	moved := c.expect(t, c.patch("/documents/"+leafID, map[string]any{"parent_id": rootID}), http.StatusOK)
	if moved["parent_id"] != rootID {
		t.Fatalf("legal re-parent did not stick: %v", moved["parent_id"])
	}
}

// TestDocumentWorkspaceIsolation: membership is the only gate, and a parent from
// another workspace is not a way around it.
func TestDocumentWorkspaceIsolation(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	stranger := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())
	doc := owner.expect(t, owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Секрет"}), http.StatusCreated)
	id := doc["id"].(string)

	if r := stranger.get("/documents/" + id); r.Status != http.StatusForbidden {
		t.Fatalf("stranger read: status %d, want 403", r.Status)
	}
	if r := stranger.get("/workspaces/" + ws + "/documents"); r.Status != http.StatusForbidden {
		t.Fatalf("stranger list: status %d, want 403", r.Status)
	}
	if r := stranger.del("/documents/" + id); r.Status != http.StatusForbidden {
		t.Fatalf("stranger delete: status %d, want 403", r.Status)
	}

	otherWS := mkWorkspace(t, stranger, "Чужой WS "+t.Name())
	otherDoc := stranger.expect(t, stranger.post("/workspaces/"+otherWS+"/documents",
		map[string]any{"title": "Чужой корень"}), http.StatusCreated)
	r := owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Подкидыш", "parent_id": otherDoc["id"]})
	if r.Status != http.StatusForbidden && r.Status != http.StatusBadRequest {
		t.Fatalf("cross-workspace parent: status %d, want 400/403\n%s", r.Status, r.Body)
	}
}

// TestDocumentProjectScope covers the project link added on the /approve of
// #2726: documents may live inside a project, survive its deletion, and — the
// regression that matters — follow it to another workspace.
func TestDocumentProjectScope(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	scoped := c.expect(t, c.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "ТЗ проекта", "project_id": s.Project}), http.StatusCreated)
	loose := c.expect(t, c.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Общая база"}), http.StatusCreated)
	if scoped["project_id"] != s.Project {
		t.Fatalf("project_id = %v, want %v", scoped["project_id"], s.Project)
	}

	filtered := c.get("/workspaces/" + s.WS + "/documents?project_id=" + s.Project).listBody(t)
	if len(filtered) != 1 || filtered[0]["id"] != scoped["id"] {
		t.Fatalf("project filter returned %#v", filtered)
	}

	// Transferring the project must re-stamp workspace_id on its documents:
	// requireMember authorizes on that column, so a stale value would leave the
	// document with the team the project just left.
	target := mkWorkspace(t, c, "Приёмник "+t.Name())
	c.expect(t, c.post("/projects/"+s.Project+"/transfer",
		map[string]any{"workspace_id": target}), http.StatusOK)

	moved := c.expect(t, c.get("/documents/"+scoped["id"].(string)), http.StatusOK)
	if moved["workspace_id"] != target {
		t.Fatalf("document kept workspace %v after transfer, want %v", moved["workspace_id"], target)
	}
	stayed := c.expect(t, c.get("/documents/"+loose["id"].(string)), http.StatusOK)
	if stayed["workspace_id"] != s.WS {
		t.Fatalf("workspace-level document moved with the project: %v", stayed["workspace_id"])
	}

	// Deleting the project frees its documents instead of taking them along.
	if r := c.del("/projects/" + s.Project); r.Status != http.StatusNoContent && r.Status != http.StatusOK {
		t.Fatalf("delete project: status %d\n%s", r.Status, r.Body)
	}
	orphan := c.expect(t, c.get("/documents/"+scoped["id"].(string)), http.StatusOK)
	if orphan["project_id"] != nil {
		t.Fatalf("project_id = %v after project delete, want null", orphan["project_id"])
	}
}

// TestDocumentSearch checks the section is reachable from the app's main
// navigation rather than only by direct link.
func TestDocumentSearch(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Инструкция по развёртыванию"}), http.StatusCreated)

	found := c.expect(t, c.get("/workspaces/"+ws+"/search?q=развёрт"), http.StatusOK)
	docs, ok := found["documents"].([]any)
	if !ok || len(docs) != 1 {
		t.Fatalf("search documents = %#v", found["documents"])
	}
	if docs[0].(map[string]any)["id"] != doc["id"] {
		t.Fatalf("search returned %v, want %v", docs[0], doc["id"])
	}
}
