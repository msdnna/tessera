package main

import (
	"bytes"
	"io"
	"mime/multipart"
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

// importDocument posts a file to the import endpoint with the placement fields
// the client sends alongside it. uploadFile cannot: placement arrives as form
// fields here, not as JSON, and that difference is exactly why the import paths
// were the ones left unguarded.
func importDocument(t *testing.T, c *client, wsID, projectID, filename string, content []byte) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if projectID != "" {
		if err := w.WriteField("project_id", projectID); err != nil {
			t.Fatalf("multipart field: %v", err)
		}
	}
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("multipart file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("multipart write: %v", err)
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/workspaces/"+wsID+"/documents/import", &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

// TestImportedDocumentProjectMustBeInSameWorkspace covers the fourth door into
// documents.project_id, the one the guard above did not reach: an import.
//
// Create and PATCH take placement as JSON and are checked; an import takes it as
// multipart form fields next to the file, through createImportedDocument, and
// that path checked parent_id and let project_id through. The consequence is the
// same one and worse, because an imported document is a shared one: pinned to a
// stranger's project it rides TransferProject's `WHERE project_id = $1` out of
// the workspace it was written in — whatever the team filled into it by then —
// and into a workspace of the pinner's choosing.
//
// The PDF branch carries the test because it is the import that needs no
// sidecar; the converted branch reaches the same createImportedDocument.
func TestImportedDocumentProjectMustBeInSameWorkspace(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	vs := mkStack(t, victim)

	author := signup(t) // member of nothing the victim owns
	as := mkStack(t, author)

	t.Run("an import cannot be pinned to a project of another workspace", func(t *testing.T) {
		r := importDocument(t, author, as.WS, vs.Project, "Договор.pdf", pdfFixture)
		if r.Status != http.StatusBadRequest {
			t.Fatalf("import into foreign project: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("an unknown project_id is refused too", func(t *testing.T) {
		r := importDocument(t, author, as.WS, "00000000-0000-0000-0000-000000000001",
			"Договор.pdf", pdfFixture)
		if r.Status != http.StatusNotFound {
			t.Fatalf("import into unknown project: got %d %s, want 404", r.Status, r.Body)
		}
	})

	t.Run("the refusal leaves no half-made document behind", func(t *testing.T) {
		docs := author.get("/workspaces/" + as.WS + "/documents").listBody(t)
		if len(docs) != 0 {
			t.Fatalf("refused imports still created %d documents: %v", len(docs), docs)
		}
	})

	t.Run("an import into the caller's own project still lands", func(t *testing.T) {
		body := author.expect(t, importDocument(t, author, as.WS, as.Project,
			"Договор.pdf", pdfFixture), http.StatusCreated)
		doc, ok := body["document"].(map[string]any)
		if !ok {
			t.Fatalf("no document in the import response: %#v", body)
		}
		if pid, _ := doc["project_id"].(string); pid != as.Project {
			t.Fatalf("project_id = %v, want own project %s", doc["project_id"], as.Project)
		}
	})
}
