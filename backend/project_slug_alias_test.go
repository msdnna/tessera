package main

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// Project addresses are unique across the instance and these tests run in
// parallel with everything else, so every address they pick is made unique.
func uniqueSlug(base string) string {
	return base + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// A project's former address keeps leading to it for 30 days (#3012): the
// board resolver and the project resolver both answer with the current address
// so the client can rewrite its URL. Only members of the project's workspace
// get through.
func TestProjectSlugAliasLeadsToNewAddress(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	oldSlug, newSlug := uniqueSlug("old"), uniqueSlug("new")
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": oldSlug}), http.StatusOK)
	bSlug := owner.get("/boards/" + s.Board).mapBody(t)["slug"].(string)

	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": newSlug}), http.StatusOK)

	b := owner.expect(t, owner.get("/board-by-slug?project="+oldSlug+"&board="+bSlug), http.StatusOK)
	if b["id"] != s.Board || b["project_slug"] != newSlug {
		t.Fatalf("old address resolved to board %v / project_slug %v, want %s / %s", b["id"], b["project_slug"], s.Board, newSlug)
	}
	b = owner.expect(t, owner.get("/board-by-slug?project="+newSlug+"&board="+bSlug), http.StatusOK)
	if b["project_slug"] != newSlug {
		t.Fatalf("current address: project_slug = %v, want %s", b["project_slug"], newSlug)
	}
	p := owner.expect(t, owner.get("/project-by-slug?slug="+oldSlug), http.StatusOK)
	if p["id"] != s.Project || p["slug"] != newSlug {
		t.Fatalf("project-by-slug(old) = %v / %v, want %s / %s", p["id"], p["slug"], s.Project, newSlug)
	}
	owner.expect(t, owner.get("/project-by-slug?slug="+newSlug), http.StatusOK)
	owner.expect(t, owner.get("/project-by-slug?slug="+uniqueSlug("nowhere")), http.StatusNotFound)
	owner.expect(t, owner.get("/project-by-slug?slug="), http.StatusNotFound)

	// An outsider learns nothing more through an old address than a live one.
	stranger := signup(t)
	stranger.expect(t, stranger.get("/board-by-slug?project="+oldSlug+"&board="+bSlug), http.StatusForbidden)
	stranger.expect(t, stranger.get("/project-by-slug?slug="+oldSlug), http.StatusForbidden)

	// A chain of renames keeps every former address leading home, and coming
	// back to an old address retires its alias (the live slug owns it again).
	third := uniqueSlug("third")
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": third}), http.StatusOK)
	for _, a := range []string{oldSlug, newSlug} {
		if got := owner.expect(t, owner.get("/project-by-slug?slug="+a), http.StatusOK); got["slug"] != third {
			t.Fatalf("alias %s → slug %v, want %s", a, got["slug"], third)
		}
	}
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": oldSlug}), http.StatusOK)
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM project_slug_aliases WHERE slug = $1`, oldSlug).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("alias on the address the project took back still stored (%d rows)", n)
	}
	if got := owner.expect(t, owner.get("/project-by-slug?slug="+third), http.StatusOK); got["slug"] != oldSlug {
		t.Fatalf("alias %s → slug %v, want %s", third, got["slug"], oldSlug)
	}
}

// After 30 days the old address stops leading anywhere, and the trash pass
// sweeps the row.
func TestProjectSlugAliasExpires(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	oldSlug, newSlug := uniqueSlug("gone"), uniqueSlug("here")
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": oldSlug}), http.StatusOK)
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": newSlug}), http.StatusOK)

	var exp time.Time
	ctx := context.Background()
	if err := testPool.QueryRow(ctx,
		`SELECT expires_at FROM project_slug_aliases WHERE slug = $1`, oldSlug).Scan(&exp); err != nil {
		t.Fatalf("alias row: %v", err)
	}
	if d := time.Until(exp); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("alias expires in %v, want ~30 days", d)
	}
	owner.expect(t, owner.get("/project-by-slug?slug="+oldSlug), http.StatusOK)

	if _, err := testPool.Exec(ctx,
		`UPDATE project_slug_aliases SET expires_at = now() - interval '1 minute' WHERE slug = $1`, oldSlug); err != nil {
		t.Fatal(err)
	}
	owner.expect(t, owner.get("/project-by-slug?slug="+oldSlug), http.StatusNotFound)
	bSlug := owner.get("/boards/" + s.Board).mapBody(t)["slug"].(string)
	owner.expect(t, owner.get("/board-by-slug?project="+oldSlug+"&board="+bSlug), http.StatusNotFound)

	testAPI.PurgeTrash(ctx)
	var n int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM project_slug_aliases WHERE slug = $1`, oldSlug).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expired alias not purged (%d rows)", n)
	}
}

