package main

import (
	"net/http"
	"strings"
	"testing"
)

// A tag name is unique per project byte-for-byte (UNIQUE (project_id, name),
// migration 0023) while nothing that READS one compares it that way: `canonPrefix`
// lowercases a namespace on all three clients, the web tag picker's
// `matchTagByName` resolves a typed name case-insensitively, the MCP `resolveTags`
// matches the same way. So «Сложность::высокая» and «сложность::высокая» were two
// live rows that every consumer treats as one name — and the killer feature draws
// one lane per row, with `tagLaneLabel` printing only the value once the grouping is
// narrowed to a prefix: two lanes captioned identically, the project's cards split
// between them, and nothing on screen to tell them apart. Same for padding: «bug »
// and «bug» sort apart, group apart and both render as «bug» (#2932).
func TestTagCaseTwinRefused(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	first := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "Сложность::высокая"}), http.StatusCreated)

	// The twin differing only in case.
	r := c.post("/projects/"+s.Project+"/tags", map[string]any{"name": "сложность::высокая"})
	if r.Status != http.StatusConflict {
		t.Fatalf("create case-twin: status %d, want 409\n%s", r.Status, r.Body)
	}
	if !strings.Contains(string(r.Body), "уже есть в проекте") {
		t.Fatalf("create case-twin: no usable sentence in %s", r.Body)
	}

	// And the one differing only in padding — the trim is part of the same answer,
	// or a name pasted with a trailing space walks straight past the case check.
	r = c.post("/projects/"+s.Project+"/tags", map[string]any{"name": "  Сложность::высокая  "})
	if r.Status != http.StatusConflict {
		t.Fatalf("create padded twin: status %d, want 409\n%s", r.Status, r.Body)
	}

	tags := c.get("/projects/" + s.Project + "/tags").listBody(t)
	if len(tags) != 1 {
		t.Fatalf("tags after two refusals = %d, want 1: %v", len(tags), tags)
	}
	if tags[0]["name"] != "Сложность::высокая" {
		t.Fatalf("surviving tag renamed itself: %v", tags[0])
	}

	// A name is stored trimmed, so the padded spelling can never become the row that
	// the next comparison has to normalise around.
	padded := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "  хотфикс  "}), http.StatusCreated)
	if padded["name"] != "хотфикс" {
		t.Fatalf("stored name = %q, want it trimmed", padded["name"])
	}

	// Blank: `binding:"required"` only rejects "", so spaces alone used to be saved
	// as a nameless tag — a blank pill and a blank lane.
	r = c.post("/projects/"+s.Project+"/tags", map[string]any{"name": "   "})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("create blank name: status %d, want 400\n%s", r.Status, r.Body)
	}
	if !strings.Contains(string(r.Body), "не может быть пустым") {
		t.Fatalf("create blank name: no usable sentence in %s", r.Body)
	}

	// ── controls: what must stay allowed ──

	// Re-spelling a tag's OWN name is an edit, not a collision with itself — this is
	// the one way to fix the casing of a tag a sync brought in.
	recased := c.expect(t, c.patch("/tags/"+first["id"].(string),
		map[string]any{"name": "сложность::Высокая"}), http.StatusOK)
	if recased["name"] != "сложность::Высокая" {
		t.Fatalf("re-casing own name: %v", recased)
	}

	// Renaming ONTO another tag's name in a different case is still a collision, and
	// the target keeps its own name — a refused rename leaves nothing half-applied.
	r = c.patch("/tags/"+padded["id"].(string), map[string]any{"name": "СЛОЖНОСТЬ::ВЫСОКАЯ"})
	if r.Status != http.StatusConflict {
		t.Fatalf("rename onto case-twin: status %d, want 409\n%s", r.Status, r.Body)
	}
	names := tagNames(c.get("/projects/" + s.Project + "/tags").listBody(t))
	if !names["сложность::Высокая"] || !names["хотфикс"] || len(names) != 2 {
		t.Fatalf("names after refused rename: %v", names)
	}

	// The scope is the project, not the workspace: the same name next door is legal,
	// and without this the 409 branch would look right while refusing something fine.
	g2 := mkGroup(t, c, s.WS, "Вторая группа")
	p2 := mkProject(t, c, s.WS, g2, "Второй проект")
	twin := c.expect(t, c.post("/projects/"+p2+"/tags",
		map[string]any{"name": "СЛОЖНОСТЬ::высокая"}), http.StatusCreated)
	if twin["id"] == first["id"] {
		t.Fatal("sibling project got the same tag row")
	}
}

// The GitLab sync is the likeliest way a project ever acquires a case-twin: a label
// vocabulary is authored by people who never see Tessera, so «Сложность::высокая»
// arrives beside the «сложность::высокая» somebody made by hand. EnsureTag's ON
// CONFLICT rides the byte-exact index, so it inserted the second row — and then
// pushed every synced issue into the lane nobody filters by, while the manual cards
// stayed in the other one (#2932).
func TestGitlabSyncReusesExistingTagSpelling(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-tagcase-user", "grp-tagcase")

	// The project's own spelling, in place before the first pull.
	mine := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "сложность::высокая", "color": "#111111"}), http.StatusCreated)

	f.addIssue(glIssue{
		IID: 1, Title: "Синхронизированная задача",
		Labels: []glLabel{
			{Title: "Сложность::высокая", Color: "#d0021b"},
			{Title: "T: свежий", Color: "#7c5cff"},
		},
	})
	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	tags := c.get("/projects/" + s.Project + "/tags").listBody(t)
	names := tagNames(tags)
	// The label reuses the row that was already there — no second «Сложность::высокая».
	if !names["сложность::высокая"] || names["Сложность::высокая"] {
		t.Fatalf("sync spelled its own twin: %v", names)
	}
	// Control: a label the project really has nothing for still becomes a tag.
	if !names["T: свежий"] {
		t.Fatalf("sync did not create the genuinely new tag: %v", names)
	}
	if len(names) != 2 {
		t.Fatalf("project tags = %d, want 2: %v", len(names), names)
	}

	// The reused row keeps the name the project gave it and takes GitLab's colour —
	// otherwise every pull would rename the tag under the reader.
	var reused map[string]any
	for _, tg := range tags {
		if tg["id"] == mine["id"] {
			reused = tg
		}
	}
	if reused == nil {
		t.Fatalf("the hand-made tag is gone: %v", tags)
	}
	if reused["color"] != "#d0021b" {
		t.Fatalf("reused tag colour = %v, want GitLab's #d0021b", reused["color"])
	}

	// And the synced card really carries it, so the board's «сложность» lane holds
	// the GitLab issue and the manual cards together.
	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if len(tasks) != 1 {
		t.Fatalf("board has %d tasks after sync, want 1", len(tasks))
	}
	if !hasTagID(tasks[0], mine["id"].(string)) {
		t.Fatalf("synced card does not carry the project's own tag: %v", tasks[0])
	}
}

// tagNames indexes a tag list by name.
func tagNames(tags []map[string]any) map[string]bool {
	out := make(map[string]bool, len(tags))
	for _, tg := range tags {
		if n, ok := tg["name"].(string); ok {
			out[n] = true
		}
	}
	return out
}

// hasTagID reports whether a board row carries the tag.
func hasTagID(task map[string]any, tagID string) bool {
	ids, _ := task["tag_ids"].([]any)
	for _, id := range ids {
		if id == tagID {
			return true
		}
	}
	return false
}
