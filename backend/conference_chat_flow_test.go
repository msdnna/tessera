package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// In-call chat (#2864, subtask #2873).
//
// The three things worth holding down here are the ones a chat gets wrong
// silently: that a page is served in reading order with a cursor that survives
// same-instant messages, that an attachment's type is decided by its bytes
// rather than by its name, and that the files behind a private call stay behind
// authorization.

// postChat sends a chat message with files attached, as the composer does.
func postChat(t *testing.T, c *client, confID, body string, files map[string][]byte) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("body", body); err != nil {
		t.Fatalf("multipart field: %v", err)
	}
	for name, content := range files {
		fw, err := w.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("multipart file: %v", err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("multipart write: %v", err)
		}
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/conferences/"+confID+"/messages", &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

// chatPage fetches a page of the chat and returns its messages.
func chatPage(t *testing.T, c *client, path string) ([]any, bool) {
	t.Helper()
	got := c.expect(t, c.get(path), http.StatusOK)
	msgs, _ := got["messages"].([]any)
	more, _ := got["has_more"].(bool)
	return msgs, more
}

// TestConferenceChatFlow covers the ordinary conversation: everyone in the
// workspace may read and write, the page comes back oldest-first, and the author
// is named so the rail does not have to resolve ids.
func TestConferenceChatFlow(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с чатом")
	mate := addMember(t, owner, s.WS)

	first := owner.expect(t, owner.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "первое"}), http.StatusCreated)
	if first["body"] != "первое" {
		t.Fatalf("posted message came back as %#v", first)
	}
	// The poster's own name comes back with the message: without it their line
	// renders as an anonymous row until the next page load.
	if first["user_name"] == nil {
		t.Fatalf("message has no author name: %#v", first)
	}
	mate.expect(t, mate.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "второе"}), http.StatusCreated)

	msgs, more := chatPage(t, mate, "/conferences/"+confID+"/messages")
	if len(msgs) != 2 {
		t.Fatalf("page has %d messages, want 2", len(msgs))
	}
	// Oldest first — the rail draws top to bottom, and a page in query order
	// would read the conversation backwards.
	if msgs[0].(map[string]any)["body"] != "первое" || msgs[1].(map[string]any)["body"] != "второе" {
		t.Fatalf("page is not in reading order: %#v", msgs)
	}
	if more {
		t.Fatalf("a two-message chat reports has_more")
	}

	// Blank submissions are refused: a chat that accepts them fills with empty
	// rows the first time somebody leans on Enter.
	if r := owner.post("/conferences/"+confID+"/messages", map[string]any{"body": "   "}); r.Status != http.StatusBadRequest {
		t.Fatalf("empty message: status %d, want 400\n%s", r.Status, r.Body)
	}

	// An outsider is not a member of the workspace, so the chat does not exist
	// for them — in either direction.
	outsider := signup(t)
	if r := outsider.get("/conferences/" + confID + "/messages"); r.Status != http.StatusForbidden {
		t.Fatalf("outsider read: status %d, want 403", r.Status)
	}
	if r := outsider.post("/conferences/"+confID+"/messages", map[string]any{"body": "привет"}); r.Status != http.StatusForbidden {
		t.Fatalf("outsider write: status %d, want 403", r.Status)
	}
}

