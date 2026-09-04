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
	c, got := serveEgress(t, http.StatusOK, `{"egress_id":"EG_1","room_name":"conf_x"}`)
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
	c, got := serveEgress(t, http.StatusOK, `{"egress_id":"EG_1"}`)
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
	// with a template name that does not exist. Same for the custom template URL
	// and the encoding preset — absent when unset, so the default install keeps
	// egress's built-in grid at its default resolution.
	for _, k := range []string{"layout", "customBaseUrl", "preset"} {
		if _, present := got.Body[k]; present {
			t.Errorf("empty %s sent as an override: %v", k, got.Body)
		}
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

// With a custom template configured (#2877) the request carries the base URL,
// the layout and the encoding preset — the three things that turn egress's dark
// 720p grid into our own room-shaped 1080p recording.
func TestStartEgressCarriesCustomTemplateAndPreset(t *testing.T) {
	c, got := serveEgress(t, http.StatusOK, `{"egress_id":"EG_1"}`)
	if _, err := c.StartRoomCompositeEgress(context.Background(), "conf_x", EgressOptions{
		Filepath:      "/data/uploads/rec/abc/rec-1.mp4",
		CustomBaseURL: "http://frontend/rec/egress",
		Layout:        "speaker",
		Preset:        "H264_1080P_30",
	}); err != nil {
		t.Fatalf("StartRoomCompositeEgress: %v", err)
	}
	if got.Body["customBaseUrl"] != "http://frontend/rec/egress" {
		t.Errorf("customBaseUrl = %v", got.Body["customBaseUrl"])
	}
	if got.Body["layout"] != "speaker" {
		t.Errorf("layout = %v", got.Body["layout"])
	}
	if got.Body["preset"] != "H264_1080P_30" {
		t.Errorf("preset = %v", got.Body["preset"])
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
	c, _ := serveEgress(t, http.StatusOK, `{"egress_id":"EG_1","room_name":"conf_x"}`)
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
		"egress_id":"EG_1","room_name":"conf_x","status":"EGRESS_COMPLETE",
		"started_at":"1756848000000000000","ended_at":"1756848120000000000",
		"file_results":[{"filename":"/data/uploads/conf/abc/rec-1.mp4",
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

// lkListEgressReply is a reply captured verbatim from livekit-server v1.13.6 on
// the recording stand (#2877, `make recording-e2e-up`), trimmed only of the
// fields we never read. It is here because the field NAMES are the point: every
// fixture above could be rewritten in whatever spelling the code happens to
// expect, and the tests would stay green while the client read nothing.
const lkListEgressReply = `{"items":[{"egress_id":"EG_FcGa6u6e35LH",
  "room_id":"RM_uHHCRYTMuCNz", "room_name":"conf_57236268-dd7d-46ad-9fef-5b0c130d5f8f",
  "source_type":"EGRESS_SOURCE_TYPE_WEB", "status":"EGRESS_COMPLETE",
  "started_at":"1788477633147899026", "ended_at":"1788477663147899026",
  "file_results":[{"filename":"/uploads/rec/57236268/1f787e4c.mp4",
                   "started_at":"1788477636756122716", "ended_at":"1788477663147899026",
                   "duration":"30000000000", "size":"524288", "location":""}],
  "error":"", "error_code":0, "details":""}], "next_page_token":null}`

func TestEgressInfoDecodesARealLiveKitReply(t *testing.T) {
	// The regression this pins: livekit-server writes twirp replies with PROTO
	// names, so `egress_id` arrives and camelCase tags match nothing. Decoding
	// such a reply does not fail — it yields a zero-valued struct whose Status
	// still looks sane, because single-word keys spell the same either way. On
	// the stand that meant a start call "succeeded" with an empty egress id,
	// the recording row lost its only handle on the job, and the stop that
	// followed reported "recording was never started" while an untracked egress
	// went on writing an mp4.
	c, _ := serveEgress(t, http.StatusOK, lkListEgressReply)
	info, err := c.GetEgress(context.Background(), "EG_FcGa6u6e35LH")
	if err != nil {
		t.Fatalf("GetEgress on a real reply: %v", err)
	}
	if info.RoomName != "conf_57236268-dd7d-46ad-9fef-5b0c130d5f8f" {
		t.Errorf("room = %q", info.RoomName)
	}
	if info.StartedAt.Time().IsZero() || info.EndedAt.Time().IsZero() {
		t.Errorf("timestamps unread: started=%v ended=%v", info.StartedAt.Time(), info.EndedAt.Time())
	}
	file := info.File()
	if file.Filename == "" || file.Size != 512<<10 || file.Length() != 30*time.Second {
		t.Errorf("file = %+v, want the captured mp4", file)
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
