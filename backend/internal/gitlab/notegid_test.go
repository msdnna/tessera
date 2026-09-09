package gitlab

import (
	"encoding/json"
	"testing"
)

// GraphQL names a note that lives inside a discussion "DiscussionNote", while the
// REST create path we push through stores it as "Note". Both are the same note, so
// the mapper must hand one spelling downstream — otherwise gl_note_id's ON CONFLICT
// misses and the user's own comment comes back as an uneditable copy (task #2865).
func TestToIssue_DiscussionNoteGIDIsCanonicalised(t *testing.T) {
	const payload = `{
      "id":"gid://gitlab/Issue/1","iid":"7","title":"t",
      "discussions":{"nodes":[
        {"id":"gid://gitlab/Discussion/d1","notes":{"nodes":[
          {"id":"gid://gitlab/DiscussionNote/11","body":"root","system":false,"createdAt":"2026-08-01T10:00:00Z"},
          {"id":"gid://gitlab/DiscussionNote/12","body":"reply","system":false,"createdAt":"2026-08-01T11:00:00Z"}
        ]}}
      ]}
    }`
	var n issueNode
	if err := json.Unmarshal([]byte(payload), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	issue := n.toIssue("https://gitlab.example")

	if len(issue.Notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(issue.Notes))
	}
	byBody := map[string]Note{}
	for _, nt := range issue.Notes {
		byBody[nt.Body] = nt
	}
	if got := byBody["root"].GlobalID; got != "gid://gitlab/Note/11" {
		t.Errorf("root GlobalID = %q, want gid://gitlab/Note/11", got)
	}
	if got := byBody["reply"].GlobalID; got != "gid://gitlab/Note/12" {
		t.Errorf("reply GlobalID = %q, want gid://gitlab/Note/12", got)
	}
	// The thread pointer is matched against stored gids too, so it needs the same
	// treatment — a raw DiscussionNote root would orphan every reply.
	if got := byBody["reply"].RootGID; got != "gid://gitlab/Note/11" {
		t.Errorf("reply RootGID = %q, want gid://gitlab/Note/11", got)
	}
}

func TestCanonNoteGID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"gid://gitlab/DiscussionNote/42", "gid://gitlab/Note/42"},
		{"gid://gitlab/Note/42", "gid://gitlab/Note/42"},
		{"  gid://gitlab/Note/42  ", "gid://gitlab/Note/42"},
		{"42", "gid://gitlab/Note/42"},
		// No numeric tail to trust — hand it back rather than invent an id.
		{"gid://gitlab/Note/abc", "gid://gitlab/Note/abc"},
		{"gid://gitlab/Note/0", "gid://gitlab/Note/0"},
		{"", ""},
	}
	for _, c := range cases {
		if got := CanonNoteGID(c.in); got != c.want {
			t.Errorf("CanonNoteGID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
