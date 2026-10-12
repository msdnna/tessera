package main

import (
	"net/http"
	"strings"
	"testing"
)

// boardViewCap mirrors handlers.maxBoardViews. Restated rather than exported:
// the number is a product decision about a dropdown, and a test that reached into
// the package for it would pass just as happily if the cap were silently raised.
const boardViewCap = 50

// TestBoardViewsAreCapped pins the bound on one user's saved layouts for one
// board.
//
// Nothing bounded this table before: the config is an opaque blob the frontend
// owns, the name is half the unique key so every fresh name inserts a row instead
// of overwriting one, and no retention pass exists. A member could therefore keep
// POSTing distinct names and grow the database by up to the route's whole 1 MiB
// body budget per call, without limit.
//
// Without the cap this test sees 55 views in the picker.
func TestBoardViewsAreCapped(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	const overshoot = 5
	refused := 0
	for i := 0; i < boardViewCap+overshoot; i++ {
		r := c.post("/boards/"+s.Board+"/views", map[string]any{
			"name": "вид " + itoa(i), "config": map[string]any{"groupMode": "column"},
		})
		switch r.Status {
		case http.StatusOK:
		case http.StatusConflict:
			refused++
		default:
			t.Fatalf("save view %d: status %d\n%s", i, r.Status, r.Body)
		}
	}
	if refused != overshoot {
		t.Fatalf("%d saves refused past the cap, want %d — the table is unbounded", refused, overshoot)
	}
	if got := len(boardViews(t, c, s.Board)); got != boardViewCap {
		t.Fatalf("%d views stored, want exactly the cap of %d", got, boardViewCap)
	}
}

// TestBoardViewOverwriteSurvivesTheCap is the constraint that makes the cap safe
// to have at all. The board autosaves the loaded view on every toolbar change, so
// a user sitting at the cap must still be able to re-save what they already have
// — refusing by count alone would freeze the layout of whoever uses the feature
// most.
func TestBoardViewOverwriteSurvivesTheCap(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	for i := 0; i < boardViewCap; i++ {
		c.expect(t, c.post("/boards/"+s.Board+"/views",
			map[string]any{"name": "вид " + itoa(i)}), http.StatusOK)
	}
	// At the cap: a new name is refused, an existing one is still overwritten.
	if r := c.post("/boards/"+s.Board+"/views", map[string]any{"name": "ещё один"}); r.Status != http.StatusConflict {
		t.Fatalf("new view at the cap: status %d, want 409\n%s", r.Status, r.Body)
	}
	got := c.expect(t, c.post("/boards/"+s.Board+"/views", map[string]any{
		"name": "вид 0", "config": map[string]any{"groupMode": "tag"},
	}), http.StatusOK)
	cfg, _ := got["config"].(map[string]any)
	if cfg["groupMode"] != "tag" {
		t.Fatalf("overwrite at the cap did not take: config=%v", got["config"])
	}
	if n := len(boardViews(t, c, s.Board)); n != boardViewCap {
		t.Fatalf("%d views after the overwrite, want %d", n, boardViewCap)
	}
}

// TestBoardViewConfigAndNameAreBounded covers the other two halves of the same
// hole: a single row could carry the route's whole body budget, and the name —
// half the unique key — had no bound either.
func TestBoardViewConfigAndNameAreBounded(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// 128 KiB of config, twice the ceiling, well under the 1 MiB body budget that
	// was the only thing stopping it before.
	big := strings.Repeat("я", 64<<10)
	if r := c.post("/boards/"+s.Board+"/views", map[string]any{
		"name": "тяжёлый", "config": map[string]any{"note": big},
	}); r.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized config: status %d, want 413\n%s", r.Status, r.Body)
	}

	long := strings.Repeat("и", 500)
	v := c.expect(t, c.post("/boards/"+s.Board+"/views",
		map[string]any{"name": long}), http.StatusOK)
	if got := len([]rune(v["name"].(string))); got != 200 {
		t.Fatalf("name stored with %d runes, want it truncated to 200", got)
	}

	if r := c.post("/boards/"+s.Board+"/views", map[string]any{"name": "   "}); r.Status != http.StatusBadRequest {
		t.Fatalf("blank name: status %d, want 400\n%s", r.Status, r.Body)
	}
}

// boardViews reads back the caller's saved views for a board.
func boardViews(t *testing.T, c *client, boardID string) []map[string]any {
	t.Helper()
	r := c.get("/boards/" + boardID + "/views")
	if r.Status != http.StatusOK {
		t.Fatalf("list views: status %d\n%s", r.Status, r.Body)
	}
	return r.listBody(t)
}
