package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestDocumentSubtreeSurvivesACycle pins that the document-subtree machinery
// terminates on cyclic rows instead of spinning.
//
// Three places used to take "the tree is a tree" on faith: both recursive CTEs
// (DocumentAncestorIDs, DeleteDocumentSubtree) were UNION ALL — which never
// converges on a ring, unlike the UNION the task and project-group walks use —
// and documentSubtreeIDs walked the parent links in Go with no visited set, its
// comment resting on "reparentAllowed rejects cycles".
//
// The handler guard does reject the ring, but it is a check-then-update with no
// transaction or lock around it: two concurrent PATCHes, A under B and B under A,
// both read an acyclic tree and both commit. A row set can also arrive cyclic
// from an import or a manual fix. Either way the consequence was wildly out of
// proportion to the cause — the Go walk is an unbounded loop appending to a
// slice *inside the API process*, so one DELETE takes the whole backend down with
// it, and cancelling the request does not stop it. The ring is planted here
// directly because that is exactly the state the code must survive, whatever put
// it there.
func TestDocumentSubtreeSurvivesACycle(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())

	top := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "верхний"}), http.StatusCreated)["id"].(string)
	child := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "вложенный", "parent_id": top}), http.StatusCreated)["id"].(string)

	// The ring the handler refuses to create: top → child → top.
	if _, err := testPool.Exec(context.Background(),
		"UPDATE documents SET parent_id = $1 WHERE id = $2", child, top); err != nil {
		t.Fatalf("plant the cycle: %v", err)
	}

	t.Run("the ancestor walk terminates", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ids, err := testQueries.DocumentAncestorIDs(ctx, uuid.MustParse(top))
		if err != nil {
			t.Fatalf("DocumentAncestorIDs on a cycle: %v (a deadline here means the CTE never converged)", err)
		}
		// Two rows, each seen once: the ring is walked, not unrolled.
		if len(ids) != 2 {
			t.Fatalf("ancestors = %d ids, want 2 (the ring's own members)", len(ids))
		}
	})

	t.Run("a re-parent request still answers", func(t *testing.T) {
		// The guard reads the ancestors of the proposed parent, so this is the
		// ancestor walk reached over HTTP. Any answer will do — the point is that
		// one arrives.
		done := make(chan int, 1)
		go func() { done <- c.patch("/documents/"+top, map[string]any{"parent_id": child}).Status }()
		select {
		case got := <-done:
			if got != http.StatusBadRequest {
				t.Fatalf("re-parent inside the ring: status %d, want 400", got)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("re-parent never answered: the ancestor CTE is spinning on the cycle")
		}
	})

	t.Run("deleting the ring terminates and takes both rows", func(t *testing.T) {
		// Both halves run here: documentSubtreeIDs (Go, collects the rooms to
		// drop) and DeleteDocumentSubtree (SQL). Recursive, so the 409 for a
		// container needs ?recursive=true.
		done := make(chan int, 1)
		go func() { done <- c.del("/documents/" + top + "?recursive=true").Status }()
		select {
		case got := <-done:
			if got != http.StatusNoContent {
				t.Fatalf("recursive delete of the ring: status %d, want 204", got)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("delete never answered: the subtree walk is spinning on the cycle")
		}
		for _, id := range []string{top, child} {
			if r := c.get("/documents/" + id); r.Status != http.StatusNotFound {
				t.Fatalf("document %s after recursive delete: status %d, want 404", id, r.Status)
			}
		}
	})
}
