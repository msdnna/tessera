package livekit

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Egress is LiveKit's recording service (#2877). It is a second twirp API on the
// same server as RoomService, but the path from here to a finished mp4 does not
// look like the RoomService calls next door, and the difference matters when
// something goes wrong:
//
//	backend --twirp--> livekit-server --redis--> egress worker --> headless
//	Chrome joins the room as a hidden participant --> mp4 on a shared volume.
//
// Consequences worth knowing before reading the code:
//
//   - Redis is not optional plumbing. livekit-server hands egress requests to
//     workers over Redis and nowhere else, and with local (non-Redis) state its
//     egress store is nil, so a server without Redis answers these calls with a
//     refusal rather than doing the work. That is why recording is a compose
//     profile that turns on Redis for the SFU too (deploy/docker-compose.yml).
//   - A start call returning successfully means a worker accepted the job, not
//     that anything has been recorded. The file appears only when the egress
//     ends, which is why the finishing side of this is a polling worker rather
//     than a response field.
//   - Filepath below is a path inside the *egress* container. It lands in the
//     uploads volume both containers share, so the backend then sees the same
//     file under its own UPLOAD_DIR — the two paths are the same file with
//     different prefixes, and neither service can see the other's.

// Egress statuses, as protojson renders livekit.EgressStatus.
//
// EgressStarting is the enum's zero value, and protojson omits zero-valued
// fields: an egress that has been accepted but not yet picked up comes back
// with no "status" key at all. So an empty status is not "unknown", it is
// STARTING — which is exactly what StartRoomCompositeEgress returns. Every
// EgressInfo produced here has already been normalised for that (see
// normalise), so callers may compare Status directly.
const (
	EgressStarting     = "EGRESS_STARTING"
	EgressActive       = "EGRESS_ACTIVE"
	EgressEnding       = "EGRESS_ENDING"
	EgressComplete     = "EGRESS_COMPLETE"
	EgressFailed       = "EGRESS_FAILED"
	EgressAborted      = "EGRESS_ABORTED"
	EgressLimitReached = "EGRESS_LIMIT_REACHED"
)

// FileInfo is the recording LiveKit produced. It is populated only once the
// egress reaches a terminal status — while recording is under way the fields
// are zero, including Filename.
type FileInfo struct {
	// Filename is the path the worker wrote, in the egress container's
	// namespace (see the package comment).
	Filename string `json:"filename"`
	// Size is the file size in bytes.
	Size protoInt64 `json:"size"`
	// Duration is the recorded length in NANOSECONDS — it is computed as
	// ended_at minus started_at, both of which are unix nanosecond stamps.
	// Use Length rather than reading it raw.
	Duration protoInt64 `json:"duration"`
	// Location is set for uploaded outputs (S3 and friends). We record to a
	// local volume, so it stays empty.
	Location string `json:"location"`
}

// Length renders Duration as a duration, so no call site has to remember which
// of LiveKit's several time units this one is.
func (f FileInfo) Length() time.Duration { return time.Duration(f.Duration) }

// EgressInfo is one recording job, at whatever stage it has reached.
type EgressInfo struct {
	EgressID string `json:"egressId"`
	RoomName string `json:"roomName"`
	Status   string `json:"status"`
	// StartedAt and EndedAt are unix NANOSECONDS, unlike Room.CreationTime
	// next door, which is unix seconds. Use the Time methods.
	StartedAt nanoSt `json:"startedAt"`
	EndedAt   nanoSt `json:"endedAt"`
	// Error carries the worker's reason for a failed egress. Worth storing:
	// it is the only place a "Chrome could not join" ever shows up on our side.
	Error string `json:"error"`
	// Files holds one entry per file output. We always request exactly one, so
	// File() is the accessor to use.
	Files []FileInfo `json:"fileResults"`
}

// File returns the single output we asked for, or a zero FileInfo while the
// recording is still running.
func (i EgressInfo) File() FileInfo {
	if len(i.Files) == 0 {
		return FileInfo{}
	}
	return i.Files[0]
}

// Done reports whether this egress will never change again.
//
// It answers on the known terminal statuses only, so a status LiveKit invents
// later reads as "still running". That is the safer default of the two — a
// poller keeps asking instead of declaring a recording lost — but it does mean
// the caller needs a wall-clock backstop rather than trusting this alone to
// terminate.
func (i EgressInfo) Done() bool {
	switch i.Status {
	case EgressComplete, EgressFailed, EgressAborted, EgressLimitReached:
		return true
	}
	return false
}

