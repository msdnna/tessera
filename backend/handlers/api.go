// Package handlers implements the HTTP resource handlers for the Tessera API,
// sharing dependencies and helpers through the API type.
package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tessera/internal/confroom"
	"tessera/internal/converter"
	"tessera/internal/db"
	"tessera/internal/docroom"
	"tessera/internal/jobs"
	"tessera/internal/livekit"
	"tessera/internal/mail"
	"tessera/internal/notify"
	"tessera/internal/realtime"
	"tessera/internal/secrets"
	"tessera/middleware"
)

// API holds the dependencies shared by all resource handlers.
type API struct {
	q              *db.Queries
	pool           *pgxpool.Pool // for multi-statement transactions (e.g. project transfer)
	hub            *realtime.Hub
	uploadDir      string
	sealer         *secrets.Sealer          // encrypts secrets at rest (GitLab PATs, channel secrets)
	assetKey       []byte                   // HMAC key for signed GitLab asset-proxy URLs
	mailer         mail.Mailer              // transactional email (invitations); no-op when SMTP unset
	publicURL      string                   // external base URL for links in emails
	senders        map[string]notify.Sender // notification channel transports, keyed by type
	jobs           *jobs.Registry           // in-memory registry of background jobs (observability + cancel)
	docRooms       *docroom.Rooms           // per-document presence/locks (nil until WireDocRooms)
	confRooms      *confroom.Rooms          // per-conference room state (nil until WireConfRooms)
	livekit        *livekit.Client          // SFU control plane + join tokens; disabled when LIVEKIT_* unset
	metrics        *middleware.Collector    // HTTP request/latency counters for /admin/metrics (nil until WireOps)
	converter      *converter.Client        // LibreOffice sidecar for document import/export; disabled when unconfigured
	version        string                   // build version, surfaced by the readiness/metrics probes
	webhooks       webhookQueue             // debounced dirty set of GitLab integrations poked by a webhook (#2594)
	egressDir      string                   // uploadDir as the egress recorder sees it (#2877); empty falls back to uploadDir
	recTemplateURL string                   // internal url of our egress recording page (#2877); empty ⇒ egress's built-in grid
	bgCtx          context.Context          // process lifetime for detached job runs (nil until WireBackgroundScope)
	bgWG           *sync.WaitGroup          // shutdown waits on it, alongside the tick loops
}

// WireBackgroundScope hands the handlers the process's shutdown context and the
// WaitGroup shutdown drains, so work started from a request but outliving it —
// the /admin/jobs "run now" action — belongs to the same lifecycle as the tick
// loops instead of running loose (#2932).
//
// Both halves matter. The context is what tells a sweep it is being shut down:
// settleDelivery reads ctx.Err() to tell a deploy stopping mid-send from a
// verdict on the delivery, and on a detached context that question always
// answers "no". The WaitGroup is what keeps the pool open under it — main closes
// the pool after draining, on the stated assumption that nothing is still
// writing.
//
// Optional wiring: unwired (the test harnesses, which own no process lifecycle)
// a detached run falls back to its own background context and nothing waits.
func (h *API) WireBackgroundScope(ctx context.Context, wg *sync.WaitGroup) {
	h.bgCtx, h.bgWG = ctx, wg
}

// WireOps injects the ops-observability dependencies that live outside NewAPI's
// core wiring — the HTTP metrics collector and the build version string — so the
// readiness and /admin/metrics handlers can report them.
func (h *API) WireOps(m *middleware.Collector, version string) {
	h.metrics = m
	h.version = version
}

// WireConverter injects the document conversion sidecar client (#2733).
//
// Wired separately rather than added to NewAPI's parameter list for the same
// reason as WireOps: the converter is optional infrastructure, and threading an
// optional dependency through a constructor every caller must satisfy is how
// that signature grew to eight positional arguments in the first place.
func (h *API) WireConverter(c *converter.Client) { h.converter = c }

// WireDocRooms injects the per-document presence/lock registry (#2729). The
// resource layer needs it for one thing only — emptying a room when its document
// is deleted — while the socket itself lives on WSHandler.
func (h *API) WireDocRooms(rooms *docroom.Rooms) { h.docRooms = rooms }

