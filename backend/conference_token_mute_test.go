// A force-mute has to survive a reconnect to the SFU (#2932).
//
// The moderation flow test next door proves the durable half: a host's
// force-mute reaches the participant's row and is still there on a fresh
// connection. What it cannot show is the other half — what the *media* token
// says — because the shared harness runs without LIVEKIT_*, so the token
// endpoint answers 503 there by design.
//
// That gap is where the bug lived. Enforcement at the SFU is a RoomService call
// narrowing the participant's publish sources, and LiveKit authorises a
// participant exactly once, at the join: a reconnecting client is a new
// participant assembled from a new token, and the narrowing is gone with the
// session that carried it. Our side still showed the person muted (the flag is
// on their row), so the roster said silenced while the microphone was legal
// again — for anyone whose client does not volunteer to respect the flag.
//
// So this boots a second router with an SFU configured, asks it for a token the
// way a reconnecting browser would, and reads the grant it gets back.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"tessera/config"
	"tessera/internal/db"
	"tessera/internal/livekit"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

const (
	sfuTestKey    = "devkey"
	sfuTestSecret = "integration-test-livekit-secret!!!!"
)

// fakeSFU is a stub livekit-server for the one RPC the token endpoint makes.
// Room creation happens there on purpose (auto-create is off on the real
// server), so without an answer here the handler would fail before minting
// anything.
type fakeSFU struct{}

func (fakeSFU) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/twirp/livekit.RoomService/CreateRoom" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"sid": "RM_test", "name": "conf"})
}

// sfuServer boots a second API with LIVEKIT_* pointing at the stub. It shares
// the database and the JWT secret with the harness, so a user registered there
// is the same user here and a conference created there is the same conference.
func sfuServer(t *testing.T) *httptest.Server {
	t.Helper()
	stub := httptest.NewServer(fakeSFU{})
	t.Cleanup(stub.Close)

	cfg := &config.Config{
		JWTSecret:        "integration-test-secret-min32chars!!", // must match the harness
		UploadDir:        testUploadDir,
		EncryptionKey:    "integration-test-encryption-key",
		PublicURL:        "http://tessera.test",
		CORSOrigin:       "*",
		RateLimitEnabled: false,
		LiveKitURL:       stub.URL,
		LiveKitPublicURL: "wss://sfu.test/livekit",
		LiveKitAPIKey:    sfuTestKey,
		LiveKitAPISecret: sfuTestSecret,
	}
	hub := realtime.NewHub()
	go hub.Run()
	r, _ := newRouter(cfg, testQueries, testPool, hub, mail.New(mail.Config{}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// mediaGrant asks the SFU-enabled server for a join token and returns the video
// grant inside it, verified against the API secret the way LiveKit would.
func mediaGrant(t *testing.T, srv *httptest.Server, c *client, confID string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/conferences/"+confID+"/token", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET token: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("token: status %d", res.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	claims := jwt.MapClaims{}
	if _, err := jwt.NewParser(jwt.WithoutClaimsValidation()).
		ParseWithClaims(body.Token, claims, func(*jwt.Token) (any, error) {
			return []byte(sfuTestSecret), nil
		}); err != nil {
		t.Fatalf("token does not verify: %v", err)
	}
	video, ok := claims["video"].(map[string]any)
	if !ok {
		t.Fatalf("no video grant in %v", claims)
	}
	return video
}

// forceMute writes the flag the way the moderation enforcer does, which is the
// state a reconnecting client meets: the room that held it in memory may be
// long gone, the row is not.
func forceMute(t *testing.T, confID, userID string) {
	t.Helper()
	if _, err := testQueries.SetConferenceParticipantMuted(context.Background(),
		db.SetConferenceParticipantMutedParams{
			ConferenceID: uuid.MustParse(confID),
			UserID:       uuid.MustParse(userID),
			ForceMuted:   true,
		}); err != nil {
		t.Fatalf("force-mute: %v", err)
	}
}

// publishSources reads canPublishSources out of a video grant. Absent is the
// answer for an unrestricted participant, so "missing" is reported rather than
// failed on.
func publishSources(video map[string]any) ([]string, bool) {
	raw, ok := video["canPublishSources"].([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		out = append(out, s.(string))
	}
	return out, true
}

// TestForceMutedParticipantGetsATokenWithoutTheMicrophone: the reconnect.
func TestForceMutedParticipantGetsATokenWithoutTheMicrophone(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка с немым участником")
	mate := addMember(t, owner, s.WS)
	mate.expect(t, mate.post("/conferences/"+confID+"/join", nil), http.StatusOK)
	forceMute(t, confID, mate.UserID)

	video := mediaGrant(t, sfuServer(t), mate, confID)
	sources, restricted := publishSources(video)
	if !restricted {
		t.Fatalf("a force-muted participant's token is unrestricted (%v) — their microphone is legal again "+
			"the moment they reconnect, while the roster still shows them silenced", video)
	}
	for _, src := range sources {
		if src == livekit.SourceMicrophone {
			t.Fatalf("the token still grants the microphone: %v", sources)
		}
	}
	// A force-mute takes the microphone, not the person's camera or the
	// presentation they are giving.
	for _, want := range []string{livekit.SourceCamera, livekit.SourceScreenShare} {
		found := false
		for _, src := range sources {
			found = found || src == want
		}
		if !found {
			t.Errorf("a force-mute also took away %s: %v", want, sources)
		}
	}
}

// TestOrdinaryParticipantGetsAnUnrestrictedToken is the negative control: the
// narrowing must not leak onto everybody else. An empty canPublishSources is
// not "unrestricted" to LiveKit — it is a participant who may publish nothing —
// so the key has to be absent entirely.
func TestOrdinaryParticipantGetsAnUnrestrictedToken(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка без модерации")
	mate := addMember(t, owner, s.WS)
	mate.expect(t, mate.post("/conferences/"+confID+"/join", nil), http.StatusOK)

	video := mediaGrant(t, sfuServer(t), mate, confID)
	if video["canPublish"] != true {
		t.Fatalf("an ordinary participant may not publish: %v", video)
	}
	if got, restricted := publishSources(video); restricted {
		t.Fatalf("canPublishSources = %v on an unmoderated participant, want absent", got)
	}
}
