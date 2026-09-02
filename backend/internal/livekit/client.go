package livekit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Config is what the backend reads from the environment (LIVEKIT_*). Leaving it
// empty disables conferences rather than failing the boot.
type Config struct {
	// URL is where the BACKEND reaches RoomService — a compose service name,
	// e.g. http://livekit:7880. Never published as a host port: it carries the
	// administrative API.
	URL string
	// PublicURL is what the BROWSER is told to connect to for signalling, e.g.
	// wss://tessera.example/livekit. It goes through Caddy on 443, so it is a
	// different address from URL, not a rewriting of it.
	PublicURL string
	APIKey    string
	APISecret string
}

// Client is safe for concurrent use.
type Client struct {
	baseURL   string
	publicURL string
	apiKey    string
	apiSecret string
	http      *http.Client
	// nowFn exists so token tests can pin the clock; production leaves it nil.
	nowFn func() time.Time
}

// DefaultTimeout bounds one RoomService call. These are small control-plane
// requests against a neighbour on the compose network — a slow one means the
// SFU is wedged, and a request goroutine should not wait on that.
const DefaultTimeout = 10 * time.Second

// New builds a client. A Config missing any of URL/APIKey/APISecret yields a
// disabled client rather than nil, so callers never have to nil-check before
// asking Enabled.
//
// The transport ignores HTTP_PROXY/HTTPS_PROXY on purpose, for the same reason
// the converter client does (#2733): LiveKit is addressed by its compose
// service name, and the default transport would hand `http://livekit:7880` to
// whatever proxy the environment names — which cannot resolve it and answers
// 502. NO_PROXY does not help, because its CIDR entries are only matched
// against a host that is already an IP literal, so a service name slips past
// `172.16.0.0/12` even though that is the network it resolves into.
func New(cfg Config) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.URL), "/"),
		publicURL: strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/"),
		apiKey:    strings.TrimSpace(cfg.APIKey),
		apiSecret: strings.TrimSpace(cfg.APISecret),
		http:      &http.Client{Timeout: DefaultTimeout, Transport: transport},
	}
}

// Enabled reports whether an SFU was configured at all.
func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != "" && c.apiKey != "" && c.apiSecret != ""
}

// PublicURL is the signalling address handed to the browser alongside a token.
// Empty when the deployment has not set one — the client then has a token and
// nowhere to use it, so callers must treat that as "not configured".
func (c *Client) PublicURL() string {
	if c == nil {
		return ""
	}
	return c.publicURL
}

func (c *Client) now() time.Time {
	if c.nowFn != nil {
		return c.nowFn()
	}
	return time.Now()
}

// Error is a refusal LiveKit itself produced — an unknown room, a participant
// who already left, a rejected token. Separated from transport failures because
// they mean different things to a caller: repeating this one will not help.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("livekit: %s (%s, status %d)", e.Message, e.Code, e.Status)
	}
	return fmt.Sprintf("livekit: %s (status %d)", e.Message, e.Status)
}

// NotFound reports the "this room or participant is gone" case, which callers
// routinely want to treat as success: kicking someone who already left, or
// deleting a room that timed out on its own, is not a failure worth surfacing.
func (e *Error) NotFound() bool {
	return e.Status == http.StatusNotFound || e.Code == "not_found"
}

// maxResponseBytes caps a RoomService reply. The largest of these is a
// participant list, bounded by the room's max_participants, so the ceiling only
// exists to keep a misbehaving server from turning one call into an unbounded
// allocation.
const maxResponseBytes = 4 << 20

