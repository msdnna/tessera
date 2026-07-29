package tools

import (
	"encoding/json"
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

func TestCleanRecurrence(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" means nil expected
	}{
		{``, ""},
		{`null`, ""},
		{`  null `, ""},
		{`{"freq":"weekly"}`, `{"freq":"weekly"}`},
	}
	for _, tc := range cases {
		got := cleanRecurrence(json.RawMessage(tc.in))
		if tc.want == "" {
			if got != nil {
				t.Errorf("cleanRecurrence(%q) = %q, want nil", tc.in, got)
			}
			continue
		}
		if string(got) != tc.want {
			t.Errorf("cleanRecurrence(%q) = %q, want %q", tc.in, got, tc.want)
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
