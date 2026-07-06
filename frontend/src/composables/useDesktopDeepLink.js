import { onMounted, onUnmounted } from 'vue'
import { isTauri } from '@/utils/serverBase'

// Desktop: clicking a native notification should focus the window and open the
// task it's about. The notification carries { task_board_id, task_number } in its
// `extra`; on click we surface the window and route to the board with the task
// query (BoardView canonicalises /board/:id → the slug URL and opens the modal).
// Best-effort — no-op on web, and if the OS doesn't deliver the action payload it
// simply won't deep-link (the window still comes forward via tray/single-instance).
export function useDesktopDeepLink(router) {
  if (!isTauri()) return

  let unlisten = null

  async function focusWindow() {
    try {
      const { getCurrentWindow } = await import('@tauri-apps/api/window')
      const w = getCurrentWindow()
      await w.show()
      await w.unminimize()
      await w.setFocus()
    } catch {
      /* ignore */
    }
  }

  onMounted(async () => {
    try {
      const { onAction } = await import('@tauri-apps/plugin-notification')
      unlisten = await onAction((notification) => {
        const extra = notification?.extra || {}
        focusWindow()
        if (extra.task_board_id) {
          router.push({
            path: `/board/${extra.task_board_id}`,
            query: extra.task_number ? { task: extra.task_number } : {},
          })
        }
      })
    } catch {
      /* plugin/listener unavailable — window still focuses via tray/single-instance */
    }
  })

  onUnmounted(() => {
    if (typeof unlisten === 'function') unlisten()
  })
}
