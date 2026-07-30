package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// muxAPI serves canned GET responses and records write requests (POST/PATCH) so
// tests can assert both the read path and what was sent.
type muxAPI struct {
	*httptest.Server
	writes map[string][]byte // "METHOD path" → last body
}

func newMux(t *testing.T, get map[string]any) (*client.Client, *muxAPI) {
	t.Helper()
	m := &muxAPI{writes: map[string][]byte{}}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			body, ok := get[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		m.writes[r.Method+" "+r.URL.Path] = buf
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cm-new"}`))
	}))
	t.Cleanup(m.Close)
	return client.New(m.URL+"/api", "tok"), m
}

func detail(id, board string) model.TaskDetail {
	d := model.TaskDetail{}
	d.ID = id
	d.BoardID = board
	d.Title = "Task"
	return d
}

func TestMoveTaskHandler(t *testing.T) {
	c, mux := newMux(t, map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
		"/api/boards/b1/columns": []model.Column{
			{ID: "c1", Name: "К работе"}, {ID: "c2", Name: "В процессе"},
		},
	})
	// Missing column → error.
	if _, _, err := moveTask(c)(context.Background(), nil, moveTaskInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error for empty column")
	}
	// Move by column name (case-insensitive).
	_, out, err := moveTask(c)(context.Background(), nil, moveTaskInput{TaskID: "t1", Column: "в процессе"})
	if err != nil || out.ColumnID != "c2" || out.Column != "В процессе" {
		t.Fatalf("moveTask: %+v %v", out, err)
	}
	if _, ok := mux.writes["PATCH /api/tasks/t1/move"]; !ok {
		t.Fatalf("move not sent: %v", mux.writes)
	}
	// Unknown column → error listing available.
	_, _, err = moveTask(c)(context.Background(), nil, moveTaskInput{TaskID: "t1", Column: "Nope"})
	if err == nil {
		t.Fatal("expected error for unknown column")
	}
}

func TestListColumnsHandler(t *testing.T) {
	doneID := "c2"
	c, _ := newMux(t, map[string]any{
		"/api/boards/b1/columns": []model.Column{
			{ID: "c1", Name: "К работе", Position: 1}, {ID: "c2", Name: "Готово", Position: 2},
		},
		"/api/boards/b1": model.Board{ID: "b1", DoneColumnID: &doneID},
	})
	if _, _, err := listColumns(c)(context.Background(), nil, listColumnsInput{}); err == nil {
		t.Fatal("expected error for empty board_id")
	}
	_, out, err := listColumns(c)(context.Background(), nil, listColumnsInput{BoardID: "b1"})
	if err != nil || len(out.Columns) != 2 {
		t.Fatalf("listColumns: %+v %v", out, err)
	}
	if out.Columns[1].ID != "c2" || !out.Columns[1].IsDone || out.Columns[0].IsDone {
		t.Fatalf("done flag wrong: %+v", out.Columns)
	}
}

func TestAddCommentHandler(t *testing.T) {
	c, mux := newMux(t, map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
	})
	// Empty body and no images → error.
	if _, _, err := addComment(c)(context.Background(), nil, addCommentInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error for empty comment")
	}
	_, out, err := addComment(c)(context.Background(), nil, addCommentInput{TaskID: "t1", Body: "hello"})
	if err != nil || out.CommentID != "cm-new" || out.TaskID != "t1" {
		t.Fatalf("addComment: %+v %v", out, err)
	}
	if _, ok := mux.writes["POST /api/tasks/t1/comments"]; !ok {
		t.Fatalf("comment not posted: %v", mux.writes)
	}
}

func TestUpdateDescriptionHandler(t *testing.T) {
	d := detail("t1", "b1")
	d.Description = "Existing"
	c, mux := newMux(t, map[string]any{"/api/tasks/t1": d})

	// Validation: empty markdown, bad mode.
	if _, _, err := updateDescription(c)(context.Background(), nil, updateDescriptionInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error for empty markdown")
	}
	if _, _, err := updateDescription(c)(context.Background(), nil,
		updateDescriptionInput{TaskID: "t1", Markdown: "x", Mode: "bogus"}); err == nil {
		t.Fatal("expected error for bad mode")
	}

	// Append with heading keeps the existing text + separator.
	_, out, err := updateDescription(c)(context.Background(), nil, updateDescriptionInput{
		TaskID: "t1", Markdown: "new line", Heading: "План",
	})
	if err != nil || out.Mode != "append" {
		t.Fatalf("updateDescription: %+v %v", out, err)
	}
	sent := mux.writes["PATCH /api/tasks/t1"]
	var body map[string]any
	_ = json.Unmarshal(sent, &body)
	desc, _ := body["description"].(string)
	if !strings.Contains(desc, "Existing") || !strings.Contains(desc, "## План") || !strings.Contains(desc, "new line") {
		t.Fatalf("appended description = %q", desc)
	}
}
