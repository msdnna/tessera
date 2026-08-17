// Rich task detail: comments (author-only edit/delete), the activity journal,
// relations by task number, file attachments and public inline-image uploads.
package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// Comments: create/list, author-only edit and delete (a fellow member gets 403).
func TestCommentsCRUD(t *testing.T) {
	t.Parallel()
	author := signup(t)
	member := signup(t)
	s := mkStack(t, author)
	id := mkTask(t, author, s.Board, s.col(t, 0), "С комментариями")["id"].(string)
	author.expect(t, author.post("/workspaces/"+s.WS+"/members", map[string]any{"email": member.Email}), http.StatusCreated)

	cm := author.expect(t, author.post("/tasks/"+id+"/comments", map[string]any{"body": "Первый!"}), http.StatusCreated)
	cmID := cm["id"].(string)
	if cm["body"] != "Первый!" {
		t.Fatalf("comment create: %v", cm)
	}

	list := author.get("/tasks/" + id + "/comments").listBody(t)
	if len(list) != 1 || list[0]["id"] != cmID {
		t.Fatalf("comment list: %v", list)
	}

	// Author edits.
	up := author.expect(t, author.patch("/comments/"+cmID, map[string]any{"body": "Исправлено"}), http.StatusOK)
	if up["body"] != "Исправлено" {
		t.Fatalf("comment update: %v", up)
	}

	// A workspace member who is not the author can neither edit nor delete.
	if r := member.patch("/comments/"+cmID, map[string]any{"body": "чужое"}); r.Status != http.StatusForbidden {
		t.Fatalf("foreign edit: %d\n%s", r.Status, r.Body)
	}
	if r := member.del("/comments/" + cmID); r.Status != http.StatusForbidden {
		t.Fatalf("foreign delete: %d\n%s", r.Status, r.Body)
	}

	// Author deletes.
	if r := author.del("/comments/" + cmID); r.Status != http.StatusNoContent {
		t.Fatalf("delete comment: %d\n%s", r.Status, r.Body)
	}
	if list = author.get("/tasks/" + id + "/comments").listBody(t); len(list) != 0 {
		t.Fatalf("comments after delete: %v", list)
	}
}

// Comment threads: replies hang off a root, the list comes back in thread order,
// depth is capped at two levels server-side, and a parent from another task is a
// 400 rather than a silently re-homed branch.
func TestCommentThreads(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	id := mkTask(t, c, s.Board, s.col(t, 0), "С тредами")["id"].(string)
	other := mkTask(t, c, s.Board, s.col(t, 0), "Соседняя")["id"].(string)

	root := c.expect(t, c.post("/tasks/"+id+"/comments", map[string]any{"body": "Корень"}), http.StatusCreated)
	rootID := root["id"].(string)
	if root["parent_id"] != nil {
		t.Fatalf("root parent: %v", root["parent_id"])
	}
	reply := c.expect(t, c.post("/tasks/"+id+"/comments",
		map[string]any{"body": "Ответ", "parent_id": rootID}), http.StatusCreated)
	if reply["parent_id"] != rootID {
		t.Fatalf("reply parent = %v, want %s", reply["parent_id"], rootID)
	}

	// Replying to a reply lands in the same thread — GitLab has no third level,
	// and the collapse happens here so Android and MCP hitting the same endpoint
	// cannot produce one.
	deep := c.expect(t, c.post("/tasks/"+id+"/comments",
		map[string]any{"body": "Ответ на ответ", "parent_id": reply["id"]}), http.StatusCreated)
	if deep["parent_id"] != rootID {
		t.Fatalf("deep reply parent = %v, want root %s", deep["parent_id"], rootID)
	}

	// A second root posted last still lists after the first thread, not after the
	// newest comment overall: order is by thread, not by bare created_at.
	c.expect(t, c.post("/tasks/"+id+"/comments", map[string]any{"body": "Второй корень"}), http.StatusCreated)
	bodies := []string{}
	for _, cm := range c.get("/tasks/" + id + "/comments").listBody(t) {
		bodies = append(bodies, cm["body"].(string))
	}
	want := []string{"Корень", "Ответ", "Ответ на ответ", "Второй корень"}
	if len(bodies) != len(want) {
		t.Fatalf("thread order: %v, want %v", bodies, want)
	}
	for i := range want {
		if bodies[i] != want[i] {
			t.Fatalf("thread order: %v, want %v", bodies, want)
		}
	}

	// A parent living on another task is a moved branch, not a typo.
	if r := c.post("/tasks/"+other+"/comments", map[string]any{"body": "Чужой тред", "parent_id": rootID}); r.Status != http.StatusBadRequest {
		t.Fatalf("cross-task parent: %d\n%s", r.Status, r.Body)
	}

	// Deleting the root promotes the oldest reply instead of cascading: one "✕"
	// must not take other people's text with it.
	if r := c.del("/comments/" + rootID); r.Status != http.StatusNoContent {
		t.Fatalf("delete root: %d\n%s", r.Status, r.Body)
	}
	after := c.get("/tasks/" + id + "/comments").listBody(t)
	if len(after) != 3 {
		t.Fatalf("comments after root delete: %v", after)
	}
	if after[0]["body"] != "Ответ" || after[0]["parent_id"] != nil {
		t.Fatalf("successor not promoted: %v", after[0])
	}
	if after[1]["body"] != "Ответ на ответ" || after[1]["parent_id"] != after[0]["id"] {
		t.Fatalf("sibling not re-hung under successor: %v", after[1])
	}
	if after[2]["body"] != "Второй корень" || after[2]["parent_id"] != nil {
		t.Fatalf("unrelated root disturbed: %v", after[2])
	}
}

