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

const POLL_MS = 60_000
const TIMEOUT_MS = 15_000

// pollServer asks one server for its unread count, standing up a short-lived
// session from that server's stored refresh token.
//
// It uses a bare axios call rather than the app's `api` instance deliberately:
// that one is bound to the active server's baseURL and to the in-memory access
// token of the session the user is actually looking at. Writing into it would
// hand server A's token to server B.
export async function pollServer(store, server) {
  const stored = sGetFor('tessera_refresh_token', server.id)
  if (!stored) {
    store.setBadge(server.id, 0)
    store.setSignedOut(server.id, true)
    return
  }
  try {
    const res = await axios.post(
      `${server.url}/api/auth/refresh`,
      { refresh_token: stored },
      { timeout: TIMEOUT_MS },
    )
    const data = res.data || {}
    // Refresh tokens rotate on every use and the backend treats a *reused* one
    // as a stolen one (it revokes the whole chain). So the new token is written
    // before anything else can go wrong — an exception between here and the end
    // of the function must not leave the old, now-dead token on disk.
    if (data.refresh_token) sSetFor('tessera_refresh_token', server.id, data.refresh_token)
    if (!data.access_token) return
    const counted = await axios.get(`${server.url}/api/notifications/unread-count`, {
      headers: { Authorization: `Bearer ${data.access_token}` },
      timeout: TIMEOUT_MS,
    })
    store.setBadge(server.id, Number(counted.data?.count) || 0)
    store.setSignedOut(server.id, false)
  } catch (e) {
    const status = e?.response?.status
    // Only an answered "no" ends the session — same rule the api layer follows.
    // An unreachable server keeps its last known badge and is retried next tick;
    // clearing the token there would sign the user out of a live account
    // because their laptop lost wifi for a minute.
    if (status === 401 || status === 403) {
      sRemoveFor('tessera_refresh_token', server.id)
      store.setBadge(server.id, 0)
      store.setSignedOut(server.id, true)
    }
  }
}

export function useServerBadges() {
  const store = useServersStore()
  let timer = 0
  let inFlight = false

  // Strictly one server at a time, and never two passes at once (the focus
  // handler and the interval can otherwise overlap). Two concurrent refreshes
  // for the same server would post the same rotated token twice — which is
  // exactly the reuse the backend revokes sessions for.
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
