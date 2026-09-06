package handlers

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "tessera/internal/db"
	"tessera/internal/jobs"
)

// ── GitLab webhooks: near-realtime sync trigger (#2594) ────
//
// The hook deliberately does NOT map the issue itself. It authenticates the
// delivery, marks the integration dirty and answers 202 immediately; a debounced
// background worker then runs the ordinary *incremental* pull. That keeps a single
// sync engine (labels → column/priority/tags, subtasks, relations, comments,
// closed policy, conflicts, journal) instead of a second path that would drift
// from the first. The cost is ~3–5 s of latency instead of instant — against the
// 5-minute polling interval it is still two orders of magnitude faster.
//
// Polling stays on: an issue hook does not report issue *deletion*, so only the
// periodic full sweep catches deletes, and a hook GitLab has quietly disabled must
// not turn into a silently broken sync.

const (
	// webhookMaxBody caps the delivery body; an issue hook with a long description
	// and many labels is fat, but never megabytes.
	webhookMaxBody = 1 << 20 // 1 MiB
	// webhookDebounce is the quiet period after the last delivery before syncing —
	// a mass label edit fires dozens of events for one logical change.
	webhookDebounce = 3 * time.Second
	// webhookMaxWait bounds the debounce: under a continuous stream, waiting for
	// silence would postpone the sync forever.
	webhookMaxWait = 15 * time.Second
	// webhookTick is how often the worker looks at the dirty set.
	webhookTick = time.Second
)

// ── Delivery classification (pure, tested in isolation) ────

// hookOutcome is what we decided to do with a delivery body.
type hookOutcome int

const (
	hookRelevant  hookOutcome = iota // → 202 + schedule an incremental pull
	hookIgnored                      // → 204: a real event we don't sync on (push, pipeline…)
	hookForeign                      // → 400: an event for a different GitLab project
	hookMalformed                    // → 400: not a GitLab hook payload at all
)

// webhookPayload is the minimum we read out of a delivery. Everything else is the
// pull's business.
type webhookPayload struct {
	ObjectKind string `json:"object_kind"`
	EventType  string `json:"event_type"`
	Project    struct {
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
}

// relevantHookKinds are the event kinds that can change what we mirror: the issue
// itself (work_item is the newer name), its comments, and the confidential
// variants GitLab sends under their own kind.
var relevantHookKinds = map[string]bool{
	"issue":              true,
	"work_item":          true,
	"note":               true,
	"confidential_issue": true,
	"confidential_note":  true,
}

// webhookDecision classifies a delivery body for the integration bound to
// projectPath. It is deliberately total and side-effect free: every branch the
// handler can take is decided here, so the whole matrix is table-testable without
// a server. reason is for the log/response, never for the client to parse.
func webhookDecision(body []byte, projectPath string) (hookOutcome, string) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return hookMalformed, "body is not JSON"
	}
	kind := p.ObjectKind
	if kind == "" {
		kind = p.EventType
	}
	if kind == "" {
		return hookMalformed, "no object_kind"
	}
	if !relevantHookKinds[kind] {
		// 204, not an error: GitLab disables a hook that keeps failing, and an
		// over-broad event selection in the project settings is the user's to fix.
		return hookIgnored, "kind " + kind + " does not affect mirrored issues"
	}
	// A hook URL pasted into the wrong project would quietly sync the wrong data.
	// An empty path can't be checked — the shared secret already proved the caller
	// holds this integration's token, so accept it rather than reject a payload
	// shape we haven't seen.
	if got := p.Project.PathWithNamespace; got != "" && !strings.EqualFold(got, projectPath) {
		return hookForeign, "event belongs to " + got
	}
	return hookRelevant, "kind " + kind
}

// ── Debounced dirty set ────────────────────────────────────

// dirtyMark remembers when an integration first went dirty in the current burst
// and when its latest delivery arrived.
type dirtyMark struct{ firstSeen, lastSeen time.Time }

// dirtyEntry is one due integration handed to the worker.
type dirtyEntry struct {
	id   uuid.UUID
	mark dirtyMark
}

// webhookQueue is the in-memory set of integrations awaiting a webhook-triggered
// pull. In-memory on purpose: the queue holds at most a few seconds of intent, and
// anything lost to a restart is picked up by the next polling tick — which is the
// fallback the task asks for.
type webhookQueue struct {
	mu    sync.Mutex
	dirty map[uuid.UUID]dirtyMark
}

