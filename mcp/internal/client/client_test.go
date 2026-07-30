package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tessera-mcp/internal/model"
)

func TestWebBaseURL(t *testing.T) {
	c := New("https://tessera.example/api/", "tok")
	if got := c.WebBaseURL(); got != "https://tessera.example" {
		t.Fatalf("WebBaseURL = %q", got)
	}
	// Trailing slash trimmed by New.
	if c.baseURL != "https://tessera.example/api" {
		t.Fatalf("baseURL = %q", c.baseURL)
	}
}

// recordingServer captures the last request seen so tests can assert on the
// method, auth header and decoded body.
type recordingServer struct {
	*httptest.Server
	lastMethod string
	lastPath   string
	lastAuth   string
	lastBody   []byte
}

func newServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*Client, *recordingServer) {
	t.Helper()
	rec := &recordingServer{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.lastMethod = r.Method
		rec.lastPath = r.URL.Path
		rec.lastAuth = r.Header.Get("Authorization")
		rec.lastBody, _ = io.ReadAll(r.Body)
		// Restore the body so handlers that re-parse it (multipart) still can.
		r.Body = io.NopCloser(bytes.NewReader(rec.lastBody))
		handler(w, r)
	}))
	t.Cleanup(rec.Close)
	return New(rec.URL+"/api", "test-token"), rec
}

func TestGetSendsAuthAndDecodes(t *testing.T) {
	c, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]model.Workspace{{ID: "w1", Name: "Personal"}})
	})
	ws, err := c.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(ws) != 1 || ws[0].Name != "Personal" {
		t.Fatalf("workspaces: %+v", ws)
	}
	if rec.lastAuth != "Bearer test-token" {
		t.Fatalf("auth header = %q", rec.lastAuth)
	}
	if rec.lastPath != "/api/workspaces" {
		t.Fatalf("path = %q", rec.lastPath)
	}
}

func TestGetNon200IsError(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid token"}`)
	})
	_, err := c.ListWorkspaces(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestSelfIDMemoised(t *testing.T) {
	calls := 0
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = io.WriteString(w, `{"user":{"id":"me-123"}}`)
	})
	if id := c.SelfID(context.Background()); id != "me-123" {
		t.Fatalf("SelfID = %q", id)
	}
	// Second call is served from the memoised value, not the server.
	if id := c.SelfID(context.Background()); id != "me-123" {
		t.Fatalf("SelfID (cached) = %q", id)
	}
	if calls != 1 {
		t.Fatalf("SelfID hit the server %d times, want 1", calls)
	}
}

func TestSelfIDBestEffortOnError(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if id := c.SelfID(context.Background()); id != "" {
		t.Fatalf("SelfID on error = %q, want empty", id)
	}
}

func TestCreateCommentPostsBody(t *testing.T) {
	c, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(model.Comment{ID: "cm1", Body: "hi"})
	})
	cm, err := c.CreateComment(context.Background(), "t1", "hi")
	if err != nil || cm.ID != "cm1" {
		t.Fatalf("CreateComment: %+v %v", cm, err)
	}
	if rec.lastMethod != http.MethodPost || rec.lastPath != "/api/tasks/t1/comments" {
		t.Fatalf("request = %s %s", rec.lastMethod, rec.lastPath)
	}
	var sent map[string]any
	_ = json.Unmarshal(rec.lastBody, &sent)
	if sent["body"] != "hi" {
		t.Fatalf("body = %v", sent)
	}
}

func TestMoveTaskPatch(t *testing.T) {
	c, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	if err := c.MoveTask(context.Background(), "t1", "c2"); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if rec.lastMethod != http.MethodPatch || rec.lastPath != "/api/tasks/t1/move" {
		t.Fatalf("request = %s %s", rec.lastMethod, rec.lastPath)
	}
	var sent map[string]any
	_ = json.Unmarshal(rec.lastBody, &sent)
	if sent["column_id"] != "c2" {
		t.Fatalf("body = %v", sent)
	}
}

func TestListBoardTasksMilestoneQuery(t *testing.T) {
	var gotQuery string
	cl, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]model.Task{})
	})
	if _, err := cl.ListBoardTasks(context.Background(), "b1", "backlog"); err != nil {
		t.Fatalf("ListBoardTasks: %v", err)
	}
	if gotQuery != "milestone=backlog" {
		t.Fatalf("query = %q", gotQuery)
	}
	// Empty milestone → no query string.
	if _, err := cl.ListBoardTasks(context.Background(), "b1", ""); err != nil {
		t.Fatalf("ListBoardTasks: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("query = %q, want empty", gotQuery)
	}
}

func TestUploadMediaMultipart(t *testing.T) {
	var gotCT, gotFilename string
	var gotData []byte
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if f, hdr, ferr := r.FormFile("file"); ferr == nil {
			gotFilename = hdr.Filename
			gotData, _ = io.ReadAll(f)
			_ = f.Close()
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"url": "/api/uploads/abc.png"})
	})
	url, err := c.UploadMedia(context.Background(), "shot.png", "", []byte("PNGDATA"))
	if err != nil {
		t.Fatalf("UploadMedia: %v", err)
	}
	if url != "/api/uploads/abc.png" {
		t.Fatalf("url = %q", url)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data") {
		t.Fatalf("content-type = %q", gotCT)
	}
	if gotFilename != "shot.png" || string(gotData) != "PNGDATA" {
		t.Fatalf("upload part: %q %q", gotFilename, gotData)
	}
}

func TestFetchImageRefResolution(t *testing.T) {
	c, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("IMG"))
	})

	// attachment: ref → /attachments/:id/download
	data, mimeType, err := c.FetchImage(context.Background(), "attachment:att1")
	if err != nil || string(data) != "IMG" || mimeType != "image/png" {
		t.Fatalf("attachment fetch: %v %q %q", err, data, mimeType)
	}
	if rec.lastPath != "/api/attachments/att1/download" {
		t.Fatalf("attachment path = %q", rec.lastPath)
	}

	// /api/... ref resolves against the web base (drops the extra /api once).
	if _, _, err := c.FetchImage(context.Background(), "/api/uploads/x.png"); err != nil {
		t.Fatalf("api-ref fetch: %v", err)
	}
	if rec.lastPath != "/api/uploads/x.png" {
		t.Fatalf("api-ref path = %q", rec.lastPath)
	}

	// Unknown scheme → error, no request.
	if _, _, err := c.FetchImage(context.Background(), "weird-ref"); err == nil {
		t.Fatal("expected error for unrecognised ref")
	}
}

func TestUpdateTaskSendsFullBody(t *testing.T) {
	c, rec := newServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	est := 2.0
	err := c.UpdateTask(context.Background(), "t1", TaskUpdate{
		Title: "T", Priority: 3, Estimate: &est, Recurrence: json.RawMessage(`{"freq":"daily"}`),
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if rec.lastMethod != http.MethodPatch || rec.lastPath != "/api/tasks/t1" {
		t.Fatalf("request = %s %s", rec.lastMethod, rec.lastPath)
	}
	var sent map[string]json.RawMessage
	_ = json.Unmarshal(rec.lastBody, &sent)
	if _, ok := sent["recurrence"]; !ok {
		t.Fatalf("recurrence not sent: %s", rec.lastBody)
	}
}
