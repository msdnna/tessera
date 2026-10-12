package main

import (
	"net/http"
	"testing"
)

// The project window's tag table (#2990, #3028): counts and origin on the list,
// one prefix renamed on its own, duplicates merged, a bulk delete undone.
func TestTagTable(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	mk := func(name string) string {
		return c.expect(t, c.post("/projects/"+s.Project+"/tags",
			map[string]any{"name": name, "color": "#7c5cff"}), http.StatusCreated)["id"].(string)
	}
	design, dizain, ui := mk("design"), mk("дизайн"), mk("ui")
	a := mkTask(t, c, s.Board, s.col(t, 0), "A")["id"].(string)
	b := mkTask(t, c, s.Board, s.col(t, 0), "B")["id"].(string)
	for _, p := range [][2]string{{a, design}, {a, dizain}, {b, dizain}, {b, ui}} {
		c.expect(t, c.post("/tasks/"+p[0]+"/tags", map[string]any{"tag_id": p[1]}), http.StatusNoContent)
	}

	counts := func() map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, tg := range c.get("/projects/" + s.Project + "/tags").listBody(t) {
			out[tg["name"].(string)] = tg
		}
		return out
	}
	got := counts()
	if got["дизайн"]["task_count"] != float64(2) || got["design"]["source"] != "user" {
		t.Fatalf("list = %v", got)
	}

	// One prefix's name, without resending the set.
	c.expect(t, c.put("/projects/"+s.Project+"/tag-prefixes",
		map[string]any{"prefixes": []map[string]any{{"prefix": "type::", "label": "Тип"}}}), http.StatusOK)
	c.expect(t, c.patch("/projects/"+s.Project+"/tag-prefixes",
		map[string]any{"prefix": "App::", "label": "Приложение"}), http.StatusOK)
	pfx := c.get("/projects/" + s.Project + "/tag-prefixes").listBody(t)
	if len(pfx) != 2 {
		t.Fatalf("prefixes = %v, want type:: kept and app:: added", pfx)
	}
	c.expect(t, c.patch("/projects/"+s.Project+"/tag-prefixes",
		map[string]any{"prefix": "app::", "label": ""}), http.StatusOK)
	if pfx := c.get("/projects/" + s.Project + "/tag-prefixes").listBody(t); len(pfx) != 1 || pfx[0]["prefix"] != "type::" {
		t.Fatalf("after clear = %v", pfx)
	}

	// Merge «дизайн» and «ui» into «design».
	m := c.expect(t, c.post("/projects/"+s.Project+"/tags/merge",
		map[string]any{"target_id": design, "source_ids": []string{dizain, ui}}), http.StatusOK)
	if m["merged"] != float64(2) {
		t.Fatalf("merge = %v", m)
	}
	got = counts()
	if len(got) != 1 || got["design"]["task_count"] != float64(2) {
		t.Fatalf("after merge = %v", got)
	}
	if d := lastTagEvent(t, c, b, "tagged"); d["tag"] != "design" {
		t.Fatalf("merge journal on b = %v", d)
	}
	if d := lastTagEvent(t, c, b, "untagged"); d["tag"] == nil {
		t.Fatalf("merge left no untagged entry on b")
	}

	// Bulk delete hands back what it removed; restore puts it back.
	del := c.expect(t, c.post("/projects/"+s.Project+"/tags/delete",
		map[string]any{"ids": []string{design}}), http.StatusOK)
	if len(counts()) != 0 {
		t.Fatalf("tags left after delete")
	}
	restored := c.expect(t, c.post("/projects/"+s.Project+"/tags/restore",
		map[string]any{"tags": del["tags"]}), http.StatusOK)
	if len(restored["tags"].([]any)) != 1 {
		t.Fatalf("restore = %v", restored)
	}
	got = counts()
	if got["design"]["id"] != design || got["design"]["task_count"] != float64(2) {
		t.Fatalf("after restore = %v", got)
	}

	// Someone else's tag cannot be merged or deleted through this project.
	other := signup(t)
	os := mkStack(t, other)
	foreign := other.expect(t, other.post("/projects/"+os.Project+"/tags",
		map[string]any{"name": "чужой"}), http.StatusCreated)["id"].(string)
	c.expect(t, c.post("/projects/"+s.Project+"/tags/delete",
		map[string]any{"ids": []string{foreign}}), http.StatusNotFound)
	other.expect(t, other.post("/projects/"+s.Project+"/tags/delete",
		map[string]any{"ids": []string{design}}), http.StatusForbidden)
}
