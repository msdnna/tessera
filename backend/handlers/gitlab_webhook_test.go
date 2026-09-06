package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// Every branch the public receiver can take is decided by webhookDecision, so the
// whole matrix is checked here without a server: what triggers a pull, what is
// answered 204 (and must NOT look like a failure to GitLab, which disables hooks
// that keep failing), and what is rejected.
func TestWebhookDecision(t *testing.T) {
	const ours = "pamir/scrum"
	cases := []struct {
		title string
		body  string
		want  hookOutcome
	}{
		{"issue event", `{"object_kind":"issue","project":{"path_with_namespace":"pamir/scrum"},"object_attributes":{"iid":7,"action":"update"}}`, hookRelevant},
		{"work item event (newer name)", `{"object_kind":"work_item","project":{"path_with_namespace":"pamir/scrum"}}`, hookRelevant},
		{"comment on an issue", `{"object_kind":"note","project":{"path_with_namespace":"pamir/scrum"}}`, hookRelevant},
		{"confidential issue", `{"object_kind":"confidential_issue","project":{"path_with_namespace":"pamir/scrum"}}`, hookRelevant},
		{"confidential note", `{"object_kind":"confidential_note","project":{"path_with_namespace":"pamir/scrum"}}`, hookRelevant},
		{"path case differs", `{"object_kind":"issue","project":{"path_with_namespace":"Pamir/Scrum"}}`, hookRelevant},
		{"kind only in event_type", `{"event_type":"issue","project":{"path_with_namespace":"pamir/scrum"}}`, hookRelevant},
		{"no project in payload", `{"object_kind":"issue"}`, hookRelevant},

		{"push is not ours", `{"object_kind":"push","project":{"path_with_namespace":"pamir/scrum"}}`, hookIgnored},
		{"pipeline is not ours", `{"object_kind":"pipeline","project":{"path_with_namespace":"pamir/scrum"}}`, hookIgnored},
		{"merge request is not ours", `{"object_kind":"merge_request","project":{"path_with_namespace":"pamir/scrum"}}`, hookIgnored},

		{"another project's issue", `{"object_kind":"issue","project":{"path_with_namespace":"someone/else"}}`, hookForeign},

		{"not JSON", `not json at all`, hookMalformed},
		{"no kind at all", `{"project":{"path_with_namespace":"pamir/scrum"}}`, hookMalformed},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			got, reason := webhookDecision([]byte(c.body), ours)
			if got != c.want {
				t.Fatalf("webhookDecision = %v (%s), want %v", got, reason, c.want)
			}
			if reason == "" {
				t.Fatalf("no reason given for %v", got)
			}
		})
	}
}

// A mass label edit in GitLab fires dozens of deliveries for one logical change.
// The queue must collapse that burst into a single pull once it goes quiet.
func TestWebhookQueueDebouncesBurst(t *testing.T) {
	var q webhookQueue
	id := uuid.New()
	base := time.Now()

	for i := range 5 {
		q.mark(id, base.Add(time.Duration(i)*300*time.Millisecond))
	}
	last := base.Add(4 * 300 * time.Millisecond)

	// Still inside the quiet period — nothing is due yet.
	if due := q.due(last.Add(webhookDebounce-time.Millisecond), webhookDebounce, webhookMaxWait); len(due) != 0 {
		t.Fatalf("burst fired early: %d due", len(due))
	}
	due := q.due(last.Add(webhookDebounce), webhookDebounce, webhookMaxWait)
	if len(due) != 1 || due[0].id != id {
		t.Fatalf("want exactly one due integration, got %+v", due)
	}
	if got := due[0].mark.lastSeen; !got.Equal(last) {
		t.Fatalf("mark.lastSeen = %v, want the newest delivery %v", got, last)
	}
	q.clear(id, due[0].mark.lastSeen)
	if again := q.due(last.Add(time.Hour), webhookDebounce, webhookMaxWait); len(again) != 0 {
		t.Fatalf("burst synced twice: %d still due", len(again))
	}
}

// Under a continuous stream the quiet period never arrives, so maxWait has to force
// the sync — otherwise a busy project would never sync via its webhook at all.
func TestWebhookQueueMaxWaitForcesSync(t *testing.T) {
	var q webhookQueue
	id := uuid.New()
	base := time.Now()

	// The burst opens at base and never goes quiet for a full debounce.
	q.mark(id, base)
	for now := base; now.Sub(base) < webhookMaxWait; {
		now = now.Add(webhookDebounce / 2)
		q.mark(id, now)
		if now.Sub(base) >= webhookMaxWait {
			break // maxWait reached — firing here is the point of the test
		}
		if due := q.due(now, webhookDebounce, webhookMaxWait); len(due) != 0 {
			t.Fatalf("fired at %v, before maxWait elapsed", now.Sub(base))
		}
	}
	if due := q.due(base.Add(webhookMaxWait), webhookDebounce, webhookMaxWait); len(due) != 1 {
		t.Fatalf("maxWait did not force a sync: %d due", len(due))
	}
}

// A delivery that arrives while the pull is running is not covered by it. Clearing
// the mark against the timestamp the run actually covered keeps that event alive
// instead of swallowing it.
func TestWebhookQueueKeepsDeliveryArrivedDuringSync(t *testing.T) {
	var q webhookQueue
	id := uuid.New()
	base := time.Now()

	q.mark(id, base)
	due := q.due(base.Add(webhookDebounce), webhookDebounce, webhookMaxWait)
	if len(due) != 1 {
		t.Fatalf("want one due integration, got %d", len(due))
	}
	// …the sync runs, and a fresh delivery lands mid-run.
	q.mark(id, base.Add(webhookDebounce+time.Second))
	q.clear(id, due[0].mark.lastSeen)

	still := q.due(base.Add(time.Hour), webhookDebounce, webhookMaxWait)
	if len(still) != 1 {
		t.Fatalf("delivery arrived during the sync was swallowed: %d due", len(still))
	}
}

// A tick that can't start the sync (another run in flight) must not consume the
// mark, or the webhook's changes are lost until the next polling sweep.
func TestWebhookQueueDueDoesNotConsumeMark(t *testing.T) {
	var q webhookQueue
	id := uuid.New()
	base := time.Now()
	q.mark(id, base)

	at := base.Add(webhookDebounce)
	if len(q.due(at, webhookDebounce, webhookMaxWait)) != 1 {
		t.Fatal("first tick saw nothing due")
	}
	if len(q.due(at, webhookDebounce, webhookMaxWait)) != 1 {
		t.Fatal("due() consumed the mark; a skipped tick would lose the event")
	}
}
