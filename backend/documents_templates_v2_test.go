package main

import (
	"net/http"
	"sort"
	"strings"
	"testing"
)

// v2 documents and templates (#3032): a document's stage (#3044), template
// categories (#3045), a document as a meeting's agenda (#3046) and board
// templates for an empty project (#3047).

func mkMilestone(t *testing.T, c *client, projectID, title string) string {
	t.Helper()
	m := c.expect(t, c.post("/projects/"+projectID+"/milestones",
		map[string]any{"title": title}), http.StatusCreated)
	return m["id"].(string)
}

func mkDocConference(t *testing.T, c *client, wsID, title string) string {
	t.Helper()
	m := c.expect(t, c.post("/workspaces/"+wsID+"/conferences",
		map[string]any{"title": title}), http.StatusCreated)
	return m["id"].(string)
}

func TestDocumentStagePinAndUnpin(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	ms := mkMilestone(t, c, s.Project, "Спринт 42")
	doc := c.expect(t, c.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Спецификация доски"}), http.StatusCreated)
	id := doc["id"].(string)
	if doc["milestone_id"] != nil {
		t.Fatalf("a new document has a stage: %v", doc["milestone_id"])
	}

	got := c.expect(t, c.put("/documents/"+id+"/milestone",
		map[string]any{"milestone_id": ms}), http.StatusOK)
	if got["milestone_id"] != ms {
		t.Fatalf("milestone_id = %v, want %s", got["milestone_id"], ms)
	}
	// The pin is not a content edit: the save token must not move, or the next
	// autosave of whoever is typing would be answered 409.
	if got["updated_at"] != doc["updated_at"] {
		t.Fatalf("pinning a stage moved updated_at: %v → %v", doc["updated_at"], got["updated_at"])
	}
	if read := c.expect(t, c.get("/documents/"+id), http.StatusOK); read["milestone_id"] != ms {
		t.Fatalf("GET lost the stage: %v", read["milestone_id"])
	}

	list := c.get("/milestones/" + ms + "/documents").listBody(t)
	if len(list) != 1 || list[0]["id"] != id {
		t.Fatalf("stage documents = %v", list)
	}

	c.expect(t, c.put("/documents/"+id+"/milestone",
		map[string]any{"milestone_id": nil}), http.StatusOK)
	if read := c.expect(t, c.get("/documents/"+id), http.StatusOK); read["milestone_id"] != nil {
		t.Fatalf("unpin kept the stage: %v", read["milestone_id"])
	}
	if list := c.get("/milestones/" + ms + "/documents").listBody(t); len(list) != 0 {
		t.Fatalf("an unpinned document is still listed: %v", list)
	}

	// Deleting the stage leaves the document, unpinned.
	c.expect(t, c.put("/documents/"+id+"/milestone",
		map[string]any{"milestone_id": ms}), http.StatusOK)
	c.expect(t, c.del("/milestones/"+ms), http.StatusNoContent)
	if read := c.expect(t, c.get("/documents/"+id), http.StatusOK); read["milestone_id"] != nil {
		t.Fatalf("a deleted stage is still pinned: %v", read["milestone_id"])
	}
}

// A stage or a meeting of another workspace is refused at the door — a pin
// across the boundary would show one team the other's titles.
func TestDocumentPinsStayInWorkspace(t *testing.T) {
	t.Parallel()
	c := signup(t)
	a := mkStack(t, c)
	b := mkStack(t, c)
	foreignMs := mkMilestone(t, c, b.Project, "Чужой этап")
	foreignConf := mkDocConference(t, c, b.WS, "Чужая встреча")
	doc := c.expect(t, c.post("/workspaces/"+a.WS+"/documents",
		map[string]any{"title": "Док"}), http.StatusCreated)
	id := doc["id"].(string)

	if r := c.put("/documents/"+id+"/milestone", map[string]any{"milestone_id": foreignMs}); r.Status != http.StatusBadRequest {
		t.Fatalf("foreign stage: status %d, want 400\n%s", r.Status, r.Body)
	}
	if r := c.put("/documents/"+id+"/conference", map[string]any{"conference_id": foreignConf}); r.Status != http.StatusBadRequest {
		t.Fatalf("foreign meeting: status %d, want 400\n%s", r.Status, r.Body)
	}
	if r := c.post("/workspaces/"+a.WS+"/documents", map[string]any{
		"title": "Повестка", "conference_id": foreignConf,
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("create with a foreign meeting: status %d, want 400\n%s", r.Status, r.Body)
	}

	// A stranger reads neither the stage's documents nor the meetings' ones.
	stranger := signup(t)
	ms := mkMilestone(t, c, a.Project, "Этап")
	if r := stranger.get("/milestones/" + ms + "/documents"); r.Status != http.StatusForbidden && r.Status != http.StatusNotFound {
		t.Fatalf("stranger reads stage documents: %d", r.Status)
	}
	if r := stranger.get("/workspaces/" + a.WS + "/conference-documents"); r.Status != http.StatusForbidden && r.Status != http.StatusNotFound {
		t.Fatalf("stranger reads meeting documents: %d", r.Status)
	}
}

func TestDocumentAgendaOfMeeting(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	conf := mkDocConference(t, c, s.WS, "Планирование Спринта 43")
	other := mkDocConference(t, c, s.WS, "Стендап")

	// Born pinned — «Повестка встречи» from the gallery.
	agenda := c.expect(t, c.post("/workspaces/"+s.WS+"/documents", map[string]any{
		"title": "Повестка: планирование", "conference_id": conf,
	}), http.StatusCreated)
	if agenda["conference_id"] != conf {
		t.Fatalf("conference_id = %v, want %s", agenda["conference_id"], conf)
	}
	plain := c.expect(t, c.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Протокол стендапа"}), http.StatusCreated)
	c.expect(t, c.put("/documents/"+plain["id"].(string)+"/conference",
		map[string]any{"conference_id": other}), http.StatusOK)

	list := c.get("/workspaces/" + s.WS + "/conference-documents").listBody(t)
	byConf := map[string]string{}
	for _, row := range list {
		byConf[row["conference_id"].(string)] = row["title"].(string)
	}
	if byConf[conf] != "Повестка: планирование" || byConf[other] != "Протокол стендапа" || len(list) != 2 {
		t.Fatalf("meeting documents = %v", list)
	}

	// Deleting the meeting keeps the protocol, unpinned.
	c.expect(t, c.del("/conferences/"+other), http.StatusNoContent)
	if read := c.expect(t, c.get("/documents/"+plain["id"].(string)), http.StatusOK); read["conference_id"] != nil {
		t.Fatalf("a deleted meeting is still pinned: %v", read["conference_id"])
	}
}

// A project transfer unpins a document's stage and meeting that the move left
// on different sides of the boundary.
func TestTransferProjectUnpinsDocumentStageAndMeeting(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся "+t.Name())

	movingMs := mkMilestone(t, owner, src.Project, "Этап переезжающего")
	stayMs := mkMilestone(t, owner, stayProject, "Этап остающегося")
	conf := mkDocConference(t, owner, src.WS, "Встреча")

	// A document of the moving project pinned to a stage that stays, and to a
	// meeting (meetings never travel).
	movingDoc := owner.expect(t, owner.post("/workspaces/"+src.WS+"/documents", map[string]any{
		"title": "Документ проекта", "project_id": src.Project,
		"milestone_id": stayMs, "conference_id": conf,
	}), http.StatusCreated)
	// A workspace document pinned to a stage that leaves with the project.
	stayDoc := owner.expect(t, owner.post("/workspaces/"+src.WS+"/documents", map[string]any{
		"title": "Документ пространства", "milestone_id": movingMs,
	}), http.StatusCreated)
	// The control: a moving document pinned to its own moving stage.
	keptDoc := owner.expect(t, owner.post("/workspaces/"+src.WS+"/documents", map[string]any{
		"title": "Свой этап", "project_id": src.Project, "milestone_id": movingMs,
	}), http.StatusCreated)

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)
	if n, _ := res["unpinned_document_stages"].(float64); n != 2 {
		t.Fatalf("unpinned_document_stages = %v, want 2", res["unpinned_document_stages"])
	}
	if n, _ := res["unpinned_document_meetings"].(float64); n != 1 {
		t.Fatalf("unpinned_document_meetings = %v, want 1", res["unpinned_document_meetings"])
	}
	for _, d := range []map[string]any{movingDoc, stayDoc} {
		got := owner.expect(t, owner.get("/documents/"+d["id"].(string)), http.StatusOK)
		if got["milestone_id"] != nil {
			t.Fatalf("%v kept a stage across the boundary", got["title"])
		}
	}
	if got := owner.expect(t, owner.get("/documents/"+movingDoc["id"].(string)), http.StatusOK); got["conference_id"] != nil {
		t.Fatal("a moved document kept its meeting in the old workspace")
	}
	if got := owner.expect(t, owner.get("/documents/"+keptDoc["id"].(string)), http.StatusOK); got["milestone_id"] != movingMs {
		t.Fatalf("a pin that travelled whole was cut: %v", got["milestone_id"])
	}
}

func TestDocumentTemplateCategories(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	docID := mkTemplateDoc(t, c, ws, "Ретро", "Что было хорошо")

	tpl := c.expect(t, c.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"document_id": docID, "category": "sprint"}), http.StatusCreated)
	if tpl["category"] != "sprint" {
		t.Fatalf("category = %v", tpl["category"])
	}
	plain := c.expect(t, c.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"document_id": docID, "title": "Без категории"}), http.StatusCreated)
	if plain["category"] != "" {
		t.Fatalf("default category = %v, want empty", plain["category"])
	}
	if r := c.post("/workspaces/"+ws+"/document-templates",
		map[string]any{"document_id": docID, "category": "mine"}); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown category: status %d, want 400", r.Status)
	}

	// Re-filed and cleared through PATCH; other fields stay.
	upd := c.expect(t, c.patch("/document-templates/"+tpl["id"].(string),
		map[string]any{"category": "meetings"}), http.StatusOK)
	if upd["category"] != "meetings" || upd["title"] != tpl["title"] {
		t.Fatalf("after re-file: %v / %v", upd["category"], upd["title"])
	}
	if r := c.patch("/document-templates/"+tpl["id"].(string),
		map[string]any{"category": "nope"}); r.Status != http.StatusBadRequest {
		t.Fatalf("PATCH unknown category: status %d", r.Status)
	}
	// A title-only PATCH does not clear the category.
	upd = c.expect(t, c.patch("/document-templates/"+tpl["id"].(string),
		map[string]any{"title": "Ретро спринта"}), http.StatusOK)
	if upd["category"] != "meetings" {
		t.Fatalf("title edit cleared the category: %v", upd["category"])
	}

	list := c.get("/workspaces/" + ws + "/document-templates").listBody(t)
	cats := map[string]string{}
	for _, row := range list {
		cats[row["title"].(string)] = row["category"].(string)
	}
	if cats["Ретро спринта"] != "meetings" || cats["Без категории"] != "" {
		t.Fatalf("gallery categories = %v", cats)
	}
}

