import { describe, it, expect } from 'vitest'
import { ref, watch, nextTick, markRaw } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — the composer bar's ResizeObserver was never attached.
//
// `onMounted` created it and observed the bar under `if (subbarEl.value)`. But the bar
// (`.subbar`) lives inside `v-if="board"`, and `board` comes from the boardView store,
// which holds null until the first load resolves — so at mount the template ref is
// still empty, `observe` is silently skipped, and nothing attaches it later: unlike
// `boardScroll`, `subbarEl` had no watcher. The board is not remounted when you switch
// boards either, so the observer stayed unattached for the whole session and the
// promise in its own comment — «re-measure the composer fit on bar resize (window /
// sidebar toggle)» — never held: narrowing the bar left the right-side tools in place
// while the nowrap chips ran past its edge, until an unrelated facet change happened
// to re-measure. The mount-time `nextTick(recomputeComposerFit)` missed for the same
// reason: no bar yet, so the very first measurement never happened either.
//
// The first half plays the lifecycle against a model of the wiring; the second pins the
// board's own, since KanbanBoard is never mounted in this suite (it pulls the whole
// board stack) — the same split cx-add-task-reopen-scope uses.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')
const store = readFileSync(resolve(root, 'src/stores/boardView.js'), 'utf8')

// A ResizeObserver stand-in: it remembers its single target and can be told that the
// element changed size, which is all the board asks of the real one.
function fakeRO(cb) {
  let target = null
  return {
    observe: (el) => (target = el),
    disconnect: () => (target = null),
    resize: () => target && cb(),
    get target() {
      return target
    },
  }
}

// The composer's wiring as the component holds it: a template ref that lands only when
// the first load resolves, an observer built on mount, and the measurement both are
// there for. `attach` is the rule under test — 'mount' is what the board did, 'watch'
// is what it does now.
function composer(attach) {
  const subbarEl = ref(null)
  const measured = [] // one entry per recomputeComposerFit that found a bar
  let ro = null
  const recompute = () => subbarEl.value && measured.push(subbarEl.value)
  const stop =
    attach === 'watch'
      ? watch(subbarEl, (el) => {
          if (!ro || !el) return
          ro.disconnect()
          ro.observe(el)
          nextTick(recompute)
        })
      : () => {}
  return {
    subbarEl,
    measured,
    stop,
    // onMounted: the observer exists from here on, and observes a bar already in place.
    mount() {
      ro = fakeRO(recompute)
      if (subbarEl.value) ro.observe(subbarEl.value)
      nextTick(recompute)
    },
    // The first load came back → `v-if="board"` flips and the ref gets the element.
    boardLoaded(el) {
      subbarEl.value = el
    },
    // The sidebar was collapsed / the window resized.
    resizeBar() {
      ro?.resize()
    },
    get observing() {
      return ro?.target ?? null
    },
  }
}

describe('the composer bar is measured after it exists (#2932)', () => {
  it('observes the bar that arrives with the first load', async () => {
    const c = composer('watch')
    c.mount() // the board is still loading: no bar to observe yet
    await nextTick()
    expect(c.observing).toBe(null)

    // markRaw: a real template ref holds a DOM element, which `ref()` never proxies —
    // a plain stand-in would be, and the identity checks below would compare a proxy.
    const bar = markRaw({ id: 'subbar' })
    c.boardLoaded(bar)
    await nextTick()
    expect(c.observing).toBe(bar)
    // The re-measure is itself deferred a tick (the bar has only just been patched in),
    // so it lands one tick behind the attach.
    await nextTick()
    expect(c.measured).toEqual([bar]) // the first measurement, the one mount could not take

    c.resizeBar()
    expect(c.measured).toEqual([bar, bar])
    c.stop()
  })

  it('is exactly what the mount-only attach got wrong', async () => {
    // The same lifecycle against the old rule, so the fix above cannot quietly start
    // passing for another reason: the bar appears, nothing ever observes it, and a
    // resize measures nothing at all.
    const c = composer('mount')
    c.mount()
    await nextTick()
    c.boardLoaded(markRaw({ id: 'subbar' }))
    await nextTick()
    expect(c.observing).toBe(null)
    c.resizeBar()
    expect(c.measured).toEqual([])
  })

  it('still observes a bar that is already there at mount', async () => {
    // The watcher fires while the observer does not exist yet (it is built in
    // onMounted), so that path has to stay covered by onMounted's own observe.
    const c = composer('watch')
    // markRaw: a real template ref holds a DOM element, which `ref()` never proxies —
    // a plain stand-in would be, and the identity checks below would compare a proxy.
    const bar = markRaw({ id: 'subbar' })
    c.boardLoaded(bar)
    await nextTick()
    c.mount()
    await nextTick()
    expect(c.observing).toBe(bar)
    c.resizeBar()
    expect(c.measured.length).toBeGreaterThan(0)
    c.stop()
  })
})

describe('the board wires the composer observer that way (#2932)', () => {
  it('re-observes the bar from a subbarEl watcher', () => {
    const start = board.indexOf('watch(subbarEl, (el) => {')
    expect(start).toBeGreaterThan(-1)
    const body = board.slice(start, board.indexOf('\n})', start))
    expect(body).toContain('if (!composerRO || !el) return')
    expect(body).toContain('composerRO.observe(el)')
    expect(body).toContain('recomputeComposerFit')
  })

  it('keeps the bar behind the board it waits for', () => {
    // The premise of the whole fix: the ref cannot be filled at mount time, because
    // the element that fills it is not rendered until `board` is there…
    const guard = board.indexOf('v-if="board"')
    const bar = board.indexOf('ref="subbarEl"')
    expect(guard).toBeGreaterThan(-1)
    expect(bar).toBeGreaterThan(guard)
    // …and `board` is the store's, which hands out null until a board is loaded.
    expect(store).toContain('const board = ref(null)')
  })

  it('leaves the mount-time observe as the already-rendered case', () => {
    expect(board).toContain('composerRO = new ResizeObserver(recomputeComposerFit)')
    expect(board).toContain('if (subbarEl.value) composerRO.observe(subbarEl.value)')
    expect(board).toContain('composerRO?.disconnect()')
  })
})
