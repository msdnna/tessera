package main

import (
	"net/http"
	"testing"
)

// Board views v2 (#2970): personal + shared lists, the star, the order and the
// rename (#2988, #3015, #3016), and the layout a new board opens in (#3017).

func viewNames(list []map[string]any) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, v["name"].(string))
	}
	return out
}

func viewByName(t *testing.T, list []map[string]any, name string) map[string]any {
	t.Helper()
	for _, v := range list {
		if v["name"] == name {
			return v
		}
	}
	t.Fatalf("view %q not in %v", name, viewNames(list))
	return nil
}

// TestSharedBoardViews: the owner (and an admin) creates shared views, every
// member reads them, a plain member can neither create nor edit them, and a
// member's personal views stay private.
func TestSharedBoardViews(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)

	shared := owner.expect(t, owner.post("/boards/"+s.Board+"/views", map[string]any{
		"name": "Спринт — команда", "scope": "board", "config": map[string]any{"layout": "board"},
	}), http.StatusOK)
	if shared["scope"] != "board" || shared["editable"] != true {
		t.Fatalf("shared view: %v", shared)
	}
	sharedID := shared["id"].(string)
	owner.expect(t, owner.post("/boards/"+s.Board+"/views", map[string]any{"name": "Моё владельца"}), http.StatusOK)

	// A plain member: may not create a shared view.
	if r := member.post("/boards/"+s.Board+"/views", map[string]any{"name": "x", "scope": "board"}); r.Status != http.StatusForbidden {
		t.Fatalf("member created a shared view: %d %s", r.Status, r.Body)
	}
	mine := member.expect(t, member.post("/boards/"+s.Board+"/views", map[string]any{"name": "Только баги"}), http.StatusOK)

	// Their list: own first, then shared (locked); never the owner's personal one.
	list := member.get("/boards/" + s.Board + "/views").listBody(t)
	if got := viewNames(list); len(got) != 2 || got[0] != "Только баги" || got[1] != "Спринт — команда" {
		t.Fatalf("member list = %v", got)
	}
	sv := viewByName(t, list, "Спринт — команда")
	if sv["editable"] != false || sv["creator_name"] == "" {
		t.Fatalf("shared view for a member: %v", sv)
	}
	// …and the lock holds on the server too.
	if r := member.patch("/views/"+sharedID, map[string]any{"name": "Взлом"}); r.Status != http.StatusForbidden {
		t.Fatalf("member renamed a shared view: %d", r.Status)
	}
	if r := member.del("/views/" + sharedID); r.Status != http.StatusForbidden {
		t.Fatalf("member deleted a shared view: %d", r.Status)
	}
	// A member may not share their own view either (the shared list is the managers').
	if r := member.patch("/views/"+mine["id"].(string), map[string]any{"scope": "board"}); r.Status != http.StatusForbidden {
		t.Fatalf("member shared a view: %d", r.Status)
	}
	// Nor touch somebody else's personal one.
	ownerList := owner.get("/boards/" + s.Board + "/views").listBody(t)
	ownersOwn := viewByName(t, ownerList, "Моё владельца")
	if r := member.patch("/views/"+ownersOwn["id"].(string), map[string]any{"name": "чужое"}); r.Status != http.StatusForbidden {
		t.Fatalf("member renamed another's personal view: %d", r.Status)
	}

	// Promote a member to admin: now they edit shared views.
	owner.expect(t, owner.patch("/workspaces/"+s.WS+"/members/"+member.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)
	member.expect(t, member.patch("/views/"+sharedID, map[string]any{"name": "Спринт"}), http.StatusOK)

	// Sharing a personal view moves it to the shared list; taking it back makes
	// it the caller's own.
	got := owner.expect(t, owner.patch("/views/"+ownersOwn["id"].(string), map[string]any{"scope": "board"}), http.StatusOK)
	if got["scope"] != "board" {
		t.Fatalf("share: %v", got)
	}
	memberList := member.get("/boards/" + s.Board + "/views").listBody(t)
	viewByName(t, memberList, "Моё владельца")
	got = member.expect(t, member.patch("/views/"+ownersOwn["id"].(string), map[string]any{"scope": "user"}), http.StatusOK)
	if got["scope"] != "user" {
		t.Fatalf("unshare: %v", got)
	}
	ownerList = owner.get("/boards/" + s.Board + "/views").listBody(t)
	for _, v := range ownerList {
		if v["name"] == "Моё владельца" {
			t.Fatalf("view taken personal by the admin still listed for the owner: %v", v)
		}
	}

	// Deleting a shared view: a manager may.
	if r := member.del("/views/" + sharedID); r.Status != http.StatusNoContent {
		t.Fatalf("admin delete shared: %d", r.Status)
	}
}

