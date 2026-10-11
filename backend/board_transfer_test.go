package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// waitJob polls a member's own run until it settles (done/failed/cancelled).
func waitJob(t *testing.T, c *client, runID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		m := c.expect(t, c.get("/jobs/"+runID), http.StatusOK)
		switch m["status"] {
		case "done", "failed", "cancelled":
			return m
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s did not settle: %v", runID, m)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func jobRunID(t *testing.T, m map[string]any) string {
	t.Helper()
	job, _ := m["job"].(map[string]any)
	id, _ := job["run_id"].(string)
	if id == "" {
		t.Fatalf("no job.run_id in %v", m)
	}
	return id
}

// Export (#3094): a background job writes the file, the member who ran it
// downloads it; task_ids narrows the export to the cards the view shows, plus
// their subtasks.
func TestBoardExportCSVAndJSON(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	a := mkTask(t, c, s.Board, s.col(t, 0), "Первая, с запятой")
	b := mkTask(t, c, s.Board, s.col(t, 1), "Вторая")
	sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Подзадача первой", "column_id": s.col(t, 0), "parent_id": a["id"],
	}), http.StatusCreated)
	_ = sub

	// CSV of the whole board.
	run := jobRunID(t, c.expect(t, c.post("/boards/"+s.Board+"/export", map[string]any{"format": "csv"}), http.StatusAccepted))
	if m := waitJob(t, c, run); m["status"] != "done" {
		t.Fatalf("csv export: %v", m)
	}
	dl := c.get("/jobs/" + run + "/download")
	if dl.Status != http.StatusOK {
		t.Fatalf("download = %d %s", dl.Status, dl.Body)
	}
	body := string(dl.Body)
	if !strings.HasPrefix(body, "\uFEFFnumber,title,") {
		t.Fatalf("csv header: %q", body[:min(60, len(body))])
	}
	for _, want := range []string{`"Первая, с запятой"`, "Вторая", "Подзадача первой"} {
		if !strings.Contains(body, want) {
			t.Fatalf("csv lacks %s:\n%s", want, body)
		}
	}
	if cd := dl.Header.Get("Content-Disposition"); !strings.Contains(cd, ".csv") {
		t.Fatalf("Content-Disposition = %q", cd)
	}

	// JSON narrowed to the first card: its subtask comes along, the other card doesn't.
	run = jobRunID(t, c.expect(t, c.post("/boards/"+s.Board+"/export", map[string]any{
		"format": "json", "task_ids": []string{a["id"].(string)},
	}), http.StatusAccepted))
	if m := waitJob(t, c, run); m["status"] != "done" {
		t.Fatalf("json export: %v", m)
	}
	dl = c.get("/jobs/" + run + "/download")
	var doc struct {
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(dl.Body, &doc); err != nil {
		t.Fatalf("json body: %v\n%s", err, dl.Body)
	}
	if len(doc.Tasks) != 2 {
		t.Fatalf("filtered export has %d tasks, want 2 (card + subtask): %s", len(doc.Tasks), dl.Body)
	}
	for _, tk := range doc.Tasks {
		if tk["title"] == b["title"] {
			t.Fatalf("filtered export leaked the other card")
		}
	}

	// Somebody else can neither read the run nor download its file.
	other := signup(t)
	if r := other.get("/jobs/" + run); r.Status != http.StatusNotFound {
		t.Fatalf("stranger GET job = %d, want 404", r.Status)
	}
	if r := other.get("/jobs/" + run + "/download"); r.Status != http.StatusNotFound {
		t.Fatalf("stranger download = %d, want 404", r.Status)
	}
	// …nor export a board of a workspace they're not in.
	if r := other.post("/boards/"+s.Board+"/export", map[string]any{"format": "csv"}); r.Status != http.StatusForbidden {
		t.Fatalf("stranger export = %d, want 403", r.Status)
	}
	if r := c.post("/boards/"+s.Board+"/export", map[string]any{"format": "xlsx"}); r.Status != http.StatusBadRequest {
		t.Fatalf("bad format = %d, want 400", r.Status)
	}
}