// The activity journal accumulates created / renamed / comment events with an
// actor attached.
func TestTaskEventsJournal(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Журнал")
	id := task["id"].(string)

	c.expect(t, c.patch("/tasks/"+id, map[string]any{"title": "Журнал 2"}), http.StatusOK)
	c.expect(t, c.post("/tasks/"+id+"/comments", map[string]any{"body": "Отметка"}), http.StatusCreated)

	events := c.get("/tasks/" + id + "/events").listBody(t)
	kinds := make([]string, 0, len(events))
	for _, e := range events {
		kinds = append(kinds, e["kind"].(string))
		if e["actor_id"] != c.UserID {
			t.Fatalf("event actor: %v", e)
		}
	}
	// NOTE: the comment event kind is "comment" (not "commented").
	for _, k := range []string{"created", "renamed", "comment"} {
		if !hasKind(kinds, k) {
			t.Fatalf("journal missing %q: %v", k, kinds)
		}
	}
}

// Relations: added by workspace task number, listed with target meta, one-way
// storage (the counterpart only gets a journal entry), guarded against self.
func TestRelationsCRUD(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	t1 := mkTask(t, c, s.Board, s.col(t, 0), "Источник")
	t2 := mkTask(t, c, s.Board, s.col(t, 0), "Цель")
	id1, id2 := t1["id"].(string), t2["id"].(string)

	// Add "blocks" by number → 201 with an empty body.
	if r := c.post("/tasks/"+id1+"/relations",
		map[string]any{"number": t2["number"], "kind": "blocks"}); r.Status != http.StatusCreated {
		t.Fatalf("add relation: %d\n%s", r.Status, r.Body)
	}
	// Default kind is "relates".
	if r := c.post("/tasks/"+id1+"/relations", map[string]any{"number": t2["number"]}); r.Status != http.StatusCreated {
		t.Fatalf("add default relation: %d\n%s", r.Status, r.Body)
	}

	rels := c.get("/tasks/" + id1 + "/relations").listBody(t)
	if len(rels) != 2 {
		t.Fatalf("relations = %d, want 2: %v", len(rels), rels)
	}
	var blocks map[string]any
	for _, r := range rels {
		if r["kind"] == "blocks" {
			blocks = r
		}
	}
	if blocks == nil || blocks["related_task_id"] != id2 ||
		blocks["related_number"] != t2["number"] || blocks["related_title"] != "Цель" {
		t.Fatalf("blocks relation meta: %v", blocks)
	}

	// Storage is one-way: the target's own relation list stays empty (it only
	// receives a "relation" journal event with the inverse kind).
	if got := c.get("/tasks/" + id2 + "/relations").listBody(t); len(got) != 0 {
		t.Fatalf("relations are stored two-way now: %v", got)
	}
	if !hasKind(eventKinds(t, c, id2), "relation") {
		t.Fatalf("target task got no relation journal event")
	}

	// Self-link → 400; unknown number → 404.
	if r := c.post("/tasks/"+id1+"/relations", map[string]any{"number": t1["number"]}); r.Status != http.StatusBadRequest {
		t.Fatalf("self relation: %d", r.Status)
	}
	if r := c.post("/tasks/"+id1+"/relations", map[string]any{"number": 999999}); r.Status != http.StatusNotFound {
		t.Fatalf("unknown number relation: %d", r.Status)
	}

	// Delete one edge.
	if r := c.del("/relations/" + blocks["id"].(string)); r.Status != http.StatusNoContent {
		t.Fatalf("delete relation: %d\n%s", r.Status, r.Body)
	}
	if rels = c.get("/tasks/" + id1 + "/relations").listBody(t); len(rels) != 1 || rels[0]["kind"] != "relates" {
		t.Fatalf("relations after delete: %v", rels)
	}
}

