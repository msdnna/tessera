package main

import (
	"net/http"
	"testing"
)

// manualCapOvershoot is how far past docManualVersionKeep (200) the test pushes.
// Enough to prove the bound holds and that the *oldest* entries are the ones that
// go, not enough to make the test a load test.
const manualCapOvershoot = 5

// TestManualDocumentVersionsAreCapped pins the bound on the manual half of a
// document's journal.
//
// The automatic half has always been capped at 50. The manual half had no bound
// of any kind: the prune skipped it by design, there is no endpoint that deletes
// a version, and three paths write into it — the snapshot button, a rollback (two
// rows per click) and raising an approval route. Every row is a full copy of the
// body, so the table grew until people stopped using the feature.
//
// Without the cap this test sees 205 versions and the first snapshot still in the
// journal.
func TestManualDocumentVersionsAreCapped(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS "+t.Name())
	doc := c.expect(t, c.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Регламент"}), http.StatusCreated)
	id := doc["id"].(string)
	saveDoc(t, c, id, doc["updated_at"].(string), "текст регламента")

	const total = 200 + manualCapOvershoot
	for i := 0; i < total; i++ {
		c.expect(t, c.post("/documents/"+id+"/versions",
			map[string]any{"label": "веха " + itoa(i)}), http.StatusCreated)
	}

	vs := docVersions(t, c, id)
	manual := 0
	labels := make(map[string]bool, len(vs))
	for _, v := range vs {
		if v["manual"] == true {
			manual++
		}
		if l, ok := v["label"].(string); ok {
			labels[l] = true
		}
	}
	if manual > 200 {
		t.Fatalf("%d manual versions survived %d snapshots — the manual half is unbounded", manual, total)
	}
	if manual != 200 {
		t.Fatalf("manual versions trimmed to %d, want exactly the newest 200", manual)
	}
	// The newest are kept and the oldest are the ones that went.
	for i := 0; i < manualCapOvershoot; i++ {
		if labels["веха "+itoa(i)] {
			t.Fatalf("snapshot %d is still in the journal — retention dropped the wrong end", i)
		}
	}
	if !labels["веха "+itoa(total-1)] {
		t.Fatal("the newest snapshot was pruned")
	}
}

// TestApprovalPinnedVersionSurvivesManualPrune is the constraint that makes the
// cap safe to have at all.
//
// A route pins the revision it collects signatures on, and
// document_approvals.version_id is ON DELETE RESTRICT — so a prune that reached
// that row would not quietly orphan the route, it would fail the statement and
// take down the snapshot the user was asking for. The pinned version is therefore
// excluded from the prune outright, however old it gets.
func TestApprovalPinnedVersionSurvivesManualPrune(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, _ := mkLinkedDoc(t, owner)
	member := approvers(t, owner, s.WS, 1)[0]

	doc := owner.expect(t, owner.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Приказ"}), http.StatusCreated)
	id := doc["id"].(string)
	saveDoc(t, owner, id, doc["updated_at"].(string), "текст приказа")

	owner.expect(t, owner.post("/documents/"+id+"/approvals",
		map[string]any{"approvers": []any{member.UserID}}), http.StatusCreated)

	// Bury the pinned snapshot under a full cap's worth of newer ones.
	const after = 200 + manualCapOvershoot
	for i := 0; i < after; i++ {
		owner.expect(t, owner.post("/documents/"+id+"/versions",
			map[string]any{"label": "веха " + itoa(i)}), http.StatusCreated)
	}

	vs := docVersions(t, owner, id)
	pinned := false
	for _, v := range vs {
		if l, ok := v["label"].(string); ok && l == "На согласование" {
			pinned = true
		}
	}
	if !pinned {
		t.Fatalf("the revision the approval route signs was pruned out of the journal (%d versions left)", len(vs))
	}
}
