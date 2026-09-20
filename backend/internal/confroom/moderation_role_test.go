// What a workspace role change does to a call already in progress (#2932).
//
// The seat is resolved once, at the upgrade: conferenceSeat reads the workspace
// role and copies "may moderate" into the participant as a boolean, and nothing
// in the room ever looks at the membership again. Removal answers that with
// EvictUser — the whole seat goes. A demotion cannot: the person is still a
// member and may still attend, so the only thing that may be taken is the right
// itself, which is what SetModeration does.
//
// The rule it has to reproduce is conferenceSeat's: a workspace manager
// moderates every call in the workspace, and everybody else moderates only the
// calls they host (the creator included — they join as RoleHost). So Role is
// exactly the part of the answer that does not depend on the workspace, and a
// demotion must never reach through it.
package confroom

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// joinAsManager is the fixture for the seat conferenceSeat hands a workspace
// admin: a plain member of the call, with moderation switched on afterwards.
func joinAsManager(rs *Rooms, confID uuid.UUID, name string) *Participant {
	p := NewParticipant(uuid.New(), name, RoleMember)
	p.CanModerate = true
	rs.Join(confID, p, false)
	return p
}

// TestDemotionTakesModerationMidCall is the finding: an admin demoted to member
// while the call is running kept kicking and force-muting it, because the board
// socket is all that a role change used to close.
func TestDemotionTakesModerationMidCall(t *testing.T) {
	rs, confID := New(), uuid.New()
	defer rs.Close()
	admin := joinAsManager(rs, confID, "Админ")
	guest := join(rs, confID, "Гость", RoleMember)

	rs.SetModeration([]uuid.UUID{confID}, admin.UserID, false)

	if admin.CanModerate {
		t.Fatal("a demoted admin still holds moderation in the live call")
	}
	drain(admin)
	drain(guest)
	if rs.lookup(confID).Kick(admin, guest.UserID, time.Now()) {
		t.Fatal("a demoted admin kicked someone out of the call")
	}
	if denied := lastOfType(admin, TypeDenied); denied == nil || denied["action"] != TypeKick {
		t.Fatalf("the refused kick was not answered: %#v", denied)
	}
	if rs.lookup(confID).SetMuted(admin, guest.UserID, true) {
		t.Fatal("a demoted admin force-muted someone in the call")
	}
}

// TestDemotionTellsTheSeat: the right reaches the client in the welcome frame
// and nowhere else, so a revocation the room keeps to itself leaves the
// ex-admin's own tab painting kick and force-mute controls whose next click
// answers "not a moderator".
//
// Told to that connection alone, deliberately: CanModerate is not in the roster
// — the rest of the room never had it to correct.
func TestDemotionTellsTheSeat(t *testing.T) {
	rs, confID := New(), uuid.New()
	defer rs.Close()
	admin := joinAsManager(rs, confID, "Админ")
	guest := join(rs, confID, "Гость", RoleMember)
	drain(admin)
	drain(guest)

	rs.SetModeration([]uuid.UUID{confID}, admin.UserID, false)

	seat := lastOfType(admin, TypeSeat)
	if seat == nil {
		t.Fatal("the ex-admin was never told their seat changed")
	}
	if seat["can_moderate"] != false {
		t.Fatalf("the seat frame did not take the right: %#v", seat)
	}
	if seat["role"] != RoleMember {
		t.Fatalf("the seat frame rewrote the call role: %#v", seat)
	}
	if other := lastOfType(guest, TypeSeat); other != nil {
		t.Fatalf("somebody else's seat frame landed in the room: %#v", other)
	}
}

// TestPromotionGrantsModerationMidCall is the mirror half: the same seam that
// takes the right has to hand it over, or a promotion reaches the call only on
// the next join — and the manager who was promoted *to* deal with the meeting
// has to leave it first.
func TestPromotionGrantsModerationMidCall(t *testing.T) {
	rs, confID := New(), uuid.New()
	defer rs.Close()
	member := join(rs, confID, "Участник", RoleMember)
	guest := join(rs, confID, "Гость", RoleMember)

	rs.SetModeration([]uuid.UUID{confID}, member.UserID, true)

	if !member.CanModerate {
		t.Fatal("a promoted admin did not get moderation in the live call")
	}
	if seat := lastOfType(member, TypeSeat); seat == nil || seat["can_moderate"] != true {
		t.Fatalf("the promotion never reached the client: %#v", seat)
	}
	if !rs.lookup(confID).Kick(member, guest.UserID, time.Now()) {
		t.Fatal("a promoted admin could not moderate the call they were promoted over")
	}
}

