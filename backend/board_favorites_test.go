// Favourite boards (#3115, migration 0089): one person's starred boards in their
// own order — add, remove, reorder, privacy, and the boards that must drop out of
// the list (soft-deleted, or in a workspace the person has left).
package main

import (
	"net/http"
	"testing"
	"time"
)

func favBoardIDs(list []map[string]any) []string {
	out := make([]string, 0, len(list))
	for _, f := range list {
		out = append(out, f["board_id"].(string))
	}
	return out
}

func sameIDs(got, want []string) bool {
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

// TestBoardFavoritesCRUDAndOrder: starring appends, a second star is a no-op,
// the move puts a row between neighbours (and at either end), and the star comes
// off — idempotently.
func TestBoardFavoritesCRUDAndOrder(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	b2 := mkBoard(t, c, s.Project, "Вторая")
	b3 := mkBoard(t, c, s.Project, "Третья")
	favs := "/workspaces/" + s.WS + "/favorites"

	if l := c.get(favs).listBody(t); len(l) != 0 {
		t.Fatalf("a fresh user has favourites: %v", l)
	}
	first := c.expect(t, c.put("/boards/"+s.Board+"/favorite", nil), http.StatusOK)
	if first["board_id"] != s.Board || first["project_id"] != s.Project || first["name"] == "" ||
		first["project_name"] == "" || first["slug"] == "" || first["project_slug"] == "" {
		t.Fatalf("favourite row: %v", first)
	}
	c.expect(t, c.put("/boards/"+b2+"/favorite", nil), http.StatusOK)
	c.expect(t, c.put("/boards/"+b3+"/favorite", nil), http.StatusOK)
	// Starring again keeps the place.
	again := c.expect(t, c.put("/boards/"+s.Board+"/favorite", nil), http.StatusOK)
	if again["position"] != first["position"] {
		t.Fatalf("re-star moved the row: %v → %v", first["position"], again["position"])
	}
	if got := favBoardIDs(c.get(favs).listBody(t)); !sameIDs(got, []string{s.Board, b2, b3}) {
		t.Fatalf("order after adding = %v", got)
	}

	// Third to the top (only a next neighbour).
	c.expect(t, c.patch("/boards/"+b3+"/favorite/move", map[string]any{"after_id": s.Board}), http.StatusOK)
	if got := favBoardIDs(c.get(favs).listBody(t)); !sameIDs(got, []string{b3, s.Board, b2}) {
		t.Fatalf("order after move to top = %v", got)
	}
	// First between the other two.
	c.expect(t, c.patch("/boards/"+s.Board+"/favorite/move", map[string]any{"before_id": b2}), http.StatusOK)
	if got := favBoardIDs(c.get(favs).listBody(t)); !sameIDs(got, []string{b3, b2, s.Board}) {
		t.Fatalf("order after move to end = %v", got)
	}
	c.expect(t, c.patch("/boards/"+s.Board+"/favorite/move",
		map[string]any{"before_id": b3, "after_id": b2}), http.StatusOK)
	if got := favBoardIDs(c.get(favs).listBody(t)); !sameIDs(got, []string{b3, s.Board, b2}) {
		t.Fatalf("order after move between = %v", got)
	}

	// Refusals: same neighbour twice, itself as a neighbour, a neighbour that is
	// not a favourite, a board that is not a favourite.
	if r := c.patch("/boards/"+s.Board+"/favorite/move", map[string]any{"before_id": b2, "after_id": b2}); r.Status != http.StatusBadRequest {
		t.Fatalf("same neighbour: %d", r.Status)
	}
	if r := c.patch("/boards/"+s.Board+"/favorite/move", map[string]any{"before_id": s.Board}); r.Status != http.StatusBadRequest {
		t.Fatalf("self neighbour: %d", r.Status)
	}
	b4 := mkBoard(t, c, s.Project, "Четвёртая")
	if r := c.patch("/boards/"+s.Board+"/favorite/move", map[string]any{"before_id": b4}); r.Status != http.StatusBadRequest {
		t.Fatalf("non-favourite neighbour: %d", r.Status)
	}
	if r := c.patch("/boards/"+b4+"/favorite/move", map[string]any{"before_id": b2}); r.Status != http.StatusNotFound {
		t.Fatalf("move a non-favourite: %d", r.Status)
	}

	// Off, and off again.
	if r := c.del("/boards/" + b2 + "/favorite"); r.Status != http.StatusNoContent {
		t.Fatalf("unstar: %d", r.Status)
	}
	if r := c.del("/boards/" + b2 + "/favorite"); r.Status != http.StatusNoContent {
		t.Fatalf("unstar twice: %d", r.Status)
	}
	if got := favBoardIDs(c.get(favs).listBody(t)); !sameIDs(got, []string{b3, s.Board}) {
		t.Fatalf("after unstar = %v", got)
	}
}

// TestBoardFavoritesArePrivate: a colleague neither sees somebody's favourites
// nor gets their socket event, and an outsider cannot star a board at all.
func TestBoardFavoritesArePrivate(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	colleague := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": colleague.Email, "role": "member"}), http.StatusCreated)
	outsider := signup(t)

	conn, st := dialWS(t, colleague.token, false)
	if st != http.StatusSwitchingProtocols || conn == nil {
		t.Fatalf("colleague socket: %d", st)
	}
	defer conn.Close()
	mine, st := dialWS(t, owner.token, false)
	if st != http.StatusSwitchingProtocols || mine == nil {
		t.Fatalf("owner socket: %d", st)
	}
	defer mine.Close()

	owner.expect(t, owner.put("/boards/"+s.Board+"/favorite", nil), http.StatusOK)
	// A workspace event after it: the colleague must get this one and nothing
	// of the favourite before it.
	owner.expect(t, owner.patch("/boards/"+s.Board, map[string]any{"name": "Переименована"}), http.StatusOK)

	// The owner's own socket carries the change (other tabs follow).
	sawOwn := false
	for i := 0; i < 10 && !sawOwn; i++ {
		ev := readEvent(t, mine, 3*time.Second)
		if ev == nil {
			break
		}
		sawOwn = ev["type"] == "favorites.changed"
	}
	if !sawOwn {
		t.Fatal("the owner's socket never got favorites.changed")
	}
	for i := 0; i < 10; i++ {
		ev := readEvent(t, conn, 3*time.Second)
		if ev == nil {
			t.Fatal("the colleague's socket never got the board rename")
		}
		if ev["type"] == "favorites.changed" {
			t.Fatalf("a colleague got somebody's favourite: %v", ev)
		}
		if ev["type"] == "board.updated" {
			break
		}
	}

	if l := colleague.get("/workspaces/" + s.WS + "/favorites").listBody(t); len(l) != 0 {
		t.Fatalf("a colleague sees somebody's favourites: %v", l)
	}
	if l := owner.get("/workspaces/" + s.WS + "/favorites").listBody(t); len(l) != 1 || l[0]["name"] != "Переименована" {
		t.Fatalf("owner's list after the rename: %v", l)
	}
	// The colleague's own star does not touch the owner's list, and their move
	// cannot name the owner's favourite as a neighbour.
	b2 := mkBoard(t, owner, s.Project, "Вторая")
	colleague.expect(t, colleague.put("/boards/"+b2+"/favorite", nil), http.StatusOK)
	if got := favBoardIDs(owner.get("/workspaces/" + s.WS + "/favorites").listBody(t)); !sameIDs(got, []string{s.Board}) {
		t.Fatalf("owner's list after the colleague's star = %v", got)
	}
	if r := colleague.patch("/boards/"+b2+"/favorite/move", map[string]any{"before_id": s.Board}); r.Status != http.StatusBadRequest {
		t.Fatalf("colleague moved against the owner's favourite: %d", r.Status)
	}

	// Outsiders: no list, no star.
	if r := outsider.get("/workspaces/" + s.WS + "/favorites"); r.Status != http.StatusForbidden {
		t.Fatalf("outsider list: %d", r.Status)
	}
	if r := outsider.put("/boards/"+s.Board+"/favorite", nil); r.Status != http.StatusForbidden {
		t.Fatalf("outsider star: %d", r.Status)
	}
	if r := outsider.del("/boards/" + s.Board + "/favorite"); r.Status != http.StatusForbidden {
		t.Fatalf("outsider unstar: %d", r.Status)
	}
}

// TestBoardFavoritesDropOut: a soft-deleted board leaves the list (and comes back
// on restore), and a member removed from the workspace loses the list with it.
func TestBoardFavoritesDropOut(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)
	b2 := mkBoard(t, owner, s.Project, "Вторая")
	favs := "/workspaces/" + s.WS + "/favorites"

	owner.expect(t, owner.put("/boards/"+s.Board+"/favorite", nil), http.StatusOK)
	owner.expect(t, owner.put("/boards/"+b2+"/favorite", nil), http.StatusOK)

	if r := owner.del("/boards/" + b2); r.Status != http.StatusOK && r.Status != http.StatusNoContent {
		t.Fatalf("delete board: %d %s", r.Status, r.Body)
	}
	if got := favBoardIDs(owner.get(favs).listBody(t)); !sameIDs(got, []string{s.Board}) {
		t.Fatalf("a deleted board stayed in favourites: %v", got)
	}
	// A deleted board cannot be starred or moved…
	if r := owner.put("/boards/"+b2+"/favorite", nil); r.Status != http.StatusNotFound {
		t.Fatalf("star a deleted board: %d", r.Status)
	}
	// …and the restore brings the star back in its place.
	if r := owner.post("/boards/"+b2+"/restore", nil); r.Status >= 300 {
		t.Fatalf("restore: %d %s", r.Status, r.Body)
	}
	if got := favBoardIDs(owner.get(favs).listBody(t)); !sameIDs(got, []string{s.Board, b2}) {
		t.Fatalf("after restore = %v", got)
	}
	// The star can come off a board in the trash, too.
	owner.del("/boards/" + b2)
	if r := owner.del("/boards/" + b2 + "/favorite"); r.Status != http.StatusNoContent {
		t.Fatalf("unstar a deleted board: %d", r.Status)
	}
	owner.post("/boards/"+b2+"/restore", nil)
	if got := favBoardIDs(owner.get(favs).listBody(t)); !sameIDs(got, []string{s.Board}) {
		t.Fatalf("unstarred-in-trash board came back starred: %v", got)
	}

	// Membership: the member's favourites go with their membership.
	member.expect(t, member.put("/boards/"+s.Board+"/favorite", nil), http.StatusOK)
	if r := owner.del("/workspaces/" + s.WS + "/members/" + member.UserID); r.Status >= 300 {
		t.Fatalf("remove member: %d %s", r.Status, r.Body)
	}
	if r := member.get(favs); r.Status != http.StatusForbidden {
		t.Fatalf("ex-member list: %d", r.Status)
	}
}
