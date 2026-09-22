package tools

import (
	"strings"
	"testing"

	"tessera-mcp/internal/model"
)

func TestCollectImages(t *testing.T) {
	desc := "intro ![diagram](/api/uploads/a.png) more"
	comments := []model.Comment{
		{Body: "see ![shot](/api/uploads/b.jpg) here"},
		{Body: "dup ![again](/api/uploads/a.png)"}, // same ref as description → deduped
		{Body: "no image here"},
	}
	attachments := []model.Attachment{
		{ID: "att1", Filename: "spec.pdf", ContentType: "application/pdf"}, // not an image
		{ID: "att2", Filename: "screen.png", ContentType: "image/png"},
		{ID: "att3", Filename: "diagram.SVG", ContentType: ""}, // image by extension
	}

	got := collectImages(desc, comments, attachments)

	want := []imageRefOut{
		{Ref: "/api/uploads/a.png", Source: "description", Label: "diagram"},
		{Ref: "/api/uploads/b.jpg", Source: "comment", Label: "shot"},
		{Ref: "attachment:att2", Source: "attachment", Label: "screen.png"},
		{Ref: "attachment:att3", Source: "attachment", Label: "diagram.SVG"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d images, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("image[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestMatchMember(t *testing.T) {
	members := []model.Member{
		{UserID: "u-alice", Email: "alice@example.com", Name: "Alice"},
		{UserID: "u-bob", Email: "bob@example.com", Name: "Bob Jones"},
	}
	ok := []struct{ ref, want string }{
		{"alice@example.com", "u-alice"},
		{"ALICE@EXAMPLE.COM", "u-alice"},
		{"Bob Jones", "u-bob"},
		{"  bob jones ", "u-bob"},
	}
	for _, tc := range ok {
		got, err := matchMember(tc.ref, members)
		if err != nil || got != tc.want {
			t.Errorf("matchMember(%q) = %q,%v; want %q,nil", tc.ref, got, err, tc.want)
		}
	}
	if _, err := matchMember("nobody", members); err == nil {
		t.Error("matchMember(nobody) expected error")
	}
}

// Two people in one workspace share a display name — the ordinary case, not the
// exotic one. Picking the first row silently assigns the task to whichever of
// them the members list happened to return first.
func TestMatchMemberRefusesAnAmbiguousName(t *testing.T) {
	members := []model.Member{
		{UserID: "u-ivan-1", Email: "ivan.petrov@example.com", Name: "Иван Петров"},
		{UserID: "u-ivan-2", Email: "i.petrov@example.com", Name: "Иван Петров"},
	}

	id, err := matchMember("Иван Петров", members)
	if err == nil {
		t.Fatalf("matchMember on a shared name returned %q; want an error naming both candidates", id)
	}
	for _, want := range []string{"ivan.petrov@example.com", "i.petrov@example.com"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not offer %q as a disambiguator", err, want)
		}
	}

	// The email still pins each of them down — that is the way out the error offers.
	for _, tc := range []struct{ ref, want string }{
		{"ivan.petrov@example.com", "u-ivan-1"},
		{"I.Petrov@Example.com", "u-ivan-2"},
	} {
		got, mErr := matchMember(tc.ref, members)
		if mErr != nil || got != tc.want {
			t.Errorf("matchMember(%q) = %q,%v; want %q,nil", tc.ref, got, mErr, tc.want)
		}
	}
}

// An email is unique per account; a display name is free text. When one member's
// name reads like another member's address, the address's owner must win however
// the list is ordered.
func TestMatchMemberPrefersTheEmailOverALookalikeName(t *testing.T) {
	impostorFirst := []model.Member{
		{UserID: "u-impostor", Email: "mallory@example.com", Name: "alice@example.com"},
		{UserID: "u-alice", Email: "alice@example.com", Name: "Alice"},
	}
	if got, err := matchMember("alice@example.com", impostorFirst); err != nil || got != "u-alice" {
		t.Errorf("matchMember = %q,%v; want the address owner u-alice,nil", got, err)
	}

	// Same set, opposite order: the answer must not depend on it.
	ownerFirst := []model.Member{impostorFirst[1], impostorFirst[0]}
	if got, err := matchMember("alice@example.com", ownerFirst); err != nil || got != "u-alice" {
		t.Errorf("matchMember (reordered) = %q,%v; want u-alice,nil", got, err)
	}
}

func TestCommentAuthor(t *testing.T) {
	name := "Alice"
	blank := "   "
	cases := []struct {
		cm   model.Comment
		want string
	}{
		{model.Comment{AuthorName: &name}, "Alice"},
		{model.Comment{AuthorName: &blank, GlAuthorName: "gl-name"}, "gl-name"},
		{model.Comment{GlAuthorLogin: "gl-login"}, "gl-login"},
		{model.Comment{}, ""},
	}
	for i, tc := range cases {
		if got := commentAuthor(tc.cm); got != tc.want {
			t.Errorf("case %d: commentAuthor = %q, want %q", i, got, tc.want)
		}
	}
}
