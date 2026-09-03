package livekit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve stands in for livekit-server: it records the twirp method, the decoded
// request body and the Authorization header, and replies with the given JSON.
func serve(t *testing.T, reply string) (*Client, *struct {
	Method string
	Body   map[string]any
	Auth   string
}) {
	t.Helper()
	got := &struct {
		Method string
		Body   map[string]any
		Auth   string
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Method = strings.TrimPrefix(r.URL.Path, "/twirp/livekit.RoomService/")
		got.Auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return New(Config{URL: srv.URL, APIKey: testKey, APISecret: testSecret}), got
}

func TestClientIgnoresEnvironmentProxy(t *testing.T) {
	// The bug this pins (#2733, hit again here): HTTP_PROXY is set in this
	// deployment for GitLab, and the default transport would send
	// `http://livekit:7880` to a proxy that cannot resolve a compose service
	// name — it answers 502 while LiveKit's log stays empty, so a correctly
	// wired install looks like a broken SFU.
	//
	// Asserted on the transport rather than behaviourally: net/http resolves the
	// proxy environment once per process and caches it, and httptest servers
	// listen on 127.0.0.1, which the proxy lookup bypasses unconditionally.
	c := New(Config{URL: "http://livekit:7880", APIKey: testKey, APISecret: testSecret})
	tr, ok := c.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", c.http.Transport)
	}
	if tr.Proxy != nil {
		t.Fatal("RoomService calls would route through the environment proxy")
	}
}

func TestRemoveParticipantCallsTwirpWithServiceToken(t *testing.T) {
	c, got := serve(t, `{}`)
	if err := c.RemoveParticipant(context.Background(), "conf_x", "user-uuid"); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if got.Method != "RemoveParticipant" {
		t.Errorf("method = %q", got.Method)
	}
	if got.Body["room"] != "conf_x" || got.Body["identity"] != "user-uuid" {
		t.Errorf("body = %v", got.Body)
	}
	token, ok := strings.CutPrefix(got.Auth, "Bearer ")
	if !ok {
		t.Fatalf("Authorization = %q, want a bearer token", got.Auth)
	}
	video := parseToken(t, token)["video"].(map[string]any)
	if video["roomAdmin"] != true {
		t.Errorf("call authenticated without the admin grant: %v", video)
	}
}

func TestMuteTrackOnlyEverMutes(t *testing.T) {
	c, got := serve(t, `{"track":{"sid":"TR_1","muted":true}}`)
	if err := c.MuteTrack(context.Background(), "conf_x", "user-uuid", "TR_1"); err != nil {
		t.Fatalf("MuteTrack: %v", err)
	}
	if got.Method != "MutePublishedTrack" {
		t.Errorf("method = %q", got.Method)
	}
	if got.Body["trackSid"] != "TR_1" {
		t.Errorf("track sid not sent under LiveKit's field name: %v", got.Body)
	}
	// Remote unmute is not a moderation action — the server has no business
	// switching somebody's microphone back on. LiveKit blocks it unless
	// enable_remote_unmute is set, and we never send muted=false regardless.
	if got.Body["muted"] != true {
		t.Errorf("muted = %v, want true", got.Body["muted"])
	}
}

func TestListParticipantsDecodesProtojsonTimestamps(t *testing.T) {
	// twirp serialises through protojson, which renders an int64 as a JSON
	// *string*. Decoded into a plain int64 field this fails with "cannot
	// unmarshal string", and the whole participant list is lost over a
	// timestamp nobody asked for.
	c, _ := serve(t, `{"participants":[
		{"sid":"PA_1","identity":"user-uuid","name":"Иван","state":"ACTIVE","joined_at":"1756848000",
		 "tracks":[{"sid":"TR_1","type":"AUDIO","source":"MICROPHONE","muted":false}]}]}`)

	people, err := c.ListParticipants(context.Background(), "conf_x")
	if err != nil {
		t.Fatalf("ListParticipants: %v", err)
	}
	if len(people) != 1 {
		t.Fatalf("got %d participants", len(people))
	}
	if people[0].Identity != "user-uuid" {
		t.Errorf("identity = %q", people[0].Identity)
	}
	if people[0].JoinedAt.Time().Unix() != 1756848000 {
		t.Errorf("joinedAt = %v", people[0].JoinedAt.Time())
	}
	if len(people[0].Tracks) != 1 || people[0].Tracks[0].Source != "MICROPHONE" {
		t.Errorf("tracks = %+v", people[0].Tracks)
	}
}

