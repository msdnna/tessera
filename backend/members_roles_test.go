// Members v2 (#3063): ownership transfer, leaving, access requests, member
// groups and activity — and, first of all, who may do which of these.
//
// The rights model is still the three roles of memberships.role plus "not a
// member" (the guest of the mockups has no role of its own yet, #3075: a guest
// here is an outsider). TestMembersRightsMatrix is the table: every new route ×
// owner / admin / member / outsider, each cell on a fresh workspace so no cell
// depends on what an earlier one did. The rest pin the invariants the table
// can't express: exactly one owner after a transfer, the owner never leaves, no
// route hands out a role above admin, and a stale request can't change a role.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// membersFixture is a workspace with one person in each role, a pending access
// request from an outsider and one member group.
type membersFixture struct {
	s                                  stack
	owner, admin, member, outsider     *client
	requester                          *client
	requestID, groupID, boardSlug, pSl string
}

func mkMembersFixture(t *testing.T) membersFixture {
	t.Helper()
	owner := signup(t)
	s := mkStack(t, owner)
	admin := mkAdmin(t, owner, s.WS)
	member := signup(t)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
		map[string]any{"email": member.Email, "role": "member"}), http.StatusCreated)
	outsider := signup(t)
	requester := signup(t)
	requester.expect(t, requester.post("/access-requests",
		map[string]any{"workspace_id": s.WS}), http.StatusAccepted)
	reqs := owner.get("/workspaces/" + s.WS + "/access-requests").listBody(t)
	if len(reqs) != 1 {
		t.Fatalf("ожидалась одна заявка, получено %d", len(reqs))
	}
	g := owner.expect(t, owner.post("/workspaces/"+s.WS+"/member-groups",
		map[string]any{"name": "Фронтенд", "member_ids": []string{member.UserID}}), http.StatusCreated)
	board := owner.expect(t, owner.get("/boards/"+s.Board), http.StatusOK)
	proj := owner.expect(t, owner.get("/projects/"+s.Project), http.StatusOK)
	return membersFixture{
		s: s, owner: owner, admin: admin, member: member, outsider: outsider, requester: requester,
		requestID: reqs[0]["id"].(string), groupID: g["id"].(string),
		boardSlug: board["slug"].(string), pSl: proj["slug"].(string),
	}
}

func (f membersFixture) as(role string) *client {
	switch role {
	case "owner":
		return f.owner
	case "admin":
		return f.admin
	case "member":
		return f.member
	}
	return f.outsider
}

