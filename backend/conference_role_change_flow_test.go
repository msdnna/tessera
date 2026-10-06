// A workspace role change, over the real socket, with the call already running
// (#2932).
//
// internal/confroom pins the rule in isolation; what is only reachable from here
// is the wiring — that UpdateMemberRole reaches the conference registry at all,
// that it finds the right workspace's calls, and that the demoted person keeps
// the seat while losing only what the role gave them. The demotion is the sharp
// end: the seat copies the workspace role into a boolean at the upgrade, so
// until this was wired an ex-admin went on kicking and force-muting the meeting
// they were already in, with nothing in the workspace able to stop them short of
// removing them from it.
package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// awaitSeatFrame waits for the server to re-state this connection's rights, the
// frame a client needs before it stops painting controls it may no longer use.
func awaitSeatFrame(t *testing.T, conn *websocket.Conn, budget time.Duration) map[string]any {
	t.Helper()
	return awaitConfFrame(t, conn, "seat", budget)
}

// TestDemotedAdminLosesModerationMidCall is the finding end to end: promote,
// join, demote, and the kick that used to land must now be refused — while the
// socket itself stays up, because the person is still a member of the workspace
// and still in the meeting.
func TestDemotedAdminLosesModerationMidCall(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка, на которой разжалуют")
	mate := addMember(t, owner, s.WS)
	owner.expect(t, owner.patch("/workspaces/"+s.WS+"/members/"+mate.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)
	guest := addMember(t, owner, s.WS)

	admin, status := dialConfWS(t, mate.token, confID)
	if admin == nil {
		t.Fatalf("admin handshake = %d", status)
	}
	defer admin.Close()
	welcome := awaitConfFrame(t, admin, "welcome", 5*time.Second)
	if welcome == nil {
		t.Fatal("no welcome for the admin")
	}
	if welcome["can_moderate"] != true {
		t.Fatalf("the admin did not join as a moderator: %#v", welcome)
	}
	victim, status := dialConfWS(t, guest.token, confID)
	if victim == nil {
		t.Fatalf("guest handshake = %d", status)
	}
	defer victim.Close()
	victimWelcome := awaitConfFrame(t, victim, "welcome", 5*time.Second)
	if victimWelcome == nil {
		t.Fatal("no welcome for the guest")
	}
	victimID, _ := victimWelcome["user_id"].(string)
	if awaitState(t, admin, 5*time.Second, func(m map[string]any) bool {
		_, ok := people(m)[victimID]
		return ok
	}) == nil {
		t.Fatal("the guest never appeared in the room snapshot")
	}

	owner.expect(t, owner.patch("/workspaces/"+s.WS+"/members/"+mate.UserID,
		map[string]any{"role": "member"}), http.StatusOK)

	seat := awaitSeatFrame(t, admin, 5*time.Second)
	if seat == nil {
		t.Fatal("the demotion never reached the call")
	}
	if seat["can_moderate"] != false {
		t.Fatalf("the seat frame left the moderation right in place: %#v", seat)
	}
	if err := admin.WriteJSON(map[string]any{"type": "kick", "user_id": victimID}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if denied := awaitConfFrame(t, admin, "denied", 5*time.Second); denied == nil || denied["action"] != "kick" {
		t.Fatalf("a demoted admin's kick was not refused: %#v", denied)
	}
	// The seat survives the demotion — this is not a removal, and ending somebody's
	// meeting is not what a role change asked for.
	if awaitSocketClose(victim, 2*time.Second) {
		t.Fatal("the guest was kicked out after all")
	}
}

// TestDemotedAdminKeepsTheirSeat: the other half of the same rule, said about
// the demoted person rather than their would-be victim. They are still a member,
// so the call goes on for them — only the moderation controls go.
func TestDemotedAdminKeepsTheirSeat(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка, с которой не выгоняют")
	mate := addMember(t, owner, s.WS)
	owner.expect(t, owner.patch("/workspaces/"+s.WS+"/members/"+mate.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)

	admin, status := dialConfWS(t, mate.token, confID)
	if admin == nil {
		t.Fatalf("admin handshake = %d", status)
	}
	defer admin.Close()
	if awaitConfFrame(t, admin, "welcome", 5*time.Second) == nil {
		t.Fatal("no welcome for the admin")
	}

	owner.expect(t, owner.patch("/workspaces/"+s.WS+"/members/"+mate.UserID,
		map[string]any{"role": "member"}), http.StatusOK)

	if awaitSeatFrame(t, admin, 5*time.Second) == nil {
		t.Fatal("the demotion never reached the call")
	}
	if awaitSocketClose(admin, 2*time.Second) {
		t.Fatal("a demotion ended the demoted member's call")
	}
	if conn, status := dialConfWS(t, mate.token, confID); status != http.StatusSwitchingProtocols {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("a demoted member could not rejoin the call: status %d", status)
	} else {
		conn.Close()
	}
}

// TestPromotionReachesTheCallInProgress is the mirror: the workspace admin
// promoted *because* a meeting is going badly should not have to leave it and
// come back before they can do anything about it.
func TestPromotionReachesTheCallInProgress(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка, на которой повышают")
	mate := addMember(t, owner, s.WS)
	guest := addMember(t, owner, s.WS)

	member, status := dialConfWS(t, mate.token, confID)
	if member == nil {
		t.Fatalf("member handshake = %d", status)
	}
	defer member.Close()
	welcome := awaitConfFrame(t, member, "welcome", 5*time.Second)
	if welcome == nil {
		t.Fatal("no welcome for the member")
	}
	if welcome["can_moderate"] != false {
		t.Fatalf("a plain member joined as a moderator: %#v", welcome)
	}
	victim, status := dialConfWS(t, guest.token, confID)
	if victim == nil {
		t.Fatalf("guest handshake = %d", status)
	}
	defer victim.Close()
	victimWelcome := awaitConfFrame(t, victim, "welcome", 5*time.Second)
	if victimWelcome == nil {
		t.Fatal("no welcome for the guest")
	}
	victimID, _ := victimWelcome["user_id"].(string)
	if awaitState(t, member, 5*time.Second, func(m map[string]any) bool {
		_, ok := people(m)[victimID]
		return ok
	}) == nil {
		t.Fatal("the guest never appeared in the room snapshot")
	}

	owner.expect(t, owner.patch("/workspaces/"+s.WS+"/members/"+mate.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)

	seat := awaitSeatFrame(t, member, 5*time.Second)
	if seat == nil || seat["can_moderate"] != true {
		t.Fatalf("the promotion never reached the call: %#v", seat)
	}
	if err := member.WriteJSON(map[string]any{"type": "kick", "user_id": victimID}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if ended := awaitConfFrame(t, victim, "ended", 5*time.Second); ended == nil || ended["reason"] != "kicked" {
		t.Fatalf("the freshly promoted admin could not moderate: %#v", ended)
	}
}

// TestRoleChangeSparesOtherWorkspacesCalls: the registry is keyed by conference,
// not by workspace, so the handler has to name the calls it means. A demotion in
// one workspace must not reach a meeting the same person is running in another,
// where they may still be an admin — the wider sweep EvictUser can afford
// (membership is re-checked on reconnect) has no such safety here.
func TestRoleChangeSparesOtherWorkspacesCalls(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	here, _ := mkConference(t, owner, "Летучка, где разжалуют")
	mate := addMember(t, owner, here.WS)
	owner.expect(t, owner.patch("/workspaces/"+here.WS+"/members/"+mate.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)

	// The same person, an admin of a workspace of their own, in a call there.
	elsewhere, otherConf := mkConference(t, mate, "Своя летучка в своём пространстве")
	guest := addMember(t, mate, elsewhere.WS)

	mine, status := dialConfWS(t, mate.token, otherConf)
	if mine == nil {
		t.Fatalf("own-workspace handshake = %d", status)
	}
	defer mine.Close()
	if awaitConfFrame(t, mine, "welcome", 5*time.Second) == nil {
		t.Fatal("no welcome in the other workspace's call")
	}
	victim, status := dialConfWS(t, guest.token, otherConf)
	if victim == nil {
		t.Fatalf("guest handshake = %d", status)
	}
	defer victim.Close()
	victimWelcome := awaitConfFrame(t, victim, "welcome", 5*time.Second)
	if victimWelcome == nil {
		t.Fatal("no welcome for the guest")
	}
	victimID, _ := victimWelcome["user_id"].(string)
	if awaitState(t, mine, 5*time.Second, func(m map[string]any) bool {
		_, ok := people(m)[victimID]
		return ok
	}) == nil {
		t.Fatal("the guest never appeared in the other workspace's room")
	}

	owner.expect(t, owner.patch("/workspaces/"+here.WS+"/members/"+mate.UserID,
		map[string]any{"role": "member"}), http.StatusOK)

	// Their own meeting is untouched: they are its creator and their own
	// workspace's owner, and the demotion happened somewhere else entirely.
	if err := mine.WriteJSON(map[string]any{"type": "kick", "user_id": victimID}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	if ended := awaitConfFrame(t, victim, "ended", 5*time.Second); ended == nil || ended["reason"] != "kicked" {
		t.Fatalf("a demotion in one workspace disarmed a call in another: %#v", ended)
	}
}