// Attachments: multipart upload, list, byte-exact download, delete.
func TestAttachmentsFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	id := mkTask(t, c, s.Board, s.col(t, 0), "С файлом")["id"].(string)
	content := []byte("attachment payload — вложение\n\x00\x01\x02 binary tail")

	r := uploadFile(t, c, "/tasks/"+id+"/attachments", "file", "отчёт.txt", content)
	att := c.expect(t, r, http.StatusCreated)
	attID := att["id"].(string)
	if att["filename"] != "отчёт.txt" || att["size"] != float64(len(content)) {
		t.Fatalf("attachment meta: %v", att)
	}

	list := c.get("/tasks/" + id + "/attachments").listBody(t)
	if len(list) != 1 || list[0]["id"] != attID {
		t.Fatalf("attachment list: %v", list)
	}
	if !hasKind(eventKinds(t, c, id), "attachment") {
		t.Fatalf("no attachment journal event")
	}

	// Download returns the exact bytes.
	dl := c.get("/attachments/" + attID + "/download")
	if dl.Status != http.StatusOK || !bytes.Equal(dl.Body, content) {
		t.Fatalf("download: status %d, %d bytes (want %d)", dl.Status, len(dl.Body), len(content))
	}

	// Missing multipart field → 400.
	if r := uploadFile(t, c, "/tasks/"+id+"/attachments", "wrongfield", "x.txt", []byte("x")); r.Status != http.StatusBadRequest {
		t.Fatalf("wrong field upload: %d", r.Status)
	}

	if r := c.del("/attachments/" + attID); r.Status != http.StatusNoContent {
		t.Fatalf("delete attachment: %d\n%s", r.Status, r.Body)
	}
	if list = c.get("/tasks/" + id + "/attachments").listBody(t); len(list) != 0 {
		t.Fatalf("attachments after delete: %v", list)
	}
	if dl = c.get("/attachments/" + attID + "/download"); dl.Status != http.StatusNotFound {
		t.Fatalf("download after delete: %d", dl.Status)
	}
}

// Inline media uploads: authenticated POST /uploads, then a public (no-token)
// GET of the returned /api/uploads/<name> URL.
func TestMediaUploadPublicServe(t *testing.T) {
	t.Parallel()
	c := signup(t)
	content := []byte("\x89PNG\r\n\x1a\nfake image body")

	r := uploadFile(t, c, "/uploads", "file", "картинка.png", content)
	m := c.expect(t, r, http.StatusCreated)
	url, _ := m["url"].(string)
	if url == "" {
		t.Fatalf("no url in upload response: %v", m)
	}

	// Served publicly — a plain GET without Authorization.
	res, err := http.Get(testServer.URL + url)
	if err != nil {
		t.Fatalf("public GET: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, content) {
		t.Fatalf("public serve: status %d, %d bytes (want %d)", res.StatusCode, len(body), len(content))
	}

	// Non-image extension is rejected; a bogus name 404s on the public route.
	if r := uploadFile(t, c, "/uploads", "file", "script.exe", []byte("MZ")); r.Status != http.StatusBadRequest {
		t.Fatalf("non-image upload: %d\n%s", r.Status, r.Body)
	}
	res2, err := http.Get(testServer.URL + "/api/uploads/no-such-file.png")
	if err != nil {
		t.Fatalf("public GET missing: %v", err)
	}
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("missing upload: %d", res2.StatusCode)
	}
}
