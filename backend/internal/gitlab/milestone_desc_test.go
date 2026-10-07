package gitlab

import (
	"encoding/json"
	"testing"
)

// The sync mirrors a GitLab milestone as a native one, and GitLab is the source
// of truth for it. The description used to be neither queried nor mapped, so
// every pull overwrote the native milestone's description with "" — including
// the one Tessera itself had just pushed (task #2968).
func TestToIssue_MilestoneDescription(t *testing.T) {
	const payload = `{
      "id":"gid://gitlab/Issue/1","iid":"7","title":"t",
      "milestone":{"id":"gid://gitlab/Milestone/41","iid":"4","title":"Sprint 1",
        "description":"Goals: ship login","state":"active",
        "startDate":"2026-08-01","dueDate":"2026-08-31","webPath":"/g/p/-/milestones/4"}
    }`
	var n issueNode
	if err := json.Unmarshal([]byte(payload), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	issue := n.toIssue("https://gitlab.example")
	if issue.MilestoneDescription != "Goals: ship login" {
		t.Fatalf("MilestoneDescription = %q, want %q", issue.MilestoneDescription, "Goals: ship login")
	}
}