// TestConferenceChatPaging walks the cursor. The page is deliberately smaller
// than the conversation, because the bug this guards against — an offset that
// skips or repeats a message when a new one arrives mid-read — only appears at
// a page boundary.
func TestConferenceChatPaging(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := mkConference(t, owner, "Длинный чат")

	const total = 5
	for _, body := range []string{"m1", "m2", "m3", "m4", "m5"} {
		owner.expect(t, owner.post("/conferences/"+confID+"/messages",
			map[string]any{"body": body}), http.StatusCreated)
	}

	page, more := chatPage(t, owner, "/conferences/"+confID+"/messages?limit=2")
	if len(page) != 2 || !more {
		t.Fatalf("first page = %d messages (has_more=%v), want 2 and true", len(page), more)
	}
	// A page is the *newest* limit messages, in reading order: opening a chat
	// shows its end, not its beginning.
	if page[0].(map[string]any)["body"] != "m4" || page[1].(map[string]any)["body"] != "m5" {
		t.Fatalf("first page is not the tail of the chat: %#v", page)
	}

	seen := []string{}
	oldest := page[0].(map[string]any)
	for _, m := range page {
		seen = append(seen, m.(map[string]any)["body"].(string))
	}
	for range total {
		// QueryEscape, not the raw timestamp: a "+03:00" offset pasted into a
		// query string arrives as a space and the cursor is rejected.
		q := "/conferences/" + confID + "/messages?limit=2" +
			"&before_at=" + url.QueryEscape(oldest["created_at"].(string)) +
			"&before_id=" + oldest["id"].(string)
		older, hasMore := chatPage(t, owner, q)
		if len(older) == 0 {
			break
		}
		next := []string{}
		for _, m := range older {
			next = append(next, m.(map[string]any)["body"].(string))
		}
		seen = append(next, seen...)
		oldest = older[0].(map[string]any)
		if !hasMore {
			break
		}
	}
	if len(seen) != total {
		t.Fatalf("paging back through the chat saw %v, want all %d messages", seen, total)
	}
	for i, want := range []string{"m1", "m2", "m3", "m4", "m5"} {
		if seen[i] != want {
			t.Fatalf("paged chat = %v, want m1..m5", seen)
		}
	}

	// A cursor with only one half is a client bug, and answering it with an
	// empty page would look like the end of the history.
	if r := owner.get("/conferences/" + confID + "/messages?before_at=2026-01-01T00:00:00Z"); r.Status != http.StatusBadRequest {
		t.Fatalf("half a cursor: status %d, want 400\n%s", r.Status, r.Body)
	}
	if r := owner.get("/conferences/" + confID + "/messages?limit=0"); r.Status != http.StatusBadRequest {
		t.Fatalf("limit=0: status %d, want 400", r.Status)
	}
}

// TestConferenceChatAttachments is the security-shaped half: what decides
// "this is a picture" is the file's leading bytes, and the bytes themselves stay
// behind workspace membership.
func TestConferenceChatAttachments(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Чат с файлами")

	created := postChat(t, owner, confID, "смотри", map[string][]byte{
		"снимок.png": []byte(pngBody),
		"notes.txt":  []byte("just text"),
	})
	if created.Status != http.StatusCreated {
		t.Fatalf("post with files: status %d\n%s", created.Status, created.Body)
	}
	msg := created.mapBody(t)
	atts, _ := msg["attachments"].([]any)
	if len(atts) != 2 {
		t.Fatalf("message carries %d attachments, want 2\n%s", len(atts), created.Body)
	}
	byName := map[string]map[string]any{}
	for _, a := range atts {
		m := a.(map[string]any)
		byName[m["filename"].(string)] = m
	}
	// The picture is recognised from its magic bytes...
	if img := byName["снимок.png"]; img == nil || img["is_image"] != true || img["content_type"] != "image/png" {
		t.Fatalf("PNG was not recognised as an image: %#v", byName["снимок.png"])
	}
	// ...and the text file is not, however it is named. This is the whole point
	// of sniffing: a `.png` suffix on arbitrary bytes must not make the rail
	// render them.
	if txt := byName["notes.txt"]; txt == nil || txt["is_image"] != false {
		t.Fatalf("a text file was reported as an image: %#v", byName["notes.txt"])
	}

	// A liar: HTML bytes under a picture's name. It stores fine — we keep the
	// user's file — but it is not an image, so no client will render it inline
	// on our own origin.
	liar := postChat(t, owner, confID, "", map[string][]byte{
		"payload.png": []byte("<html><script>alert(1)</script></html>"),
	})
	if liar.Status != http.StatusCreated {
		t.Fatalf("post with a mislabelled file: status %d\n%s", liar.Status, liar.Body)
	}
	la := liar.mapBody(t)["attachments"].([]any)[0].(map[string]any)
	if la["is_image"] != false {
		t.Fatalf("HTML named .png was reported as an image: %#v", la)
	}

	attID := byName["снимок.png"]["id"].(string)
	dl := owner.get("/conference-attachments/" + attID)
	if dl.Status != http.StatusOK {
		t.Fatalf("download: status %d\n%s", dl.Status, dl.Body)
	}
	if !bytes.Equal(dl.Body, []byte(pngBody)) {
		t.Fatalf("download returned %d bytes, want the uploaded file", len(dl.Body))
	}
	// Served as a download with the type the sniff found, never as renderable
	// content: the client that shows a picture fetches these bytes and renders
	// them from a blob, so nothing is lost and the origin stays clean.
	if ct := dl.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("download Content-Type = %q, want image/png", ct)
	}
	if cd := dl.Header.Get("Content-Disposition"); cd == "" || cd[:10] != "attachment" {
		t.Fatalf("download Content-Disposition = %q, want an attachment", cd)
	}
	if dl.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download is missing nosniff")
	}

	// Knowing the attachment id is not permission to read it.
	outsider := signup(t)
	if r := outsider.get("/conference-attachments/" + attID); r.Status != http.StatusForbidden {
		t.Fatalf("outsider download: status %d, want 403\n%s", r.Status, r.Body)
	}
	_ = s
}

