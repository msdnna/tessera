package main

import (
	"net/http"
	"strings"
	"testing"
)

// docJSON builds a one-paragraph ProseMirror document.
func docJSON(text string) map[string]any {
	return map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "paragraph",
				"attrs": map[string]any{"id": "b1"},
				"content": []any{
					map[string]any{"type": "text", "text": text},
				},
			},
		},
	}
}

// TestDocumentContentSaveAndPreview covers the save path end to end: the body
// round-trips, and the preview the tile grid shows is derived server-side (the
// list query deliberately never carries content).
func TestDocumentContentSaveAndPreview(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Спецификация"}), http.StatusCreated)
	id := doc["id"].(string)

	saved := c.expect(t, c.patch("/documents/"+id+"/content", map[string]any{
		"content":    docJSON("Первый абзац документа"),
		"updated_at": doc["updated_at"],
	}), http.StatusOK)
	if saved["preview"] != "Первый абзац документа" {
		t.Fatalf("preview = %v", saved["preview"])
	}
	if saved["updated_at"] == doc["updated_at"] {
		t.Fatal("updated_at did not move after a content save")
	}

	got := c.expect(t, c.get("/documents/"+id), http.StatusOK)
	content, ok := got["content"].(map[string]any)
	if !ok || content["type"] != "doc" {
		t.Fatalf("content came back as %#v", got["content"])
	}

	list := c.get("/workspaces/" + ws + "/documents").listBody(t)
	if list[0]["preview"] != "Первый абзац документа" {
		t.Fatalf("list preview = %v", list[0]["preview"])
	}
	if _, present := list[0]["content"]; present {
		t.Fatal("list started returning content")
	}
}

// TestDocumentContentConflict is the reason the endpoint takes a version at
// all: autosave means the same document can be open twice, and the second
// writer must be told rather than silently overwriting the first.
func TestDocumentContentConflict(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Протокол"}), http.StatusCreated)
	id := doc["id"].(string)
	stale := doc["updated_at"]

	first := c.expect(t, c.patch("/documents/"+id+"/content", map[string]any{
		"content": docJSON("правка первого"), "updated_at": stale,
	}), http.StatusOK)

	r := c.patch("/documents/"+id+"/content", map[string]any{
		"content": docJSON("правка второго"), "updated_at": stale,
	})
	if r.Status != http.StatusConflict {
		t.Fatalf("stale save status %d, want 409\n%s", r.Status, r.Body)
	}

	// The first writer's text must still be there.
	got := c.expect(t, c.get("/documents/"+id), http.StatusOK)
	if !strings.Contains(string(c.get("/documents/"+id).Body), "правка первого") {
		t.Fatalf("conflicting save overwrote the stored document: %#v", got["content"])
	}

	// Saving against the fresh version works again.
	c.expect(t, c.patch("/documents/"+id+"/content", map[string]any{
		"content": docJSON("правка второго"), "updated_at": first["updated_at"],
	}), http.StatusOK)
}

// TestDocumentContentRejectsForeignNodes checks the server-side allow-list. The
// editor schema drops unknown nodes on the client, but this endpoint takes JSON
// from anyone holding a token — without the check, "the schema is the
// allow-list" would only hold for callers using our frontend.
func TestDocumentContentRejectsForeignNodes(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Заметка"}), http.StatusCreated)
	id := doc["id"].(string)
	at := doc["updated_at"]

	cases := []struct {
		name    string
		content map[string]any
	}{
		{"unknown node", map[string]any{
			"type":    "doc",
			"content": []any{map[string]any{"type": "script", "attrs": map[string]any{}}},
		}},
		{"unknown attribute", map[string]any{
			"type": "doc",
			"content": []any{map[string]any{
				"type":  "paragraph",
				"attrs": map[string]any{"onclick": "alert(1)"},
			}},
		}},
		{"unknown mark", map[string]any{
			"type": "doc",
			"content": []any{map[string]any{
				"type": "paragraph",
				"content": []any{map[string]any{
					"type":  "text",
					"text":  "x",
					"marks": []any{map[string]any{"type": "script"}},
				}},
			}},
		}},
		{"javascript link", map[string]any{
			"type": "doc",
			"content": []any{map[string]any{
				"type": "paragraph",
				"content": []any{map[string]any{
					"type": "text",
					"text": "x",
					"marks": []any{map[string]any{
						"type":  "link",
						"attrs": map[string]any{"href": "javascript:alert(1)"},
					}},
				}},
			}},
		}},
		{"not a doc root", map[string]any{"type": "paragraph"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := c.patch("/documents/"+id+"/content",
				map[string]any{"content": tc.content, "updated_at": at})
			if r.Status != http.StatusBadRequest {
				t.Fatalf("status %d, want 400\n%s", r.Status, r.Body)
			}
		})
	}
}

// TestDocumentAssetSigned covers the reason document images do not reuse
// /api/uploads: that route is guarded only by an unguessable filename, and
// #2718 requires document content not to be reachable by guessing a URL.
func TestDocumentAssetSigned(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "С картинкой"}), http.StatusCreated)
	id := doc["id"].(string)

	png := []byte("\x89PNG\r\n\x1a\nfake-image-bytes")
	up := uploadFile(t, c, "/documents/"+id+"/assets", "file", "shot.png", png)
	if up.Status != http.StatusCreated {
		t.Fatalf("upload status %d\n%s", up.Status, up.Body)
	}
	url := up.mapBody(t)["url"].(string)
	if !strings.HasPrefix(url, "/api/documents/asset?") || !strings.Contains(url, "sig=") {
		t.Fatalf("asset url = %q, want a signed /api/documents/asset link", url)
	}

	// The signed URL serves the bytes without auth (an <img> cannot send one).
	path := strings.TrimPrefix(url, "/api")
	got := doReq(t, "", http.MethodGet, path, nil)
	if got.Status != http.StatusOK {
		t.Fatalf("signed fetch status %d\n%s", got.Status, got.Body)
	}
	if string(got.Body) != string(png) {
		t.Fatal("served bytes differ from the uploaded ones")
	}

	// A tampered signature is refused — the signature, not the name, is the
	// capability here.
	bad := doReq(t, "", http.MethodGet, path[:len(path)-2]+"00", nil)
	if bad.Status != http.StatusForbidden && bad.Status != http.StatusNotFound {
		t.Fatalf("tampered signature status %d, want 403/404", bad.Status)
	}
}