// mark records a delivery for an integration, opening a burst if none is open.
func (q *webhookQueue) mark(id uuid.UUID, now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.dirty == nil {
		q.dirty = map[uuid.UUID]dirtyMark{}
	}
	m, ok := q.dirty[id]
	if !ok {
		m.firstSeen = now
	}
	m.lastSeen = now
	q.dirty[id] = m
}

// due lists the integrations whose burst has gone quiet (debounce) or has been
// running long enough that waiting for quiet is pointless (maxWait). Marks are
// left in place: the caller clears one only once its sync actually ran, so a
// tick that can't start the sync doesn't drop the event.
func (q *webhookQueue) due(now time.Time, debounce, maxWait time.Duration) []dirtyEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []dirtyEntry
	for id, m := range q.dirty {
		if now.Sub(m.lastSeen) >= debounce || now.Sub(m.firstSeen) >= maxWait {
			out = append(out, dirtyEntry{id: id, mark: m})
		}
	}
	return out
}

// clear drops an integration's mark, but only when no newer delivery arrived while
// its sync was running — otherwise that delivery would be swallowed by the sync it
// is not covered by.
func (q *webhookQueue) clear(id uuid.UUID, syncedThrough time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if m, ok := q.dirty[id]; ok && !m.lastSeen.After(syncedThrough) {
		delete(q.dirty, id)
	}
}

// ── Public receiver ────────────────────────────────────────

// GitlabWebhook accepts a delivery for one binding and schedules an incremental
// pull. Public (no session): authentication is the per-integration shared secret in
// X-Gitlab-Token. The integration id is in the URL rather than resolved from the
// payload's project path, because that path is unique only within a workspace
// (gitlab_integrations_ws_project_key) — two workspaces may mirror one project.
//
// Answers within milliseconds by design: GitLab times a delivery out after 10 s and
// disables a hook that keeps failing, so no sync may happen on this goroutine.
func (h *API) GitlabWebhook(c *gin.Context) {
	integID, ok := parseID(c, "integrationId")
	if !ok {
		return
	}
	integ, err := h.q.GetGitlabIntegration(c, integID)
	// Unknown, disabled or hook-less binding: 404 without detail, so the endpoint
	// can't be used to probe which integration ids exist.
	if err != nil || !integ.WebhookEnabled || integ.WebhookSecretEnc == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	secret, derr := h.sealer.Decrypt(integ.WebhookSecretEnc)
	if derr != nil || secret == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// Constant-time compare; a token that is empty (or the wrong length) must not
	// short-circuit into a match.
	got := c.GetHeader("X-Gitlab-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	body, rerr := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, webhookMaxBody))
	if rerr != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "body too large"})
		return
	}
	switch outcome, reason := webhookDecision(body, integ.ProjectPath); outcome {
	case hookIgnored:
		c.Status(http.StatusNoContent)
		return
	case hookForeign, hookMalformed:
		log.Printf("gitlab webhook integ=%s rejected: %s", integ.ID, reason)
		c.JSON(http.StatusBadRequest, gin.H{"error": "unexpected payload"})
		return
	}

	if err := h.q.MarkGitlabWebhookSeen(c, integ.ID); err != nil {
		fail(c, err)
		return
	}
	// Disabled bindings still stamp last_webhook_at (so the UI can show the hook is
	// wired up) but must not trigger a pull.
	if integ.Enabled {
		h.webhooks.mark(integ.ID, time.Now())
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": integ.Enabled})
}

// ── Debounce worker ────────────────────────────────────────

// RunWebhookSyncWorker drains the webhook dirty set on a short tick. Blocks until
// ctx is done; start it via spawn so it joins graceful shutdown.
func (h *API) RunWebhookSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(webhookTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobGitlabWebhookCron, opWebhookScan)
			// Shares the auto-sync worker's advisory lock: the dirty set is
			// process-local, but the *sync* it triggers must not run beside the cron
			// one on another process.
			h.withAdvisoryLock(ctx, "gitlab_sync", func() { h.drainWebhookDirty(ctx) })
		}
	}
}