// An alias doesn't hold the address: another project may take it, and from
// then on the address is the new project's.
func TestProjectSlugAliasYieldsToNewHolder(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	oldSlug, newSlug := uniqueSlug("left"), uniqueSlug("moved")
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": oldSlug}), http.StatusOK)
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": newSlug}), http.StatusOK)

	other := signup(t)
	ws2 := mkWorkspace(t, other, "WS taker")
	taker := other.expect(t, other.post("/workspaces/"+ws2+"/projects",
		map[string]any{"name": "Захватчик", "slug": oldSlug}), http.StatusCreated)
	if taker["slug"] != oldSlug {
		t.Fatalf("taker slug = %v, want %s", taker["slug"], oldSlug)
	}
	got := other.expect(t, other.get("/project-by-slug?slug="+oldSlug), http.StatusOK)
	if got["id"] != taker["id"] {
		t.Fatalf("address resolved to %v, want the new holder %v", got["id"], taker["id"])
	}
	// The project that left it no longer reaches it — through the address it
	// now belongs to a workspace the owner isn't in.
	owner.expect(t, owner.get("/project-by-slug?slug="+oldSlug), http.StatusForbidden)

	// Same through a rename onto a freed address.
	left2, moved2 := uniqueSlug("left2"), uniqueSlug("moved2")
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": left2}), http.StatusOK)
	owner.expect(t, owner.patch("/projects/"+s.Project+"/slug", map[string]any{"slug": moved2}), http.StatusOK)
	p2 := mkProject(t, owner, s.WS, s.Group, "Второй")
	owner.expect(t, owner.patch("/projects/"+p2+"/slug", map[string]any{"slug": left2}), http.StatusOK)
	if got := owner.expect(t, owner.get("/project-by-slug?slug="+left2), http.StatusOK); got["id"] != p2 {
		t.Fatalf("renamed-onto address resolved to %v, want %s", got["id"], p2)
	}
}

// A taken address answers 409 with the first free variant and — for members
// of the holder's workspace only — the holder's name and group (#3013).
func TestProjectSlugTakenNamesHolderAndSuggests(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS taken")
	g := mkGroup(t, owner, ws, "Маркетинг")
	taken := uniqueSlug("site")
	holder := owner.expect(t, owner.post("/workspaces/"+ws+"/projects",
		map[string]any{"name": "Сайт", "slug": taken, "group_id": g}), http.StatusCreated)
	// The first free variant is base-2, unless that is taken too.
	owner.expect(t, owner.post("/workspaces/"+ws+"/projects",
		map[string]any{"name": "Сайт 2", "slug": taken + "-2"}), http.StatusCreated)
	p := mkProject(t, owner, ws, g, "Другой")

	for _, r := range []resp{
		owner.post("/workspaces/"+ws+"/projects", map[string]any{"name": "Ещё", "slug": taken}),
		owner.patch("/projects/"+p+"/slug", map[string]any{"slug": taken}),
	} {
		if r.Status != http.StatusConflict {
			t.Fatalf("status %d, want 409\n%s", r.Status, r.Body)
		}
		body := r.mapBody(t)
		if body["error"] != "slug already taken" {
			t.Fatalf("error = %v", body["error"])
		}
		if body["suggestion"] != taken+"-3" {
			t.Fatalf("suggestion = %v, want %s-3", body["suggestion"], taken)
		}
		tb, ok := body["taken_by"].(map[string]any)
		if !ok {
			t.Fatalf("taken_by missing for a member: %s", r.Body)
		}
		if tb["id"] != holder["id"] || tb["name"] != "Сайт" || tb["group_name"] != "Маркетинг" || tb["workspace_id"] != ws {
			t.Fatalf("taken_by = %v", tb)
		}
	}

	// Someone outside the holder's workspace hears only that it's taken.
	outsider := signup(t)
	ws2 := mkWorkspace(t, outsider, "WS outsider")
	r := outsider.post("/workspaces/"+ws2+"/projects", map[string]any{"name": "Мой", "slug": taken})
	if r.Status != http.StatusConflict {
		t.Fatalf("outsider: status %d, want 409", r.Status)
	}
	body := r.mapBody(t)
	if _, leaked := body["taken_by"]; leaked {
		t.Fatalf("holder named to an outsider: %s", r.Body)
	}
	if body["suggestion"] != taken+"-3" {
		t.Fatalf("outsider suggestion = %v", body["suggestion"])
	}
	// The suggestion really is free.
	outsider.expect(t, outsider.post("/workspaces/"+ws2+"/projects",
		map[string]any{"name": "Мой", "slug": body["suggestion"]}), http.StatusCreated)
}