func columnKeys(t *testing.T, c *client, boardID string) []string {
	t.Helper()
	var keys []string
	for _, col := range c.get("/boards/" + boardID + "/columns").listBody(t) {
		k, _ := col["name_key"].(string)
		keys = append(keys, k)
	}
	return keys
}

func TestBoardTemplates(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	g := mkGroup(t, c, ws, "Группа")
	p := mkProject(t, c, ws, g, "Пустой проект")

	for _, tc := range []struct {
		template, layout string
		keys             []string
		tagPrefix        string
	}{
		{"sprint", "board", []string{"backlog", "todo", "in_progress", "review", "done"}, "тип::"},
		{"bugs", "list", []string{"new", "confirmed", "in_progress", "verify", "closed"}, "серьёзность::"},
		{"roadmap", "timeline", []string{"ideas", "planned", "in_progress", "shipped"}, "квартал::"},
	} {
		t.Run(tc.template, func(t *testing.T) {
			b := c.expect(t, c.post("/projects/"+p+"/boards", map[string]any{
				"name": "Доска " + tc.template, "template": tc.template,
			}), http.StatusCreated)
			if b["default_layout"] != tc.layout {
				t.Fatalf("layout = %v, want %s", b["default_layout"], tc.layout)
			}
			keys := columnKeys(t, c, b["id"].(string))
			if strings.Join(keys, ",") != strings.Join(tc.keys, ",") {
				t.Fatalf("columns = %v, want %v", keys, tc.keys)
			}
			// The rightmost column completes tasks.
			cols := c.get("/boards/" + b["id"].(string) + "/columns").listBody(t)
			if b["done_column_id"] != cols[len(cols)-1]["id"] {
				t.Fatalf("done column = %v, want the last one", b["done_column_id"])
			}
			var tagged []string
			for _, tag := range c.get("/projects/" + p + "/tags").listBody(t) {
				if name := tag["name"].(string); strings.HasPrefix(name, tc.tagPrefix) {
					tagged = append(tagged, name)
				}
			}
			if len(tagged) == 0 {
				t.Fatalf("no %q tags seeded", tc.tagPrefix)
			}
		})
	}

	// A second board from the same template does not duplicate the tags.
	before := len(c.get("/projects/" + p + "/tags").listBody(t))
	c.expect(t, c.post("/projects/"+p+"/boards", map[string]any{
		"name": "Ещё спринт", "template": "sprint",
	}), http.StatusCreated)
	if after := len(c.get("/projects/" + p + "/tags").listBody(t)); after != before {
		t.Fatalf("tags %d → %d on a repeat template", before, after)
	}

	// An explicit layout wins over the template's.
	b := c.expect(t, c.post("/projects/"+p+"/boards", map[string]any{
		"name": "Роадмап списком", "template": "roadmap", "default_layout": "list",
	}), http.StatusCreated)
	if b["default_layout"] != "list" {
		t.Fatalf("layout = %v, want list", b["default_layout"])
	}

	if r := c.post("/projects/"+p+"/boards", map[string]any{"name": "X", "template": "scrum"}); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown template: status %d, want 400", r.Status)
	}

	// No template — the four default columns, as before.
	plain := mkBoard(t, c, p, "Обычная")
	if keys := columnKeys(t, c, plain); strings.Join(keys, ",") != "todo,in_progress,review,done" {
		t.Fatalf("plain board columns = %v", keys)
	}
}

// Tags are user data written in the creator's language; columns are keyed and
// captioned by each client.
func TestBoardTemplateTagsFollowCreatorLanguage(t *testing.T) {
	t.Parallel()
	c := signup(t)
	setLang(t, c, "en")
	ws := mkWorkspace(t, c, "WS "+t.Name())
	g := mkGroup(t, c, ws, "Group")
	p := mkProject(t, c, ws, g, "Bugs")
	c.expect(t, c.post("/projects/"+p+"/boards", map[string]any{
		"name": "Bugs", "template": "bugs",
	}), http.StatusCreated)
	var names []string
	for _, tag := range c.get("/projects/" + p + "/tags").listBody(t) {
		names = append(names, tag["name"].(string))
	}
	sort.Strings(names)
	want := "severity::critical,severity::high,severity::low,severity::medium"
	if strings.Join(names, ",") != want {
		t.Fatalf("tags = %v, want %s", names, want)
	}
}
