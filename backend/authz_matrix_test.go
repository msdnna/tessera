// Authz matrix over the protected routes of router.go.
//
// Three sweeps documenting current behavior:
//   - no token → 401 on every protected route (middleware.Auth);
//   - someone else's resource → 403 (requireMember/requireManager) once the
//     resource resolves, 404 when the id doesn't exist at all;
//   - malformed UUID param → 400 (parseID), except GET /boards/:id which
//     falls back to slug lookup and answers 404.
//
// The first sweep reads the route table of the built router rather than a list
// kept here. It used to be a hand-written list "kept in sync with router.go",
// and by the time that was noticed it covered 156 of 227 protected routes: the
// whole /documents and /conferences families, the job console and the webhook
// secret routes had been added without it. A route that the list forgets is a
// route whose authentication nothing asserts, and the omission is invisible —
// the sweep still passes, over fewer routes. So the list is now the *public*
// surface, which is short, rarely changes, and has to be declared on purpose.
package main

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

// dummyID is a well-formed UUID that never exists in the test database
// (all real rows use gen_random_uuid).
const dummyID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

// publicKind says why a route sits outside the protected group.
type publicKind int

const (
	// publicAnon needs no credential at all: health, the login surface, a blob
	// served by unguessable name.
	publicAnon publicKind = iota
	// publicSelfAuth is outside middleware.Auth only because the caller cannot
	// send an Authorization header (a browser WebSocket, an <img>), and the
	// handler authenticates for itself — a bearer in the subprotocol, the media
	// cookie, an HMAC signature, a shared secret. Anonymous access must still be
	// refused, which is what TestAuthzPublicSurface checks.
	publicSelfAuth
)

// publicRoute is one declared exemption from the 401 sweep.
type publicRoute struct {
	kind publicKind
	// query is appended to the path for the anonymous probe. A signed proxy
	// answers 400 to a request carrying no parameters at all, and that 400 proves
	// nothing about the signature check, so the probe hands it a well-formed
	// query with a deliberately wrong signature instead.
	query string
	// wantAnon is the exact status an anonymous caller must get from a
	// publicSelfAuth route. Pinned rather than "anything but 2xx", because the
	// interesting regression does not produce a 2xx: a socket that stopped
	// checking the bearer would fail the *upgrade* of this header-less GET and
	// answer 400, which has to read as a failure rather than as a refusal.
	wantAnon int
}

// unsignedSig is a signature that cannot be the HMAC of anything.
const unsignedSig = "0000000000000000000000000000000000000000000000000000000000000000"

// publicAPIRoutes is every /api route deliberately registered outside the
// protected group, keyed "METHOD /path" with gin's own param placeholders (the
// path as router.go writes it, minus the /api prefix).
//
// This is the allowlist the 401 sweep subtracts: a new route is protected as far
// as the test is concerned unless it is named here. Registering one in the
// public group by mistake — the failure the sweep exists to catch — therefore
// fails the test instead of quietly widening the anonymous surface.
func publicAPIRoutes() map[string]publicRoute {
	anon := publicRoute{kind: publicAnon}
	selfAuth := func(status int, query string) publicRoute {
		return publicRoute{kind: publicSelfAuth, query: query, wantAnon: status}
	}
	// Enough of a query for the three signed proxies to reach their HMAC check.
	assetQ := "?ws=" + dummyID +
		"&p=" + base64.RawURLEncoding.EncodeToString([]byte("/uploads/deadbeef/x.png")) +
		"&sig=" + unsignedSig
	avatarQ := "?ws=" + dummyID +
		"&u=" + base64.RawURLEncoding.EncodeToString([]byte("https://gitlab.invalid/avatar.png")) +
		"&sig=" + unsignedSig
	docAssetQ := "?doc=" + dummyID + "&ws=" + dummyID + "&n=x.png&sig=" + unsignedSig
	return map[string]publicRoute{
		"GET /health":         anon,
		"GET /health/ready":   anon,
		"GET /version":        anon,
		"GET /client-config":  anon,
		"POST /sentry-tunnel": anon,
		// The credential surface: a caller that has no session is the point.
		"POST /auth/register":        anon,
		"POST /auth/login":           anon,
		"POST /auth/refresh":         anon,
		"POST /auth/logout":          anon,
		"POST /auth/verify-email":    anon,
		"POST /auth/forgot-password": anon,
		"POST /auth/reset-password":  anon,
		// The recovery link's deadline and the invitation page before any
		// session exists (#3035): the token in the body is the credential.
		"POST /auth/reset-password/check": anon,
		"POST /invitations/lookup":        anon,
		"POST /invitations/decline":       anon,
		"GET /auth/providers":             anon,
		"GET /auth/gitlab/authorize":      anon,
		"GET /auth/gitlab/callback":       anon,
		// The desktop/mobile OAuth return trades a one-time handoff code for the
		// session: the caller has none yet by construction (#2932).
		"POST /auth/oauth/handoff": anon,
		// Inline media and avatars: an <img> sends no header. /uploads/:name is
		// guarded by the media cookie only when MEDIA_REQUIRE_AUTH is on (off in
		// this harness, as in the default install), and an avatar blob is keyed by
		// user id on purpose.
		"GET /uploads/:name":    anon,
		"GET /users/:id/avatar": anon,
		// Sockets: the browser WebSocket API cannot send Authorization, so each
		// handler checks the bearer (header or subprotocol) *before* upgrading —
		// which is why a plain GET gets a 401 and not a handshake error.
		"GET /ws":                 selfAuth(http.StatusUnauthorized, ""),
		"GET /documents/:id/ws":   selfAuth(http.StatusUnauthorized, ""),
		"GET /conferences/:id/ws": selfAuth(http.StatusUnauthorized, ""),
		// Signed proxies: the capability is the HMAC over the query string.
		"GET /gitlab/asset":    selfAuth(http.StatusForbidden, assetQ),
		"GET /gitlab/avatar":   selfAuth(http.StatusForbidden, avatarQ),
		"GET /documents/asset": selfAuth(http.StatusForbidden, docAssetQ),
		// GitLab has no Tessera session; the binding's shared secret in
		// X-Gitlab-Token authenticates. An unknown binding is 404 without detail,
		// so the route can't be used to enumerate integration ids.
		"POST /gitlab/webhook/:integrationId": selfAuth(http.StatusNotFound, ""),
	}
}