// TestConferenceChatDeletion pins the moderation boundary: your own message is
// yours to retract, someone else's is not — unless you run the call.
func TestConferenceChatDeletion(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Чат с модерацией")
	mate := addMember(t, owner, s.WS)

	mine := mate.expect(t, mate.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "моё"}), http.StatusCreated)
	theirs := owner.expect(t, owner.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "чужое"}), http.StatusCreated)

	// A plain member cannot remove the host's line.
	if r := mate.del("/conference-messages/" + theirs["id"].(string)); r.Status != http.StatusForbidden {
		t.Fatalf("member deleting the host's message: status %d, want 403\n%s", r.Status, r.Body)
	}
	// Their own, they can.
	if r := mate.del("/conference-messages/" + mine["id"].(string)); r.Status != http.StatusNoContent {
		t.Fatalf("member deleting their own message: status %d, want 204\n%s", r.Status, r.Body)
	}
	// And the host can remove anyone's — the same boundary the rest of the call
	// draws around kick and force-mute.
	second := mate.expect(t, mate.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "снова моё"}), http.StatusCreated)
	if r := owner.del("/conference-messages/" + second["id"].(string)); r.Status != http.StatusNoContent {
		t.Fatalf("host deleting a member's message: status %d, want 204\n%s", r.Status, r.Body)
	}

	msgs, _ := chatPage(t, owner, "/conferences/"+confID+"/messages")
	if len(msgs) != 1 || msgs[0].(map[string]any)["body"] != "чужое" {
		t.Fatalf("after deletions the chat holds %#v, want only the host's line", msgs)
	}
}

// TestConferenceChatNudgesTheRoom pins the half of the design that is easy to
// lose in a refactor: the socket carries a payload-free "the chat changed", and
// the message itself travels over HTTP. Putting bodies on that socket would let
// a busy conversation trip the room's slow-participant eviction and disconnect
// the very people reading it.
func TestConferenceChatNudgesTheRoom(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := mkConference(t, owner, "Чат и сокет")

	conn, status := dialConfWS(t, owner.token, confID)
	if conn == nil {
		t.Fatalf("conference socket handshake: status %d", status)
	}
	defer conn.Close()
	if awaitConfFrame(t, conn, "welcome", 3*time.Second) == nil {
		t.Fatalf("no welcome frame")
	}

	owner.expect(t, owner.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "по сокету"}), http.StatusCreated)

	nudge := awaitConfFrame(t, conn, "chat", 3*time.Second)
	if nudge == nil {
		t.Fatalf("posting a message did not nudge the room socket")
	}
	// Payload-free on purpose — the client answers it by refetching the page.
	if _, has := nudge["body"]; has {
		t.Fatalf("the chat nudge carries a payload: %#v", nudge)
	}
}

// TestConferenceChatSurvivesEndingTheSession: a conference is a reusable room
// (#2879), so ending a session pauses it rather than archiving it — the chat, the
// closest thing the call has to a protocol, stays writable and keeps its history
// for the next session on the same conference.
func TestConferenceChatSurvivesEndingTheSession(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := mkConference(t, owner, "Ежедневная летучка")
	owner.expect(t, owner.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "во время"}), http.StatusCreated)
	owner.expect(t, owner.post("/conferences/"+confID+"/end", nil), http.StatusOK)

	// The room can be used again, so the chat is not sealed: a follow-up message
	// lands and the earlier one is still there.
	owner.expect(t, owner.post("/conferences/"+confID+"/messages",
		map[string]any{"body": "после"}), http.StatusCreated)
	msgs, _ := chatPage(t, owner, "/conferences/"+confID+"/messages")
	if len(msgs) != 2 {
		t.Fatalf("the reusable call's chat has %d messages, want both", len(msgs))
	}
}
