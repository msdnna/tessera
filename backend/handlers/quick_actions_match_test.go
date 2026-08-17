package handlers

import (
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
			got, ok := matchMember(members, c.login)
			if got != c.want || ok != (c.want != uuid.Nil) {
				t.Fatalf("matchMember(%q) = %v,%v want %v", c.login, got, ok, c.want)
			}
		})
	}
}
