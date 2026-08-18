// Authz matrix over the protected routes of router.go.
//
// Three sweeps documenting current behavior:
//   - no token → 401 on every protected route (middleware.Auth);
//   - someone else's resource → 403 (requireMember/requireManager) once the
//     resource resolves, 404 when the id doesn't exist at all;
//   - malformed UUID param → 400 (parseID), except GET /boards/:id which
//     falls back to slug lookup and answers 404.
package main

import (
	"net/http"
	"testing"
)

// dummyID is a well-formed UUID that never exists in the test database
// (all real rows use gen_random_uuid).
const dummyID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

// protectedRoutes lists every route behind middleware.Auth (keep in sync with
// router.go), with path params substituted so gin matches the route.
func protectedRoutes() []struct{ method, path string } {
	u := dummyID
	return []struct{ method, path string }{
		{"GET", "/auth/me"},
		{"POST", "/auth/resend-verification"},
		{"POST", "/auth/tokens"},
		{"GET", "/auth/tokens"},
		{"DELETE", "/auth/tokens/" + u},
		{"PATCH", "/users/me"},
		{"PUT", "/users/me/password"},
		{"PUT", "/users/me/preferences"},
		{"PUT", "/users/me/avatar"},
		{"DELETE", "/users/me/avatar"},
		{"POST", "/workspaces"},
		{"GET", "/workspaces"},
		{"GET", "/workspaces/" + u},
		{"PATCH", "/workspaces/" + u},
		{"DELETE", "/workspaces/" + u},
		{"GET", "/workspaces/" + u + "/search"},
		{"GET", "/workspaces/" + u + "/tasks"},
		{"GET", "/workspaces/" + u + "/tasks/by-number/1"},
		{"GET", "/board-by-slug"},
		{"GET", "/workspaces/" + u + "/summary"},
		{"GET", "/workspaces/" + u + "/members"},
		{"POST", "/workspaces/" + u + "/members"},
		{"PATCH", "/workspaces/" + u + "/members/" + u},
		{"DELETE", "/workspaces/" + u + "/members/" + u},
		{"POST", "/workspaces/" + u + "/invitations"},
		{"GET", "/workspaces/" + u + "/invitations"},
		{"DELETE", "/workspaces/" + u + "/invitations/" + u},
		{"POST", "/invitations/accept"},
		{"GET", "/admin/users"},
		{"PATCH", "/admin/users/" + u + "/active"},
		{"PATCH", "/admin/users/" + u + "/admin"},
		{"POST", "/admin/users/" + u + "/reset-link"},
		{"GET", "/admin/oauth/gitlab"},
		{"PUT", "/admin/oauth/gitlab"},
		{"POST", "/workspaces/" + u + "/groups"},
		{"GET", "/workspaces/" + u + "/groups"},
		{"POST", "/workspaces/" + u + "/projects"},
		{"GET", "/workspaces/" + u + "/projects"},
		{"POST", "/projects/" + u + "/tags"},
		{"GET", "/projects/" + u + "/tags"},
		{"GET", "/workspaces/" + u + "/tags"},
		{"GET", "/projects/" + u + "/tag-prefixes"},
		{"PUT", "/projects/" + u + "/tag-prefixes"},
		{"GET", "/workspaces/" + u + "/tag-prefixes"},
		{"PUT", "/workspaces/" + u + "/estimation"},
		{"PUT", "/projects/" + u + "/estimation"},
		{"POST", "/workspaces/" + u + "/notes"},
		{"GET", "/workspaces/" + u + "/notes"},
		{"PATCH", "/groups/" + u},
		{"PATCH", "/groups/" + u + "/move"},
		{"DELETE", "/groups/" + u},
		{"GET", "/projects/" + u},
		{"PATCH", "/projects/" + u},
		{"PATCH", "/projects/" + u + "/move"},
		{"POST", "/projects/" + u + "/transfer"},
		{"DELETE", "/projects/" + u},
		{"POST", "/projects/" + u + "/boards"},
		{"GET", "/projects/" + u + "/boards"},
		{"GET", "/boards/" + u},
		{"PATCH", "/boards/" + u},
		{"PATCH", "/boards/" + u + "/done-column"},
		{"DELETE", "/boards/" + u},
		{"POST", "/boards/" + u + "/columns"},
		{"GET", "/boards/" + u + "/columns"},
		{"POST", "/boards/" + u + "/tasks"},
		{"GET", "/boards/" + u + "/tasks"},
		{"GET", "/boards/" + u + "/subtasks"},
		{"GET", "/boards/" + u + "/archive"},
		{"GET", "/boards/" + u + "/dependencies"},
		{"GET", "/boards/" + u + "/views"},
		{"POST", "/boards/" + u + "/views"},
		{"DELETE", "/views/" + u},
		{"PATCH", "/columns/" + u},
		{"PATCH", "/columns/" + u + "/move"},
		{"DELETE", "/columns/" + u},
		{"GET", "/tasks/" + u},
		{"PATCH", "/tasks/" + u},
		{"PATCH", "/tasks/" + u + "/move"},
		{"PATCH", "/tasks/" + u + "/eisenhower"},
		{"PATCH", "/tasks/" + u + "/parent"},
		{"PATCH", "/tasks/" + u + "/transfer"},
		{"PATCH", "/tasks/" + u + "/archive"},
		{"PATCH", "/tasks/" + u + "/restore"},
		{"DELETE", "/tasks/" + u},
		{"POST", "/tasks/" + u + "/tags"},
		{"DELETE", "/tasks/" + u + "/tags/" + u},
		{"POST", "/tasks/" + u + "/assignees"},
		{"DELETE", "/tasks/" + u + "/assignees/" + u},
		{"POST", "/tasks/" + u + "/gitlab-assignees"},
		{"DELETE", "/tasks/" + u + "/gitlab-assignees/someone"},
		{"POST", "/tasks/" + u + "/gitlab-issue"},
		{"POST", "/tasks/" + u + "/gitlab-group"},
		{"DELETE", "/tasks/" + u + "/gitlab-group"},
		{"GET", "/tasks/" + u + "/events"},
		{"GET", "/tasks/" + u + "/comments"},
		{"POST", "/tasks/" + u + "/comments"},
		{"PATCH", "/comments/" + u},
		{"DELETE", "/comments/" + u},
		{"GET", "/tasks/" + u + "/relations"},
		{"POST", "/tasks/" + u + "/relations"},
		{"DELETE", "/relations/" + u},
		{"GET", "/tasks/" + u + "/attachments"},
		{"POST", "/tasks/" + u + "/attachments"},
		{"GET", "/attachments/" + u + "/download"},
		{"DELETE", "/attachments/" + u},
		{"POST", "/uploads"},
		{"GET", "/notifications"},
		{"GET", "/notifications/unread-count"},
		{"POST", "/notifications/" + u + "/read"},
		{"POST", "/notifications/read-all"},
		{"GET", "/notification-channels"},
		{"POST", "/notification-channels"},
		{"PATCH", "/notification-channels/" + u},
		{"DELETE", "/notification-channels/" + u},
		{"POST", "/notification-channels/" + u + "/test"},
		{"POST", "/notification-devices"},
		{"POST", "/notification-template-preview"},
		{"GET", "/notification-prefs"},
		{"PUT", "/notification-prefs"},
		{"PATCH", "/tasks/" + u + "/due-notify"},
		{"GET", "/notification-routes"},
		{"POST", "/notification-routes"},
		{"PATCH", "/notification-routes/" + u},
		{"DELETE", "/notification-routes/" + u},
		{"PATCH", "/tags/" + u},
		{"DELETE", "/tags/" + u},
		{"GET", "/workspaces/" + u + "/milestones"},
		{"GET", "/projects/" + u + "/milestones"},
		{"POST", "/projects/" + u + "/milestones"},
		{"PATCH", "/milestones/" + u},
		{"DELETE", "/milestones/" + u},
		{"POST", "/tasks/" + u + "/milestone"},
		{"DELETE", "/tasks/" + u + "/milestone"},
		{"POST", "/milestones/" + u + "/gitlab"},
		{"GET", "/notes/" + u},
		{"PATCH", "/notes/" + u},
		{"DELETE", "/notes/" + u},
		{"GET", "/gitlab/connection"},
		{"POST", "/gitlab/connection"},
		{"DELETE", "/gitlab/connection"},
		{"GET", "/workspaces/" + u + "/gitlab/integrations"},
		{"POST", "/workspaces/" + u + "/gitlab/integrations"},
		{"PUT", "/workspaces/" + u + "/gitlab/integrations/" + u},
		{"DELETE", "/workspaces/" + u + "/gitlab/integrations/" + u},
		{"POST", "/workspaces/" + u + "/gitlab/integrations/" + u + "/sync"},
		{"GET", "/workspaces/" + u + "/gitlab/members"},
		{"GET", "/workspaces/" + u + "/gitlab/issue-templates"},
		{"GET", "/workspaces/" + u + "/gitlab/sync-runs"},
		{"GET", "/workspaces/" + u + "/gitlab/sync-runs/" + u + "/actions"},
		{"POST", "/workspaces/" + u + "/gitlab/sync-runs/" + u + "/actions/" + u + "/retry"},
		{"GET", "/workspaces/" + u + "/gitlab/conflicts"},
		{"POST", "/tasks/" + u + "/gitlab/conflicts/" + u + "/resolve"},
		{"POST", "/reminders"},
		{"GET", "/reminders"},
		{"PATCH", "/reminders/" + u},
		{"DELETE", "/reminders/" + u},
	}
}

