package main

import (
	"net/http"
	"testing"
)

// TestLegacyBoardSlugResolvesInCallersScope pins that a bare /boards/<slug> link
// lands on a board the caller can actually see, and always on the same one.
//
// Board slugs stopped being globally unique in 0025 (they are unique per project
// now — mkStack names every fixture board «Доска», so a slug is shared by design),
// but the legacy lookup behind GET /boards/:id stayed `WHERE slug = $1 LIMIT 1`:
// unscoped and unordered. The row Postgres happens to hand back first is
// frequently a board in a workspace the caller is not a member of — their own
// link then answers 403 and BoardView swallows it into an empty screen — and for
// someone who is a member of both, it is a different workspace's board, with the
// URL silently canonicalised to it.
func TestLegacyBoardSlugResolvesInCallersScope(t *testing.T) {
	t.Parallel()

	// A name no other test uses, so the only boards on this slug are ours.
	const boardName = "Стенд слагскоуп"

	stranger := signup(t)
	sStranger := mkStack(t, stranger)
	// Created first: this is the row an unordered LIMIT 1 reaches first.
	strangerBoard := mkBoard(t, stranger, sStranger.Project, boardName)

	me := signup(t)
	sMine := mkStack(t, me)
	myBoard := mkBoard(t, me, sMine.Project, boardName)

	got := me.expect(t, me.get("/boards/"+myBoard), http.StatusOK)
	slug, _ := got["slug"].(string)
	if slug == "" {
		t.Fatalf("board %s has no slug: %v", myBoard, got)
	}
	other := stranger.expect(t, stranger.get("/boards/"+strangerBoard), http.StatusOK)
	if other["slug"] != slug {
		t.Fatalf("premise broken: slugs %v and %v differ, wanted the collision", other["slug"], slug)
	}

	t.Run("the slug resolves to the caller's own board", func(t *testing.T) {
		b := me.expect(t, me.get("/boards/"+slug), http.StatusOK)
		if b["id"] != myBoard {
			t.Fatalf("/boards/%s = %v, want my own board %s", slug, b["id"], myBoard)
		}
		// Same slug, other caller, other board — each side sees its own.
		b = stranger.expect(t, stranger.get("/boards/"+slug), http.StatusOK)
		if b["id"] != strangerBoard {
			t.Fatalf("/boards/%s = %v, want the stranger's board %s", slug, b["id"], strangerBoard)
		}
	})

	t.Run("a board route behind the same slug is reachable too", func(t *testing.T) {
		// The Gantt graph resolves the param the same way; it must not 403 on a
		// slug whose first global match belongs to someone else.
		me.expect(t, me.get("/boards/"+slug+"/dependencies"), http.StatusOK)
	})

	t.Run("a slug nobody owns is 404, not someone else's board", func(t *testing.T) {
		if r := me.get("/boards/net-takoy-doski-slagskoup"); r.Status != http.StatusNotFound {
			t.Fatalf("unknown slug → %d, want 404", r.Status)
		}
	})

	t.Run("with two of the caller's own workspaces the hint decides", func(t *testing.T) {
		// Same collision inside one account: a legacy link carries no project, so
		// the client passes the workspace it is currently in as a hint.
		ws2 := mkWorkspace(t, me, "WS2 "+t.Name())
		g2 := mkGroup(t, me, ws2, "Группа")
		p2 := mkProject(t, me, ws2, g2, "Проект второй слагскоуп")
		second := mkBoard(t, me, p2, boardName)

		b := me.expect(t, me.get("/boards/"+slug+"?workspace="+ws2), http.StatusOK)
		if b["id"] != second {
			t.Fatalf("hinted ws2 → %v, want %s", b["id"], second)
		}
		b = me.expect(t, me.get("/boards/"+slug+"?workspace="+sMine.WS), http.StatusOK)
		if b["id"] != myBoard {
			t.Fatalf("hinted my first ws → %v, want %s", b["id"], myBoard)
		}
		// No hint: whichever board answers, it must answer every time — a link
		// that flips between boards on reload is worse than one that picks wrong.
		first := me.expect(t, me.get("/boards/"+slug), http.StatusOK)
		again := me.expect(t, me.get("/boards/"+slug), http.StatusOK)
		if first["id"] != again["id"] {
			t.Fatalf("unhinted lookup is unstable: %v then %v", first["id"], again["id"])
		}
		if first["id"] != myBoard {
			t.Fatalf("unhinted lookup = %v, want the oldest of my boards %s", first["id"], myBoard)
		}
	})
}