// call performs one twirp request. LiveKit's RoomService is plain JSON over
// HTTP at /twirp/livekit.RoomService/<Method>, authenticated with the same JWT
// as a join token but carrying the administrative grants (serviceToken).
func (c *Client) call(ctx context.Context, method, room string, in, out any) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	token, err := c.serviceToken(room)
	if err != nil {
		return err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	endpoint := c.baseURL + "/twirp/livekit.RoomService/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return twirpError(resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// twirpError decodes twirp's `{"code":"not_found","msg":"…"}` envelope, falling
// back to the raw body when the failure came from something else in front of
// LiveKit (a proxy error page, say).
func twirpError(status int, body []byte) error {
	var payload struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	e := &Error{Status: status, Message: strings.TrimSpace(string(body))}
	if err := json.Unmarshal(body, &payload); err == nil && (payload.Code != "" || payload.Msg != "") {
		e.Code, e.Message = payload.Code, payload.Msg
	}
	if len(e.Message) > 300 {
		e.Message = e.Message[:300] + "…"
	}
	if e.Message == "" {
		e.Message = "request failed"
	}
	return e
}

// Room is the subset of LiveKit's Room we care about.
type Room struct {
	SID             string  `json:"sid"`
	Name            string  `json:"name"`
	NumParticipants int     `json:"numParticipants"`
	CreationTime    epochSt `json:"creationTime"`
}

// RoomOptions are the per-room ceilings passed at creation. Zero values mean
// "use the server's config" (deploy/livekit.yaml), which is the normal case.
type RoomOptions struct {
	// EmptyTimeout is how long an empty room survives, in seconds.
	EmptyTimeout int
	// MaxParticipants caps the room; 0 leaves the server default in place.
	MaxParticipants int
	// Metadata is opaque to LiveKit and echoed to every participant.
	Metadata string
}

// CreateRoom creates the room for a conference. Not optional plumbing: the
// server runs with room.auto_create=false (deploy/livekit.yaml), precisely so a
// token cannot conjure a room by itself, which means nobody can join until we
// have created it here.
//
// Creating a room that already exists is not an error for LiveKit — it returns
// the existing one — so this is safe to call on every join.
func (c *Client) CreateRoom(ctx context.Context, name string, opts RoomOptions) (Room, error) {
	req := map[string]any{"name": name}
	if opts.EmptyTimeout > 0 {
		req["emptyTimeout"] = opts.EmptyTimeout
	}
	if opts.MaxParticipants > 0 {
		req["maxParticipants"] = opts.MaxParticipants
	}
	if opts.Metadata != "" {
		req["metadata"] = opts.Metadata
	}
	var room Room
	if err := c.call(ctx, "CreateRoom", name, req, &room); err != nil {
		return Room{}, err
	}
	return room, nil
}

// TrackInfo describes one published track. Type is "AUDIO"/"VIDEO" and Source
// is "CAMERA"/"MICROPHONE"/"SCREEN_SHARE"/… — string enums, as protojson
// renders them.
type TrackInfo struct {
	SID    string `json:"sid"`
	Type   string `json:"type"`
	Source string `json:"source"`
	Name   string `json:"name"`
	Muted  bool   `json:"muted"`
}

// Participant is LiveKit's view of who is in a room. Identity is the Tessera
// user id we minted the token with, which is what lets a caller line this list
// up against our own membership.
type Participant struct {
	SID      string      `json:"sid"`
	Identity string      `json:"identity"`
	Name     string      `json:"name"`
	State    string      `json:"state"`
	JoinedAt epochSt     `json:"joinedAt"`
	Tracks   []TrackInfo `json:"tracks"`
	Metadata string      `json:"metadata"`
}

// ListParticipants returns who LiveKit currently believes is in the room. This
// is the media server's truth, which can differ from our own room state for a
// few seconds after somebody drops — use it to reconcile, not as the primary
// presence source.
func (c *Client) ListParticipants(ctx context.Context, room string) ([]Participant, error) {
	var out struct {
		Participants []Participant `json:"participants"`
	}
	if err := c.call(ctx, "ListParticipants", room, map[string]any{"room": room}, &out); err != nil {
		return nil, err
	}
	return out.Participants, nil
}

// RemoveParticipant kicks somebody out of a room.
//
// The permission check belongs to the caller and must have already happened:
// LiveKit is doing what our service token tells it, with no idea who asked.
func (c *Client) RemoveParticipant(ctx context.Context, room, identity string) error {
	return c.call(ctx, "RemoveParticipant", room, map[string]any{
		"room": room, "identity": identity,
	}, nil)
}

// MuteTrack force-mutes one published track — the admin's "mute for everyone".
// trackSID comes from ListParticipants.
//
// Only muting is offered, never the reverse. LiveKit refuses a remote unmute
// unless room.enable_remote_unmute is turned on, and we deliberately leave it
// off: switching somebody's microphone back on from the server is not a
// moderation action, it is eavesdropping.
func (c *Client) MuteTrack(ctx context.Context, room, identity, trackSID string) error {
	return c.call(ctx, "MutePublishedTrack", room, map[string]any{
		"room": room, "identity": identity, "trackSid": trackSID, "muted": true,
	}, nil)
}

// Track sources, as protojson renders LiveKit's TrackSource enum. They are the
// vocabulary of SetPublishSources below — a force-mute is expressed as "you may
// publish everything except MICROPHONE", not as a blanket publish ban, because
// taking someone's voice away must not also take away their screen share.
const (
	SourceCamera      = "CAMERA"
	SourceMicrophone  = "MICROPHONE"
	SourceScreenShare = "SCREEN_SHARE"
	SourceScreenAudio = "SCREEN_SHARE_AUDIO"
)

// AllSources is what an unrestricted participant may publish.
func AllSources() []string {
	return []string{SourceCamera, SourceMicrophone, SourceScreenShare, SourceScreenAudio}
}

// SourcesWithoutMic is AllSources minus the microphone — the permission set of a
// force-muted participant.
func SourcesWithoutMic() []string {
	return []string{SourceCamera, SourceScreenShare, SourceScreenAudio}
}

// SetPublishSources restricts what a participant may publish from now on.
//
// This is the half of force-mute that MuteTrack cannot do. MutePublishedTrack
// silences the track that exists *right now*; nothing stops the client from
// publishing a fresh one a second later, and a modified client will. Narrowing
// the permission is what makes the silence hold, and LiveKit applies it to the
// live connection — the participant does not have to reconnect for it to bite.
//
// The list is passed explicitly in both directions: an *empty* canPublishSources
// means "no restriction" to LiveKit, so lifting a force-mute has to name every
// source rather than clearing the field.
func (c *Client) SetPublishSources(ctx context.Context, room, identity string, sources []string) error {
	return c.call(ctx, "UpdateParticipant", room, map[string]any{
		"room": room, "identity": identity,
		"permission": map[string]any{
			"canSubscribe":      true,
			"canPublish":        true,
			"canPublishData":    true,
			"canPublishSources": sources,
		},
	}, nil)
}

// DeleteRoom ends a conference for everyone and disconnects the participants.
func (c *Client) DeleteRoom(ctx context.Context, room string) error {
	return c.call(ctx, "DeleteRoom", room, map[string]any{"room": room}, nil)
}

// epochSt is a LiveKit timestamp. It needs its own type because twirp
// serialises protobuf through protojson, and protojson renders an int64 as a
// JSON *string* ("1756848000") — decoding that into an int64 field fails with
// "cannot unmarshal string", which would break the whole reply over a field
// nobody was even looking at. Numbers are accepted too, so this keeps working
// if the encoding ever changes.
type epochSt int64

func (e *epochSt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*e = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*e = epochSt(v)
	return nil
}

// Time renders the timestamp; zero stays a zero Time rather than 1970.
func (e epochSt) Time() time.Time {
	if e == 0 {
		return time.Time{}
	}
	return time.Unix(int64(e), 0)
}