func TestMembersRightsMatrix(t *testing.T) {
	t.Parallel()
	roles := []string{"owner", "admin", "member", "outsider"}
	type action struct {
		name string
		do   func(f membersFixture, c *client) resp
		want map[string]int
	}
	ws := func(f membersFixture) string { return "/workspaces/" + f.s.WS }
	actions := []action{
		{"list members", func(f membersFixture, c *client) resp { return c.get(ws(f) + "/members") },
			map[string]int{"owner": 200, "admin": 200, "member": 200, "outsider": 403}},
		{"transfer ownership", func(f membersFixture, c *client) resp {
			return c.post(ws(f)+"/owner", map[string]any{"user_id": f.member.UserID})
		}, map[string]int{"owner": 200, "admin": 403, "member": 403, "outsider": 403}},
		{"leave", func(f membersFixture, c *client) resp { return c.post(ws(f)+"/leave", nil) },
			map[string]int{"owner": 403, "admin": 204, "member": 204, "outsider": 403}},
		{"list access requests", func(f membersFixture, c *client) resp { return c.get(ws(f) + "/access-requests") },
			map[string]int{"owner": 200, "admin": 200, "member": 403, "outsider": 403}},
		{"approve request", func(f membersFixture, c *client) resp {
			return c.post(ws(f)+"/access-requests/"+f.requestID+"/approve", map[string]any{"role": "member"})
		}, map[string]int{"owner": 204, "admin": 204, "member": 403, "outsider": 403}},
		{"decline request", func(f membersFixture, c *client) resp {
			return c.post(ws(f)+"/access-requests/"+f.requestID+"/decline", nil)
		}, map[string]int{"owner": 204, "admin": 204, "member": 403, "outsider": 403}},
		{"request access", func(f membersFixture, c *client) resp {
			return c.post("/access-requests", map[string]any{"workspace_id": f.s.WS})
		}, map[string]int{"owner": 409, "admin": 409, "member": 409, "outsider": 202}},
		{"list groups", func(f membersFixture, c *client) resp { return c.get(ws(f) + "/member-groups") },
			map[string]int{"owner": 200, "admin": 200, "member": 200, "outsider": 403}},
		{"create group", func(f membersFixture, c *client) resp {
			return c.post(ws(f)+"/member-groups", map[string]any{"name": "Бэкенд"})
		}, map[string]int{"owner": 201, "admin": 201, "member": 403, "outsider": 403}},
		{"update group", func(f membersFixture, c *client) resp {
			return c.patch("/member-groups/"+f.groupID, map[string]any{"name": "Веб"})
		}, map[string]int{"owner": 200, "admin": 200, "member": 403, "outsider": 403}},
		{"set group members", func(f membersFixture, c *client) resp {
			return c.patch("/member-groups/"+f.groupID, map[string]any{"member_ids": []string{c.UserID}})
		}, map[string]int{"owner": 200, "admin": 200, "member": 403, "outsider": 403}},
		{"delete group", func(f membersFixture, c *client) resp { return c.del("/member-groups/" + f.groupID) },
			map[string]int{"owner": 204, "admin": 204, "member": 403, "outsider": 403}},
	}
	// A few cells at a time: 48 fresh fixtures at once exhaust the shared
	// server's connection limit, one at a time took minutes under -race in CI.
	sem := make(chan struct{}, 4)
	for _, a := range actions {
		for _, role := range roles {
			t.Run(a.name+"/"+role, func(t *testing.T) {
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
				f := mkMembersFixture(t)
				r := a.do(f, f.as(role))
				if r.Status != a.want[role] {
					t.Errorf("%s как %s: status %d, want %d\n%s", a.name, role, r.Status, a.want[role], r.Body)
				}
				// Whatever the cell did, the workspace still has exactly one owner.
				assertOneOwner(t, f.admin, f.owner, f.s.WS)
			})
		}
	}
}

// assertOneOwner counts owners in the member list, read by whoever of the two
// is still a member.
func assertOneOwner(t *testing.T, a, b *client, wsID string) {
	t.Helper()
	r := a.get("/workspaces/" + wsID + "/members")
	if r.Status != http.StatusOK {
		r = b.get("/workspaces/" + wsID + "/members")
	}
	n := 0
	for _, m := range r.listBody(t) {
		if m["role"] == "owner" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("в пространстве %d владельцев, должен быть ровно один", n)
	}
}

func notificationEvents(t *testing.T, c *client) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, n := range c.get("/notifications").listBody(t) {
		p, _ := n["payload"].(map[string]any)
		if p == nil {
			p = map[string]any{}
		}
		p["_text"] = n["text"]
		out = append(out, p)
	}
	return out
}

func hasEvent(evs []map[string]any, name string) map[string]any {
	for _, e := range evs {
		if e["event"] == name {
			return e
		}
	}
	return nil
}

