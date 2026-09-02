package confroom

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// drain reads every frame queued for a participant without blocking.
func drain(p *Participant) []map[string]any {
	var out []map[string]any
	for {
		select {
		case raw, ok := <-p.Out():
			if !ok {
				return out
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			out = append(out, m)
		default:
			return out
		}
	}
}

// lastOfType returns the most recent frame of a type in a participant's queue,
// or nil when none arrived.
func lastOfType(p *Participant, want string) map[string]any {
	var last map[string]any
	for _, m := range drain(p) {
		if m["type"] == want {
			last = m
		}
	}
	return last
}

// join is the fixture shorthand: a named participant of the given role, already
// in the room.
func join(rs *Rooms, confID uuid.UUID, name, role string) *Participant {
	p := NewParticipant(uuid.New(), name, role)
	rs.Join(confID, p)
	return p
}

// stageHolder names whoever is on the stage, or "" when it is free.
func stageHolder(s StateMsg) string {
	if s.Stage == nil {
		return ""
	}
	return s.Stage.Name
}

// queueNames lists the waiting line in order.
func queueNames(s StateMsg) []string {
	out := make([]string, 0, len(s.Queue))
	for _, q := range s.Queue {
		out = append(out, q.Name)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestJoinWelcomesAndCountsTabs covers the two things every other test leans
// on: a fresh participant learns which connection it is, and one person with
// two tabs open is one person in the room, not two.
func TestJoinWelcomesAndCountsTabs(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	host := join(rs, confID, "Хост", RoleHost)

	welcome := lastOfType(host, TypeWelcome)
	if welcome == nil || welcome["conn_id"] != host.ID.String() || welcome["role"] != RoleHost {
		t.Fatalf("welcome frame is wrong: %#v", welcome)
	}
	if welcome["stage_ttl_ms"] != float64(StageTTL.Milliseconds()) {
		t.Fatalf("welcome carries stage_ttl_ms=%v, want %d", welcome["stage_ttl_ms"], StageTTL.Milliseconds())
	}

	// Same user, second tab.
	second := NewParticipant(host.UserID, "Хост", RoleHost)
	rs.Join(confID, second)

	state := rs.rooms[confID].State(time.Now())
	if len(state.Participants) != 1 {
		t.Fatalf("two tabs of one person showed as %d participants", len(state.Participants))
	}
	if state.Participants[0].Conns != 2 {
		t.Fatalf("conns = %d, want 2", state.Participants[0].Conns)
	}
}

// TestMediaFlagsAggregateOverTabs guards the tile badge: a microphone live in
// any of a person's tabs means the person is audible, and a state that said
// otherwise would paint a muted badge over someone who is talking.
func TestMediaFlagsAggregateOverTabs(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	first := join(rs, confID, "Ира", RoleMember)
	second := NewParticipant(first.UserID, "Ира", RoleMember)
	rs.Join(confID, second)
	room := rs.rooms[confID]

	room.SetMedia(second, true, false)
	got := room.State(time.Now()).Participants[0]
	if !got.Mic || got.Cam {
		t.Fatalf("mic/cam = %v/%v, want true/false", got.Mic, got.Cam)
	}
}

// TestStageQueueOrderAndHostPriority is the arbitration rule from the task:
// members queue in the order they asked, and a host jumps ahead of them but not
// ahead of another host.
func TestStageQueueOrderAndHostPriority(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	presenter := join(rs, confID, "Первый", RoleMember)
	second := join(rs, confID, "Второй", RoleMember)
	third := join(rs, confID, "Третий", RoleMember)
	host := join(rs, confID, "Хост", RoleHost)
	r := rs.rooms[confID]
	now := time.Now()

	if !r.RequestScreen(presenter, now) {
		t.Fatal("the first request for a free stage was not granted")
	}
	if r.RequestScreen(second, now) || r.RequestScreen(third, now) {
		t.Fatal("a busy stage was handed to a second presenter")
	}

	state := r.State(now)
	if stageHolder(state) != "Первый" {
		t.Fatalf("stage held by %q, want Первый", stageHolder(state))
	}
	if !eq(queueNames(state), []string{"Второй", "Третий"}) {
		t.Fatalf("queue = %v, want [Второй Третий]", queueNames(state))
	}

	// A host asking while a *member* presents takes over at once, and the
	// interrupted member goes back to the head of the line — interrupted, not
	// passed over.
	if !r.RequestScreen(host, now) {
		t.Fatal("a host did not preempt a member on the stage")
	}
	state = r.State(now)
	if stageHolder(state) != "Хост" {
		t.Fatalf("stage held by %q after the host asked, want Хост", stageHolder(state))
	}
	if !eq(queueNames(state), []string{"Первый", "Второй", "Третий"}) {
		t.Fatalf("queue after preemption = %v, want [Первый Второй Третий]", queueNames(state))
	}

	// A second host waits behind the first, but ahead of every member.
	other := join(rs, confID, "Второй хост", RoleHost)
	if r.RequestScreen(other, now) {
		t.Fatal("a host preempted another host")
	}
	if got := queueNames(r.State(now)); !eq(got, []string{"Второй хост", "Первый", "Второй", "Третий"}) {
		t.Fatalf("queue with a waiting host = %v", got)
	}

	// Releasing hands the stage to the head of the queue rather than freeing it.
	r.ReleaseScreen(host, now)
	if got := stageHolder(r.State(now)); got != "Второй хост" {
		t.Fatalf("stage after release went to %q, want Второй хост", got)
	}
}

// TestStageExpiresWithoutRefresh covers the client that dies without closing
// its socket: without the TTL the stage would be parked forever and everyone
// behind it stuck.
func TestStageExpiresWithoutRefresh(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	presenter := join(rs, confID, "Первый", RoleMember)
	waiting := join(rs, confID, "Второй", RoleMember)
	r := rs.rooms[confID]
	now := time.Now()

	r.RequestScreen(presenter, now)
	r.RequestScreen(waiting, now)

	// A refresh inside the window keeps it.
	r.RefreshScreen(presenter, now.Add(StageTTL/2))
	if got := stageHolder(r.State(now.Add(StageTTL - time.Second))); got != "Первый" {
		t.Fatalf("a refreshed stage was taken away: holder %q", got)
	}
	// Someone else's refresh must not keep a dead presenter alive.
	r.RefreshScreen(waiting, now.Add(StageTTL))
	if got := stageHolder(r.State(now.Add(2 * StageTTL))); got != "Второй" {
		t.Fatalf("stage after the TTL elapsed = %q, want Второй", got)
	}
}

// TestLeaveReleasesStage — closing the tab has to free the stage immediately,
// or every accidental refresh costs the room half a minute of dead screen.
func TestLeaveReleasesStage(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	presenter := join(rs, confID, "Первый", RoleMember)
	waiting := join(rs, confID, "Второй", RoleMember)
	r := rs.rooms[confID]
	now := time.Now()

	r.RequestScreen(presenter, now)
	r.RequestScreen(waiting, now)
	rs.Leave(confID, presenter)

	state := r.State(now)
	if stageHolder(state) != "Второй" {
		t.Fatalf("stage after the presenter left = %q, want Второй", stageHolder(state))
	}
	if len(state.Queue) != 0 {
		t.Fatalf("queue after promotion = %v, want empty", queueNames(state))
	}
}

// TestModerationRequiresHost is the security half of the room: the server is
// the only arbiter, so a member's kick and force-mute must be refused *here*
// and answered, not merely hidden in the UI.
func TestModerationRequiresHost(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	host := join(rs, confID, "Хост", RoleHost)
	member := join(rs, confID, "Участник", RoleMember)
	victim := join(rs, confID, "Жертва", RoleMember)
	r := rs.rooms[confID]
	now := time.Now()

	if r.Kick(member, victim.UserID, now) {
		t.Fatal("a plain member kicked someone")
	}
	if denied := lastOfType(member, TypeDenied); denied == nil || denied["action"] != TypeKick {
		t.Fatalf("the refused kick was not answered: %#v", denied)
	}
	if r.SetMuted(member, victim.UserID, true) {
		t.Fatal("a plain member force-muted someone")
	}
	if r.Kick(host, host.UserID, now) {
		t.Fatal("a host kicked themselves")
	}
	// Hosts must not be able to eject each other — that is how a call ends up
	// with nobody able to run it.
	other := join(rs, confID, "Второй хост", RoleHost)
	if r.Kick(host, other.UserID, now) {
		t.Fatal("a host kicked another host")
	}

	if !r.Kick(host, member.UserID, now) {
		t.Fatal("a host could not kick a member")
	}
	if r.Size() != 3 {
		t.Fatalf("after the kick the room holds %d sockets, want 3", r.Size())
	}

	// The kicked socket is closed — with the reason delivered first, because a
	// client that just goes quiet cannot tell a kick from a network drop — and
	// its user is kept out for the cooldown, so the button means something
	// before #2872 makes it durable.
	frames := drain(member)
	if len(frames) == 0 || frames[len(frames)-1]["type"] != TypeEnded {
		t.Fatalf("the kicked participant was not told why: %#v", frames)
	}
	if _, ok := <-member.Out(); ok {
		t.Fatal("a kicked participant's socket was left open")
	}
	if _, kicked := rs.KickedUntil(confID, member.UserID); !kicked {
		t.Fatal("a kicked user may reconnect immediately")
	}
	if _, kicked := rs.KickedUntil(confID, victim.UserID); kicked {
		t.Fatal("the cooldown hit the wrong user")
	}
}

// TestForceMuteSticksThroughAReport is the point of persisting the flag in the
// room at all: a muted participant whose client reports a live microphone must
// still show as muted, or the mute button is a suggestion.
func TestForceMuteSticksThroughAReport(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	host := join(rs, confID, "Хост", RoleHost)
	member := join(rs, confID, "Участник", RoleMember)
	r := rs.rooms[confID]

	r.SetMedia(member, true, true)
	if !r.SetMuted(host, member.UserID, true) {
		t.Fatal("a host could not force-mute a member")
	}
	if !r.Muted(member.UserID) {
		t.Fatal("the force-mute flag was not recorded")
	}
	r.SetMedia(member, true, true) // the client insists it is unmuted
	for _, v := range r.State(time.Now()).Participants {
		if v.UserID == member.UserID.String() {
			if v.Mic {
				t.Fatal("a force-muted participant reported a live microphone")
			}
			if !v.ForceMuted {
				t.Fatal("the snapshot does not carry force_muted")
			}
			if !v.Cam {
				t.Fatal("force-muting the microphone also turned the camera off")
			}
		}
	}
	if !r.SetMuted(host, member.UserID, false) || r.Muted(member.UserID) {
		t.Fatal("unmuting did not clear the flag")
	}
}

// TestHandsKeepTheirOrder — the raise timestamp is what orders "wants to
// speak", so re-raising an already raised hand must not send someone to the
// back of the line on a reconnect.
func TestHandsKeepTheirOrder(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	p := join(rs, confID, "Участник", RoleMember)
	r := rs.rooms[confID]
	now := time.Now()

	r.RaiseHand(p, true, now)
	r.RaiseHand(p, true, now.Add(time.Minute))
	got := r.State(now).Participants[0]
	if got.HandAt == nil || !got.HandAt.Equal(now) {
		t.Fatalf("hand_at = %v, want the first raise %v", got.HandAt, now)
	}
	r.RaiseHand(p, false, now.Add(2*time.Minute))
	if r.State(now).Participants[0].HandAt != nil {
		t.Fatal("lowering the hand left it up")
	}
}

// TestDropTellsTheRoomWhy — a call that just goes quiet is indistinguishable
// from a network drop, and the client reconnects in a loop.
func TestDropTellsTheRoomWhy(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	p := join(rs, confID, "Участник", RoleMember)

	rs.Drop(confID, "ended")
	if ended := lastOfType(p, TypeEnded); ended == nil || ended["reason"] != "ended" {
		t.Fatalf("no ended frame before the close: %#v", ended)
	}
	if _, ok := <-p.Out(); ok {
		t.Fatal("the socket was left open after the conference ended")
	}
	if rs.Count() != 0 {
		t.Fatalf("the dropped room is still registered (%d)", rs.Count())
	}
}

// TestEmptyRoomSurvivesOnlyItsCooldown: forgetting the room is the same as
// forgetting the kick, but keeping it after the cooldown elapses is a leak of
// one entry per conference ever opened.
func TestEmptyRoomSurvivesOnlyItsCooldown(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	host := join(rs, confID, "Хост", RoleHost)
	member := join(rs, confID, "Участник", RoleMember)
	now := time.Now()

	rs.rooms[confID].Kick(host, member.UserID, now)
	rs.Leave(confID, host)
	if rs.Count() != 1 {
		t.Fatal("the room was forgotten while a kick cooldown was still running")
	}
	rs.SweepAll(now.Add(KickCooldown + time.Second))
	if rs.Count() != 0 {
		t.Fatalf("an empty room outlived its cooldown (%d rooms)", rs.Count())
	}

	// And an ordinary empty room goes away at once.
	solo := join(rs, confID, "Один", RoleMember)
	rs.Leave(confID, solo)
	if rs.Count() != 0 {
		t.Fatalf("an empty room without cooldowns was kept (%d rooms)", rs.Count())
	}
}

// TestSlowParticipantIsEvicted holds the delivery contract: a dropped frame
// would leave this client's view of the stage frozen, so the room closes it and
// makes it reconnect to a fresh snapshot instead.
func TestSlowParticipantIsEvicted(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	p := join(rs, confID, "Молчун", RoleMember)
	r := rs.rooms[confID]
	now := time.Now()

	// Nobody is reading p.Out(); enough state changes overflow the buffer.
	for i := 0; i < sendBuffer*2; i++ {
		r.RaiseHand(p, i%2 == 0, now.Add(time.Duration(i)*time.Second))
	}
	drain(p)
	select {
	case _, ok := <-p.Out():
		if ok {
			t.Fatal("the slow participant is still receiving frames")
		}
	default:
		t.Fatal("the slow participant was not evicted")
	}
}