// Every protected route rejects a request without a bearer token with 401
// before any handler logic runs.
func TestAuthzNoToken(t *testing.T) {
	t.Parallel()
	for _, rt := range protectedRoutes() {
		r := doReq(t, "", rt.method, rt.path, nil)
		if r.Status != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401\n%s", rt.method, rt.path, r.Status, r.Body)
		}
	}
}

// A non-member probing another user's resources gets 403 once the resource
// resolves (requireMember), 404 for ids that don't exist, and 400 for
// malformed UUID params. Workspace-scoped routes check membership on the
// param itself, so an unknown workspace id also reads as 403.
func TestAuthzForeignResources(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "authz probe")
	taskID := task["id"].(string)
	note := owner.expect(t, owner.post("/workspaces/"+s.WS+"/notes",
		map[string]any{"title": "authz note"}), http.StatusCreated)
	ms := owner.expect(t, owner.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "authz milestone"}), http.StatusCreated)
	tag := owner.expect(t, owner.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "authz-tag"}), http.StatusCreated)

	stranger := signup(t) // member of nothing owner owns

	cases := []struct {
		name, method, prefix, id, suffix string
		body                             any
		wantForeign, wantMissing, wantBad int
	}{
		// Workspace routes authorize on the :id param directly → unknown id
		// is indistinguishable from a foreign one (403), except DELETE which
		// loads the row first (owner check) → 404 for unknown.
		{"get workspace", "GET", "/workspaces/", s.WS, "", nil, 403, 403, 400},
		{"patch workspace", "PATCH", "/workspaces/", s.WS, "", map[string]any{"name": "x"}, 403, 403, 400},
		{"delete workspace", "DELETE", "/workspaces/", s.WS, "", nil, 403, 404, 400},
		{"list members", "GET", "/workspaces/", s.WS, "/members", nil, 403, 403, 400},
		{"list ws tags", "GET", "/workspaces/", s.WS, "/tags", nil, 403, 403, 400},
		{"list ws notes", "GET", "/workspaces/", s.WS, "/notes", nil, 403, 403, 400},

		// Resource routes load the row first → 404 unknown, then member → 403.
		{"patch group", "PATCH", "/groups/", s.Group, "", map[string]any{"name": "x"}, 403, 404, 400},
		{"delete group", "DELETE", "/groups/", s.Group, "", nil, 403, 404, 400},

		{"get project", "GET", "/projects/", s.Project, "", nil, 403, 404, 400},
		{"patch project", "PATCH", "/projects/", s.Project, "", map[string]any{"name": "x"}, 403, 404, 400},
		{"delete project", "DELETE", "/projects/", s.Project, "", nil, 403, 404, 400},
		{"list boards", "GET", "/projects/", s.Project, "/boards", nil, 403, 404, 400},
		{"list milestones", "GET", "/projects/", s.Project, "/milestones", nil, 403, 404, 400},
		{"list tags", "GET", "/projects/", s.Project, "/tags", nil, 403, 404, 400},

		// GET /boards/:id treats a non-UUID param as a slug → 404, not 400.
		{"get board", "GET", "/boards/", s.Board, "", nil, 403, 404, 404},
		{"patch board", "PATCH", "/boards/", s.Board, "", map[string]any{"name": "x"}, 403, 404, 400},
		{"delete board", "DELETE", "/boards/", s.Board, "", nil, 403, 404, 400},
		{"list columns", "GET", "/boards/", s.Board, "/columns", nil, 403, 404, 400},
		{"list board tasks", "GET", "/boards/", s.Board, "/tasks", nil, 403, 404, 400},
		{"create task", "POST", "/boards/", s.Board, "/tasks",
			map[string]any{"title": "x", "column_id": s.col(t, 0)}, 403, 404, 400},

		{"patch column", "PATCH", "/columns/", s.col(t, 0), "", map[string]any{"name": "x"}, 403, 404, 400},
		{"delete column", "DELETE", "/columns/", s.col(t, 0), "", nil, 403, 404, 400},

		{"get task", "GET", "/tasks/", taskID, "", nil, 403, 404, 400},
		{"patch task", "PATCH", "/tasks/", taskID, "", map[string]any{"title": "x"}, 403, 404, 400},
		{"delete task", "DELETE", "/tasks/", taskID, "", nil, 403, 404, 400},
		{"list comments", "GET", "/tasks/", taskID, "/comments", nil, 403, 404, 400},
		{"list events", "GET", "/tasks/", taskID, "/events", nil, 403, 404, 400},

		{"get note", "GET", "/notes/", note["id"].(string), "", nil, 403, 404, 400},
		{"patch note", "PATCH", "/notes/", note["id"].(string), "", map[string]any{"title": "x"}, 403, 404, 400},
		{"delete note", "DELETE", "/notes/", note["id"].(string), "", nil, 403, 404, 400},

		{"patch milestone", "PATCH", "/milestones/", ms["id"].(string), "", map[string]any{"title": "x"}, 403, 404, 400},
		{"delete milestone", "DELETE", "/milestones/", ms["id"].(string), "", nil, 403, 404, 400},

		{"patch tag", "PATCH", "/tags/", tag["id"].(string), "", map[string]any{"name": "x"}, 403, 404, 400},
		{"delete tag", "DELETE", "/tags/", tag["id"].(string), "", nil, 403, 404, 400},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if r := stranger.do(tc.method, tc.prefix+tc.id+tc.suffix, tc.body); r.Status != tc.wantForeign {
				t.Errorf("foreign: status %d, want %d\n%s", r.Status, tc.wantForeign, r.Body)
			}
			if r := stranger.do(tc.method, tc.prefix+dummyID+tc.suffix, tc.body); r.Status != tc.wantMissing {
				t.Errorf("missing id: status %d, want %d\n%s", r.Status, tc.wantMissing, r.Body)
			}
			if r := stranger.do(tc.method, tc.prefix+"not-a-uuid"+tc.suffix, tc.body); r.Status != tc.wantBad {
				t.Errorf("bad uuid: status %d, want %d\n%s", r.Status, tc.wantBad, r.Body)
			}
		})
	}

	// Nothing of the owner's got modified by the probes.
	owner.expect(t, owner.get("/tasks/"+taskID), http.StatusOK)
	owner.expect(t, owner.get("/workspaces/"+s.WS), http.StatusOK)
}
