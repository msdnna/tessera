package main

import (
	"net/http"
	"testing"
)

// TestTransferProjectSeversCrossWorkspaceDocParents covers the one link of the
// class the sibling transfer tests do not: a document's own parent (#2932).
//
// A document's nesting and its project are independent columns, so a parent and
// its child can sit in different projects — and only one of them travels when a
// project is transferred. parentInWorkspace refuses such a pair at the door
// ("parent belongs to another workspace"), yet the transfer manufactures one,
// and nothing downstream expects it:
//
//   - CountDocumentChildren counts children with no workspace filter, so the
//     stranded parent answers 409 "document has nested documents" naming a child
//     its workspace cannot see — an undeletable document with an empty child list.
//   - DeleteDocumentSubtree walks parent_id recursively, also unfiltered, so
//     ?recursive=true reaches across the boundary and deletes another
//     workspace's documents. documentSubtreeIDs walks only the caller's flat
//     list, so those documents' live rooms are not even dropped: their editors
//     keep typing into rows that no longer exist (the #2729 harm).
//   - ListDocuments is per workspace and the tree is assembled client-side from
//     parent_id, so a child whose parent is not in the list renders nowhere at
//     all — the document is invisible on both sides of the transfer.
func TestTransferProjectSeversCrossWorkspaceDocParents(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// A second project in the source workspace: it stays behind, and that is what
	// puts the two ends of a nesting apart. The name carries t.Name() because a
	// project slug is globally unique and the transfer tests run side by side.
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся проект "+t.Name())

	// Pair A — the child leaves, the parent stays.
	stayParent := mkProjectDoc(t, owner, src.WS, stayProject, "Папка остающегося "+t.Name())
	movingChild := mkChildDoc(t, owner, src.WS, src.Project, stayParent, "Уезжающая глава "+t.Name())
	// Pair B — the parent leaves, the child stays.
	movingParent := mkProjectDoc(t, owner, src.WS, src.Project, "Папка переезжающего "+t.Name())
	stayChild := mkChildDoc(t, owner, src.WS, stayProject, movingParent, "Остающаяся глава "+t.Name())
	// The control: nested inside the moving project, crosses no boundary.
	travelling := mkChildDoc(t, owner, src.WS, src.Project, movingParent, "Глава-попутчица "+t.Name())

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	t.Run("the transfer reports what it detached", func(t *testing.T) {
		if n, _ := res["detached_documents"].(float64); n != 2 {
			t.Fatalf("detached_documents = %v, want 2: %v", res["detached_documents"], res)
		}
	})

	t.Run("no nesting survives the boundary", func(t *testing.T) {
		for ws, want := range map[string]map[string]any{
			src.WS: {stayChild: nil},
			dst.WS: {movingChild: nil, travelling: movingParent},
		} {
			docs := docsByID(t, owner, ws)
			for id, wantParent := range want {
				d, ok := docs[id]
				if !ok {
					t.Fatalf("document %s is not in the tree of workspace %s at all: %v", id, ws, docs)
				}
				if got := d["parent_id"]; got != wantParent {
					t.Fatalf("document %s: parent_id = %v, want %v", id, got, wantParent)
				}
			}
		}
	})

	t.Run("the stranded parent is deletable again", func(t *testing.T) {
		owner.expect(t, owner.del("/documents/"+stayParent), http.StatusNoContent)
		owner.expect(t, owner.get("/documents/"+movingChild), http.StatusOK)
	})

	t.Run("a recursive delete cannot reach into the other workspace", func(t *testing.T) {
		owner.expect(t, owner.del("/documents/"+movingParent+"?recursive=true"), http.StatusNoContent)
		owner.expect(t, owner.get("/documents/"+stayChild), http.StatusOK)
		// Positive control: the subtree it does own is gone.
		if r := owner.get("/documents/" + travelling); r.Status != http.StatusNotFound {
			t.Fatalf("the child that travelled with its parent survived the recursive delete: %d", r.Status)
		}
	})
}

// mkChildDoc creates a document nested under parentID and owned by projectID —
// the two independent columns whose disagreement the transfer has to resolve.
func mkChildDoc(t *testing.T, c *client, wsID, projectID, parentID, title string) string {
	t.Helper()
	m := c.expect(t, c.post("/workspaces/"+wsID+"/documents",
		map[string]any{"title": title, "project_id": projectID, "parent_id": parentID}), http.StatusCreated)
	return m["id"].(string)
}

// docsByID reads a workspace's document list the way the clients do — flat, with
// the tree left to be assembled from parent_id — and keys it by id.
func docsByID(t *testing.T, c *client, wsID string) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, d := range c.get("/workspaces/" + wsID + "/documents").listBody(t) {
		out[d["id"].(string)] = d
	}
	return out
}
