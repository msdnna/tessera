import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { notifications as api } from '@/api'
import { getDeviceId, notificationsSupported } from '@/utils/device'
import { isTauri } from '@/utils/serverBase'

// Human title for a native (OS) notification, by kind.
const KIND_TITLE = {
  assigned: 'Назначена задача',
  comment: 'Новый комментарий',
  mention: 'Вас упомянули',
  updated: 'Задача изменена',
  moved: 'Задача перемещена',
  archived: 'Задача архивирована',
  due_soon: 'Скоро дедлайн',
  reminder: 'Напоминание',
}

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
      const title = KIND_TITLE[n.kind] || 'Tessera'
      if (isTauri()) {
        const { isPermissionGranted, requestPermission, sendNotification } = await import(
          '@tauri-apps/plugin-notification'
        )
        let granted = await isPermissionGranted()
        if (!granted) granted = (await requestPermission()) === 'granted'
        if (granted)
          sendNotification({
            title,
            body: n.text,
            // Carried back on click for deep-linking (see useDesktopDeepLink).
            extra: { task_board_id: n.task_board_id, task_number: n.task_number },
          })
        return
      }
      if (!notificationsSupported() || Notification.permission !== 'granted') return
      void new Notification(title, { body: n.text, tag: n.id })
    } catch {
      /* notifications unavailable — ignore */
    }
  }

  async function markRead(id) {
    const n = items.value.find((x) => x.id === id)
    if (n && !n.read_at) n.read_at = new Date().toISOString()
    try {
      await api.markRead(id)
    } catch {
      /* best-effort */
    }
  }

  async function markAllRead() {
    const now = new Date().toISOString()
    for (const n of items.value) if (!n.read_at) n.read_at = now
    try {
      await api.markAllRead()
    } catch {
      /* best-effort */
    }
  }

  function reset() {
    items.value = []
    loaded.value = false
  }

  return { items, unread, loaded, load, onEvent, markRead, markAllRead, reset }
})
