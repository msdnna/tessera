// «Входящие» (#3040, migration 0088): tags and @-mentions of a record. Tags are
// names — the project's spelling when the record is tied to a task or project,
// free labels otherwise; mentions tell people only through a note shared with
// the workspace, never through a private note or a reminder.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func strList(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

func sameStrings(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestRecordTagsAreStoredAndFiltered(t *testing.T) {
	t.Parallel()
	u := signup(t)
	s := mkStack(t, u)
	u.expect(t, u.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "Frontend", "color": "#7c5cff"}), http.StatusCreated)
	task := mkTask(t, u, s.Board, s.col(t, 0), "Релиз 0.150")
	taskID := task["id"].(string)

	// Without a project: free labels — trimmed, «#» dropped, case repeats folded.
	free := u.expect(t, u.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": "Идеи", "visibility": "private", "tags": []string{"#релиз", "Релиз", " идеи ", ""},
	}), http.StatusCreated)
	if got := strList(free["tags"]); !sameStrings(got, "релиз", "идеи") {
		t.Fatalf("free labels: %v", got)
	}
	// Tied to a task: the project's tag is spelled the project's way; a name the
	// project lacks stays a free label (the record does not create project tags).
	onTask := u.expect(t, u.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": "Повестка", "task_id": taskID, "tags": []string{"frontend", "релиз"},
	}), http.StatusCreated)
	if got := strList(onTask["tags"]); !sameStrings(got, "Frontend", "релиз") {
		t.Fatalf("tags of a note on a task: %v", got)
	}
	for _, tg := range u.get("/projects/" + s.Project + "/tags").listBody(t) {
		if strings.EqualFold(tg["name"].(string), "релиз") {
			t.Fatal("a record's label became a project tag")
		}
	}
	soon := time.Now().Add(time.Hour).Format(time.RFC3339)
	rem := u.expect(t, u.post("/reminders", map[string]any{
		"remind_at": soon, "message": "Созвон", "task_id": taskID, "tags": []string{"FRONTEND"},
	}), http.StatusCreated)
	if got := strList(rem["tags"]); !sameStrings(got, "Frontend") {
		t.Fatalf("reminder tags: %v", got)
	}
	plain := u.expect(t, u.post("/reminders", map[string]any{
		"remind_at": soon, "message": "Без тегов", "workspace_id": s.WS,
	}), http.StatusCreated)
	if tags, ok := plain["tags"].([]any); !ok || len(tags) != 0 {
		t.Fatalf("a reminder without tags must carry [] (not null): %v", plain["tags"])
	}

	// An edit without tags keeps them (Android sends title/body/done only);
	// with tags replaces them.
	rid := rem["id"].(string)
	kept := u.expect(t, u.patch("/reminders/"+rid, map[string]any{"message": "Созвон по релизу"}), http.StatusOK)
	if got := strList(kept["tags"]); !sameStrings(got, "Frontend") {
		t.Fatalf("absent tags dropped them: %v", got)
	}
	nid := free["id"].(string)
	keptN := u.expect(t, u.patch("/notes/"+nid, map[string]any{"title": "Идеи", "body": "x"}), http.StatusOK)
	if got := strList(keptN["tags"]); !sameStrings(got, "релиз", "идеи") {
		t.Fatalf("absent note tags dropped them: %v", got)
	}
	// Tying the note to the task re-reads its labels against the project.
	relinked := u.expect(t, u.patch("/notes/"+nid, map[string]any{
		"title": "Идеи", "task_id": taskID, "tags": []string{"идеи", "frontend"},
	}), http.StatusOK)
	if got := strList(relinked["tags"]); !sameStrings(got, "идеи", "Frontend") {
		t.Fatalf("relinked note tags: %v", got)
	}
	// Snoozing keeps the tags.
	snoozed := u.expect(t, u.post("/reminders/"+rid+"/snooze", map[string]any{
		"remind_at": time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	}), http.StatusOK)
	if got := strList(snoozed["tags"]); !sameStrings(got, "Frontend") {
		t.Fatalf("snooze dropped the tags: %v", got)
	}

	if r := u.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": "x", "tags": []string{strings.Repeat("я", 65)},
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("an overlong tag: %d, want 400", r.Status)
	}

	// The feed carries the tags and filters by one.
	ids := func(feed map[string]any, key string) map[string]bool {
		out := map[string]bool{}
		list, _ := feed[key].([]any)
		for _, x := range list {
			out[x.(map[string]any)["id"].(string)] = true
		}
		return out
	}
	all := u.expect(t, u.get("/workspaces/"+s.WS+"/inbox"), http.StatusOK)
	if n := len(ids(all, "notes")) + len(ids(all, "reminders")); n != 4 {
		t.Fatalf("unfiltered feed: %d records, want 4", n)
	}
	byTag := u.expect(t, u.get("/workspaces/"+s.WS+"/inbox?tag=%23FRONTEND"), http.StatusOK)
	notes, rems := ids(byTag, "notes"), ids(byTag, "reminders")
	if len(notes) != 2 || !notes[nid] || !notes[onTask["id"].(string)] || len(rems) != 1 || !rems[rid] {
		t.Fatalf("feed filtered by #frontend: notes %v, reminders %v", notes, rems)
	}
}