// Succeeded reports whether a finished egress left a usable file.
//
// EgressLimitReached counts: it means the recording ran into the
// session_limits ceiling from deploy/egress.yaml and was cut short, but
// everything up to that point was written and is playable. Treating it as a
// failure would throw away a two-hour recording over its last second.
func (i EgressInfo) Succeeded() bool {
	return i.Status == EgressComplete || i.Status == EgressLimitReached
}

// normalise fills in what protojson left out. Currently that is only the
// omitted zero-value status, but it is the difference between "recording is
// starting" and an empty string that no switch matches.
func (i *EgressInfo) normalise() {
	if i.Status == "" {
		i.Status = EgressStarting
	}
}

// EgressOptions are the knobs of one room recording. The zero value is valid:
// it records the room with LiveKit's default layout and both media kinds.
type EgressOptions struct {
	// Filepath is where the worker writes, inside the egress container.
	// Required — LiveKit would otherwise pick a templated name of its own and
	// we would not know what to put in the database.
	Filepath string
	// Layout is the grid template, e.g. "grid" or "speaker". Empty leaves
	// egress's own default, which is what we want unless a user asks.
	Layout string
	// AudioOnly drops the video track: a much smaller file for a call that was
	// a conversation rather than a screen share.
	AudioOnly bool
}

// StartRoomCompositeEgress asks LiveKit to record a room into a single mp4.
//
// The permission check belongs to the caller and must have already happened —
// as with the RoomService calls, LiveKit only verifies our signature and has no
// idea who asked. A returned EgressInfo means a worker took the job; the file
// does not exist yet, and its size and duration arrive only once the egress
// reaches a terminal status.
func (c *Client) StartRoomCompositeEgress(ctx context.Context, room string, opts EgressOptions) (EgressInfo, error) {
	if strings.TrimSpace(room) == "" {
		return EgressInfo{}, errors.New("livekit: egress needs a room")
	}
	if strings.TrimSpace(opts.Filepath) == "" {
		return EgressInfo{}, errors.New("livekit: egress needs a filepath")
	}
	req := map[string]any{
		"roomName":  room,
		"audioOnly": opts.AudioOnly,
		// fileOutputs, not the singular "file": the latter still works but is
		// marked deprecated in livekit_egress.proto, and the repeated field is
		// what the unified StartEgress request uses.
		"fileOutputs": []map[string]any{{
			"fileType": "MP4",
			"filepath": opts.Filepath,
			// Without this the worker drops a sibling .json manifest next to
			// the mp4 — in our case straight into the attachments volume,
			// where a file nobody wrote through the API would be a puzzle.
			"disableManifest": true,
		}},
	}
	if opts.Layout != "" {
		req["layout"] = opts.Layout
	}
	return c.egressCall(ctx, "StartRoomCompositeEgress", req)
}

// StopEgress ends a recording early. The reply is the final EgressInfo.
//
// Stopping an egress that has already finished is refused rather than ignored
// — LiveKit answers failed_precondition ("egress with status X cannot be
// stopped"). Callers that race the natural end of a recording should treat
// AlreadyStopped as success.
func (c *Client) StopEgress(ctx context.Context, egressID string) (EgressInfo, error) {
	if strings.TrimSpace(egressID) == "" {
		return EgressInfo{}, errors.New("livekit: stop needs an egress id")
	}
	return c.egressCall(ctx, "StopEgress", map[string]any{"egressId": egressID})
}

// AlreadyStopped reports the "this egress is past stopping" refusal, which a
// caller that asked for a stop normally wants to treat as success: the
// recording it wanted ended is ended.
func (e *Error) AlreadyStopped() bool {
	return e.Code == "failed_precondition" && strings.Contains(e.Message, "cannot be stopped")
}

