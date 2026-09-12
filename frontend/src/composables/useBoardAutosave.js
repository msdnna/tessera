import { onBeforeUnmount, watch } from 'vue'

// Debounced re-save of the *named* saved view while autosave is on.
//
// Split out of KanbanBoard for the same reason `useBoardViewConfig` was: the
// mechanics (signature watch, debounce, guard, teardown) are generic, the shape of
// a view config is not. The caller passes `config()` and `save()`; this owns the timer.
//
// The teardown is the load-bearing part. The card-display half of a view config
// (cardSize / stackFields / showEmpty / fieldVis) lives in the boardView store, and
// the board's own `onBeforeUnmount` resets that store. A timer left running past
// unmount therefore fires against a *reset* store and silently overwrites the user's
// named view with the defaults — and, because a freshly mounted board restores its
// own layout into the same store ref, with the wrong `layout` too, which drops the
// view out of its board's list entirely (views are filtered by layout). So the timer
// is cancelled, not flushed: once the store is torn down there is nothing worth saving.
export const AUTOSAVE_MS = 700 // longer than VIEW_PERSIST_MS: this one costs a request

export function useBoardAutosave({ enabled, viewName, config, save, guarded }) {
  let timer = null

  watch(
    () => (enabled.value && viewName.value ? JSON.stringify(config()) : ''),
    (sig) => {
      if (!sig || guarded()) return
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        timer = null
        const name = viewName.value.trim()
        if (!name) return
        save(name, config())
      }, AUTOSAVE_MS)
    },
  )

  onBeforeUnmount(() => {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  })
}
