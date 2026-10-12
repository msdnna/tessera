package main

import (
	"net/http"
	"strings"
	"testing"
)

// A tag's name is unique per project (UNIQUE(project_id, name), migration 0023),
// and both write paths reach that index from a plain text field: the «создать»
// row in the task modal offers whatever is typed without consulting the project's
// tags, and the rename field in TagManager saves on blur as well as on Enter.
// Colliding is therefore an ordinary thing to do, so it has to come back as a
// refusal the reader can act on rather than "internal error" (#2932).
func TestTagNameCollision(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	first := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "релиз"}), http.StatusCreated)
	second := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "хотфикс"}), http.StatusCreated)

	// Create onto a taken name.
	r := c.post("/projects/"+s.Project+"/tags", map[string]any{"name": "релиз"})
	if r.Status != http.StatusConflict {
		t.Fatalf("create duplicate: status %d, want 409\n%s", r.Status, r.Body)
	}
	if !strings.Contains(string(r.Body), "уже есть в проекте") {
		t.Fatalf("create duplicate: no usable sentence in %s", r.Body)
	}

	// Rename onto a taken name — and the target keeps its own name, so a refused
	// rename leaves nothing half-applied.
	r = c.patch("/tags/"+second["id"].(string), map[string]any{"name": "релиз"})
	if r.Status != http.StatusConflict {
		t.Fatalf("rename onto taken name: status %d, want 409\n%s", r.Status, r.Body)
	}
	tags := c.get("/projects/" + s.Project + "/tags").listBody(t)
	if len(tags) != 2 {
		t.Fatalf("tags after two refusals = %d, want 2", len(tags))
	}
	names := map[string]bool{}
	for _, tg := range tags {
		names[tg["name"].(string)] = true
	}
	if !names["релиз"] || !names["хотфикс"] {
		t.Fatalf("names after refused rename: %v", names)
	}

	// The constraint is per project, not per workspace: the same name in a
	// sibling project is not a collision. Without this the 409 branch would look
	// right while actually refusing something legal.
	g2 := mkGroup(t, c, s.WS, "Вторая группа")
	p2 := mkProject(t, c, s.WS, g2, "Второй проект")
	twin := c.expect(t, c.post("/projects/"+p2+"/tags",
		map[string]any{"name": "релиз"}), http.StatusCreated)
	if twin["id"] == first["id"] {
		t.Fatal("sibling project got the same tag row")
	}

	// Renaming to the name it already has must not trip the index either (the
	// recolour path sends the current name alongside the new colour).
	c.expect(t, c.patch("/tags/"+first["id"].(string),
		map[string]any{"name": "релиз", "color": "#0eb0a9"}), http.StatusOK)
}