// CloseDocRooms stops the lock sweeper and disconnects everyone still in a
// document, so a restart doesn't leave clients holding locks against a process
// that no longer exists.
func (h *API) CloseDocRooms() {
	if h.docRooms != nil {
		h.docRooms.Close()
	}
}

// WireConfRooms injects the per-conference room registry (#2869). As with
// documents, the resource layer needs it only to empty a room — when the call
// ends or the conference is deleted — while the socket itself lives on
// WSHandler.
func (h *API) WireConfRooms(rooms *confroom.Rooms) { h.confRooms = rooms }

// WireLiveKit injects the SFU client (#2871). livekit.New tolerates an empty
// config and reports itself disabled, so an install without LIVEKIT_* set gets a
// non-nil client that answers "off" — the token handler turns that into a 503
// instead of every call site nil-checking.
func (h *API) WireLiveKit(c *livekit.Client) { h.livekit = c }

// CloseConfRooms stops the stage sweeper and disconnects everyone still in a
// call, so a restart doesn't leave clients waiting on a screen-share stage that
// no process can release.
func (h *API) CloseConfRooms() {
	if h.confRooms != nil {
		h.confRooms.Close()
	}
}

// dropConfRoom empties a conference's room, telling whoever is still connected
// why. Guarded because the registry is optional wiring — the GitLab and job
// tests build an API without it.
//
// Both halves of the call go down, presence and media (#2932). The registry only
// owns the roster; the audio and video live in the SFU, which authorises a
// participant once at the join and never re-reads anything afterwards — so a
// teardown that stopped here would close the sockets while the room itself
// played on, for anyone who ignores the frame it just sent or who re-presents
// the join token they still hold. dropConfMedia is deliberately outside the nil
// guard: a presence room that no longer exists (nobody connected, or a backend
// that restarted since) says nothing about whether the SFU still has one.
func (h *API) dropConfRoom(confID uuid.UUID, reason string) {
	if h.confRooms != nil {
		h.confRooms.Drop(confID, reason)
	}
	h.dropConfMedia(confID)
}

// dropDocRooms empties the rooms of documents that have just been deleted,
// closing the sockets of whoever is still in them (#2729).
//
// Owed by every path a document can die on, not only DeleteDocument: a room
// socket is keyed by document id and authorises exactly once, at the upgrade, so
// nothing about the row disappearing reaches it. Left alone, the people who had
// it open keep their presence badges, keep holding block locks, and keep typing
// into a row that no longer exists — they find out on the next autosave, which
// answers 404 with their text still only in the browser. Guarded because the
// registry is optional wiring the GitLab and job tests build without.
func (h *API) dropDocRooms(ids []uuid.UUID) {
	if h.docRooms == nil {
		return
	}
	for _, id := range ids {
		h.docRooms.Drop(id)
	}
}

// revokeUserSockets cuts every live connection a user holds after their access
// is taken away (#2932).
//
// Three fan-outs, three registries, one revocation. The hub scopes by workspace
// and can be told to drop a user; the document and conference rooms are keyed by
// their own object and authorise exactly once, at the upgrade — so dropping only
// the hub socket leaves an ex-member holding block locks in someone else's
// document and moderating someone else's call. All three are guarded: the
// registries are optional wiring the GitLab and job tests build without.
func (h *API) revokeUserSockets(userID uuid.UUID, reason string) {
	h.hub.DropUser(userID)
	if h.docRooms != nil {
		h.docRooms.EvictUser(userID)
	}
	if h.confRooms != nil {
		h.confRooms.EvictUser(userID, reason)
	}
}

