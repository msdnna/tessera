// The relation kind a link is stored under must be one the clients can name (#2932).
//
// «/relate #N» passed its command key to applyRelation unchanged, so the row
// landed with kind 'relate' — a fifth kind nothing else in the product knows.
// The schema comment, both clients' label maps (`task.relations.kind.*` on web,
// relKindLabel on Android), gitlab.RelationKind and the MCP link tool all list
// the same four; a client that meets another one prints it verbatim, so the
// «Связи» tab read a bare English «relate» where «связана с» belonged.
//
// It also doubled the row. Uniqueness is (task_id, related_task_id, kind) and
// ListTaskRelations dedups by (other end, kind), so the same pair linked once by
// the command and once from the tab showed up twice — and the ✕
// (DeleteRelationBothWays, also per kind) took only its own half away.
package main

import (
	"fmt"
	"net/http"
	"testing"
)

// TestQuickActionRelateStoresCanonicalKind pins the command's own row: «/relate»
// must write the kind «relates» the tab labels, not its command key.
func TestQuickActionRelateStoresCanonicalKind(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	colID := s.col(t, 0)

	src := mkTask(t, c, s.Board, colID, "Источник связи")
	dst := mkTask(t, c, s.Board, colID, "Цель связи")
	srcID := src["id"].(string)
	dstNum, _ := dst["number"].(float64)

	r := c.post("/tasks/"+srcID+"/comments", map[string]any{"body": fmt.Sprintf("/relate #%d", int64(dstNum))})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	if applied, failed := commandSummary(t, r); len(applied) != 1 || applied[0] != "relate" || len(failed) != 0 {
		t.Fatalf("summary applied=%v failed=%v, want [relate] / []", applied, failed)
	}

	rows := c.get("/tasks/" + srcID + "/relations").listBody(t)
	if len(rows) != 1 {
		t.Fatalf("got %d relations, want 1\n%v", len(rows), rows)
	}
	if got := relKinds(t, rows)[dstNum]; got != "relates" {
		t.Fatalf("kind = %q, want %q — the tab prints an unknown kind verbatim", got, "relates")
	}
}

// TestQuickActionRelateMatchesTabLink is the consequence the user sees: linking
// the same pair from the tab and with the command must leave ONE relation, and
// the ✕ must take it away for good.
func TestQuickActionRelateMatchesTabLink(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	colID := s.col(t, 0)

	src := mkTask(t, c, s.Board, colID, "Источник")
	dst := mkTask(t, c, s.Board, colID, "Цель")
	srcID := src["id"].(string)
	dstNum, _ := dst["number"].(float64)

	// The tab's own way of making the link…
	c.expect(t, c.post("/tasks/"+srcID+"/relations",
		map[string]any{"number": dst["number"], "kind": "relates"}), http.StatusCreated)
	// …and the command, meaning the same thing.
	c.post("/tasks/"+srcID+"/comments", map[string]any{"body": fmt.Sprintf("/relate #%d", int64(dstNum))})

	rows := c.get("/tasks/" + srcID + "/relations").listBody(t)
	if len(rows) != 1 {
		t.Fatalf("got %d relations for one pair, want 1 — the command and the tab must agree on the kind\n%v",
			len(rows), rows)
	}

	// One ✕ leaves nothing behind.
	relID, _ := rows[0]["id"].(string)
	c.expect(t, c.del("/relations/"+relID), http.StatusNoContent)
	if left := c.get("/tasks/" + srcID + "/relations").listBody(t); len(left) != 0 {
		t.Fatalf("%d relations survive the ✕, want 0\n%v", len(left), left)
	}
}

// TestAddRelationRejectsUnknownKind guards the same door from the REST side: the
// endpoint takes `kind` straight from the body, and a kind outside the closed set
// is not a link the product can render.
func TestAddRelationRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	colID := s.col(t, 0)

	src := mkTask(t, c, s.Board, colID, "Источник")
	dst := mkTask(t, c, s.Board, colID, "Цель")
	srcID := src["id"].(string)

	r := c.post("/tasks/"+srcID+"/relations", map[string]any{"number": dst["number"], "kind": "relate"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 for an unknown kind\n%s", r.Status, r.Body)
	}
	if rows := c.get("/tasks/" + srcID + "/relations").listBody(t); len(rows) != 0 {
		t.Fatalf("%d relations written despite the rejection\n%v", len(rows), rows)
	}

	// The four the product speaks still go through; an empty kind still defaults.
	for _, kind := range []string{"", "relates", "blocks", "blocked_by", "duplicates"} {
		body := map[string]any{"number": dst["number"]}
		if kind != "" {
			body["kind"] = kind
		}
		c.expect(t, c.post("/tasks/"+srcID+"/relations", body), http.StatusCreated)
	}
}