func TestCreateRoomOmitsUnsetCeilings(t *testing.T) {
	// Reply captured from livekit-server v1.13.6 (#2877): PROTO field names, not
	// the camelCase JSON names of the same proto. Read under camelCase tags this
	// decodes without error into zeros, which is how an empty egress id once
	// reached the database — see TestEgressInfoDecodesARealLiveKitReply.
	c, got := serve(t, `{"sid":"RM_1","name":"conf_x","num_participants":2,"creation_time":"1756848000"}`)
	room, err := c.CreateRoom(context.Background(), "conf_x", RoomOptions{})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.SID != "RM_1" || room.Name != "conf_x" {
		t.Errorf("room = %+v", room)
	}
	if room.NumParticipants != 2 || room.CreationTime.Time().Unix() != 1756848000 {
		t.Errorf("room = %+v — multi-word fields read as zero", room)
	}
	// Sending zeros would override the server config (deploy/livekit.yaml) with
	// "no participants allowed, tear down immediately", which is not what an
	// unset option means.
	for _, field := range []string{"emptyTimeout", "maxParticipants"} {
		if _, present := got.Body[field]; present {
			t.Errorf("%s sent as a zero override: %v", field, got.Body)
		}
	}
}

func TestTwirpFailureSurfacesAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"not_found","msg":"requested room does not exist"}`))
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL, APIKey: testKey, APISecret: testSecret})
	err := c.DeleteRoom(context.Background(), "conf_gone")
	var lkErr *Error
	if !errors.As(err, &lkErr) {
		t.Fatalf("err = %v, want *livekit.Error", err)
	}
	// Callers lean on this to stay quiet about kicking someone who already left
	// or deleting a room that timed out on its own.
	if !lkErr.NotFound() {
		t.Errorf("NotFound() = false for %v", lkErr)
	}
	if lkErr.Message != "requested room does not exist" {
		t.Errorf("message = %q", lkErr.Message)
	}
}

func TestNonTwirpFailureKeepsRawBody(t *testing.T) {
	// What a proxy or a misrouted Caddy answers: an HTML error page, not twirp
	// JSON. Losing it would leave an operator with a bare status code.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	err := New(Config{URL: srv.URL, APIKey: testKey, APISecret: testSecret}).
		RemoveParticipant(context.Background(), "conf_x", "user-uuid")
	var lkErr *Error
	if !errors.As(err, &lkErr) {
		t.Fatalf("err = %v, want *livekit.Error", err)
	}
	if lkErr.Status != http.StatusBadGateway || !strings.Contains(lkErr.Message, "502") {
		t.Errorf("error = %v", lkErr)
	}
}

func TestDisabledClientDoesNotCallOut(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	// URL present, credentials missing — the case an install lands in when it
	// deployed the SFU but never generated the keys.
	c := New(Config{URL: srv.URL})
	if err := c.RemoveParticipant(context.Background(), "conf_x", "u"); !errors.Is(err, ErrDisabled) {
		t.Errorf("RemoveParticipant = %v, want ErrDisabled", err)
	}
	if _, err := c.ListParticipants(context.Background(), "conf_x"); !errors.Is(err, ErrDisabled) {
		t.Errorf("ListParticipants = %v, want ErrDisabled", err)
	}
	if called {
		t.Error("a disabled client still reached the server")
	}
}