// syncRoomModeration re-resolves what a member may moderate in this workspace's
// live calls after their role changed (#2932).
//
// revokeUserSockets is the wrong tool for a demotion — it ends the person's call
// — and doing nothing is the wrong answer too: the conference seat copies the
// workspace role into a boolean at the upgrade and never looks again, so an
// ex-admin keeps kicking and force-muting the meeting they are already in. This
// is the middle: the seat stays, the right that came with the role does not.
//
// The conference list is read here rather than in the registry because only this
// side knows which calls belong to the workspace whose role changed. A storage
// error is not worth failing the role change over — the membership row is
// already written and is what every later request reads — but it does leave a
// stale right in a live room, so it is logged rather than swallowed.
func (h *API) syncRoomModeration(ctx context.Context, workspaceID, userID uuid.UUID, role string) {
	if h.confRooms == nil {
		return
	}
	ids, err := h.q.ConferenceIDsInWorkspace(ctx, workspaceID)
	if err != nil {
		log.Printf("conference moderation not re-resolved for %s in %s: %v", userID, workspaceID, err)
		return
	}
	h.confRooms.SetModeration(ids, userID, role == "owner" || role == "admin")
}

// SocketRevoker hands revokeUserSockets to AuthHandler, which owns the two
// password doors but none of the three socket registries (#2932). Same shape as
// ConfEnforcer: one behaviour, handed across the seam as a function rather than
// by giving the auth layer the hub, the document rooms and the conference rooms
// to reach into.
func (h *API) SocketRevoker() func(uuid.UUID, string) { return h.revokeUserSockets }

// ScopeRefresher hands AuthHandler the way to re-scope a user's board sockets
// after it granted them a membership (#2932). Hub.DropUser alone, not
// revokeUserSockets: the client reconnects and resolves its workspaces again,
// while the document and conference rooms — which a grant did not touch — stay
// as they are.
func (h *API) ScopeRefresher() func(uuid.UUID) { return h.hub.DropUser }

// notifyConfRoom nudges everyone currently in a conference that something they
// are looking at changed outside the socket — today the chat (#2873).
//
// The frame carries no payload on purpose: the room evicts a participant whose
// buffer overflows, so putting message bodies (let alone attachments) on that
// channel would let a busy conversation disconnect the people having it. A
// conference nobody has open has no room and this is a no-op, which is correct —
// the next arrival loads the chat over HTTP anyway.
func (h *API) notifyConfRoom(confID uuid.UUID, msgType string) {
	if h.confRooms != nil {
		h.confRooms.Notify(confID, msgType)
	}
}

// WireRecording injects the path prefix the egress recorder uses for the shared
// uploads volume (#2877). Separate from WireLiveKit because it is not the SFU's
// business: it is our own filesystem, seen from another container.
func (h *API) WireRecording(egressUploadDir, templateURL string) {
	h.egressDir = egressUploadDir
	h.recTemplateURL = templateURL
}

// setConfRecording / clearConfRecording move the room's recording indicator.
// Guarded like the calls above — the GitLab and job tests build an API with no
// room registry at all.
func (h *API) setConfRecording(confID uuid.UUID, v *confroom.RecordingView) {
	if h.confRooms != nil {
		h.confRooms.SetRecording(confID, v)
	}
}

func (h *API) clearConfRecording(confID uuid.UUID, recordingID string) {
	if h.confRooms != nil {
		h.confRooms.ClearRecording(confID, recordingID)
	}
}

// NewAPI wires the shared handler dependencies, building the secret sealer from
// the configured encryption key.
func NewAPI(q *db.Queries, pool *pgxpool.Pool, hub *realtime.Hub, uploadDir, encryptionKey string, mailer mail.Mailer, publicURL, fcmCredentialsFile string) *API {
	sealer, err := secrets.NewSealer(encryptionKey)
	if err != nil {
		// config.New guarantees a non-empty key, so this is unreachable in practice.
		log.Fatalf("failed to init secret sealer: %v", err)
	}
	ak := sha256.Sum256([]byte(encryptionKey + ":gitlab-asset"))
	// A TextHandler renders the background-task summary in logfmt (key=value), with
	// the call site as source=… — the format the ops log asks for.
	jobLog := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true}))
	return &API{
		q: q, pool: pool, hub: hub, uploadDir: uploadDir, sealer: sealer, assetKey: ak[:],
		mailer: mailer, publicURL: publicURL, senders: buildSenders(mailer, fcmCredentialsFile), jobs: jobs.New(jobLog),
	}
}