func TestTransferOwnership(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	ws := "/workspaces/" + f.s.WS

	// Refusals first: nobody but a member, not to oneself, not to an outsider.
	f.owner.expect(t, f.owner.post(ws+"/owner", map[string]any{"user_id": f.owner.UserID}), http.StatusBadRequest)
	f.owner.expect(t, f.owner.post(ws+"/owner", map[string]any{"user_id": f.outsider.UserID}), http.StatusBadRequest)
	f.owner.expect(t, f.owner.post(ws+"/owner", map[string]any{}), http.StatusBadRequest)
	// An admin can't hand the workspace to themselves.
	f.admin.expect(t, f.admin.post(ws+"/owner", map[string]any{"user_id": f.admin.UserID}), http.StatusForbidden)

	f.owner.expect(t, f.owner.post(ws+"/owner", map[string]any{"user_id": f.member.UserID}), http.StatusOK)
	if got := memberRoleOf(t, f.owner, f.s.WS, f.member.UserID); got != "owner" {
		t.Fatalf("новый владелец: роль %q", got)
	}
	if got := memberRoleOf(t, f.owner, f.s.WS, f.owner.UserID); got != "admin" {
		t.Fatalf("бывший владелец: роль %q, ждали admin", got)
	}
	assertOneOwner(t, f.owner, f.member, f.s.WS)
	wsRow := f.member.expect(t, f.member.get(ws), http.StatusOK)
	if wsRow["owner_id"] != f.member.UserID {
		t.Fatalf("workspaces.owner_id = %v, ждали нового владельца", wsRow["owner_id"])
	}

	// The old owner is an admin now, and an admin's limits apply: no second
	// transfer, no demoting the new owner, no deleting the workspace.
	f.owner.expect(t, f.owner.post(ws+"/owner", map[string]any{"user_id": f.owner.UserID}), http.StatusForbidden)
	f.owner.expect(t, f.owner.patch(ws+"/members/"+f.member.UserID, map[string]any{"role": "member"}), http.StatusForbidden)
	f.owner.expect(t, f.owner.del(ws), http.StatusForbidden)
	// The old owner may now leave; the workspace keeps its (new) owner.
	f.owner.expect(t, f.owner.post(ws+"/leave", nil), http.StatusNoContent)
	assertOneOwner(t, f.member, f.admin, f.s.WS)

	ev := hasEvent(notificationEvents(t, f.member), "workspace_owner_transferred")
	if ev == nil {
		t.Fatal("новый владелец не получил уведомление о передаче")
	}
}

// No route grants owner: not AddMember, not an invitation, not an approval.
func TestNoRouteGrantsOwner(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	ws := "/workspaces/" + f.s.WS
	f.owner.expect(t, f.owner.post(ws+"/members", map[string]any{"email": f.outsider.Email, "role": "owner"}), http.StatusBadRequest)
	f.owner.expect(t, f.owner.patch(ws+"/members/"+f.member.UserID, map[string]any{"role": "owner"}), http.StatusBadRequest)
	f.owner.expect(t, f.owner.post(ws+"/invitations", map[string]any{"email": "x-owner@test.local", "role": "owner"}), http.StatusBadRequest)
	f.admin.expect(t, f.admin.post(ws+"/access-requests/"+f.requestID+"/approve", map[string]any{"role": "owner"}), http.StatusBadRequest)
	assertOneOwner(t, f.owner, f.admin, f.s.WS)
}

func TestOwnerCannotLeaveOrBeRemoved(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	ws := "/workspaces/" + f.s.WS
	f.owner.expect(t, f.owner.post(ws+"/leave", nil), http.StatusForbidden)
	f.owner.expect(t, f.owner.del(ws+"/members/"+f.owner.UserID), http.StatusForbidden)
	f.admin.expect(t, f.admin.del(ws+"/members/"+f.owner.UserID), http.StatusForbidden)
	assertOneOwner(t, f.owner, f.admin, f.s.WS)
	// A member who leaves loses access at once and drops out of the groups.
	f.member.expect(t, f.member.post(ws+"/leave", nil), http.StatusNoContent)
	f.member.expect(t, f.member.get(ws+"/members"), http.StatusForbidden)
	for _, g := range f.owner.get(ws + "/member-groups").listBody(t) {
		if ids, _ := g["member_ids"].([]any); len(ids) != 0 {
			t.Fatalf("ушедший остался в группе: %v", ids)
		}
	}
}

