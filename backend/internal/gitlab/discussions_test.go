package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubGitLabBody is stubGitLab with a caller-chosen response body — the discussion
// endpoints are read for their payload, not just their status.
func stubGitLabBody(t *testing.T, status int, body string, sink *captured) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sink.method = r.Method
		sink.path = r.URL.EscapedPath()
		sink.form = map[string]string{}
		for k := range r.PostForm {
			sink.form[k] = r.PostForm.Get(k)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok-123")
}

func TestCreateIssueDiscussion(t *testing.T) {
	var got captured
	c := stubGitLabBody(t, http.StatusCreated,
		`{"id":"abc123sha","notes":[{"id":42}]}`, &got)

	discID, noteGID, err := c.CreateIssueDiscussion(context.Background(), "grp/project", 7, "hello")
	if err != nil {
		t.Fatalf("CreateIssueDiscussion: %v", err)
	}
	if got.method != http.MethodPost {
		t.Errorf("method = %s, want POST", got.method)
	}
	if !strings.HasSuffix(got.path, "/issues/7/discussions") {
		t.Errorf("path = %s", got.path)
	}
	if got.form["body"] != "hello" {
		t.Errorf("body = %q", got.form["body"])
	}
	// The discussion id is the whole point of posting roots this way: without it
	// a later reply has no thread to aim at.
	if discID != "abc123sha" {
		t.Errorf("discussion id = %q, want abc123sha", discID)
	}
	if noteGID != "gid://gitlab/Note/42" {
		t.Errorf("note gid = %q", noteGID)
	}
}

func TestCreateIssueDiscussionNote(t *testing.T) {
	var got captured
	c := stubGitLabBody(t, http.StatusCreated, `{"id":99}`, &got)

	gid, err := c.CreateIssueDiscussionNote(context.Background(), "grp/project", 7, "abc123sha", "reply")
	if err != nil {
		t.Fatalf("CreateIssueDiscussionNote: %v", err)
	}
	if !strings.HasSuffix(got.path, "/issues/7/discussions/abc123sha/notes") {
		t.Errorf("path = %s", got.path)
	}
	if got.form["body"] != "reply" {
		t.Errorf("body = %q", got.form["body"])
	}
	if gid != "gid://gitlab/Note/99" {
		t.Errorf("gid = %q", gid)
	}
}

// The mapper must carry the thread structure out of the discussions payload:
// the first non-system note of a discussion is the root (RootGID empty) and every
// later note in it points back at that root.
func TestToIssue_DiscussionThreads(t *testing.T) {
	const payload = `{
      "id":"gid://gitlab/Issue/1","iid":"7","title":"t",
      "discussions":{"nodes":[
        {"id":"gid://gitlab/Discussion/d1","notes":{"nodes":[
          {"id":"gid://gitlab/Note/1","body":"root one","system":false,"createdAt":"2026-08-01T10:00:00Z"},
          {"id":"gid://gitlab/Note/2","body":"reply one","system":false,"createdAt":"2026-08-01T11:00:00Z"}
        ]}},
        {"id":"gid://gitlab/Discussion/d2","notes":{"nodes":[
          {"id":"gid://gitlab/Note/3","body":"changed status","system":true,"createdAt":"2026-08-01T10:30:00Z"},
          {"id":"gid://gitlab/Note/4","body":"root two","system":false,"createdAt":"2026-08-01T10:40:00Z"}
        ]}}
      ]}
    }`
	var n issueNode
	if err := json.Unmarshal([]byte(payload), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	issue := n.toIssue("https://gitlab.example")

	if len(issue.Notes) != 3 {
		t.Fatalf("got %d notes, want 3 (the system note must be dropped)", len(issue.Notes))
	}
	// Flat and chronological, so a reply never precedes the root it answers.
	want := []string{"root one", "root two", "reply one"}
	for i, w := range want {
		if issue.Notes[i].Body != w {
			t.Errorf("note[%d] = %q, want %q", i, issue.Notes[i].Body, w)
		}
	}
	byBody := map[string]Note{}
	for _, nt := range issue.Notes {
		byBody[nt.Body] = nt
	}
	if got := byBody["root one"].RootGID; got != "" {
		t.Errorf("root one RootGID = %q, want empty", got)
	}
	if got := byBody["reply one"].RootGID; got != "gid://gitlab/Note/1" {
		t.Errorf("reply one RootGID = %q", got)
	}
	if got := byBody["reply one"].DiscussionID; got != "gid://gitlab/Discussion/d1" {
		t.Errorf("reply one DiscussionID = %q", got)
	}
	// A system note opening a discussion must not become the thread root: the
	// first *user* note does.
	if got := byBody["root two"].RootGID; got != "" {
		t.Errorf("root two RootGID = %q, want empty (system note is not a root)", got)
	}
}
