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

  async function load() {
    try {
      const res = await api.list()
      items.value = res.data || []
      loaded.value = true
    } catch {
      /* offline / unauthorized — leave whatever we had */
    }
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

  function reset() {
    items.value = []
    loaded.value = false
  }

  return { items, unread, loaded, load, onEvent, markRead, markAllRead, reset }
})
