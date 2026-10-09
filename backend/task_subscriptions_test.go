// Подписки на задачу (#2963, миграция 0083): автор, исполнители и комментаторы
// подписаны неявно, кто угодно из пространства может подписаться явно, участник
// может отписаться и перестаёт слышать новости задачи, оставаясь исполнителем.
package main

import (
	"net/http"
	"testing"
)

func subscriberIDs(t *testing.T, v any) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	list, ok := v.([]any)
	if !ok {
		t.Fatalf("subscribers is not a list: %#v", v)
	}
	for _, x := range list {
		out[x.(map[string]any)["id"].(string)] = true
	}
	return out
}

func countKind(t *testing.T, c *client, kind string) int {
	t.Helper()
	n := 0
	for _, x := range c.get("/notifications").listBody(t) {
		if k, _ := x["kind"].(string); k == kind {
			n++
		}
	}
	return n
}

func TestTaskSubscriptions(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	assignee, commenter, follower := signup(t), signup(t), signup(t)
	for _, u := range []*client{assignee, commenter, follower} {
		owner.expect(t, owner.post("/workspaces/"+s.WS+"/members",
			map[string]any{"email": u.Email, "role": "member"}), http.StatusCreated)
	}
	stranger := signup(t)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Подписки")
	id := task["id"].(string)
	owner.expect(t, owner.post("/tasks/"+id+"/assignees",
		map[string]any{"user_id": assignee.UserID}), http.StatusNoContent)
	commenter.expect(t, commenter.post("/tasks/"+id+"/comments",
		map[string]any{"body": "я тут"}), http.StatusCreated)

	t.Run("implicit: author, assignee, commenter", func(t *testing.T) {
		d := owner.expect(t, owner.get("/tasks/"+id), http.StatusOK)
		got := subscriberIDs(t, d["subscribers"])
		for _, u := range []*client{owner, assignee, commenter} {
			if !got[u.UserID] {
				t.Fatalf("%s is a participant but not subscribed: %v", u.Email, got)
			}
		}
		if got[follower.UserID] || len(got) != 3 {
			t.Fatalf("unexpected subscribers: %v", got)
		}
		if d["subscribed"] != true {
			t.Fatalf("the author should see subscribed=true: %v", d["subscribed"])
		}
		if f := follower.expect(t, follower.get("/tasks/"+id), http.StatusOK); f["subscribed"] != false {
			t.Fatalf("a bystander should see subscribed=false: %v", f["subscribed"])
		}
	})

	t.Run("explicit follow brings the task's news", func(t *testing.T) {
		r := follower.expect(t, follower.put("/tasks/"+id+"/subscription",
			map[string]any{"subscribed": true}), http.StatusOK)
		if r["subscribed"] != true || !subscriberIDs(t, r["subscribers"])[follower.UserID] {
			t.Fatalf("follow: %v", r)
		}
		before := countKind(t, follower, "comment")
		owner.expect(t, owner.post("/tasks/"+id+"/comments",
			map[string]any{"body": "новость"}), http.StatusCreated)
		if countKind(t, follower, "comment") != before+1 {
			t.Fatal("a follower did not hear about a comment")
		}
	})

	t.Run("an assignee who unsubscribed hears nothing, stays assigned", func(t *testing.T) {
		r := assignee.expect(t, assignee.put("/tasks/"+id+"/subscription",
			map[string]any{"subscribed": false}), http.StatusOK)
		if r["subscribed"] != false || subscriberIDs(t, r["subscribers"])[assignee.UserID] {
			t.Fatalf("unsubscribe: %v", r)
		}
		before := countKind(t, assignee, "comment")
		owner.expect(t, owner.post("/tasks/"+id+"/comments",
			map[string]any{"body": "ещё новость"}), http.StatusCreated)
		if countKind(t, assignee, "comment") != before {
			t.Fatal("an unsubscribed assignee still hears comments")
		}
		d := owner.expect(t, owner.get("/tasks/"+id), http.StatusOK)
		assigned := false
		for _, a := range d["assignees"].([]any) {
			if a.(map[string]any)["id"] == assignee.UserID {
				assigned = true
			}
		}
		if !assigned {
			t.Fatal("unsubscribing must not unassign")
		}
	})

	t.Run("subscribing back restores the implicit participant", func(t *testing.T) {
		r := assignee.expect(t, assignee.put("/tasks/"+id+"/subscription",
			map[string]any{"subscribed": true}), http.StatusOK)
		if !subscriberIDs(t, r["subscribers"])[assignee.UserID] {
			t.Fatalf("resubscribe: %v", r)
		}
	})

	t.Run("validation and access", func(t *testing.T) {
		if r := follower.put("/tasks/"+id+"/subscription", map[string]any{}); r.Status != http.StatusBadRequest {
			t.Fatalf("missing subscribed: %d", r.Status)
		}
		if r := stranger.put("/tasks/"+id+"/subscription", map[string]any{"subscribed": true}); r.Status == http.StatusOK {
			t.Fatal("a non-member subscribed to a task of a foreign workspace")
		}
	})
}