// drainWebhookDirty syncs every integration whose webhook burst has settled. One
// run per integration; the mark survives a tick that couldn't start (another sync
// in flight) so the event is retried rather than lost.
func (h *API) drainWebhookDirty(ctx context.Context) {
	now := time.Now()
	for _, e := range h.webhooks.due(now, webhookDebounce, webhookMaxWait) {
		integ, err := h.q.GetGitlabIntegration(ctx, e.id)
		if err != nil || !integ.Enabled {
			// Deleted or switched off while the burst was settling — drop it.
			h.webhooks.clear(e.id, now)
			continue
		}
		cred, actor, credOK := h.syncCredential(ctx, integ)
		if !credOK {
			// No service token and no owner credential: nothing can drive the pull.
			// Dropping the mark avoids a hot retry loop; polling reports the same.
			h.webhooks.clear(e.id, now)
			continue
		}
		syncCtx, cancel := context.WithCancel(ctx)
		handle, started := h.jobs.Begin(gitlabSyncKey(integ.ID), gitlabSyncName(integ), jobs.KindSync, integ.WorkspaceID.String(), cancel)
		if !started {
			// A manual or auto run is already in flight — keep the mark and retry on
			// the next tick, so its changes aren't missed by the run already going.
			cancel()
			continue
		}
		handle.SetOp(syncOpKey("incremental"), syncOpLabel("incremental"))
		created, updated, serr := h.runSync(syncCtx, integ, cred, actor, "webhook", "incremental")
		handle.SetCounts(created, updated)
		handle.Finish(serr)
		cancel()
		// Only deliveries up to the ones this run covered are cleared; anything that
		// arrived meanwhile reopens the burst.
		h.webhooks.clear(e.id, e.mark.lastSeen)
		if serr != nil {
			log.Printf("gitlab webhook sync ws=%s: %v", integ.WorkspaceID, serr)
			continue
		}
		if created+updated > 0 {
			log.Printf("gitlab webhook sync ws=%s: +%d new, ~%d updated", integ.WorkspaceID, created, updated)
		}
	}
}

// syncCredential resolves what drives an unattended pull for an integration, in the
// same order as autoSyncDue: the instance service token when configured (the owner
// is then only the journal actor), else the owner's personal PAT.
func (h *API) syncCredential(ctx context.Context, integ db.GitlabIntegration) (db.GitlabCredential, uuid.UUID, bool) {
	actor := uuid.Nil
	if _, _, hasService := h.serviceGitlabConn(ctx); hasService {
		if integ.OwnerUserID != nil {
			actor = *integ.OwnerUserID
		}
		return db.GitlabCredential{}, actor, true
	}
	if integ.OwnerUserID == nil {
		return db.GitlabCredential{}, uuid.Nil, false
	}
	cred, err := h.q.GetGitlabCredential(ctx, *integ.OwnerUserID)
	if err != nil {
		return db.GitlabCredential{}, uuid.Nil, false
	}
	return cred, *integ.OwnerUserID, true
}

// ── Secret management (authenticated) ──────────────────────

// EnableGitlabWebhook generates a fresh shared secret for a binding, turns the hook
// on and returns the URL + secret to paste into GitLab's Settings → Webhooks. The
// secret is shown HERE AND ONLY HERE — it is stored encrypted and never read back
// by any GET. Calling it again rotates the secret (the old one stops working).
func (h *API) EnableGitlabWebhook(c *gin.Context) {
	integ, _, ok := h.integrationInWorkspace(c)
	if !ok {
		return
	}
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fail(c, err)
		return
	}
	secret := hex.EncodeToString(raw)
	enc, err := h.sealer.Encrypt(secret)
	if err != nil {
		fail(c, err)
		return
	}
	updated, err := h.q.SetGitlabWebhook(c, db.SetGitlabWebhookParams{ID: integ.ID, WebhookSecretEnc: enc})
	if err != nil {
		fail(c, err)
		return
	}
	view := h.fullIntegrationView(c, updated)
	view.WebhookURL = h.webhookURL(c, updated.ID)
	c.JSON(http.StatusOK, gin.H{"integration": view, "secret": secret})
}

// DisableGitlabWebhook turns the hook off and wipes the stored secret, so deliveries
// that still carry it are refused. The hook itself stays in the GitLab project — we
// never created it, and we don't remove it.
func (h *API) DisableGitlabWebhook(c *gin.Context) {
	integ, _, ok := h.integrationInWorkspace(c)
	if !ok {
		return
	}
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	updated, err := h.q.ClearGitlabWebhook(c, integ.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, h.fullIntegrationView(c, updated))
}

// webhookURL builds the address to register in GitLab. PUBLIC_URL is the truth in
// production; in dev it is usually unset, so fall back to the request's own host —
// which is exactly the address the admin is looking at.
func (h *API) webhookURL(c *gin.Context, integrationID uuid.UUID) string {
	base := strings.TrimRight(h.publicURL, "/")
	if base == "" {
		scheme := "http"
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + c.Request.Host
	}
	return base + "/api/gitlab/webhook/" + integrationID.String()
}
