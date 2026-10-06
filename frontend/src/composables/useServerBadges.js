import axios from 'axios'
import { onMounted, onBeforeUnmount } from 'vue'
import { useServersStore } from '@/stores/servers'
import { sGetFor, sSetFor, sRemoveFor } from '@/utils/serverStorage'

// Unread-notification counts for the servers the app is NOT currently pointed at
// (#2930) — so the rail can say *where* something arrived, not just that the
// bell on this server is quiet.
//
// The active server is excluded on purpose: its count already comes from the
// live notifications store over the websocket, and asking again would be a
// second request for a number we have.
//
// Why this is careful about tokens. A refresh token is the ONLY handle to a
// server's session, it is single-use, and refreshing rotates it: the backend
// revokes the presented token and issues a new one. So every refresh this poller
// does is a moment where, if the response is lost (a dropped desktop link, the
// window sleeping mid-request), the new token never reaches disk and the old one
// is already dead. Left alone, the *next* poll 60 s later would present that dead
// token, get a 401, and delete it — silently signing the user out of a server
// they never touched, which is exactly the "switching logs me out" report that
// sent #2930 back. Two defences below: (1) hold the short-lived access token in
// memory and only refresh when it has actually expired, so rotations are rare
// (~once per access-token lifetime) instead of once a minute; (2) when a refresh
// response is lost, retry at once — inside the backend's 30 s rotation grace the
// revoked token still answers with a fresh pair, so a lost rotation heals instead
// of stranding the session.

const POLL_MS = 60_000
const TIMEOUT_MS = 8_000
// Treat a cached access token as usable until a minute before its 15 min TTL —
// the margin absorbs clock skew and the round-trip so the count request never
// races the expiry. A refresh only happens once this runs out.
const ACCESS_TTL_MS = 15 * 60_000
const ACCESS_MARGIN_MS = 60_000
// Self-heal budget for a lost rotation. Both attempts have to finish inside the
// backend's 30 s replay grace, so the total stays well under it:
// 8 + 1.5 + 8 ≈ 17.5 s.
const RETRY_GAP_MS = 1_500
const REFRESH_ATTEMPTS = 2

// serverId -> { token, exp } — access tokens live only in memory (like the
// active session's, see api/index.js) and die with the page. Never persisted.
const accessCache = new Map()

