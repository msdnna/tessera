//go:build e2e

// What the v2 task modal reads beyond the task itself (#2958): the activity
// feed draws a move as «from → to», and a link row shows the linked task's
// status — so the journal has to record the source column and the relations
// list has to carry the related task's column.
package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTaskActivityCarriesMoveSourceAndLinkStatus(t *testing.T) {
	s := startServer(t, nil)
	a := s.register(t, "activity")
	st := s.mkStack(t, a)
	if len(st.Columns) < 2 {
		t.Fatalf("need two seeded columns, got %d", len(st.Columns))
	}
	first, second := st.Columns[0], st.Columns[1]

	mk := func(title string) map[string]any {
		return expect(t, s.post(t, "/boards/"+st.Board+"/tasks", a.Access,
			map[string]any{"column_id": first["id"], "title": title}), http.StatusCreated)
	}
	moved := mk("Переезжает")
	linked := mk("Связанная")

	expect(t, s.call(t, request{
		Method: http.MethodPatch, Path: "/tasks/" + moved["id"].(string) + "/move",
		Token: a.Access, Body: map[string]any{"column_id": second["id"]},
	}), http.StatusOK)

	var events []struct {
		Kind string         `json:"kind"`
		Data map[string]any `json:"data"`
	}
	r := s.get(t, "/tasks/"+moved["id"].(string)+"/events", a.Access)
	if err := json.Unmarshal(r.Body, &events); err != nil {
		t.Fatalf("decode events: %v\n%s", err, r.Body)
	}
	found := false
	for _, e := range events {
		if e.Kind != "moved" {
			continue
		}
		found = true
		if e.Data["from"] != first["name"] || e.Data["to"] != second["name"] {
			t.Errorf("moved event data = %v, want from %v to %v", e.Data, first["name"], second["name"])
		}
	}
	if !found {
		t.Fatalf("no moved event in %s", r.Body)
	}

	// Link the moved task to the one still in the first column: the row must say
	// which column that is, and that it is the board's first.
	expect(t, s.post(t, "/tasks/"+moved["id"].(string)+"/relations", a.Access,
		map[string]any{"number": linked["number"], "kind": "relates"}), http.StatusCreated)
	var rels []map[string]any
	r = s.get(t, "/tasks/"+moved["id"].(string)+"/relations", a.Access)
	if err := json.Unmarshal(r.Body, &rels); err != nil {
		t.Fatalf("decode relations: %v\n%s", err, r.Body)
	}
	if len(rels) != 1 {
		t.Fatalf("relations = %d, want 1\n%s", len(rels), r.Body)
	}
	rel := rels[0]
	if rel["related_column_name"] != first["name"] || rel["related_column_color"] != first["color"] {
		t.Errorf("relation column = %v / %v, want %v / %v",
			rel["related_column_name"], rel["related_column_color"], first["name"], first["color"])
	}
	if rel["related_column_first"] != true {
		t.Errorf("related_column_first = %v, want true", rel["related_column_first"])
	}
}
