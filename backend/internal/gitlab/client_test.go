package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captured holds what the stub GitLab REST server saw.
type captured struct {
	method string
	path   string // EscapedPath (keeps %2F in the project path)
	token  string
	form   map[string]string
}

func stubGitLab(t *testing.T, status int, sink *captured) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sink.method = r.Method
		sink.path = r.URL.EscapedPath()
		sink.token = r.Header.Get("PRIVATE-TOKEN")
		sink.form = map[string]string{}
		for k := range r.PostForm {
			sink.form[k] = r.PostForm.Get(k)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok-123")
}

func TestUpdateIssueState(t *testing.T) {
	var got captured
	c := stubGitLab(t, http.StatusOK, &got)
	if err := c.UpdateIssueState(context.Background(), "grp/project", 7, "close"); err != nil {
		t.Fatalf("UpdateIssueState: %v", err)
	}
	if got.method != http.MethodPut {
		t.Errorf("method = %s, want PUT", got.method)
	}
	if !strings.HasSuffix(got.path, "/api/v4/projects/grp%2Fproject/issues/7") {
		t.Errorf("path = %s", got.path)
	}
	if got.token != "tok-123" {
		t.Errorf("token = %q", got.token)
	}
	if got.form["state_event"] != "close" {
		t.Errorf("state_event = %q, want close", got.form["state_event"])
	}
}

func TestSetIssueLabels(t *testing.T) {
	var got captured
	c := stubGitLab(t, http.StatusOK, &got)
	if err := c.SetIssueLabels(context.Background(), "grp/project", 7, []string{"P: High"}, []string{"P: Low", "P: Critical"}); err != nil {
		t.Fatalf("SetIssueLabels: %v", err)
	}
	if got.form["add_labels"] != "P: High" {
		t.Errorf("add_labels = %q", got.form["add_labels"])
	}
	if got.form["remove_labels"] != "P: Low,P: Critical" {
		t.Errorf("remove_labels = %q", got.form["remove_labels"])
	}
}

func TestSetIssueLabels_NoOp(t *testing.T) {
	var got captured
	c := stubGitLab(t, http.StatusInternalServerError, &got) // would error if it hit the wire
	if err := c.SetIssueLabels(context.Background(), "grp/project", 7, nil, nil); err != nil {
		t.Fatalf("empty SetIssueLabels should be a no-op, got %v", err)
	}
	if got.method != "" {
		t.Error("empty SetIssueLabels should not call the server")
	}
}

func TestCreateIssueNote(t *testing.T) {
	var got captured
	c := stubGitLab(t, http.StatusCreated, &got)
	if _, err := c.CreateIssueNote(context.Background(), "grp/project", 7, "hello"); err != nil {
		t.Fatalf("CreateIssueNote: %v", err)
	}
	if got.method != http.MethodPost {
		t.Errorf("method = %s, want POST", got.method)
	}
	if !strings.HasSuffix(got.path, "/issues/7/notes") {
		t.Errorf("path = %s", got.path)
	}
	if got.form["body"] != "hello" {
		t.Errorf("body = %q", got.form["body"])
	}
}

func TestCreateIssueNote_ReturnsGID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 42}`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "tok")
	gid, err := c.CreateIssueNote(context.Background(), "grp/project", 7, "hi")
	if err != nil {
		t.Fatalf("CreateIssueNote: %v", err)
	}
	if gid != "gid://gitlab/Note/42" {
		t.Errorf("gid = %q, want gid://gitlab/Note/42", gid)
	}
}

func TestRest_APIErrorCarriesStatus(t *testing.T) {
	var got captured
	c := stubGitLab(t, http.StatusForbidden, &got)
	err := c.UpdateIssueState(context.Background(), "grp/project", 7, "close")
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if ae.Status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", ae.Status)
	}
}
