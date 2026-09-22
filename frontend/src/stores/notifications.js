import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { notifications as api } from '@/api'
import { getDeviceId, notificationsSupported } from '@/utils/device'
import { isTauri } from '@/utils/serverBase'
import { notificationText, notificationTitle } from '@/utils/notificationText'

// notifications store — persistent, server-backed feed for the bell (feature
// #3). New notifications also arrive live over the workspace socket.
export const useNotificationsStore = defineStore('notifications', () => {
  const items = ref([])
  const loaded = ref(false)

  const unread = computed(() => items.value.filter((n) => !n.read_at).length)

  // The feed has no single caller: it is read at mount and again on every resync
  // (see utils/shellResync.js) — which is to say, exactly while the link is
  // flapping. So two requests can be in flight at once, and a bare assignment
  // would leave on screen whichever one ANSWERED last rather than whichever one
  // was ASKED last (#2932).
  let loadSeq = 0
  // Rows the socket delivered while a load was in flight. The server took its
  // snapshot before them, so the answer cannot know about them — and dropping
  // them is the loudest form of this bug: a resync raises the load, the
  // just-restored socket delivers a mention, and the answer erases both the row
  // and the badge. That missed-while-offline traffic is what resync exists for.
  // Only the flights still open collect; once an answer is on screen the server
  // knows about the row and decides for itself whether it is still in the feed.
  const arrivals = new Set()

  async function load() {
    const seq = ++loadSeq
    const live = []
    arrivals.add(live)
    try {
      const res = await api.list()
      if (seq !== loadSeq) return // a later load has already been asked for
      const rows = res.data || []
      for (let i = live.length - 1; i >= 0; i--) {
        if (!rows.some((x) => x.id === live[i].id)) rows.unshift(live[i])
      }
      items.value = rows.map(withLocalRead)
      loaded.value = true
    } catch {
      /* offline / unauthorized — leave whatever we had */
    } finally {
      arrivals.delete(live)
    }
  }

  // Read state only ever moves one way — nothing in the product makes a row
  // unread again — so an optimistic mark taken while the load was in flight
  // outranks an answer that predates it. Without this, clicking a notification
  // at the moment of a resync puts the row (and the badge) back to unread until
  // the next load. A refusal still rolls the row back: `restore` runs on
  // whatever array is current by then.
  function withLocalRead(n) {
    if (n.read_at) return n
    const prev = items.value.find((x) => x.id === n.id)
    return prev?.read_at ? { ...n, read_at: prev.read_at } : n
  }

  // Handle a realtime "notification" event addressed to the current user.
  function onEvent(ev, meId) {
    if (ev.type !== 'notification') return
    if (!ev.data || ev.data.user_id !== meId) return
    const n = ev.data.notification
    if (!n) return
    // Raise a native OS notification if a routing rule targeted this device.
    maybeNotifyDevice(ev.data.device_targets, n)
    if (items.value.some((x) => x.id === n.id)) return
    items.value.unshift(n)
    if (items.value.length > 50) items.value.pop()
    // Remembered for every load still in flight, whose answer was snapshotted
    // before this row existed (see `arrivals`).
    for (const live of arrivals) live.push(n)
  }

  // Raise a native OS notification when this device's id is among the event's
  // targets. Best-effort. On desktop (Tauri) this goes through the notification
  // plugin (the WebKitGTK webview may lack window.Notification); on web it uses
  // the Web Notifications API unchanged.
  async function maybeNotifyDevice(targets, n) {
    try {
      if (!Array.isArray(targets) || !targets.includes(getDeviceId())) return
      // Title and body are rendered from the payload in the user's language; a
      // pre-payload row still delivers its stored sentence (see notificationText).
      const title = notificationTitle(n.kind)
      const body = notificationText(n)
      if (isTauri()) {
        const { isPermissionGranted, requestPermission, sendNotification } =
          await import('@tauri-apps/plugin-notification')
        let granted = await isPermissionGranted()
        if (!granted) granted = (await requestPermission()) === 'granted'
        if (granted)
          sendNotification({
            title,
            body,
            // Carried back on click for deep-linking (see useDesktopDeepLink).
            // A conference invitation carries a call id instead of a board
            // (#2875) — it is about no task.
            extra: {
              task_board_id: n.task_board_id,
              task_number: n.task_number,
              conference_id: n.payload?.conference_id,
            },
          })
        return
      }
      if (!notificationsSupported() || Notification.permission !== 'granted') return
      void new Notification(title, { body, tag: n.id })
    } catch {
      /* notifications unavailable — ignore */
    }
  }

  // Marking read is optimistic — the bell answers the click before the server
  // does — and an optimistic write owes the reader a rollback. Both calls used to
  // swallow the refusal in an empty `catch`: the unread count dropped to zero, the
  // server kept every row unread, and the feed went on claiming otherwise until the
  // next load() brought them all back with no explanation. The board settled this
  // class long ago (onColChange, onQuadChange, moveSidebar*: report and resync);
  // these two were the last optimistic writes without an undo (#2932).
  //
  // The toast comes in as an argument, like useSidebarDnd's: useMessage() is only
  // reachable from a setup context and a store is instantiated from anywhere.
  //
  // Rollback restores exactly what the optimistic pass overwrote, looked up by id:
  // a row read earlier must not be dragged back into unread, a row that arrived
  // over the socket mid-flight was never touched, and one the feed dropped
  // meanwhile is not resurrected.
  async function markRead(id, message) {
    const n = items.value.find((x) => x.id === id)
    // The request goes out either way — the row may have aged out of the capped
    // feed — but only a write of our own is undone.
    const marked = !!n && !n.read_at
    if (marked) n.read_at = new Date().toISOString()
    try {
      await api.markRead(id)
    } catch (e) {
      if (marked) restore([id])
      message?.error(e.message)
    }
  }

  async function markAllRead(message) {
    const now = new Date().toISOString()
    const marked = []
    for (const n of items.value) {
      if (n.read_at) continue
      n.read_at = now
      marked.push(n.id)
    }
    try {
      await api.markAllRead()
    } catch (e) {
      restore(marked)
      message?.error(e.message)
    }
  }

  function restore(ids) {
    for (const id of ids) {
      const n = items.value.find((x) => x.id === id)
      if (n) n.read_at = null
    }
  }

  // reset drops the feed when the session ends. The bell is per-user and the app
  // does not reload on sign-out — only a server switch does (stores/servers.js) —
  // so without this the next sign-in in the same tab inherits the previous
  // reader's rows and badge: their mentions, with the task titles in them, on
  // someone else's screen. Visible until load() answers, and for good when it
  // does not: the catch there deliberately keeps whatever was on screen, which is
  // right for a flapping link and wrong for a feed that belongs to nobody (#2932).
  //
  // Bumping the sequence is the other half. A load raised for the departing
  // session can still be in flight, and an answer that lands after the clear is
  // indistinguishable from a fresh one — it would refill the feed a moment later.
  //
  // Emptying `arrivals` changes nothing on screen (the sequence check already
  // throws that answer away before it reads its collector); it is there so a
  // request that never settles — the offline case this feed is full of — stops
  // being a place every later onEvent appends to for the rest of the tab's life.
  function reset() {
    loadSeq++
    arrivals.clear()
    items.value = []
    loaded.value = false
  }

  return { items, unread, loaded, load, onEvent, markRead, markAllRead, reset }
})
