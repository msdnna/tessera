import { computed, nextTick, onBeforeUnmount, watch } from 'vue'

// Per-board, per-layout toolbar persistence (localStorage, per device).
//
// Group/sort/filter state is kept independently per layout: switching board↔timeline
// swaps the live state in and out of the per-layout slots, so each layout remembers
// its own grouping/sort/filters (a status filter set on the timeline doesn't leak
// into the board, where that facet isn't even offered).
//
// The caller owns the *shape* of that state — it passes `snapshot`/`load`/`defaults`
// (and `migrate` for the pre-per-layout blob). This composable owns the *mechanics*:
// the storage key, the per-layout slots, the debounced write, the layout-swap watcher
// and — the part that actually bites — the `restoring`/`swapping` mutex.
//
// That mutex exists because loading state into the very refs a watcher persists is a
// feedback loop: restoring a view writes the refs, the deep watcher sees the writes
// and schedules a persist, and on a layout swap that persist would land in the *new*
// layout's slot. Both flags are cleared on `nextTick`, i.e. after the watcher has
// already run and bailed. Everything that writes the state en masse must go through
// `restoreView` or `guard`; anything that only *reads* it while a load is in flight
// asks `isGuarded()`.
export const VIEW_PERSIST_MS = 300 // idle time after the last toolbar change before we write

export function useBoardViewConfig({ boardId, layout, defaults, snapshot, load, migrate }) {
  const storageKey = computed(() => `tessera_view_${boardId.value}`)
  const byLayout = {}
  let restoring = false
  let swapping = false

  // True while state is being loaded in bulk (restore / view apply / layout swap):
  // persistence and autosave must stand down until it settles.
  function isGuarded() {
    return restoring || swapping
  }

  function writeView() {
    if (restoring) return
    try {
      byLayout[layout.value] = snapshot()
      localStorage.setItem(
        storageKey.value,
        JSON.stringify({ layout: layout.value, toolbars: byLayout }),
      )
    } catch {
      /* storage full / disabled — non-fatal */
    }
  }

  // The persist watcher fires on every search keystroke; a synchronous localStorage
  // write per keystroke is a visible input-lag source on mid hardware. Debounce so
  // we persist once the user pauses, and flush on unmount so nothing is lost.
  let persistTimer = null
  function persistView() {
    if (isGuarded()) return
    if (persistTimer) clearTimeout(persistTimer)
    persistTimer = setTimeout(() => {
      persistTimer = null
      writeView()
    }, VIEW_PERSIST_MS)
  }

  function restoreView() {
    restoring = true
    try {
      const raw = localStorage.getItem(storageKey.value)
      if (raw) {
        const v = JSON.parse(raw)
        if (v.toolbars) {
          Object.assign(byLayout, v.toolbars)
          if (v.layout) layout.value = v.layout
        } else {
          // Migrate the old single-config format into the current layout's slot.
          if (v.layout) layout.value = v.layout
          byLayout[layout.value] = migrate(v, layout.value)
        }
        load(byLayout[layout.value] || defaults(layout.value))
      } else {
        load(defaults(layout.value))
      }
    } catch {
      load(defaults(layout.value))
    } finally {
      nextTick(() => (restoring = false))
    }
  }

  // Run a bulk state write (e.g. applying a saved view) without the layout-swap
  // watcher or the persist watcher reacting to it: the caller sets layout AND the
  // toolbar fields itself, so the swap must not also fire and clobber them.
  function guard(apply) {
    restoring = true
    apply()
    nextTick(() => {
      restoring = false
      persistView()
    })
  }

  // Swap the toolbar state when the layout changes (each layout keeps its own).
  watch(layout, (newL, oldL) => {
    if (restoring || newL === oldL) return
    swapping = true
    byLayout[oldL] = snapshot()
    load(byLayout[newL] || defaults(newL))
    nextTick(() => {
      swapping = false
      persistView()
    })
  })

  onBeforeUnmount(() => {
    if (persistTimer) {
      clearTimeout(persistTimer)
      persistTimer = null
      writeView()
    }
  })

  return { restoreView, persistView, isGuarded, guard }
}