func TestRecordMentionsNotifyOnlyThroughSharedNotes(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	mate := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": mate.Email, "role": "member"}), http.StatusCreated)
	outsider := signup(t)

	conn, st := dialWS(t, mate.token, false)
	if st != http.StatusSwitchingProtocols || conn == nil {
		t.Fatalf("mate socket handshake: %d", st)
	}
	defer conn.Close()

	const secret = "подарок на день рождения"
	private := owner.expect(t, owner.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": secret, "visibility": "private",
		"mentions": []string{mate.UserID, outsider.UserID, mate.UserID},
	}), http.StatusCreated)
	if got := strList(private["mentions"]); !sameStrings(got, mate.UserID) {
		t.Fatalf("stored mentions (outsider and repeat must drop): %v", got)
	}
	soon := time.Now().Add(time.Hour).Format(time.RFC3339)
	rem := owner.expect(t, owner.post("/reminders", map[string]any{
		"remind_at": soon, "message": "Спросить про иллюстрации", "workspace_id": s.WS,
		"mentions": []string{mate.UserID},
	}), http.StatusCreated)
	if got := strList(rem["mentions"]); !sameStrings(got, mate.UserID) {
		t.Fatalf("reminder mentions: %v", got)
	}
	if n := mentionCount(t, mate); n != 0 {
		t.Fatalf("a private note / reminder notified the mentioned: %d", n)
	}

	shared := owner.expect(t, owner.post("/workspaces/"+s.WS+"/notes", map[string]any{
		"title": "Повестка ретро", "body": "спросить @" + mate.Email + " про релиз",
		"mentions": []string{mate.UserID},
	}), http.StatusCreated)
	if n := mentionCount(t, mate); n != 1 {
		t.Fatalf("a shared note's mention: %d notifications, want 1", n)
	}
	var row map[string]any
	for _, n := range mate.get("/notifications").listBody(t) {
		if n["kind"] == "mention" {
			row = n
		}
	}
	p, _ := row["payload"].(map[string]any)
	if p["event"] != "note_mention" || p["note_id"] != shared["id"] || p["title"] != "Повестка ретро" {
		t.Fatalf("mention notification payload: %v", row)
	}
	// An edit that names the same person again stays quiet.
	owner.expect(t, owner.patch("/notes/"+shared["id"].(string), map[string]any{
		"title": "Повестка ретро", "body": "спросить @" + mate.Email + " про релиз 0.150",
		"mentions": []string{mate.UserID},
	}), http.StatusOK)
	if n := mentionCount(t, mate); n != 1 {
		t.Fatalf("a repeated mention notified again: %d", n)
	}
	// The private note goes shared: now the person it names can read it, and is told.
	owner.expect(t, owner.patch("/notes/"+private["id"].(string), map[string]any{
		"title": secret, "visibility": "workspace",
	}), http.StatusOK)
	if n := mentionCount(t, mate); n != 2 {
		t.Fatalf("a private note going shared: %d mention notifications, want 2", n)
	}

	// The socket: the private note (until it went shared) and the reminder never
	// reached the mate; the shared note did.
	sawShared := false
	deadline := time.Now().Add(3 * time.Second)
	for {
		ev := readEvent(t, conn, time.Until(deadline))
		if ev == nil {
			break
		}
		raw, _ := json.Marshal(ev)
		if strings.Contains(string(raw), rem["id"].(string)) {
			t.Fatalf("the mate's socket received the owner's reminder: %s", raw)
		}
		if ev["type"] == "note.created" && strings.Contains(string(raw), secret) {
			t.Fatalf("the mate's socket received the private note: %s", raw)
		}
		if ev["type"] == "note.created" && strings.Contains(string(raw), shared["id"].(string)) {
			sawShared = true
		}
		if ev["type"] == "note.updated" && strings.Contains(string(raw), private["id"].(string)) {
			break // the private note went shared — the last event of interest
		}
	}
	if !sawShared {
		t.Fatal("the shared note never reached the mate's socket")
	}
}
