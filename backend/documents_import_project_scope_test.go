package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// Where an imported document lands is the server's decision, and the part of it
// that has to be checked is the project: `documents.project_id` is a plain
// reference with no workspace of its own, and ReassignProjectDocumentsWorkspace
// re-stamps `workspace_id` on every document a transferred project names —
// `WHERE project_id = $1`, nothing else. So a document pinned across the
// boundary is not merely mislabelled: the next time that project moves, the
// document moves with it, out of the workspace it was imported into.
//
// CreateDocument and UpdateDocument both refuse the cross-workspace pin. The
// import path — shared by the office and PDF imports — parsed the id and used
// it, which is what these cover. PDF is the vehicle because it needs no sidecar.

// uploadFileFields is uploadFile with extra form fields, for the import routes
// that take placement (parent_id / project_id) alongside the file.
func uploadFileFields(t *testing.T, c *client, path, field, filename string, content []byte, fields map[string]string) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("multipart field %s: %v", k, err)
		}
	}
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("multipart write: %v", err)
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api"+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

func TestDocumentImportRejectsProjectFromAnotherWorkspace(t *testing.T) {
	c := signup(t)
	home := mkWorkspace(t, c, "WS "+t.Name())
	other := mkWorkspace(t, c, "WS other "+t.Name())
	foreign := mkProject(t, c, other, mkGroup(t, c, other, "Группа"), "Чужой проект")

	res := uploadFileFields(t, c, "/workspaces/"+home+"/documents/import",
		"file", "Смета.pdf", pdfFixture, map[string]string{"project_id": foreign})
	if res.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", res.Status, res.Body)
	}
	// And nothing was created: the refusal has to come before the document, the
	// way a failed conversion leaves none behind.
	if n := len(c.get("/workspaces/" + home + "/documents").listBody(t)); n != 0 {
		t.Fatalf("a refused import left %d documents behind", n)
	}
}

// The guard must not cost the ordinary case: a project of the importing
// workspace still pins, and the document comes back carrying it.
func TestDocumentImportKeepsProjectFromTheSameWorkspace(t *testing.T) {
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	proj := mkProject(t, c, ws, mkGroup(t, c, ws, "Группа"), "Свой проект")

	res := uploadFileFields(t, c, "/workspaces/"+ws+"/documents/import",
		"file", "Смета.pdf", pdfFixture, map[string]string{"project_id": proj})
	body := c.expect(t, res, http.StatusCreated)
	doc, ok := body["document"].(map[string]any)
	if !ok {
		t.Fatalf("no document in the import response: %#v", body)
	}
	if doc["project_id"] != proj {
		t.Fatalf("project_id = %v, want %s", doc["project_id"], proj)
	}
}
