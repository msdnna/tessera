// The workspace-wide estimation unit is a workspace setting, and it must be
// gated like one.
//
// Three writes land on the workspaces row: the rename (UpdateWorkspace), the
// quick-action dictionary (SetWorkspaceCommands) and this one. The first two sit
// behind requireManager; this one stood behind requireMember, so any participant
// could change what every estimate in the workspace means. Nothing is converted
// when the unit moves — the column stores minutes and the clients resolve
// project → workspace → built-in default to render it — so flipping time→points
// re-labels every «2 ч» on every board as points, and moving hours_per_day
// re-scales every day-shaped figure, including the one the backend itself parses
// (estimateConfig feeds ParseEstimate, so «/estimate 1д» changes meaning too).
//
// The project-level override is deliberately NOT tightened: it has its own menu
// entry on the project row, it is scoped to one project, and applyTag-style
// per-project settings are member-level throughout. The second test is the
// negative control that says so — a gate that swallowed both would look just as
// green here (#2932).
package main

import (
	"net/http"
	"testing"
)

// mkMember signs up a user and adds them to the workspace as a plain member.
func mkMember(t *testing.T, owner *client, wsID string) *client {
	t.Helper()
	m := signup(t)
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": m.Email, "role": "member"}), http.StatusCreated)
	return m
}

// workspaceEstimationUnit reads the unit currently stored on the workspace, or
// "" when the workspace inherits the built-in default.
func workspaceEstimationUnit(t *testing.T, c *client, wsID string) string {
	t.Helper()
	ws := c.expect(t, c.get("/workspaces/"+wsID), http.StatusOK)
	est, ok := ws["estimation"].(map[string]any)
	if !ok {
		return ""
	}
	unit, _ := est["unit"].(string)
	return unit
}

func TestWorkspaceEstimationIsManagerOnly(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с оценкой")
	member := mkMember(t, owner, wsID)

	// The gate is the point, but a 403 alone would also be satisfied by a member
	// who cannot see the workspace at all — so first prove they are in it.
	member.expect(t, member.get("/workspaces/"+wsID), http.StatusOK)

	member.expect(t, member.put("/workspaces/"+wsID+"/estimation",
		map[string]any{"unit": "points", "points_scale": "fibonacci"}), http.StatusForbidden)

	// Refused, not merely unreported: the read-back is what catches a handler that
	// writes and then answers 403.
	if unit := workspaceEstimationUnit(t, owner, wsID); unit != "" {
		t.Fatalf("единица после отказа = %q, ожидалась пустая (наследование)", unit)
	}

	// An admin is a manager, so the setting is not locked to the owner alone.
	admin := mkAdmin(t, owner, wsID)
	admin.expect(t, admin.put("/workspaces/"+wsID+"/estimation",
		map[string]any{"unit": "points", "points_scale": "fibonacci"}), http.StatusOK)
	if unit := workspaceEstimationUnit(t, owner, wsID); unit != "points" {
		t.Fatalf("единица после записи админом = %q, want points", unit)
	}
}

func TestProjectEstimationStaysOpenToMembers(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID := mkWorkspace(t, owner, "Пространство с проектной оценкой")
	groupID := mkGroup(t, owner, wsID, "Группа")
	projectID := mkProject(t, owner, wsID, groupID, "Проект")
	member := mkMember(t, owner, wsID)

	member.expect(t, member.put("/projects/"+projectID+"/estimation",
		map[string]any{"unit": "custom", "custom_label": "попугаи"}), http.StatusOK)

	p := owner.expect(t, owner.get("/projects/"+projectID), http.StatusOK)
	est, ok := p["estimation"].(map[string]any)
	if !ok {
		t.Fatalf("проект без estimation после записи участником: %v", p["estimation"])
	}
	if unit, _ := est["unit"].(string); unit != "custom" {
		t.Fatalf("единица проекта = %q, want custom", unit)
	}
}
