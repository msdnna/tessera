// Package livekit talks to the LiveKit SFU that carries conference media
// (#2864): it mints the access tokens a browser needs to join a room, and calls
// the server's RoomService API for the things only we may decide — who gets
// removed, who gets muted for everyone, when a room is torn down.
//
// Deliberately written without LiveKit's own SDKs. Both halves of its protocol
// are things this repository already has: an access token is a plain HS256 JWT
// with one custom claim, and RoomService is JSON over HTTP (twirp). Pulling in
// the server SDK would add a proto/grpc dependency tree to a backend that needs
// neither.
//
// The security model behind every choice here: LiveKit does not know what a
// Tessera user is and must never learn. It only verifies a signature. So the
// whole permission model stays on our side and a token is a narrow, short-lived
// warrant for one room — see Grants and JoinToken.
package livekit

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrDisabled is returned by every call when no LiveKit is configured.
// Conferences are an optional part of the install, exactly like the office
// converter: a deployment without an SFU keeps working and only this feature
// reports itself unavailable.
var ErrDisabled = errors.New("livekit is not configured")

// TokenTTL bounds a join token's life. It is short on purpose: the token is
// needed only for the moment of joining — LiveKit keeps an already established
// session alive without it — so a leaked one expires long before anybody can
// pass it around.
const TokenTTL = 15 * time.Minute

// MinSecretLen mirrors livekit-server's own requirement: it refuses to start
// with an API secret shorter than this, and a token signed with a short secret
// would be rejected at join time with an error that says nothing useful.
const MinSecretLen = 32

// Grants is the permission set carried inside a join token. It is intentionally
// a small closed struct rather than a free-form map: the dangerous LiveKit
// grants (roomAdmin, roomCreate, roomList) have no field here at all, so no
// call site can hand them to a browser by accident. Room administration is done
// by our backend through RoomService, where our own role checks run first.
type Grants struct {
	// Room is the one room this token may join. Required — a video grant with
	// no room name is accepted by LiveKit as "any room".
	Room string
	// CanPublish lets the participant send audio/video. False makes a listener.
	CanPublish bool
	// CanSubscribe lets the participant receive other people's tracks.
	CanSubscribe bool
	// CanPublishData allows the data channel (used later for in-room signalling
	// that does not deserve a round trip through our WS).
	CanPublishData bool
	// Hidden keeps the participant out of everyone else's participant list —
	// for a recorder or a future observer role, not for ordinary members.
	Hidden bool
}

// videoGrant is the `video` claim LiveKit reads. The JSON names are LiveKit's,
// not ours. The three omitted-by-design grants are documented on Grants.
type videoGrant struct {
	Room           string `json:"room"`
	RoomJoin       bool   `json:"roomJoin,omitempty"`
	RoomAdmin      bool   `json:"roomAdmin,omitempty"`
	RoomCreate     bool   `json:"roomCreate,omitempty"`
	RoomList       bool   `json:"roomList,omitempty"`
	CanPublish     bool   `json:"canPublish"`
	CanSubscribe   bool   `json:"canSubscribe"`
	CanPublishData bool   `json:"canPublishData"`
	Hidden         bool   `json:"hidden,omitempty"`
}

// claims is a LiveKit token: registered JWT claims (iss = API key, sub =
// participant identity) plus the video grant and an optional display name.
type claims struct {
	jwt.RegisteredClaims
	Name  string     `json:"name,omitempty"`
	Video videoGrant `json:"video"`
}

// JoinToken mints a token letting identity join g.Room for ttl.
//
// identity is what LiveKit uses to address a participant in RoomService calls
// (RemoveParticipant, MutePublishedTrack), so it must be stable and unique per
// user — pass the Tessera user UUID, not a display name. name is cosmetic.
//
// A zero or negative ttl means TokenTTL.
func (c *Client) JoinToken(identity, name string, g Grants, ttl time.Duration) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		// An empty identity is not a harmless default: LiveKit would let two
		// such participants collide, and a RemoveParticipant aimed at one of
		// them would hit whichever the server happened to hold.
		return "", errors.New("livekit: token needs a participant identity")
	}
	if strings.TrimSpace(g.Room) == "" {
		// Without a room name the grant means "any room on this server".
		return "", errors.New("livekit: token needs a room")
	}
	if ttl <= 0 {
		ttl = TokenTTL
	}
	now := c.now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.apiKey,
			Subject:   identity,
			ID:        identity,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Name: name,
		Video: videoGrant{
			Room:           g.Room,
			RoomJoin:       true,
			CanPublish:     g.CanPublish,
			CanSubscribe:   g.CanSubscribe,
			CanPublishData: g.CanPublishData,
			Hidden:         g.Hidden,
		},
	})
	return tok.SignedString([]byte(c.apiSecret))
}

// serviceToken is the credential our own RoomService calls carry. It is the
// same JWT shape with the administrative grants turned on, and it never leaves
// the backend: it is built per request and lives for a minute.
func (c *Client) serviceToken(room string) (string, error) {
	now := c.now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.apiKey,
			Subject:   c.apiKey,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
		Video: videoGrant{
			Room:       room,
			RoomAdmin:  true,
			RoomCreate: true,
			RoomList:   true,
		},
	})
	return tok.SignedString([]byte(c.apiSecret))
}

// RoomName maps a Tessera conference id onto a LiveKit room name. Prefixed so a
// room name can never be mistaken for anything else on a server that might one
// day host more than conferences, and derived rather than free-form so a user
// cannot pick a name that collides with somebody else's meeting.
func RoomName(conferenceID fmt.Stringer) string {
	return "conf_" + conferenceID.String()
}