// positionGap is the spacing used when appending to the end of a list.
const positionGap = 65536.0

// positionBetween computes a float position for an item dropped between two
// neighbours (float midpoint). nil prev = top of list, nil next = end.
func positionBetween(prev, next *float64) float64 {
	switch {
	case prev != nil && next != nil:
		return (*prev + *next) / 2
	case prev != nil:
		return *prev + positionGap
	case next != nil:
		return *next / 2
	default:
		return positionGap
	}
}

// positionExhausted reports that the midpoint landed *on* a neighbour instead of
// between them — float8 has no room left in that slot. It happens after ~52 drops
// into the same gap (each one halves it) and immediately when the two neighbours
// already share a position. Leaving such a position in place is not a cosmetic
// rounding loss: two cards with one position have no defined order, and every
// later drop into that slot computes the same value again, so the slot is dead.
func positionExhausted(pos float64, prev, next *float64) bool {
	return (prev != nil && pos <= *prev) || (next != nil && pos >= *next)
}

// rejectSameNeighbour answers 400 (and reports true) when a reorder names one
// item as both of its neighbours. That asks to be dropped into a zero-width
// slot: the midpoint of a position with itself is that position, so the item
// would land exactly on top of its "neighbour". No respacing can open that slot,
// because there is no slot — which is why this is a refusal and not a recovery.
func rejectSameNeighbour(c *gin.Context, beforeID, afterID *uuid.UUID) bool {
	if beforeID == nil || afterID == nil || *beforeID != *afterID {
		return false
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "before_id and after_id must differ"})
	return true
}

// positionForReorder is positionBetween with a way out of an exhausted slot, for
// the lists that order themselves by midpoint but are not tasks: board columns,
// projects and project groups. When the midpoint collapses onto a neighbour,
// `respace` puts the sibling list back on the positionGap ladder and `neighbors`
// re-reads the two neighbour positions so the midpoint can be computed again.
// (Cards have their own version — positionForDrop — because which list to
// respace depends on whether the card is a subtask.)
func (h *API) positionForReorder(c *gin.Context, prev, next *float64,
	respace func() error, neighbors func() (*float64, *float64, bool),
) (float64, bool) {
	pos := positionBetween(prev, next)
	if !positionExhausted(pos, prev, next) {
		return pos, true
	}
	if err := respace(); err != nil {
		fail(c, err)
		return 0, false
	}
	prev, next, ok := neighbors()
	if !ok {
		return 0, false
	}
	return positionBetween(prev, next), true
}

// membershipResult is the cached outcome of a per-request membership lookup.
type membershipResult struct {
	role string
	ok   bool
}

// lookupMembership resolves the caller's membership in a workspace, memoized for
// the lifetime of the request. A single request often checks membership more than
// once (e.g. requireMember in a handler, then memberRole for a permission gate, or
// nested resource resolution) — without this each check was its own SQL round-trip.
func (h *API) lookupMembership(c *gin.Context, workspaceID uuid.UUID) membershipResult {
	key := "membership:" + workspaceID.String()
	if v, exists := c.Get(key); exists {
		return v.(membershipResult)
	}
	res := membershipResult{}
	m, err := h.q.GetMembership(c, db.GetMembershipParams{
		WorkspaceID: workspaceID,
		UserID:      middleware.CurrentUser(c),
	})
	if err == nil {
		res.role = m.Role
		res.ok = true
	}
	c.Set(key, res)
	return res
}

// requireMember authorizes the current user against a workspace. It writes a
// 403 and returns false when the user is not a member.
func (h *API) requireMember(c *gin.Context, workspaceID uuid.UUID) bool {
	if h.lookupMembership(c, workspaceID).ok {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this workspace"})
	return false
}

// memberRole returns the caller's role in a workspace, or "" if not a member.
func (h *API) memberRole(c *gin.Context, workspaceID uuid.UUID) string {
	return h.lookupMembership(c, workspaceID).role
}

// requireManager authorizes the caller as owner or admin of a workspace — the
// roles that manage members, roles and workspace settings (permission matrix,
// U1). Writes a 403 and returns false otherwise (non-members included).
func (h *API) requireManager(c *gin.Context, workspaceID uuid.UUID) bool {
	switch h.memberRole(c, workspaceID) {
	case "owner", "admin":
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "requires admin or owner"})
	return false
}

