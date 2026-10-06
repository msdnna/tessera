package handlers

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// The @-autocomplete inserts the GitLab login for OAuth-linked members, so a
// quick action typed as `/assign @e.polyansky` must resolve even when that
// login matches neither the email nor the display name (#2715).
func TestMatchMember(t *testing.T) {
	id := uuid.New()
	members := []db.ListMembersRow{
		{UserID: id, Email: "eugene@corp.example", Name: "Евгений Полянский", GlUsername: "e.polyansky"},
		{UserID: uuid.New(), Email: "bob@corp.example", Name: "Bob Fox"},
	}
	for _, c := range []struct {
		name, login string
		want        uuid.UUID
	}{
		{"gitlab login", "e.polyansky", id},
		{"gitlab login, mixed case", "E.Polyansky", id},
		{"full email", "eugene@corp.example", id},
		{"email local part", "eugene", id},
		{"display name", "евгений полянский", id},
		{"unknown", "nobody", uuid.Nil},
		{"empty login must not match the loginless member", "", uuid.Nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, rivals := matchMember(members, c.login)
			if got != c.want {
				t.Fatalf("matchMember(%q) = %v want %v", c.login, got, c.want)
			}
			if len(rivals) != 0 {
				t.Fatalf("matchMember(%q) reported %d rivals, want none", c.login, len(rivals))
			}
		})
	}
}

// A mention is resolved by the CLOSEST shape, not by list order. Before #2932
// the loop ran over members and tried all four shapes on each, so the first row
// that answered to anything won — and «@ivan» went to whoever happened to sit
// higher on the roster rather than to the member whose GitLab login it is.
func TestMatchMemberPrefersTheMoreSpecificShape(t *testing.T) {
	owner := uuid.New()
	for _, c := range []struct {
		name, login string
		members     []db.ListMembersRow
		want        uuid.UUID
	}{
		{
			// The autocomplete inserts gl_username; another member's email local
			// part reads the same and is listed first.
			name: "gitlab login beats an earlier local part", login: "ivan",
			members: []db.ListMembersRow{
				{UserID: uuid.New(), Email: "ivan@corp.example", Name: "Пётр Седов"},
				{UserID: owner, Email: "i.smirnov@corp.example", Name: "Иван Смирнов", GlUsername: "ivan"},
			},
			want: owner,
		},
		{
			// A display name that reads like someone else's address local part.
			name: "local part beats an earlier display name", login: "ivan",
			members: []db.ListMembersRow{
				{UserID: uuid.New(), Email: "petr@corp.example", Name: "ivan"},
				{UserID: owner, Email: "ivan@corp.example", Name: "Иван Смирнов"},
			},
			want: owner,
		},
		{
			// The full address is the shape nothing can outrank.
			name: "full email beats an earlier display name", login: "ivan@corp.example",
			members: []db.ListMembersRow{
				{UserID: uuid.New(), Email: "petr@corp.example", Name: "ivan@corp.example"},
				{UserID: owner, Email: "ivan@corp.example", Name: "Иван Смирнов"},
			},
			want: owner,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, rivals := matchMember(c.members, c.login)
			if len(rivals) != 0 {
				t.Fatalf("unexpected rivals: %d", len(rivals))
			}
			if got != c.want {
				t.Fatalf("matchMember(%q) = %v want %v", c.login, got, c.want)
			}
		})
	}
}

// Two members answering to the same shape resolve to NOBODY. Picking one of
// them is the failure mode this replaces: the command reported the handle that
// was typed and put the task on someone else.
func TestMatchMemberRefusesAnAmbiguousHandle(t *testing.T) {
	members := []db.ListMembersRow{
		{UserID: uuid.New(), Email: "ivan.a@corp.example", Name: "Иван"},
		{UserID: uuid.New(), Email: "ivan.b@corp.example", Name: "иван"},
	}
	got, rivals := matchMember(members, "Иван")
	if got != uuid.Nil {
		t.Fatalf("matchMember resolved an ambiguous name to %v", got)
	}
	if len(rivals) != 2 {
		t.Fatalf("rivals = %d, want both candidates", len(rivals))
	}
	// The rivals are what the writer is offered to retype, so they must carry the
	// emails that tell them apart.
	labels := memberLabels(rivals)
	for _, want := range []string{"ivan.a@corp.example", "ivan.b@corp.example"} {
		if !strings.Contains(labels, want) {
			t.Fatalf("memberLabels(%q) does not name %s", labels, want)
		}
	}
}

// An ambiguous handle is not a miss: «не нашёл участника» about a name the
// workspace carries twice sends the writer looking for a typo that isn't there.
func TestResolveMentionsSeparatesAmbiguousFromMissing(t *testing.T) {
	known := uuid.New()
	members := []db.ListMembersRow{
		{UserID: known, Email: "bob@corp.example", Name: "Bob Fox"},
		{UserID: uuid.New(), Email: "ivan.a@corp.example", Name: "ivan"},
		{UserID: uuid.New(), Email: "ivan.b@corp.example", Name: "ivan"},
	}
	refs, part := resolveMentions(members, []string{"bob", "ivan", "nobody"})
	if len(refs) != 1 || refs[0].id != known {
		t.Fatalf("refs = %v, want only the unambiguous member", refs)
	}
	if len(part.missing) != 1 || part.missing[0] != "@nobody" {
		t.Fatalf("missing = %v, want only @nobody", part.missing)
	}
	if len(part.ambiguous) != 1 || !strings.Contains(part.ambiguous[0], "@ivan") {
		t.Fatalf("ambiguous = %v, want @ivan with its candidates", part.ambiguous)
	}

	// Partial success still applies, and the summary says why @ivan was skipped.
	part.done = append(part.done, "@bob")
	s := part.summary("назначен", "не найдены")
	for _, want := range []string{"@bob", "не найдены: @nobody", "неоднозначно: @ivan"} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary %q does not contain %q", s, want)
		}
	}

	// And when nothing resolved at all, both halves get their own sentence.
	err := mentionMissErr(part)
	if err == nil || !strings.Contains(err.Error(), "@nobody") || !strings.Contains(err.Error(), "неоднозначно") {
		t.Fatalf("mentionMissErr = %v, want both halves named", err)
	}
}