// TestBoardViewDefaultOrderRename covers the star (one per list), the move
// between neighbours and the rename with its name clash.
func TestBoardViewDefaultOrderRename(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	ids := map[string]string{}
	for _, n := range []string{"А", "Б", "В"} {
		v := c.expect(t, c.post("/boards/"+s.Board+"/views", map[string]any{"name": n}), http.StatusOK)
		ids[n] = v["id"].(string)
	}
	// Created in order → listed in order (not alphabetically by accident: Б is
	// renamed to Я below and must keep its place).
	c.expect(t, c.patch("/views/"+ids["Б"], map[string]any{"name": "Я"}), http.StatusOK)
	if got := viewNames(c.get("/boards/" + s.Board + "/views").listBody(t)); got[0] != "А" || got[1] != "Я" || got[2] != "В" {
		t.Fatalf("order after rename = %v", got)
	}
	// Rename into a taken name → 409.
	if r := c.patch("/views/"+ids["В"], map[string]any{"name": "А"}); r.Status != http.StatusConflict {
		t.Fatalf("rename clash: %d %s", r.Status, r.Body)
	}
	// Move В to the top (no previous neighbour, next = А).
	c.expect(t, c.patch("/views/"+ids["В"], map[string]any{"after_id": ids["А"]}), http.StatusOK)
	if got := viewNames(c.get("/boards/" + s.Board + "/views").listBody(t)); got[0] != "В" || got[1] != "А" || got[2] != "Я" {
		t.Fatalf("order after move = %v", got)
	}
	// Move А between Я and the end.
	c.expect(t, c.patch("/views/"+ids["А"], map[string]any{"before_id": ids["Б"]}), http.StatusOK)
	if got := viewNames(c.get("/boards/" + s.Board + "/views").listBody(t)); got[2] != "А" {
		t.Fatalf("order after move to end = %v", got)
	}

	// The star: one per list.
	c.expect(t, c.patch("/views/"+ids["А"], map[string]any{"is_default": true}), http.StatusOK)
	c.expect(t, c.patch("/views/"+ids["В"], map[string]any{"is_default": true}), http.StatusOK)
	stars := 0
	for _, v := range c.get("/boards/" + s.Board + "/views").listBody(t) {
		if v["is_default"] == true {
			stars++
			if v["name"] != "В" {
				t.Fatalf("star on %v, want В", v["name"])
			}
		}
	}
	if stars != 1 {
		t.Fatalf("%d stars, want 1", stars)
	}
	// A shared star lives in its own list next to the personal one.
	c.expect(t, c.post("/boards/"+s.Board+"/views", map[string]any{
		"name": "Общий", "scope": "board", "is_default": true,
	}), http.StatusOK)
	stars = 0
	for _, v := range c.get("/boards/" + s.Board + "/views").listBody(t) {
		if v["is_default"] == true {
			stars++
		}
	}
	if stars != 2 {
		t.Fatalf("%d stars across both lists, want 2", stars)
	}
	// Un-star.
	c.expect(t, c.patch("/views/"+ids["В"], map[string]any{"is_default": false}), http.StatusOK)
	// A neighbour from another list is refused.
	shared := viewByName(t, c.get("/boards/"+s.Board+"/views").listBody(t), "Общий")
	if r := c.patch("/views/"+ids["А"], map[string]any{"after_id": shared["id"]}); r.Status != http.StatusBadRequest {
		t.Fatalf("cross-list move: %d", r.Status)
	}
}

// TestBoardDefaultLayout: a board is created in a chosen view (#3017), kanban
// when none is named; an unknown one is refused; an update without the field
// keeps it.
func TestBoardDefaultLayout(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	b := c.get("/boards/" + s.Board).mapBody(t)
	if b["default_layout"] != "board" {
		t.Fatalf("default layout = %v, want board", b["default_layout"])
	}
	l := c.expect(t, c.post("/projects/"+s.Project+"/boards", map[string]any{
		"name": "Список", "default_layout": "list",
	}), http.StatusCreated)
	if l["default_layout"] != "list" {
		t.Fatalf("list board: %v", l["default_layout"])
	}
	if r := c.post("/projects/"+s.Project+"/boards", map[string]any{"name": "x", "default_layout": "pie"}); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown layout accepted: %d", r.Status)
	}
	u := c.expect(t, c.patch("/boards/"+l["id"].(string), map[string]any{"name": "Список 2"}), http.StatusOK)
	if u["default_layout"] != "list" {
		t.Fatalf("rename dropped the layout: %v", u["default_layout"])
	}
}