// Import (#3095): a CSV with a mapping creates tasks in the matched columns with
// tags (created in the project when new), assignees matched by e-mail, priority
// and dates; unknown values are warnings in the run's log, not failures.
func TestBoardImportCSV(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col1 := s.Columns[1]["name"].(string)

	csvBody := "Название;Описание;Статус;Метки;Кто;Важность;Срок\n" +
		"Импорт один;Текст;" + col1 + ";alpha, beta;" + c.Email + ";high;2026-11-05\n" +
		"Импорт два;;Нет такой;;nobody@example.com;странный;05.11.2026\n" +
		";пустой заголовок;;;;;\n"
	mapping, _ := json.Marshal(map[string]string{
		"title": "Название", "description": "Описание", "column": "Статус", "tags": "Метки",
		"assignees": "Кто", "priority": "Важность", "due_date": "Срок",
	})
	r := uploadFileFields(t, c, "/boards/"+s.Board+"/import", "file", "tasks.csv", []byte(csvBody),
		map[string]string{"mapping": string(mapping)})
	if r.Status != http.StatusAccepted {
		t.Fatalf("import = %d %s", r.Status, r.Body)
	}
	run := jobRunID(t, r.mapBody(t))
	m := waitJob(t, c, run)
	if m["status"] != "done" {
		t.Fatalf("import job: %v", m)
	}
	if n, _ := m["created"].(float64); n != 2 {
		t.Fatalf("created = %v, want 2", m["created"])
	}
	if w, _ := m["log_warns"].(float64); w < 4 {
		t.Fatalf("log_warns = %v, want ≥4 (column, assignee, priority, empty title)", m["log_warns"])
	}

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	var one map[string]any
	for _, tk := range tasks {
		if tk["title"] == "Импорт один" {
			one = tk
		}
	}
	if one == nil {
		t.Fatalf("imported task missing: %v", tasks)
	}
	if one["column_id"] != s.col(t, 1) {
		t.Errorf("column = %v, want the matched %s", one["column_id"], col1)
	}
	if p, _ := one["priority"].(float64); p != 3 {
		t.Errorf("priority = %v, want 3", one["priority"])
	}
	// Compare the instant: the server may render it in its own zone (+03:00).
	if d, _ := time.Parse(time.RFC3339, one["due_date"].(string)); !d.Equal(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("due_date = %v, want date-only UTC midnight", one["due_date"])
	}
	if ids, _ := one["tag_ids"].([]any); len(ids) != 2 {
		t.Errorf("tag_ids = %v, want 2", one["tag_ids"])
	}
	if ids, _ := one["assignee_ids"].([]any); len(ids) != 1 || ids[0] != c.UserID {
		t.Errorf("assignee_ids = %v, want [me]", one["assignee_ids"])
	}

	// No title column → refused up front.
	bad, _ := json.Marshal(map[string]string{"description": "Описание"})
	if r := uploadFileFields(t, c, "/boards/"+s.Board+"/import", "file", "tasks.csv", []byte(csvBody),
		map[string]string{"mapping": string(bad)}); r.Status != http.StatusBadRequest {
		t.Fatalf("import without title mapping = %d, want 400", r.Status)
	}
}

// A JSON file in the shape our export writes imports back.
func TestBoardImportJSONRoundTrip(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	body := `{"tasks":[{"title":"Из JSON","tags":["x","y"],"priority":"urgent","column":"` +
		s.Columns[0]["name"].(string) + `"}]}`
	mapping, _ := json.Marshal(map[string]string{"title": "title", "tags": "tags", "priority": "priority", "column": "column"})
	r := uploadFileFields(t, c, "/boards/"+s.Board+"/import", "file", "tasks.json", []byte(body),
		map[string]string{"mapping": string(mapping)})
	if r.Status != http.StatusAccepted {
		t.Fatalf("import = %d %s", r.Status, r.Body)
	}
	if m := waitJob(t, c, jobRunID(t, r.mapBody(t))); m["status"] != "done" || m["created"].(float64) != 1 {
		t.Fatalf("json import: %v", m)
	}
}
