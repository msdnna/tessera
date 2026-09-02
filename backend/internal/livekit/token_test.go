package livekit

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	testKey    = "devkey"
	testSecret = "devsecret_at_least_32_characters_long"
)

func testClient() *Client {
	return New(Config{URL: "http://livekit:7880", APIKey: testKey, APISecret: testSecret})
}

// parseToken verifies the signature with the API secret — the way LiveKit
// itself would — and returns the claims as a map so a test can assert on what
// is *absent*, which a typed struct would silently fill with a zero value.
//
// Time claims are not validated here: the tests that care about them pin the
// clock to a fixed date and assert on the values directly, and a validating
// parse would then reject the token for being issued in another year rather
// than telling us anything about the token.
func parseToken(t *testing.T, token string) jwt.MapClaims {
	t.Helper()
	out := jwt.MapClaims{}
	_, err := jwt.NewParser(jwt.WithoutClaimsValidation()).
		ParseWithClaims(token, out, func(tok *jwt.Token) (any, error) {
			if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(testSecret), nil
		})
	if err != nil {
		t.Fatalf("token does not verify against the API secret: %v", err)
	}
	return out
}

func TestJoinTokenCarriesIdentityAndRoom(t *testing.T) {
	tok, err := testClient().JoinToken("user-uuid", "Иван", Grants{
		Room: "conf_x", CanPublish: true, CanSubscribe: true, CanPublishData: true,
	}, 0)
	if err != nil {
		t.Fatalf("JoinToken: %v", err)
	}
	c := parseToken(t, tok)

	// iss must be the API key: that is how LiveKit picks which secret to verify
	// with on a server configured with several keys.
	if c["iss"] != testKey {
		t.Errorf("iss = %v, want the API key", c["iss"])
	}
	// sub is the identity RoomService calls address later (kick, force-mute).
	if c["sub"] != "user-uuid" {
		t.Errorf("sub = %v, want the identity", c["sub"])
	}
	if c["name"] != "Иван" {
		t.Errorf("name = %v", c["name"])
	}
	video, ok := c["video"].(map[string]any)
	if !ok {
		t.Fatalf("video grant is %T", c["video"])
	}
	if video["room"] != "conf_x" || video["roomJoin"] != true {
		t.Errorf("video = %v, want a join grant scoped to conf_x", video)
	}
	if video["canPublish"] != true || video["canSubscribe"] != true || video["canPublishData"] != true {
		t.Errorf("publish/subscribe grants not carried: %v", video)
	}
}

func TestJoinTokenNeverGrantsAdministration(t *testing.T) {
	// The point of Grants being a closed struct: there is no call site that can
	// ask for these, and if one is ever added to videoGrant with the wrong
	// default, a participant could kick everyone else straight from the browser,
	// bypassing every role check we do server-side.
	tok, err := testClient().JoinToken("user-uuid", "", Grants{Room: "conf_x", CanSubscribe: true}, 0)
	if err != nil {
		t.Fatalf("JoinToken: %v", err)
	}
	video := parseToken(t, tok)["video"].(map[string]any)
	for _, grant := range []string{"roomAdmin", "roomCreate", "roomList"} {
		if v, present := video[grant]; present && v == true {
			t.Errorf("join token carries %s", grant)
		}
	}
}

func TestJoinTokenExpiresQuickly(t *testing.T) {
	c := testClient()
	now := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	c.nowFn = func() time.Time { return now }

	tok, err := c.JoinToken("user-uuid", "", Grants{Room: "conf_x"}, 0)
	if err != nil {
		t.Fatalf("JoinToken: %v", err)
	}
	claims := parseToken(t, tok)
	exp, err := claims.GetExpirationTime()
	if err != nil {
		t.Fatalf("exp: %v", err)
	}
	if got := exp.Sub(now); got != TokenTTL {
		t.Errorf("token lives %s, want %s", got, TokenTTL)
	}
	nbf, err := claims.GetNotBefore()
	if err != nil || !nbf.Equal(now) {
		t.Errorf("nbf = %v (err %v), want issue time", nbf, err)
	}
}

func TestJoinTokenRejectsUnscopedRequests(t *testing.T) {
	c := testClient()
	// A grant with no room means "any room on this server" to LiveKit — i.e. a
	// pass into everyone else's meeting.
	if _, err := c.JoinToken("user-uuid", "", Grants{}, 0); err == nil {
		t.Error("token minted without a room")
	}
	// Two participants with an empty identity collide, and a kick aimed at one
	// hits whichever LiveKit happens to hold.
	if _, err := c.JoinToken("  ", "", Grants{Room: "conf_x"}, 0); err == nil {
		t.Error("token minted without an identity")
	}
}

func TestDisabledClientMintsNothing(t *testing.T) {
	c := New(Config{})
	if c.Enabled() {
		t.Fatal("client with no config reports itself enabled")
	}
	if _, err := c.JoinToken("user-uuid", "", Grants{Room: "conf_x"}, 0); !errors.Is(err, ErrDisabled) {
		t.Errorf("JoinToken on a disabled client = %v, want ErrDisabled", err)
	}
	// Half a configuration is still no configuration: signing with an empty
	// secret would produce a token LiveKit rejects at join time.
	half := New(Config{URL: "http://livekit:7880", APIKey: testKey})
	if half.Enabled() {
		t.Error("client with no secret reports itself enabled")
	}
}

func TestServiceTokenIsAdministrativeAndShortLived(t *testing.T) {
	c := testClient()
	now := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	c.nowFn = func() time.Time { return now }

	tok, err := c.serviceToken("conf_x")
	if err != nil {
		t.Fatalf("serviceToken: %v", err)
	}
	claims := parseToken(t, tok)
	video := claims["video"].(map[string]any)
	if video["roomAdmin"] != true || video["roomCreate"] != true {
		t.Errorf("service token cannot administer rooms: %v", video)
	}
	if video["canPublish"] != false || video["canSubscribe"] != false {
		t.Errorf("service token asks to publish media: %v", video)
	}
	exp, _ := claims.GetExpirationTime()
	if got := exp.Sub(now); got > time.Minute {
		t.Errorf("service token lives %s — it never leaves the backend and should not outlive the call", got)
	}
}

func TestRoomNameIsDerivedFromConferenceID(t *testing.T) {
	id := uuid.MustParse("6a08b960-1e03-4a9c-9eba-710e25dbfd1c")
	if got := RoomName(id); got != "conf_6a08b960-1e03-4a9c-9eba-710e25dbfd1c" {
		t.Fatalf("RoomName = %q", got)
	}
}
