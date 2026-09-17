import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { ref, watch, nextTick } from 'vue'
import { useBoardViewConfig } from '@/composables/useBoardViewConfig'

// Regression test for #2932 — loading a saved view silently dropped the columns the
// reader had expanded by hand.
//
// «Сворачивать пустые колонки» (autoCollapseEmpty) folds every lane holding no cards,
// and an explicit per-column override beats it, so an empty column can still be opened
// from its strip. Turning the toggle ON has to clear those overrides, or it reads as
// dead — that is what `watch(autoCollapseEmpty, …)` is for.
//
// But the same ref is also written in bulk by `loadCustomize`, which every restore path
// funnels through: applying a saved view, swapping layout, and the localStorage restore
// on mount. There the overrides arriving next to it ARE the saved state, not leftovers.
// A view saved with the toggle on and two empty columns expanded loaded correctly only
// when the board already had the toggle on (no transition → no watcher). Coming from a
// board with it off, the flip fired, the loop deleted exactly those overrides, and the
// persist watcher wrote the loss back to localStorage — the same view restoring two
// different boards depending on what was on screen before it, and destructively.
//
// The fix stands the watcher down while a bulk load is in flight, on the very flag
// `persistView` and the autosave already use.
//
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// watcher is read out of the source the way cx-new-column-not-collapsed.spec.js does;
// the timing the fix rests on is exercised against the real composable below. Vitest
// runs with frontend/ as cwd; __dirname doesn't exist in ESM.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

// Body of a function declaration, by brace matching from its `function <name>(`.
function fnBody(source, name) {
  const start = source.indexOf(`function ${name}(`)
  if (start === -1) return null
  const open = source.indexOf('{', start)
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i)
  }
  return null
}

// The `watch(autoCollapseEmpty, (on) => { … })` callback, by brace matching.
function collapseWatcher(source) {
  const at = source.indexOf('watch(autoCollapseEmpty,')
  if (at === -1) return null
  const open = source.indexOf('{', source.indexOf('=>', at))
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i)
  }
  return null
}

// Minimal stand-in for the caller: a snapshot/load pair over one ref, which is all the
// mutex timing needs. `layout` is a real ref so the swap watcher can be driven.
function harness(loaded = {}) {
  const layout = ref('board')
  const flag = ref(false)
  const seen = []
  const config = useBoardViewConfig({
    boardId: ref('b1'),
    layout,
    defaults: () => ({ flag: false }),
    snapshot: () => ({ flag: flag.value }),
    load: (s) => (flag.value = !!s.flag),
    migrate: (v) => ({ flag: !!v.flag }),
    ...loaded,
  })
  // Mirrors the real watcher's registration: a plain (pre-flush) watcher on the ref
  // that loadCustomize writes. It records what the guard flag said when it ran.
  watch(flag, (on) => seen.push({ on, guarded: config.isGuarded() }))
  return { ...config, layout, flag, seen }
}

describe('bulk view load keeps manual collapse overrides (#2932)', () => {
  it('stands the prune down while a view load is in flight', () => {
    const watcher = collapseWatcher(board)
    expect(watcher).toBeTruthy()
    // Both halves of the early return: nothing to do when the toggle went off, and
    // nothing to do when it was a restore that moved it rather than the reader.
    expect(watcher).toMatch(/if\s*\(!on\s*\|\|\s*viewLoadInFlight\(\)\)\s*return/)
  })

  it('still clears the overrides of empty columns on a real flip', () => {
    // The guard must not cost the toggle its whole job — without this loop an
    // expanded-by-hand empty column beats the setting and it reads as dead.
    const watcher = collapseWatcher(board)
    expect(watcher).toMatch(/for \(const dcol of displayColumns\.value\)/)
    expect(watcher).toMatch(
      /colCount\(dcol\) === 0 && k in colCollapse\)\s*delete colCollapse\[k\]/,
    )
  })

  it('is the same flag persistence and autosave stand down on', () => {
    // A second, privately-owned flag would drift out of step with the mutex that
    // actually brackets the bulk writes.
    expect(board).toMatch(/isGuarded:\s*viewLoadInFlight/)
    expect(board).toMatch(/guarded:\s*viewLoadInFlight/)
  })

  it('routes applying a saved view through that mutex', () => {
    // applyView is the path the bug was reported on; if it ever stops being guarded
    // the watcher's new early return never triggers there.
    const body = fnBody(board, 'applyView')
    expect(body).toBeTruthy()
    expect(body).toMatch(/withViewLoad\(\(\) =>/)
    expect(body).toContain('applyViewConfig(v.config)')
  })

  it('writes the toggle and the overrides through one bulk loader', () => {
    // Both halves of the restored state land in loadCustomize — that shared body is
    // why guarding the watcher covers the saved view, the layout swap and the
    // localStorage restore at once.
    const body = fnBody(board, 'loadCustomize')
    expect(body).toBeTruthy()
    expect(body).toContain('Object.assign(colCollapse, s.colCollapse || {})')
    expect(body).toMatch(/autoCollapseEmpty\.value = !!s\.autoCollapseEmpty/)
    // …and both the view blob and the localStorage slot carry the overrides, or there
    // would be nothing to protect.
    expect(fnBody(board, 'currentViewConfig')).toContain('colCollapse: { ...colCollapse }')
    expect(fnBody(board, 'snapshotToolbar')).toContain('colCollapse: { ...colCollapse }')
  })

  it('reports the load as in flight to a watcher woken by withViewLoad', async () => {
    // The timing the fix rests on, against the real composable: the flag is cleared on
    // nextTick, and a pre-flush watcher job runs before that — so the watcher still
    // sees the load. If this ever inverted, the early return would never fire.
    const h = harness()
    h.guard(() => (h.flag.value = true))
    await nextTick()
    expect(h.seen).toEqual([{ on: true, guarded: true }])
    expect(h.isGuarded()).toBe(false)
  })

  it('reports it for the layout swap and the localStorage restore too', async () => {
    const swap = harness()
    swap.guard(() => (swap.flag.value = true))
    await nextTick()
    swap.seen.length = 0
    // Each layout keeps its own slot: 'timeline' has none, so it loads the defaults
    // (flag off) and coming back re-loads the snapshot taken on the way out (flag on).
    swap.layout.value = 'timeline'
    await nextTick()
    swap.layout.value = 'board'
    await nextTick()
    expect(swap.seen).toEqual([
      { on: false, guarded: true },
      { on: true, guarded: true },
    ])

    localStorage.setItem(
      'tessera_view_b1',
      JSON.stringify({ layout: 'board', toolbars: { board: { flag: true } } }),
    )
    const restore = harness()
    restore.restoreView()
    await nextTick()
    expect(restore.seen).toEqual([{ on: true, guarded: true }])
    localStorage.removeItem('tessera_view_b1')
  })

  it('leaves a reader-driven flip unguarded', async () => {
    // The other side of the same timing: a plain write outside guard()/restoreView()
    // must reach the watcher with the flag down, or the prune never runs at all.
    const h = harness()
    h.flag.value = true
    await nextTick()
    expect(h.seen).toEqual([{ on: true, guarded: false }])
  })
})