// broadcast fans a domain event out to connected clients scoped to a workspace.
func (h *API) broadcast(workspaceID uuid.UUID, eventType string, payload any) {
	h.broadcastEvent(workspaceID, eventType, "", payload)
}

// broadcastAs is broadcast with the acting user attached, so clients can
// attribute the event to a person (board-activity toasts). Use it for
// user-initiated actions that carry a gin.Context.
func (h *API) broadcastAs(c *gin.Context, workspaceID uuid.UUID, eventType string, payload any) {
	h.broadcastEvent(workspaceID, eventType, middleware.CurrentUser(c).String(), payload)
}

// broadcastByActor is broadcastAs for code paths that carry the acting user as a
// plain id instead of a gin.Context (the recurrence engine, shared by the HTTP
// handler and the worker). uuid.Nil means "nobody in particular" and MUST stay an
// empty actor: a zero uuid on the wire reads to clients as a real person, and the
// board-activity toast would credit the night's worker run to a member (#2932).
func (h *API) broadcastByActor(workspaceID uuid.UUID, eventType string, actorID uuid.UUID, payload any) {
	h.broadcastEvent(workspaceID, eventType, actorTag(actorID), payload)
}

// actorTag renders an actor id for the wire: empty string for uuid.Nil.
func actorTag(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

// broadcastToUser fans an event out to one person's sockets only, still inside
// the workspace scope. Use it whenever the payload is that person's business
// alone — leaving such an event to the plain workspace broadcast puts its text in
// every colleague's socket and leans on the client to throw it away.
func (h *API) broadcastToUser(workspaceID, userID uuid.UUID, eventType string, payload any) {
	h.broadcastEvent(workspaceID, eventType, "", payload, userID)
}

// broadcastEvent marshals the payload and fans the event out; actor may be "".
// An optional addressee narrows delivery to that user's sockets.
func (h *API) broadcastEvent(workspaceID uuid.UUID, eventType, actor string, payload any, to ...uuid.UUID) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var user uuid.UUID
	if len(to) > 0 {
		user = to[0]
	}
	h.hub.Broadcast(realtime.Event{
		Scope: workspaceID.String(), Type: eventType, Actor: actor, Data: data, User: user,
	})
}

// ptr returns a pointer to v — for the nullable sqlc params, where a literal
// constant cannot be addressed inline.
func ptr[T any](v T) *T { return &v }

// parseID reads a uuid path param, writing 400 on failure.
func parseID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return uuid.Nil, false
	}
	return id, true
}

// notFound reports whether err is pgx.ErrNoRows and, if so, writes 404.
//
// It is a writer, not a predicate: on true the response is already sent and the
// caller's only move is `return`. A caller with a better sentence to offer must
// reach for notFoundMsg instead of writing one after this — gin appends a second
// render to the first rather than replacing it.
func notFound(c *gin.Context, err error) bool {
	return notFoundMsg(c, err, "not found")
}

// notFoundMsg is notFound with the route's own sentence — the one that names
// WHICH task number or email came up empty, rather than the bare "not found".
//
// It exists because the obvious way to say that spelled `if notFound(c, err) {
// c.JSON(404, …) }`, and that is two answers to one request: gin's Render does
// not replace a body already written, it writes after it, so the wire carried
// `{"error":"not found"}{"error":"задача #5 не найдена"}`. Both clients parse a
// refusal strictly — axios' default transform runs JSON.parse, Gson refuses a
// document that is not fully consumed — so the doubled body parsed as nothing at
// all and the toast fell back to axios' own «Request failed with status code
// 404». The sentence the route went out of its way to write was unreachable from
// either client, and asking for it cost a malformed body (#2932).
func notFoundMsg(c *gin.Context, err error, format string, args ...any) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf(format, args...)})
		return true
	}
	return false
}

