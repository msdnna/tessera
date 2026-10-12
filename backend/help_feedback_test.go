package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// «Статья помогла?» (#3071): a vote is a per-user acknowledgement keyed
// helpful:<slug>:<lang>:yes|no, and the administration reads the totals back
// from GET /admin/help-feedback — summed per article and language, admins only.
func TestHelpFeedbackTotals(t *testing.T) {
	t.Parallel()
	// The table is shared with every other parallel test, so the article is
	// this run's own.
	slug := fmt.Sprintf("hf-%d", time.Now().UnixNano())
	vote := func(c *client, lang, v string) {
		t.Helper()
		c.expect(t, c.post("/users/me/acknowledgements", map[string]any{
			"key": "helpful:" + slug + ":" + lang + ":" + v,
		}), http.StatusOK)
	}

	a, b, d := signup(t), signup(t), signup(t)
	vote(a, "ru", "yes")
	vote(b, "ru", "yes")
	vote(d, "ru", "no")
	vote(a, "en", "no")
	// A second press of the same answer is the same acknowledgement — one vote.
	vote(a, "ru", "yes")
	// Not a vote: the totals skip keys that do not end in yes|no.
	a.expect(t, a.post("/users/me/acknowledgements", map[string]any{
		"key": "helpful:" + slug + ":ru:maybe",
	}), http.StatusOK)

	// A plain member is refused. Demoted explicitly: on a fresh database the
	// very first signup of the run becomes the instance admin.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = FALSE WHERE id = $1::uuid`, a.UserID); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if r := a.get("/admin/help-feedback"); r.Status != http.StatusForbidden {
		t.Fatalf("non-admin: want 403, got %d\n%s", r.Status, r.Body)
	}

	adm := signup(t)
	makeAdmin(t, adm)
	r := adm.get("/admin/help-feedback")
	if r.Status != http.StatusOK {
		t.Fatalf("admin: %d\n%s", r.Status, r.Body)
	}
	rows := r.listBody(t)
	got := map[string][2]float64{}
	for _, r := range rows {
		if r["slug"] != slug {
			continue
		}
		lang, _ := r["lang"].(string)
		yes, _ := r["yes"].(float64)
		no, _ := r["no"].(float64)
		got[lang] = [2]float64{yes, no}
	}
	if got["ru"] != [2]float64{2, 1} || got["en"] != [2]float64{0, 1} || len(got) != 2 {
		t.Fatalf("totals for %s: %v", slug, got)
	}
}