func TestAccessRequestFlow(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	ws := "/workspaces/" + f.s.WS
	wsName := f.owner.expect(t, f.owner.get(ws), http.StatusOK)["name"].(string)

	// By a board link (the project/board slug pair a non-member has in the URL).
	asker := signup(t)
	r := asker.post("/access-requests", map[string]any{
		"project_slug": f.pSl, "board_slug": f.boardSlug, "message": "Пустите, пожалуйста",
	})
	asker.expect(t, r, http.StatusAccepted)
	if strings.Contains(string(r.Body), wsName) {
		t.Fatalf("ответ на заявку раскрывает имя пространства: %s", r.Body)
	}
	// Idempotent: a second click is still one request.
	asker.expect(t, asker.post("/access-requests", map[string]any{"board_id": f.s.Board}), http.StatusAccepted)
	reqs := f.admin.get(ws + "/access-requests").listBody(t)
	var mine map[string]any
	n := 0
	for _, q := range reqs {
		if q["user_id"] == asker.UserID {
			mine = q
			n++
		}
	}
	if n != 1 {
		t.Fatalf("заявок от одного человека: %d, ждали 1", n)
	}
	if mine["board_name"] != "Доска" || mine["message"] != "Пустите, пожалуйста" {
		t.Fatalf("заявка без доски или сообщения: %v", mine)
	}
	// Managers are told, a plain member is not.
	if hasEvent(notificationEvents(t, f.owner), "access_requested") == nil ||
		hasEvent(notificationEvents(t, f.admin), "access_requested") == nil {
		t.Fatal("владелец и админ должны получить уведомление о заявке")
	}
	if hasEvent(notificationEvents(t, f.member), "access_requested") != nil {
		t.Fatal("участник не должен получать уведомление о заявке")
	}
	// The requester can't decide their own request.
	asker.expect(t, asker.post(ws+"/access-requests/"+mine["id"].(string)+"/approve", nil), http.StatusForbidden)

	f.admin.expect(t, f.admin.post(ws+"/access-requests/"+mine["id"].(string)+"/approve",
		map[string]any{"role": "admin"}), http.StatusNoContent)
	if got := memberRoleOf(t, f.owner, f.s.WS, asker.UserID); got != "admin" {
		t.Fatalf("одобренный: роль %q", got)
	}
	f.owner.expect(t, f.owner.post(ws+"/access-requests/"+mine["id"].(string)+"/decline", nil), http.StatusConflict)
	if hasEvent(notificationEvents(t, asker), "access_approved") == nil {
		t.Fatal("одобренный не получил уведомление")
	}

	// Decline: the author is told without the workspace's name, and can't ask
	// again right away.
	f.owner.expect(t, f.owner.post(ws+"/access-requests/"+f.requestID+"/decline", nil), http.StatusNoContent)
	ev := hasEvent(notificationEvents(t, f.requester), "access_declined")
	if ev == nil {
		t.Fatal("автор отклонённой заявки не получил уведомление")
	}
	if b, _ := json.Marshal(ev); strings.Contains(string(b), wsName) {
		t.Fatalf("отказ раскрывает имя пространства: %s", b)
	}
	f.requester.expect(t, f.requester.post("/access-requests", map[string]any{"workspace_id": f.s.WS}), http.StatusTooManyRequests)
	f.requester.expect(t, f.requester.get(ws+"/members"), http.StatusForbidden)

	// Targets that don't resolve.
	asker2 := signup(t)
	asker2.expect(t, asker2.post("/access-requests", map[string]any{"workspace_id": dummyID}), http.StatusNotFound)
	asker2.expect(t, asker2.post("/access-requests", map[string]any{"board_id": dummyID}), http.StatusNotFound)
	asker2.expect(t, asker2.post("/access-requests", map[string]any{}), http.StatusBadRequest)
}

// A manager of another workspace can't decide this workspace's requests, not
// even through their own workspace's route.
func TestAccessRequestForeignWorkspace(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	other := signup(t)
	otherWS := mkWorkspace(t, other, "Чужое")
	other.expect(t, other.post("/workspaces/"+otherWS+"/access-requests/"+f.requestID+"/approve", nil), http.StatusNotFound)
	other.expect(t, other.post("/workspaces/"+f.s.WS+"/access-requests/"+f.requestID+"/approve", nil), http.StatusForbidden)
	f.requester.expect(t, f.requester.get("/workspaces/"+f.s.WS+"/members"), http.StatusForbidden)
}