// The delete confirmations name what goes with the project or workspace
// (#3014): live boards and every task on them, subtasks and archived included.
// Members only.
func TestDeleteConfirmationCounts(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)
	top := mkTask(t, owner, s.Board, col, "верхняя")["id"].(string)
	sub := mkTask(t, owner, s.Board, col, "подзадача")["id"].(string)
	owner.expect(t, owner.patch("/tasks/"+sub+"/parent", map[string]any{"parent_id": top}), http.StatusOK)
	arch := mkTask(t, owner, s.Board, col, "в архиве")["id"].(string)
	owner.expect(t, owner.patch("/tasks/"+arch+"/archive", map[string]any{}), http.StatusNoContent)

	b2 := mkBoard(t, owner, s.Project, "Вторая")
	cols2 := owner.get("/boards/" + b2 + "/columns").listBody(t)
	mkTask(t, owner, b2, cols2[0]["id"].(string), "на второй")

	// A board in the trash is already gone for the reader — not counted.
	b3 := mkBoard(t, owner, s.Project, "В корзине")
	cols3 := owner.get("/boards/" + b3 + "/columns").listBody(t)
	mkTask(t, owner, b3, cols3[0]["id"].(string), "в удалённой")
	owner.expect(t, owner.del("/boards/"+b3), http.StatusNoContent)

	pc := owner.expect(t, owner.get("/projects/"+s.Project+"/counts"), http.StatusOK)
	if pc["boards"] != float64(2) || pc["tasks"] != float64(4) {
		t.Fatalf("project counts = %v, want boards 2, tasks 4", pc)
	}

	p2 := mkProject(t, owner, s.WS, s.Group, "Пустой")
	ec := owner.expect(t, owner.get("/projects/"+p2+"/counts"), http.StatusOK)
	if ec["boards"] != float64(0) || ec["tasks"] != float64(0) {
		t.Fatalf("empty project counts = %v", ec)
	}

	wc := owner.expect(t, owner.get("/workspaces/"+s.WS+"/counts"), http.StatusOK)
	if wc["projects"] != float64(2) || wc["boards"] != float64(2) || wc["tasks"] != float64(4) {
		t.Fatalf("workspace counts = %v, want projects 2, boards 2, tasks 4", wc)
	}

	// A plain member may read them; an outsider may not.
	mate := addMember(t, owner, s.WS)
	mate.expect(t, mate.get("/projects/"+s.Project+"/counts"), http.StatusOK)
	mate.expect(t, mate.get("/workspaces/"+s.WS+"/counts"), http.StatusOK)
	stranger := signup(t)
	stranger.expect(t, stranger.get("/projects/"+s.Project+"/counts"), http.StatusForbidden)
	stranger.expect(t, stranger.get("/workspaces/"+s.WS+"/counts"), http.StatusForbidden)
}