// ListEgress returns the recordings LiveKit knows about, newest state first
// come, no ordering promised. Both filters are optional and combine; passing
// neither lists everything the server is holding.
//
// activeOnly narrows the result to egresses that have not finished. That is the
// cheap way to ask "is this room being recorded right now", but it is not how
// the polling worker should ask: a job that failed disappears from the active
// list, and a poller filtering on active would read that as "still going" until
// something else timed it out.
func (c *Client) ListEgress(ctx context.Context, room string, activeOnly bool) ([]EgressInfo, error) {
	req := map[string]any{}
	if room != "" {
		req["roomName"] = room
	}
	if activeOnly {
		req["active"] = true
	}
	var out struct {
		Items []EgressInfo `json:"items"`
	}
	if err := c.egressRequest(ctx, "ListEgress", req, &out); err != nil {
		return nil, err
	}
	for i := range out.Items {
		out.Items[i].normalise()
	}
	return out.Items, nil
}

// ErrEgressGone is returned by GetEgress when LiveKit has no such egress. It is
// a real possibility rather than a corrupt-database case: livekit-server keeps
// finished egresses in Redis for a while and then forgets them, so a row we are
// still polling can outlive the server's memory of it. A caller should mark
// such a recording failed rather than poll it forever.
var ErrEgressGone = errors.New("livekit: no such egress")

// GetEgress fetches one egress by id.
//
// Implemented over ListEgress because the Egress twirp API has no by-id read of
// its own — the id is a filter on the list, and an unknown id is an empty list
// rather than an error, which is what ErrEgressGone turns into something a
// caller can branch on.
func (c *Client) GetEgress(ctx context.Context, egressID string) (EgressInfo, error) {
	if strings.TrimSpace(egressID) == "" {
		return EgressInfo{}, errors.New("livekit: get needs an egress id")
	}
	var out struct {
		Items []EgressInfo `json:"items"`
	}
	if err := c.egressRequest(ctx, "ListEgress", map[string]any{"egressId": egressID}, &out); err != nil {
		return EgressInfo{}, err
	}
	for i := range out.Items {
		if out.Items[i].EgressID == egressID {
			out.Items[i].normalise()
			return out.Items[i], nil
		}
	}
	return EgressInfo{}, ErrEgressGone
}

// egressCall runs one Egress RPC whose reply is a bare EgressInfo.
func (c *Client) egressCall(ctx context.Context, method string, in any) (EgressInfo, error) {
	var info EgressInfo
	if err := c.egressRequest(ctx, method, in, &info); err != nil {
		return EgressInfo{}, err
	}
	info.normalise()
	return info, nil
}

// egressRequest is the transport half: mint a recording token, POST it to
// /twirp/livekit.Egress/<method>.
func (c *Client) egressRequest(ctx context.Context, method string, in, out any) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	token, err := c.recordToken()
	if err != nil {
		return err
	}
	return c.twirp(ctx, "livekit.Egress", method, token, in, out)
}

// recordToken is the credential the Egress API accepts, and it is deliberately
// not serviceToken: livekit-server gates every egress RPC on the roomRecord
// grant alone (EnsureRecordPermission), which roomAdmin does not imply, so a
// RoomService token is rejected here with a permission error.
//
// It also carries no room name, because that check never looks at one — a room
// in the grant would read like a restriction that is not actually enforced.
// What keeps this safe is the same thing as next door: the token is built per
// request, lives for a minute and never leaves the backend.
func (c *Client) recordToken() (string, error) {
	now := c.now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.apiKey,
			Subject:   c.apiKey,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
		Video: videoGrant{RoomRecord: true},
	})
	return tok.SignedString([]byte(c.apiSecret))
}

// protoInt64 is a protobuf int64 as protojson renders it — a JSON *string*.
// Same hazard epochSt documents in client.go: decoding "1048576" into a plain
// int64 fails outright and takes the whole reply with it.
type protoInt64 int64

func (v *protoInt64) UnmarshalJSON(b []byte) error {
	n, err := parseProtoInt64(b)
	if err != nil {
		return err
	}
	*v = protoInt64(n)
	return nil
}

// nanoSt is a LiveKit unix-NANOSECOND timestamp. Egress stamps its times with
// time.Now().UnixNano() while RoomService uses seconds, so the two cannot share
// a type: feeding nanoseconds to time.Unix would land in the year 56 million.
type nanoSt int64

func (n *nanoSt) UnmarshalJSON(b []byte) error {
	v, err := parseProtoInt64(b)
	if err != nil {
		return err
	}
	*n = nanoSt(v)
	return nil
}

// Time renders the timestamp; zero stays a zero Time rather than 1970.
func (n nanoSt) Time() time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(n))
}