// Approving a request of someone who became a member meanwhile leaves their role
// alone — otherwise an old request is a role change by another door.
func TestStaleAccessRequestKeepsRole(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	ws := "/workspaces/" + f.s.WS
	f.owner.expect(t, f.owner.post(ws+"/members", map[string]any{"email": f.requester.Email, "role": "member"}), http.StatusCreated)
	f.admin.expect(t, f.admin.post(ws+"/access-requests/"+f.requestID+"/approve", map[string]any{"role": "admin"}), http.StatusNoContent)
	if got := memberRoleOf(t, f.owner, f.s.WS, f.requester.UserID); got != "member" {
		t.Fatalf("устаревшая заявка поменяла роль: %q", got)
	}
}

func TestMemberGroups(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	ws := "/workspaces/" + f.s.WS
	// An outsider can't be put in a group.
	f.owner.expect(t, f.owner.post(ws+"/member-groups", map[string]any{
		"name": "Инфра", "member_ids": []string{f.member.UserID, f.outsider.UserID},
	}), http.StatusBadRequest)
	f.owner.expect(t, f.owner.patch("/member-groups/"+f.groupID, map[string]any{
		"member_ids": []string{f.outsider.UserID},
	}), http.StatusBadRequest)
	// Names are unique per workspace, case-insensitively, and not empty.
	f.owner.expect(t, f.owner.post(ws+"/member-groups", map[string]any{"name": "фронтенд"}), http.StatusConflict)
	f.owner.expect(t, f.owner.post(ws+"/member-groups", map[string]any{"name": "   "}), http.StatusBadRequest)

	g := f.admin.expect(t, f.admin.patch("/member-groups/"+f.groupID, map[string]any{
		"member_ids": []string{f.member.UserID, f.admin.UserID, f.member.UserID},
	}), http.StatusOK)
	if ids, _ := g["member_ids"].([]any); len(ids) != 2 {
		t.Fatalf("участников группы %d, ждали 2: %v", len(ids), g["member_ids"])
	}
	if g["name"] != "Фронтенд" {
		t.Fatalf("PATCH без name переименовал группу: %v", g["name"])
	}
	// Removing a member takes them out of the group.
	f.owner.expect(t, f.owner.del(ws+"/members/"+f.member.UserID), http.StatusNoContent)
	for _, row := range f.owner.get(ws + "/member-groups").listBody(t) {
		for _, id := range row["member_ids"].([]any) {
			if id == f.member.UserID {
				t.Fatal("удалённый участник остался в группе")
			}
		}
	}
	// Another workspace's manager can't touch the group.
	other := signup(t)
	mkWorkspace(t, other, "Чужое")
	other.expect(t, other.patch("/member-groups/"+f.groupID, map[string]any{"name": "x"}), http.StatusForbidden)
	other.expect(t, other.del("/member-groups/"+f.groupID), http.StatusForbidden)
	other.expect(t, other.del("/member-groups/"+dummyID), http.StatusNotFound)
}

// Activity: being in the app (token issue) and acting in the workspace.
func TestMemberActivity(t *testing.T) {
	t.Parallel()
	f := mkMembersFixture(t)
	row := func(uid string) map[string]any {
		for _, m := range f.owner.get("/workspaces/" + f.s.WS + "/members").listBody(t) {
			if m["user_id"] == uid {
				return m
			}
		}
		t.Fatalf("нет %s", uid)
		return nil
	}
	if row(f.member.UserID)["last_seen_at"] == nil {
		t.Fatal("last_seen_at пуст после входа")
	}
	if v := row(f.member.UserID)["last_active_at"]; v != nil {
		t.Fatalf("last_active_at = %v до любого действия", v)
	}
	// A read is not an action.
	f.member.expect(t, f.member.get("/boards/"+f.s.Board), http.StatusOK)
	if v := row(f.member.UserID)["last_active_at"]; v != nil {
		t.Fatalf("чтение отметилось как действие: %v", v)
	}
	mkTask(t, f.member, f.s.Board, f.s.col(t, 0), "действие")
	if row(f.member.UserID)["last_active_at"] == nil {
		t.Fatal("last_active_at пуст после создания задачи")
	}
}
