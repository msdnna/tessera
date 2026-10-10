// The page «Все уведомления» (#3049): the feed read backwards from a keyset
// cursor, with the filter applied by the server, and the unread count per
// filter for the counters on the feed's tabs.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"tessera/internal/db"
)

type notifPage struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

func getNotifPage(t *testing.T, c *client, query string) notifPage {
	t.Helper()
	r := c.get("/notifications/page" + query)
	if r.Status != http.StatusOK {
		t.Fatalf("GET /notifications/page%s = %d: %s", query, r.Status, r.Body)
	}
	var p notifPage
	if err := json.Unmarshal(r.Body, &p); err != nil {
		t.Fatalf("decode page: %v\n%s", err, r.Body)
	}
	return p
}

// seedNotification writes a notification row straight into the table: what the
// page reads is the row, not how it came to be.
func seedNotification(t *testing.T, userID, kind, text string) uuid.UUID {
	t.Helper()
	n, err := testQueries.CreateNotification(context.Background(), db.CreateNotificationParams{
		UserID: uuid.MustParse(userID), Kind: kind, Text: text, Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	return n.ID
}

func TestNotificationsPageWalksTheWholeFeedOnce(t *testing.T) {
	t.Parallel()
	c := signup(t)
	kinds := []string{"assigned", "mention", "comment", "mention", "integration_sync", "assigned", "integration_conflict"}
	var ids []uuid.UUID
	for i, k := range kinds {
		ids = append(ids, seedNotification(t, c.UserID, k, k+string(rune('a'+i))))
	}

	t.Run("pages of three, newest first, each row once", func(t *testing.T) {
		seen := map[string]bool{}
		var order []string
		query := "?limit=3"
		for pages := 0; ; pages++ {
			if pages > 5 {
				t.Fatal("the cursor never ran out")
			}
			p := getNotifPage(t, c, query)
			for _, n := range p.Items {
				id := n["id"].(string)
				if seen[id] {
					t.Fatalf("row %s came twice", id)
				}
				seen[id] = true
				order = append(order, id)
			}
			if p.NextCursor == nil {
				break
			}
			if len(p.Items) != 3 {
				t.Fatalf("a page with a next cursor holds %d rows, want 3", len(p.Items))
			}
			query = "?limit=3&cursor=" + url.QueryEscape(*p.NextCursor)
		}
		if len(order) != len(ids) {
			t.Fatalf("walked %d rows, want %d", len(order), len(ids))
		}
		for i, id := range order {
			if want := ids[len(ids)-1-i].String(); id != want {
				t.Fatalf("row %d = %s, want %s (newest first)", i, id, want)
			}
		}
	})

	t.Run("the filter is the server's", func(t *testing.T) {
		p := getNotifPage(t, c, "?filter=mentions&limit=1")
		if len(p.Items) != 1 || p.Items[0]["kind"] != "mention" || p.NextCursor == nil {
			t.Fatalf("first page of mentions = %v (next %v)", p.Items, p.NextCursor)
		}
		p = getNotifPage(t, c, "?filter=mentions&limit=1&cursor="+url.QueryEscape(*p.NextCursor))
		if len(p.Items) != 1 || p.Items[0]["kind"] != "mention" || p.NextCursor != nil {
			t.Fatalf("second page of mentions = %v (next %v)", p.Items, p.NextCursor)
		}
		gl := getNotifPage(t, c, "?filter=gitlab")
		if len(gl.Items) != 2 {
			t.Fatalf("gitlab filter = %d rows, want the sync report and the conflict", len(gl.Items))
		}
	})

	t.Run("unread counts per filter, and the read ones drop out of «unread»", func(t *testing.T) {
		cnt := c.expect(t, c.get("/notifications/unread-count"), http.StatusOK)
		if cnt["count"] != float64(len(kinds)) {
			t.Fatalf("count = %v, want %d", cnt["count"], len(kinds))
		}
		g, _ := cnt["by_group"].(map[string]any)
		if g["mentions"] != float64(2) || g["assigned"] != float64(2) || g["gitlab"] != float64(2) {
			t.Fatalf("by_group = %v", g)
		}
		c.expect(t, c.post("/notifications/"+ids[1].String()+"/read", nil), http.StatusNoContent)
		u := getNotifPage(t, c, "?filter=unread&limit=100")
		if len(u.Items) != len(kinds)-1 {
			t.Fatalf("unread = %d rows, want %d", len(u.Items), len(kinds)-1)
		}
		for _, n := range u.Items {
			if n["id"] == ids[1].String() {
				t.Fatal("a read row is still under «unread»")
			}
		}
	})

	t.Run("someone else's rows are not on the page", func(t *testing.T) {
		other := signup(t)
		if p := getNotifPage(t, other, ""); len(p.Items) != 0 {
			t.Fatalf("a fresh user sees %d rows", len(p.Items))
		}
	})
}

func TestNotificationsPageRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	c := signup(t)
	for _, q := range []string{"?filter=everything", "?cursor=nope", "?cursor=123_not-a-uuid", "?limit=0", "?limit=x"} {
		if r := c.get("/notifications/page" + q); r.Status != http.StatusBadRequest {
			t.Fatalf("GET /notifications/page%s = %d, want 400", q, r.Status)
		}
	}
	// The bare list keeps its shape for the clients that read it.
	seedNotification(t, c.UserID, "assigned", "x")
	if l := c.get("/notifications").listBody(t); len(l) != 1 {
		t.Fatalf("GET /notifications = %d rows, want 1", len(l))
	}
}
