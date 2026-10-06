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
  // The key `byLayout` currently describes. KanbanBoard does NOT remount when you
  // switch boards — it re-calls `restoreView` on the same composable — so both the
  // slot map and a debounced write outlive the board they were made for unless
  // `restoreView` re-points them. Writes go to `activeKey`, not to the live
  // `storageKey`, so a pending one lands on the board it was scheduled for.
  let activeKey = null
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
        activeKey || storageKey.value,
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

  // Land a debounced write right now, under the key it was scheduled for.
  function flushPending() {
    if (!persistTimer) return
    clearTimeout(persistTimer)
    persistTimer = null
    writeView()
  }

  function restoreView() {
    // Bank a write still in flight on the board it was made for, then start from an
    // empty slot map: a board that has no stored slot for some layout must fall back
    // to the defaults, not to the previous board's slot — and `writeView` stores the
    // whole map, so a stale slot would also be copied into this board's key.
    flushPending()
    if (activeKey !== storageKey.value) {
      for (const k of Object.keys(byLayout)) delete byLayout[k]
      activeKey = storageKey.value
    }
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

  onBeforeUnmount(flushPending)

  return { restoreView, persistView, isGuarded, guard }
}
