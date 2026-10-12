package realtime

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// Presence on a board (#3116): who is looking at a board right now. A socket
// announces the board it shows with a small client→server frame
//
//	{"type":"presence","board_id":"<uuid>"}   — "" leaves
//
// and leaves it implicitly when it closes. The hub keeps the sockets per board in
// memory (nothing is stored: presence is what is true this second) and, on every
// change, broadcasts the board's people to its workspace as
//
//	{"type":"board.presence","data":{"board_id":"…","user_ids":["…"]}}
//
// A user with two tabs on a board counts once. The board is checked against the
// socket's own workspace set through BoardResolver, so a socket cannot put itself
// on a board it cannot see.

// EventBoardPresence is the server→client event with a board's present users.
const EventBoardPresence = "board.presence"

// BoardResolver returns the workspace of a live board, or ok=false.
type BoardResolver func(boardID uuid.UUID) (workspaceID string, ok bool)

type presence struct {
	mu      sync.Mutex
	resolve BoardResolver
	boards  map[uuid.UUID]map[*Client]struct{}
	scope   map[uuid.UUID]string  // board → workspace
	at      map[*Client]uuid.UUID // socket → its board
}

func newPresence() *presence {
	return &presence{
		boards: map[uuid.UUID]map[*Client]struct{}{},
		scope:  map[uuid.UUID]string{},
		at:     map[*Client]uuid.UUID{},
	}
}

// SetBoardResolver wires the board → workspace lookup; until it is set, presence
// frames are ignored.
func (h *Hub) SetBoardResolver(r BoardResolver) {
	h.presence.mu.Lock()
	h.presence.resolve = r
	h.presence.mu.Unlock()
}

type clientFrame struct {
	Type    string `json:"type"`
	BoardID string `json:"board_id"`
}

// onFrame handles one client→server frame. Unknown frames are ignored — the
// socket stays a broadcast consumer first.
func (h *Hub) onFrame(c *Client, raw []byte) {
	var f clientFrame
	if json.Unmarshal(raw, &f) != nil || f.Type != "presence" {
		return
	}
	board := uuid.Nil
	if f.BoardID != "" {
		id, err := uuid.Parse(f.BoardID)
		if err != nil {
			return
		}
		board = id
	}
	h.setPresence(c, board)
}

// setPresence moves a socket to a board (uuid.Nil — to none) and announces the
// boards whose people changed.
func (h *Hub) setPresence(c *Client, board uuid.UUID) {
	p := h.presence
	ws := ""
	if board != uuid.Nil {
		p.mu.Lock()
		resolve := p.resolve
		p.mu.Unlock()
		if resolve == nil {
			return
		}
		var ok bool
		// Outside the lock: a DB lookup must not stall every other socket.
		if ws, ok = resolve(board); !ok || !c.canSee(ws) {
			return
		}
	}
	p.mu.Lock()
	old, had := p.at[c]
	if had && old == board {
		p.mu.Unlock()
		return
	}
	var events []Event
	if had {
		delete(p.boards[old], c)
		delete(p.at, c)
		events = append(events, p.snapshot(old))
		if len(p.boards[old]) == 0 {
			delete(p.boards, old)
			delete(p.scope, old)
		}
	}
	if board != uuid.Nil {
		if p.boards[board] == nil {
			p.boards[board] = map[*Client]struct{}{}
		}
		p.boards[board][c] = struct{}{}
		p.scope[board] = ws
		p.at[c] = board
		events = append(events, p.snapshot(board))
	}
	p.mu.Unlock()
	for _, ev := range events {
		h.Broadcast(ev)
	}
}

// snapshot is the board.presence event for a board; p.mu is held.
func (p *presence) snapshot(board uuid.UUID) Event {
	seen := map[uuid.UUID]struct{}{}
	ids := []string{}
	for c := range p.boards[board] {
		if _, dup := seen[c.userID]; dup {
			continue
		}
		seen[c.userID] = struct{}{}
		ids = append(ids, c.userID.String())
	}
	sort.Strings(ids)
	data, _ := json.Marshal(map[string]any{"board_id": board.String(), "user_ids": ids})
	return Event{Scope: p.scope[board], Type: EventBoardPresence, Data: data}
}

// BoardPresence returns who is on a board now (tests, a late joiner's GET).
func (h *Hub) BoardPresence(board uuid.UUID) []string {
	p := h.presence
	p.mu.Lock()
	defer p.mu.Unlock()
	var out struct {
		UserIDs []string `json:"user_ids"`
	}
	_ = json.Unmarshal(p.snapshot(board).Data, &out)
	return out.UserIDs
}
