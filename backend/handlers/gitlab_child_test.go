package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"tessera/internal/db"
	"tessera/internal/gitlab"
)

// Child work-item pushes (#2592). The parts worth pinning down here are the pure ones:
// which kind an attach turns into, the gate that decides whether a subtask may be
// pushed at all, and the reservation that keeps a retry from opening a second issue.

func TestChildKind(t *testing.T) {
	if got := childKind(false); got != gitlab.KindChildCreate {
		t.Errorf("childKind(unlinked) = %q, want %q", got, gitlab.KindChildCreate)
	}
	if got := childKind(true); got != gitlab.KindChildAttach {
		t.Errorf("childKind(linked) = %q, want %q", got, gitlab.KindChildAttach)
	}
	for _, k := range []string{gitlab.KindChildCreate, gitlab.KindChildAttach, gitlab.KindChildDetach} {
		if !gitlab.IsChildKind(k) {
			t.Errorf("IsChildKind(%q) = false", k)
		}
	}
	// The structural kinds must stay outside the binding vocabulary: performWriteback
	// branches on IsChildKind before it ever resolves a binding, and a trigger that
	// answered true here would skip the whole conflict/binding path.
	for _, k := range []string{gitlab.TrigColumn, gitlab.TrigCompletion, gitlab.TrigLabels, gitlab.TrigComment, "state"} {
		if gitlab.IsChildKind(k) {
			t.Errorf("IsChildKind(%q) = true, want false", k)
		}
	}
}

func TestCheckChildGate(t *testing.T) {
	grouped := &db.GitlabLink{GlIsGroup: true}
	ungrouped := &db.GitlabLink{}
	on := gitlab.Writeback{PushChildren: true}
	auto := gitlab.Writeback{PushChildren: true, AutoGroupOnChild: true}

	cases := []struct {
		name        string
		kind        string
		enabled     bool
		wb          gitlab.Writeback
		parent      *db.GitlabLink
		childLinked bool
		want        int
	}{
		{"create under grouped parent", gitlab.KindChildCreate, true, on, grouped, false, 0},
		{"attach linked child", gitlab.KindChildAttach, true, on, grouped, true, 0},
		{"detach linked child", gitlab.KindChildDetach, true, on, nil, true, 0},

		{"integration off", gitlab.KindChildCreate, false, on, grouped, false, http.StatusBadRequest},
		{"push_children off", gitlab.KindChildCreate, true, gitlab.Writeback{}, grouped, false, http.StatusBadRequest},
		{"parent not linked", gitlab.KindChildCreate, true, on, nil, false, http.StatusBadRequest},
		// The load-bearing one: a child work item under an ungrouped parent is an issue
		// the next pull detaches again, because the pull reads an ungrouped parent as
		// having no children at all.
		{"parent not grouped", gitlab.KindChildCreate, true, on, ungrouped, false, http.StatusBadRequest},
		{"parent not grouped, auto-group on", gitlab.KindChildCreate, true, auto, ungrouped, false, 0},
		// Creating for an already-linked subtask would open a second issue for one task.
		{"create for linked child", gitlab.KindChildCreate, true, on, grouped, true, http.StatusConflict},
		{"attach unlinked child", gitlab.KindChildAttach, true, on, grouped, false, http.StatusBadRequest},
		// Detach needs only the child's own issue — the parent may already be gone.
		{"detach unlinked child", gitlab.KindChildDetach, true, on, nil, false, http.StatusBadRequest},
		{"detach ignores ungrouped parent", gitlab.KindChildDetach, true, on, ungrouped, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg := checkChildGate(tc.kind, tc.enabled, tc.wb, tc.parent, tc.childLinked)
			if status != tc.want {
				t.Fatalf("checkChildGate = (%d, %q), want status %d", status, msg, tc.want)
			}
			if status != 0 && msg == "" {
				t.Error("refused without a reason")
			}
		})
	}
}

// TestReservedIssue covers the anti-duplicate reservation: the iid is written into the
// outbox payload before the link, and comes back through JSONB as a float64. Reading it
// as anything else would silently mean "nothing reserved" — and the retry would open a
// second GitLab issue for the same subtask.
func TestReservedIssue(t *testing.T) {
	var decoded map[string]any
	raw := []byte(`{"gl_iid": 42, "gl_global_id": "gid://gitlab/Issue/900", "gl_web_url": "https://gl/x/-/issues/42"}`)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	iid, gid, url := reservedIssue(decoded)
	if iid != 42 || gid != "gid://gitlab/Issue/900" || url != "https://gl/x/-/issues/42" {
		t.Fatalf("reservedIssue = (%d, %q, %q), want the reserved issue", iid, gid, url)
	}
	for _, p := range []map[string]any{
		nil,
		{},
		{"gl_iid": float64(0)},
		{"gl_iid": "42"}, // a string is not a reservation we wrote
	} {
		if got, _, _ := reservedIssue(p); got != 0 {
			t.Errorf("reservedIssue(%v) = %d, want 0", p, got)
		}
	}
}

// TestPushSummaryCoversChildKinds guards the journal: an unlabelled kind falls through
// to the raw change_kind, which is what the user would then read in the sync log.
func TestPushSummaryCoversChildKinds(t *testing.T) {
	iid := int64(7)
	for _, kind := range []string{gitlab.KindChildCreate, gitlab.KindChildAttach, gitlab.KindChildDetach} {
		got := pushSummary(kind, nil, &iid)
		if got == "Issue #7: "+kind {
			t.Errorf("pushSummary(%q) fell through to the raw kind: %q", kind, got)
		}
	}
}
