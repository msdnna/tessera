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
	"time"
)

// serveEgress stands in for the Egress half of livekit-server. Separate from
// serve() in client_test.go because the twirp prefix differs, and that prefix is
// half of what these tests are pinning.
func serveEgress(t *testing.T, status int, reply string) (*Client, *struct {
	Path string
	Body map[string]any
	Auth string
}) {
	t.Helper()
	got := &struct {
		Path string
		Body map[string]any
		Auth string
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		got.Auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return New(Config{URL: srv.URL, APIKey: testKey, APISecret: testSecret}), got
}

func TestStartEgressUsesEgressServiceAndRecordGrant(t *testing.T) {
	c, got := serveEgress(t, http.StatusOK, `{"egressId":"EG_1","roomName":"conf_x"}`)
	info, err := c.StartRoomCompositeEgress(context.Background(), "conf_x", EgressOptions{
		Filepath: "/data/uploads/conf/abc/rec-1.mp4",
	})
	if err != nil {
		t.Fatalf("StartRoomCompositeEgress: %v", err)
	}
	if got.Path != "/twirp/livekit.Egress/StartRoomCompositeEgress" {
		t.Errorf("path = %q — recording does not live on RoomService", got.Path)
	}
	if info.EgressID != "EG_1" {
		t.Errorf("egress id = %q", info.EgressID)
	}

	// livekit-server gates every egress RPC on roomRecord alone
	// (EnsureRecordPermission), and roomAdmin does not imply it: a RoomService
	// token here comes back as a permission error, not as a recording.
	token, ok := strings.CutPrefix(got.Auth, "Bearer ")
	if !ok {
		t.Fatalf("Authorization = %q, want a bearer token", got.Auth)
	}
	video := parseToken(t, token)["video"].(map[string]any)
	if video["roomRecord"] != true {
		t.Errorf("egress call authenticated without the record grant: %v", video)
	}
}

func TestStartEgressRequestShape(t *testing.T) {
	c, got := serveEgress(t, http.StatusOK, `{"egressId":"EG_1"}`)
	if _, err := c.StartRoomCompositeEgress(context.Background(), "conf_x", EgressOptions{
		Filepath: "/data/uploads/conf/abc/rec-1.mp4",
	}); err != nil {
		t.Fatalf("StartRoomCompositeEgress: %v", err)
	}
	if got.Body["roomName"] != "conf_x" {
		t.Errorf("body = %v", got.Body)
	}
	// A layout nobody chose must not be sent: an empty string is a *value* to
	// LiveKit, not an absent field, and it would replace egress's own default
	// with a template name that does not exist.
	if _, present := got.Body["layout"]; present {
		t.Errorf("empty layout sent as an override: %v", got.Body)
	}

	outputs, ok := got.Body["fileOutputs"].([]any)
	if !ok || len(outputs) != 1 {
		t.Fatalf("fileOutputs = %v — the singular `file` field is deprecated upstream", got.Body["fileOutputs"])
	}
	out := outputs[0].(map[string]any)
	if out["filepath"] != "/data/uploads/conf/abc/rec-1.mp4" || out["fileType"] != "MP4" {
		t.Errorf("output = %v", out)
	}
	// Without this the worker writes a sibling .json manifest into the volume
	// the backend serves attachments from.
	if out["disableManifest"] != true {
		t.Errorf("manifest not disabled: %v", out)
	}
}

func TestStartEgressRefusesIncompleteRequest(t *testing.T) {
	// Both of these would otherwise reach LiveKit and be accepted: an empty
	// room name matches nothing, and a missing filepath makes egress invent a
	// templated name we could never find again.
	c, _ := serveEgress(t, http.StatusOK, `{}`)
	if _, err := c.StartRoomCompositeEgress(context.Background(), "", EgressOptions{Filepath: "/x.mp4"}); err == nil {
		t.Error("started an egress with no room")
	}
	if _, err := c.StartRoomCompositeEgress(context.Background(), "conf_x", EgressOptions{}); err == nil {
		t.Error("started an egress with no filepath")
	}
}

func TestStartingEgressHasNoStatusField(t *testing.T) {
	// EGRESS_STARTING is the enum's zero value and protojson omits zero-valued
	// fields, so a just-accepted egress arrives with no "status" key at all.
	// Left raw that is an empty string no switch matches, and a poller would
	// treat a healthy recording as being in an unknown state.
	c, _ := serveEgress(t, http.StatusOK, `{"egressId":"EG_1","roomName":"conf_x"}`)
	info, err := c.StartRoomCompositeEgress(context.Background(), "conf_x", EgressOptions{Filepath: "/x.mp4"})
	if err != nil {
		t.Fatalf("StartRoomCompositeEgress: %v", err)
	}
	if info.Status != EgressStarting {
		t.Errorf("status = %q, want %q", info.Status, EgressStarting)
	}
	if info.Done() {
		t.Error("a starting egress reported itself finished")
	}
}

func TestEgressInfoDecodesProtojsonInt64s(t *testing.T) {
	// Every int64 here is quoted by protojson, and the units differ from the
	// RoomService calls next door: egress stamps times in NANOSECONDS
	// (time.Now().UnixNano()), and FileInfo.Duration is ended_at minus
	// started_at, so it is a nanosecond span rather than seconds.
	const startedNs = int64(1756848000_000000000)
	c, _ := serveEgress(t, http.StatusOK, `{"items":[{
		"egressId":"EG_1","roomName":"conf_x","status":"EGRESS_COMPLETE",
		"startedAt":"1756848000000000000","endedAt":"1756848120000000000",
		"fileResults":[{"filename":"/data/uploads/conf/abc/rec-1.mp4",
		                "size":"10485760","duration":"120000000000"}]}]}`)

	info, err := c.GetEgress(context.Background(), "EG_1")
	if err != nil {
		t.Fatalf("GetEgress: %v", err)
	}
	if info.StartedAt.Time().UnixNano() != startedNs {
		t.Errorf("startedAt = %v", info.StartedAt.Time())
	}
	if !info.Done() || !info.Succeeded() {
		t.Errorf("status %q read as unfinished", info.Status)
	}
	file := info.File()
	if file.Size != 10<<20 {
		t.Errorf("size = %d", file.Size)
	}
	if file.Length() != 2*time.Minute {
		t.Errorf("duration = %v, want 2m — nanoseconds read as some other unit", file.Length())
	}
}

func TestLimitReachedKeepsTheRecording(t *testing.T) {
	// session_limits.file_output_max_duration in deploy/egress.yaml cuts a
	// forgotten recording off at two hours. Everything written up to that point
	// is a playable file, so reading this as a failure would discard two hours
	// of a meeting over its last second.
	info := EgressInfo{Status: EgressLimitReached}
	if !info.Done() || !info.Succeeded() {
		t.Errorf("EGRESS_LIMIT_REACHED: done=%v succeeded=%v", info.Done(), info.Succeeded())
	}
	failed := EgressInfo{Status: EgressFailed}
	if !failed.Done() || failed.Succeeded() {
		t.Errorf("EGRESS_FAILED: done=%v succeeded=%v", failed.Done(), failed.Succeeded())
	}
}

func TestUnknownStatusStaysRunning(t *testing.T) {
	// A status LiveKit adds later must not read as terminal — the poller would
	// finalise a recording that is still being written, and store the size of a
	// file that is still growing.
	info := EgressInfo{Status: "EGRESS_SOMETHING_NEW"}
	if info.Done() || info.Succeeded() {
		t.Errorf("unknown status treated as finished: done=%v", info.Done())
	}
}

func TestGetEgressReportsAGoneRecording(t *testing.T) {
	// The Egress API has no by-id read: the id is a filter on ListEgress, and
	// an id the server has forgotten is an empty list rather than a 404. Left
	// as a nil-info success, a poller would keep asking about it forever.
	c, got := serveEgress(t, http.StatusOK, `{}`)
	if _, err := c.GetEgress(context.Background(), "EG_gone"); !errors.Is(err, ErrEgressGone) {
		t.Errorf("err = %v, want ErrEgressGone", err)
	}
	if got.Body["egressId"] != "EG_gone" {
		t.Errorf("id not sent as a filter: %v", got.Body)
	}
}

func TestStopEgressRecognisesAnAlreadyFinishedRecording(t *testing.T) {
	// Racing the natural end of a recording is normal: the host presses stop
	// while the room is already emptying. LiveKit refuses with
	// failed_precondition, and a caller needs to tell that apart from a real
	// failure so it does not show the user an error for the thing they asked
	// for having happened.
	c, got := serveEgress(t, http.StatusBadRequest,
		`{"code":"failed_precondition","msg":"egress with status EGRESS_COMPLETE cannot be stopped"}`)
	_, err := c.StopEgress(context.Background(), "EG_1")
	if err == nil {
		t.Fatal("StopEgress: want an error")
	}
	if got.Path != "/twirp/livekit.Egress/StopEgress" {
		t.Errorf("path = %q", got.Path)
	}
	var lkErr *Error
	if !errors.As(err, &lkErr) || !lkErr.AlreadyStopped() {
		t.Errorf("err = %v, want an AlreadyStopped refusal", err)
	}
	if lkErr.NotFound() {
		t.Error("a stop refusal read as a missing room")
	}
}

func TestEgressCallsAreDisabledWithoutAnSFU(t *testing.T) {
	// A deployment with no LiveKit keeps working and only conferences report
	// themselves unavailable — the same contract as the RoomService calls.
	c := New(Config{})
	if _, err := c.StartRoomCompositeEgress(context.Background(), "conf_x", EgressOptions{Filepath: "/x.mp4"}); !errors.Is(err, ErrDisabled) {
		t.Errorf("start: err = %v, want ErrDisabled", err)
	}
	if _, err := c.ListEgress(context.Background(), "conf_x", true); !errors.Is(err, ErrDisabled) {
		t.Errorf("list: err = %v, want ErrDisabled", err)
	}
}

func TestListEgressOmitsUnsetFilters(t *testing.T) {
	c, got := serveEgress(t, http.StatusOK, `{"items":[]}`)
	if _, err := c.ListEgress(context.Background(), "", false); err != nil {
		t.Fatalf("ListEgress: %v", err)
	}
	// active=false is not "any": LiveKit reads a present field as a filter, and
	// an unset room name would otherwise match nothing.
	for _, field := range []string{"roomName", "active"} {
		if _, present := got.Body[field]; present {
			t.Errorf("%s sent as an empty filter: %v", field, got.Body)
		}
	}
}