// isUniqueViolation reports Postgres's 23505.
//
// Worth a helper because a unique index plays two different roles here. Most of
// them guard against a bug, and tripping one belongs in `fail` — a 500 is the
// honest answer. But a few sit on names the user types (a project's tag names)
// or on a race the schema is deliberately arbitrating (the one-active-recording
// index), and there a 23505 is the expected outcome of an ordinary request. Those
// call sites answer 409 with a sentence instead of handing the reader "internal
// error" and Sentry a typed exception for something nobody broke.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// fail writes a generic 500.
// fail writes a generic 500 to the client and logs the real cause server-side,
// tagged with the request's method/path/id so a production log line can be tied
// back to a user report. The client only ever sees "internal error" — the cause
// never leaks. err may be nil (a handful of call sites fail without one).
func fail(c *gin.Context, err error) {
	slog.Error("request failed",
		"err", err,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"request_id", middleware.GetRequestID(c),
	)
	// Hand the real cause to the middleware chain. Nothing renders c.Errors, so
	// this stays invisible to the client; it is what lets middleware.SentryReport
	// capture a typed exception instead of the generic "HTTP 500" message it
	// would otherwise have to synthesise from the status line.
	if err != nil {
		_ = c.Error(err)
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}

// soft records a best-effort operation's failure without touching the response.
// Ignoring the result is often deliberate (a due-date already stamped, a log row
// that didn't write), but staying silent about it makes "why did the sync
// overwrite this" unanswerable. The op label names the action for the log; a nil
// error is silent. ctx is a gin.Context or a worker's context.Context.
func soft(ctx context.Context, op string, err error) {
	if err == nil {
		return
	}
	slog.Warn("best-effort op failed",
		"op", op,
		"err", err,
		"request_id", middleware.GetRequestID(ctx),
	)
}

// inTx runs fn inside a database transaction, rolling back if fn returns an
// error (or panics on the way out) and committing otherwise. It centralises the
// begin/rollback/commit boilerplate so multi-statement mutations stay
// all-or-nothing — a partial apply leaves orphans (a workspace with no
// membership) or drops data (a DELETE that commits before its re-INSERT).
func (h *API) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after a successful Commit
	if err := fn(h.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// callerLocation resolves the calendar the caller lives in, for the handlers that
// bucket data by day: the zone sent with the request, else the one stored in the
// profile, else the server's own.
//
// The query parameter comes first because an empty profile zone does not mean UTC
// on the web client — it means "whatever the browser is", a blank the server
// cannot re-derive, which is why anything sent to the backend carries the resolved
// zone (see resolvedTimezone in frontend/src/utils/format.js). A zone that fails
// to load falls through to the next source rather than failing the request: these
// are summary counts, and a client that sent a typo gets the old behaviour, not a
// broken screen.
func (h *API) callerLocation(c *gin.Context, uid uuid.UUID) *time.Location {
	if loc, ok := loadLocation(c.Query("tz")); ok {
		return loc
	}
	return h.actorLocation(c, &uid)
}

// actorLocation is callerLocation for work that runs outside the request that
// caused it — the GitLab writeback worker pushing a due date, say. There is no
// query parameter to consult there, so the stored profile zone is the whole
// answer, and an actor who never set one (or a row old enough to predate the
// column) leaves the server's own zone, which is what that code did before.
func (h *API) actorLocation(ctx context.Context, uid *uuid.UUID) *time.Location {
	if uid != nil {
		if p, err := h.q.GetUserPreferences(ctx, *uid); err == nil {
			if loc, ok := loadLocation(p.Timezone); ok {
				return loc
			}
		}
	}
	return time.Local
}

// loadLocation resolves an IANA zone name, reporting whether it named one at all.
// An empty string is "unset", not UTC — time.LoadLocation maps it to UTC, which
// would swallow the difference between "no preference" and "explicitly UTC".
func loadLocation(tz string) (*time.Location, bool) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return nil, false
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, false
	}
	return loc, true
}
