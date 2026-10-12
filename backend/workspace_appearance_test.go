package main

import (
	"net/http"
	"strings"
	"testing"
)

// The rail tile of a workspace (#2955): colour + icon live on the workspace,
// come back in the list, survive a rename, and the mode falls back to badge.
func TestWorkspaceAppearance(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := c.expect(t, c.post("/workspaces", map[string]any{"name": "Плитка"}), http.StatusCreated)
	id := ws["id"].(string)
	if ws["color"] != "" || ws["icon"] != "" || ws["icon_mode"] != "badge" {
		t.Fatalf("new workspace appearance = %v/%v/%v, want empty/empty/badge", ws["color"], ws["icon"], ws["icon_mode"])
	}

	got := c.expect(t, c.put("/workspaces/"+id+"/appearance",
		map[string]any{"color": "#2f80ed", "icon": "rocket", "icon_mode": "bogus"}), http.StatusOK)
	if got["color"] != "#2f80ed" || got["icon"] != "rocket" || got["icon_mode"] != "badge" {
		t.Fatalf("appearance = %v", got)
	}

	// A rename (the settings form) leaves the tile alone.
	c.expect(t, c.patch("/workspaces/"+id, map[string]any{"name": "Плитка 2"}), http.StatusOK)
	var row map[string]any
	for _, w := range c.get("/workspaces").listBody(t) {
		if w["id"] == id {
			row = w
		}
	}
	if row == nil || row["color"] != "#2f80ed" || row["icon"] != "rocket" {
		t.Fatalf("list after rename = %v", row)
	}

	c.expect(t, c.put("/workspaces/"+id+"/appearance",
		map[string]any{"icon": strings.Repeat("x", 65)}), http.StatusBadRequest)
}