// TestDemotionSparesTheHost: the host label is the call's own, not the
// workspace's. Whoever scheduled the meeting joins as RoleHost, and losing the
// admin badge must not leave their own meeting with nobody able to run it.
func TestDemotionSparesTheHost(t *testing.T) {
	rs, confID := New(), uuid.New()
	defer rs.Close()
	host := join(rs, confID, "Ведущий", RoleHost)
	guest := join(rs, confID, "Гость", RoleMember)

	rs.SetModeration([]uuid.UUID{confID}, host.UserID, false)

	if !host.CanModerate {
		t.Fatal("a demotion stripped the host of their own meeting")
	}
	if !rs.lookup(confID).Kick(host, guest.UserID, time.Now()) {
		t.Fatal("the host could not moderate after being demoted in the workspace")
	}
}

// TestDemotionIsScopedToTheNamedCalls: the registry is keyed by conference and
// knows nothing about workspaces, so a demotion that swept every room the way
// EvictUser does would strip the same person in another workspace's call, where
// they may still be an admin. The caller names the conferences; nothing else
// moves.
func TestDemotionIsScopedToTheNamedCalls(t *testing.T) {
	rs := New()
	defer rs.Close()
	here, elsewhere := uuid.New(), uuid.New()
	uid := uuid.New()
	mine := NewParticipant(uid, "Я", RoleMember)
	mine.CanModerate = true
	rs.Join(here, mine, false)
	other := NewParticipant(uid, "Я в чужом пространстве", RoleMember)
	other.CanModerate = true
	rs.Join(elsewhere, other, false)

	rs.SetModeration([]uuid.UUID{here}, uid, false)

	if mine.CanModerate {
		t.Fatal("the named call kept the revoked right")
	}
	if !other.CanModerate {
		t.Fatal("a demotion in one workspace reached a call in another")
	}
}

// TestModerationSyncIsQuietWhenNothingMoves: a role change that confirms the
// role the person already had says nothing. AddMember is an upsert and carries
// this call on every re-add, so "the role did not move" is the common case, not
// the odd one — and a seat frame per no-op is a client told its rights changed
// when they did not.
func TestModerationSyncIsQuietWhenNothingMoves(t *testing.T) {
	rs, confID := New(), uuid.New()
	defer rs.Close()
	member := join(rs, confID, "Участник", RoleMember)
	drain(member)

	rs.SetModeration([]uuid.UUID{confID}, member.UserID, false)

	if seat := lastOfType(member, TypeSeat); seat != nil {
		t.Fatalf("a no-op role change sent a seat frame anyway: %#v", seat)
	}
}

// TestModerationSyncSurvivesRoomsThatAreNotLive: the caller lists the
// workspace's conferences from the database, and a room exists only while
// someone is in it — so most of that list names nothing. Unknown ids, and an
// empty list, are the ordinary case, not an error.
func TestModerationSyncSurvivesRoomsThatAreNotLive(t *testing.T) {
	rs, confID := New(), uuid.New()
	defer rs.Close()
	admin := joinAsManager(rs, confID, "Админ")

	rs.SetModeration(nil, admin.UserID, false)
	if !admin.CanModerate {
		t.Fatal("an empty conference list changed a room it never named")
	}
	rs.SetModeration([]uuid.UUID{uuid.New(), confID, uuid.New()}, admin.UserID, false)
	if admin.CanModerate {
		t.Fatal("a list with dead conferences in it skipped the live one")
	}
}

// TestModerationSyncReachesEveryTab: identity in a room is the connection, not
// the person — the same admin may have the call open twice, and a revocation
// that stopped at the first tab would leave the second one moderating.
func TestModerationSyncReachesEveryTab(t *testing.T) {
	rs, confID := New(), uuid.New()
	defer rs.Close()
	uid := uuid.New()
	first := NewParticipant(uid, "Админ", RoleMember)
	first.CanModerate = true
	rs.Join(confID, first, false)
	second := NewParticipant(uid, "Админ", RoleMember)
	second.CanModerate = true
	rs.Join(confID, second, false)
	guest := join(rs, confID, "Гость", RoleMember)

	rs.SetModeration([]uuid.UUID{confID}, uid, false)

	if rs.lookup(confID).Kick(first, guest.UserID, time.Now()) {
		t.Fatal("the first tab kept moderation after the demotion")
	}
	if rs.lookup(confID).Kick(second, guest.UserID, time.Now()) {
		t.Fatal("the second tab kept moderation after the demotion")
	}
}