// Drops the in-memory access-token cache. Exported for tests, which reuse the
// module across cases and would otherwise see one case's cached token in the next.
export function resetBadgeCache() {
  accessCache.clear()
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// isTransient tells a lost/unavailable refresh (retry — the session may be fine)
// apart from an answered "no" (401/403 — the session is genuinely over). No
// response at all, a timeout, or a 5xx/408/429 all mean "we never got a verdict".
function isTransient(err) {
  const status = err?.response?.status
  return !err?.response || status >= 500 || status === 408 || status === 429
}

// rotate exchanges the stored refresh token for a fresh pair, persisting the new
// refresh token the instant it arrives and caching the access token in memory.
// Retries a lost response inside the rotation grace so it heals rather than
// stranding the token. Returns the access token, or null when there is none to
// get (no stored token). Throws only on a definitive 401/403 (session over) — a
// purely transient failure resolves to the last cached access token (possibly
// null), leaving the stored refresh token untouched for the next poll.
async function rotate(server) {
  const stored = sGetFor('tessera_refresh_token', server.id)
  if (!stored) return null
  let lastErr = null
  for (let attempt = 0; attempt < REFRESH_ATTEMPTS; attempt++) {
    if (attempt > 0) await sleep(RETRY_GAP_MS)
    try {
      const res = await axios.post(
        `${server.url}/api/auth/refresh`,
        { refresh_token: stored },
        { timeout: TIMEOUT_MS },
      )
      const data = res.data || {}
      // Persist the rotated token before anything else can go wrong — an
      // exception after this point must not leave the old, now-dead token on disk.
      if (data.refresh_token) sSetFor('tessera_refresh_token', server.id, data.refresh_token)
      if (!data.access_token) return null
      accessCache.set(server.id, { token: data.access_token, exp: Date.now() + ACCESS_TTL_MS })
      return data.access_token
    } catch (e) {
      lastErr = e
      if (!isTransient(e)) throw e // 401/403: stop retrying, the session is gone
    }
  }
  // Every attempt was transient — we still don't know the session's fate. Leave
  // the stored token alone (the grace window may yet accept it) and fall back to
  // whatever access token we already had, if any.
  if (lastErr) throw Object.assign(lastErr, { _transient: true })
  return accessCache.get(server.id)?.token || null
}

// pollServer refreshes one inactive server's unread count. It refreshes the
// session only when the cached access token has run out, so most polls are a
// single GET that touches no refresh token at all.
export async function pollServer(store, server) {
  if (!sGetFor('tessera_refresh_token', server.id)) {
    accessCache.delete(server.id)
    store.setBadge(server.id, 0)
    store.setSignedOut(server.id, true)
    return
  }
  try {
    let access = validAccess(server.id)
    if (!access) access = await rotate(server)
    if (!access) return
    let count
    try {
      count = await fetchCount(server, access)
    } catch (e) {
      // A 401 on the count means the cached access token expired early; a fresh
      // rotation is the fix, and its own 401 (below) is the real verdict.
      if (e?.response?.status !== 401) throw e
      accessCache.delete(server.id)
      access = await rotate(server)
      if (!access) return
      count = await fetchCount(server, access)
    }
    store.setBadge(server.id, count)
    store.setSignedOut(server.id, false)
    // The same token lists that server's workspaces for its group on the rail
    // (#2955). Best-effort: a failure keeps the cached tiles.
    try {
      store.setWorkspaces(server.id, await fetchWorkspaces(server, access))
    } catch {
      /* keep the cached list */
    }
  } catch (e) {
    // Only an answered "no" ends the session. A transient failure keeps the last
    // known badge and is retried next tick — clearing the token there would sign
    // the user out of a live account over a dropped connection.
    if (e?._transient || isTransient(e)) return
    accessCache.delete(server.id)
    sRemoveFor('tessera_refresh_token', server.id)
    store.setBadge(server.id, 0)
    store.setSignedOut(server.id, true)
  }
}

function validAccess(serverId) {
  const c = accessCache.get(serverId)
  if (c && c.exp - ACCESS_MARGIN_MS > Date.now()) return c.token
  return null
}

async function fetchCount(server, access) {
  const res = await axios.get(`${server.url}/api/notifications/unread-count`, {
    headers: { Authorization: `Bearer ${access}` },
    timeout: TIMEOUT_MS,
  })
  return Number(res.data?.count) || 0
}

async function fetchWorkspaces(server, access) {
  const res = await axios.get(`${server.url}/api/workspaces`, {
    headers: { Authorization: `Bearer ${access}` },
    timeout: TIMEOUT_MS,
  })
  return Array.isArray(res.data) ? res.data : []
}

export function useServerBadges() {
  const store = useServersStore()
  let timer = 0
  let inFlight = false

  // Strictly one server at a time, and never two passes at once (the focus
  // handler and the interval can otherwise overlap). Two concurrent refreshes
  // for the same server would race the same rotation and waste the grace window.
  async function poll() {
    if (inFlight || !store.enabled) return
    inFlight = true
    try {
      for (const server of store.others) {
        if (store.signedOut[server.id]) continue
        await pollServer(store, server)
      }
    } finally {
      inFlight = false
    }
  }

  function onFocus() {
    poll()
  }

  onMounted(() => {
    if (!store.enabled) return
    poll()
    timer = setInterval(poll, POLL_MS)
    window.addEventListener('focus', onFocus)
  })

  onBeforeUnmount(() => {
    if (timer) clearInterval(timer)
    window.removeEventListener('focus', onFocus)
  })

  return { poll }
}
