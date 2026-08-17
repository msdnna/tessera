package main

import (
	"net/http"
	"testing"
)

// Template gallery (#2734). The endpoints are small; what these tests are
// actually about is the two invariants the feature rests on — a template is a
// *copy* (it outlives its source document and is unaffected by later edits to
// it), and a body that arrives from outside the editor goes through the same
// schema check as a save.

// mkTemplateDoc creates a document with a saved body and returns its id.
func mkTemplateDoc(t *testing.T, c *client, ws, title, text string) string {
	t.Helper()
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": title, "icon": "📋"}), http.StatusCreated)
	id := doc["id"].(string)
	c.expect(t, c.patch("/documents/"+id+"/content", map[string]any{
		"content":    docJSON(text),
		"updated_at": doc["updated_at"],
	}), http.StatusOK)
	return id
}

func TestDocumentTemplateFromDocument(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	docID := mkTemplateDoc(t, c, ws, "Протокол совещания", "Повестка встречи")

	tpl := c.expect(t, c.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"document_id": docID}), http.StatusCreated)
	// Title and icon carry over from the document when the request omits them —
	// saving the open document as a template must not demand retyping its name.
	if tpl["title"] != "Протокол совещания" || tpl["icon"] != "📋" {
		t.Fatalf("template = %v / %v", tpl["title"], tpl["icon"])
	}
	if tpl["preview"] != "Повестка встречи" {
		t.Fatalf("preview = %v", tpl["preview"])
	}

	// The gallery list carries previews, not bodies.
	list := c.get("/workspaces/" + ws + "/document-templates").listBody(t)
	if len(list) != 1 {
		t.Fatalf("gallery has %d entries", len(list))
	}
	if _, present := list[0]["content"]; present {
		t.Fatal("gallery list started returning content")
	}
	if list[0]["author_name"] == nil {
		t.Fatal("gallery entry lost its author")
	}
}

// A document created from a template is born with the body, in one request —
// and it is a copy: deleting the template afterwards leaves it alone.
func TestDocumentFromTemplateCopiesBody(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	docID := mkTemplateDoc(t, c, ws, "Исходный", "Текст шаблона")
	tpl := c.expect(t, c.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"document_id": docID}), http.StatusCreated)
	tplID := tpl["id"].(string)

	made := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Совещание 15.08", "template_id": tplID}), http.StatusCreated)
	if made["preview"] != "Текст шаблона" {
		t.Fatalf("new document preview = %v", made["preview"])
	}
	if made["icon"] != "📋" {
		t.Fatalf("template icon did not carry over: %v", made["icon"])
	}
	content, ok := made["content"].(map[string]any)
	if !ok || content["type"] != "doc" {
		t.Fatalf("content came back as %#v", made["content"])
	}

	c.expect(t, c.del("/document-templates/"+tplID), http.StatusNoContent)
	after := c.expect(t, c.get("/documents/"+made["id"].(string)), http.StatusOK)
	if after["preview"] != "Текст шаблона" {
		t.Fatalf("document changed when its template was deleted: %v", after["preview"])
	}
	if code := c.get("/document-templates/" + tplID).Status; code != http.StatusNotFound {
		t.Fatalf("deleted template still readable: %d", code)
	}
}

// The upload path: the client parses a .md or .json file itself and posts the
// tree. That is the one place a body reaches the database without having passed
// through the editor, so the schema check is the only guard there is.
func TestDocumentTemplateFromUploadedContent(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	tpl := c.expect(t, c.post("/workspaces/"+ws+"/document-templates", map[string]any{
		"title":       "Загруженный",
		"description": "Из файла",
		"content":     docJSON("Тело из файла"),
	}), http.StatusCreated)
	if tpl["preview"] != "Тело из файла" {
		t.Fatalf("preview = %v", tpl["preview"])
	}

	bad := c.post("/workspaces/"+ws+"/document-templates", map[string]any{
		"title": "Вредный",
		"content": map[string]any{
			"type":    "doc",
			"content": []any{map[string]any{"type": "script", "text": "alert(1)"}},
		},
	})
	if bad.Status != http.StatusBadRequest {
		t.Fatalf("foreign node accepted: %d", bad.Status)
	}

	empty := c.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"content": docJSON("без имени")})
	if empty.Status != http.StatusBadRequest {
		t.Fatalf("template without a title accepted: %d", empty.Status)
	}
}

// Templates are workspace-scoped, and both directions have to hold: a document
// from another workspace cannot be templated, and a template from another
// workspace cannot seed a document. Without the second check, knowing an id
// would be enough to read a foreign body through the create response.
func TestDocumentTemplateWorkspaceScope(t *testing.T) {
	t.Parallel()
	c := signup(t)
	mine := mkWorkspace(t, c, "WS mine "+t.Name())
	other := mkWorkspace(t, c, "WS other "+t.Name())
	docID := mkTemplateDoc(t, c, other, "Чужой", "Чужое тело")

	cross := c.post("/workspaces/"+mine+"/document-templates",
		map[string]any{"document_id": docID})
	if cross.Status != http.StatusBadRequest {
		t.Fatalf("templated a document from another workspace: %d", cross.Status)
	}

	tpl := c.expect(t, c.post("/workspaces/"+other+"/document-templates",
		map[string]any{"document_id": docID}), http.StatusCreated)
	seeded := c.post("/workspaces/"+mine+"/documents",
		map[string]any{"title": "Попытка", "template_id": tpl["id"]})
	if seeded.Status != http.StatusBadRequest {
		t.Fatalf("seeded a document from another workspace's template: %d", seeded.Status)
	}

	// A stranger sees neither the gallery nor the template.
	outsider := signup(t)
	if code := outsider.get("/workspaces/" + other + "/document-templates").Status; code == http.StatusOK {
		t.Fatal("non-member read the gallery")
	}
	if code := outsider.get("/document-templates/" + tpl["id"].(string)).Status; code == http.StatusOK {
		t.Fatal("non-member read a template")
	}
}

func TestDocumentTemplateRename(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	docID := mkTemplateDoc(t, c, ws, "Черновик", "Тело")
	tpl := c.expect(t, c.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"document_id": docID}), http.StatusCreated)
	id := tpl["id"].(string)

	updated := c.expect(t, c.patch("/document-templates/"+id, map[string]any{
		"title": "Протокол", "description": "Формат команды",
	}), http.StatusOK)
	if updated["title"] != "Протокол" || updated["description"] != "Формат команды" {
		t.Fatalf("update = %v / %v", updated["title"], updated["description"])
	}
	if updated["preview"] != "Тело" {
		t.Fatalf("rename touched the body: %v", updated["preview"])
	}
	if code := c.patch("/document-templates/"+id, map[string]any{"title": ""}).Status; code != http.StatusBadRequest {
		t.Fatalf("empty title accepted: %d", code)
	}
}