// apiRoute is one route of the built router, with the /api prefix stripped.
type apiRoute struct{ method, tmpl string }

// key is the publicAPIRoutes lookup form.
func (r apiRoute) key() string { return r.method + " " + r.tmpl }

// path substitutes gin's params so the router actually matches the route: ids
// get a well-formed UUID that resolves to nothing, everything else a value no
// row carries. What comes back is therefore 401/403/404 — never a side effect.
func (r apiRoute) path() string {
	segs := strings.Split(r.tmpl, "/")
	for i, s := range segs {
		name, isParam := strings.CutPrefix(s, ":")
		if !isParam {
			continue
		}
		switch {
		case name == "number":
			segs[i] = "1"
		case strings.EqualFold(name, "id"), strings.HasSuffix(name, "Id"), strings.HasSuffix(name, "ID"):
			segs[i] = dummyID
		default:
			segs[i] = "nothing-by-this-name"
		}
	}
	return strings.Join(segs, "/")
}

// apiRoutes reads every /api route off the engine the harness is serving.
func apiRoutes(t *testing.T) []apiRoute {
	t.Helper()
	if testRouter == nil {
		t.Fatal("testRouter is nil — the harness did not boot")
	}
	var out []apiRoute
	for _, ri := range testRouter.Routes() {
		tmpl, ok := strings.CutPrefix(ri.Path, "/api")
		if !ok || tmpl == "" {
			continue // static/SPA routes are not part of the API surface
		}
		out = append(out, apiRoute{method: ri.Method, tmpl: tmpl})
	}
	if len(out) == 0 {
		t.Fatal("no /api routes found on the router")
	}
	return out
}

// protectedRoutes is every /api route the public allowlist does not exempt.
func protectedRoutes(t *testing.T) []apiRoute {
	t.Helper()
	public := publicAPIRoutes()
	var out []apiRoute
	for _, r := range apiRoutes(t) {
		if _, exempt := public[r.key()]; !exempt {
			out = append(out, r)
		}
	}
	return out
}

// Every protected route rejects a request without a bearer token with 401
// before any handler logic runs.
func TestAuthzNoToken(t *testing.T) {
	t.Parallel()
	routes := protectedRoutes(t)
	// A guard on the guard: if the allowlist ever swallowed most of the surface
	// (a stray "/" entry, a refactor that moved routes out of /api), the sweep
	// would still pass over whatever was left. The count only grows.
	if len(routes) < 200 {
		t.Fatalf("only %d protected routes found — the sweep has lost coverage", len(routes))
	}
	for _, rt := range routes {
		r := doReq(t, "", rt.method, rt.path(), nil)
		if r.Status != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401\n%s", rt.method, rt.path(), r.Status, r.Body)
		}
	}
}

// The public surface is exactly what publicAPIRoutes declares, and the routes
// that authenticate for themselves do authenticate.
//
// The second half is the part with teeth: /ws, the document and conference
// sockets and the three signed proxies are outside middleware.Auth because their
// callers cannot send a header, so nothing but their own first lines stands
// between an anonymous request and a workspace's content. Losing that check
// would look like nothing from the outside — the route would simply start
// answering.
func TestAuthzPublicSurface(t *testing.T) {
	t.Parallel()
	routes := apiRoutes(t)
	public := publicAPIRoutes()
	registered := map[string]bool{}
	for _, r := range routes {
		registered[r.key()] = true
	}
	// A stale exemption is a route that no longer exists — or worse, one that was
	// renamed and is now swept as protected under its old name while the new name
	// sits in the public group.
	for key := range public {
		if !registered[key] {
			t.Errorf("publicAPIRoutes names %q, which the router does not register", key)
		}
	}
	for _, r := range routes {
		pr, ok := public[r.key()]
		if !ok || pr.kind != publicSelfAuth {
			continue
		}
		res := doReq(t, "", r.method, r.path()+pr.query, nil)
		if res.Status != pr.wantAnon {
			t.Errorf("%s %s: anonymous request answered %d, want %d — this route authenticates for itself\n%s",
				r.method, r.path()+pr.query, res.Status, pr.wantAnon, res.Body)
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
		name, method, prefix, id, suffix  string
		body                              any
		wantForeign, wantMissing, wantBad int
	}{
		// Workspace routes authorize on the :id param directly → unknown id
		// is indistinguishable from a foreign one (403), except DELETE which
		// loads the row first (owner check) → 404 for unknown.
		{"get workspace", "GET", "/workspaces/", s.WS, "", nil, 403, 403, 400},
		{"patch workspace", "PATCH", "/workspaces/", s.WS, "", map[string]any{"name": "x"}, 403, 403, 400},
		{"workspace appearance", "PUT", "/workspaces/", s.WS, "/appearance", map[string]any{"color": "#fff"}, 403, 403, 400},
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
