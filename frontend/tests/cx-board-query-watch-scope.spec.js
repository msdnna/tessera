import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { computed, reactive, shallowRef, watch, nextTick } from 'vue'

// Regression test for #2932 — opening a card reloaded the whole board.
//
// KanbanBoard reloads its task set when the URL's *scope* changes: `?milestone=` (the
// étape overlay) and `?archived=1` (the read-only archive). That watcher was written as
// ONE getter returning a pair:
//
//   watch(() => [route.query.milestone, route.query.archived], ([ms]) => { … load() })
//
// A getter source is compared with `Object.is`, and a fresh array is never `Object.is`
// to the previous one — so the callback fires every time the getter's DEPENDENCY is
// invalidated, not only when the two values it reads differ. That dependency is
// `route.query` as a whole: vue-router exposes it as a computed over `currentRoute`,
// and every navigation publishes a new route object, whichever key actually moved.
//
// The board's own URL carries the open card (`?task=<number>`), pushed by `openTask`,
// rewritten by `applyTaskQuery`'s canonicalization and stripped again by `closeTask`.
// Each of those ran the scope callback, i.e. a full NON-silent `load()` — ten requests
// and the n-spin that fades the entire board behind the modal (#2695 is precisely why
// the silent mode exists). Task edits made in the modal already come back through
// `@changed` → `onChanged`; nothing needed this reload.
//
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// watcher's shape is read out of the source the way cx-view-name-board-switch.spec.js
// does, and the reactivity it rests on is exercised against a route stub built like
// vue-router's own below. Vitest runs with frontend/ as cwd; __dirname doesn't exist in
// ESM.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

// The `watch(<sources>, ([ms]) => …)` that reacts to the URL scope, as written.
function scopeWatchSources(source) {
  const at = source.indexOf('route.query.milestone, () => route.query.archived')
  if (at !== -1) return source.slice(source.lastIndexOf('watch(', at) + 'watch('.length, at + 60)
  const legacy = source.indexOf('route.query.milestone, route.query.archived')
  if (legacy === -1) return null
  return source.slice(source.lastIndexOf('watch(', legacy) + 'watch('.length, legacy + 50)
}

// A route shaped like vue-router's: `currentRoute` is a shallowRef replaced wholesale on
// every navigation, and `route.query` is a computed over it. That is what makes the key
// a watcher reads irrelevant to whether it is woken.
function makeRoute(initial = {}) {
  const currentRoute = shallowRef({ query: { ...initial } })
  const route = reactive({ query: computed(() => currentRoute.value.query) })
  const navigate = (query) => {
    currentRoute.value = { query: { ...query } }
    return nextTick()
  }
  return { route, navigate }
}

describe('the board reloads for URL scope changes only (#2932)', () => {
  it('a single array-returning getter wakes on an unrelated query key', async () => {
    const { route, navigate } = makeRoute({ milestone: 'sprint-7' })
    const calls = []
    watch(
      () => [route.query.milestone, route.query.archived],
      ([ms, archived]) => calls.push([ms, archived]),
    )

    await navigate({ milestone: 'sprint-7', task: '252' }) // openTask
    await navigate({ milestone: 'sprint-7', task: '252' }) // canonicalization (same values)
    await navigate({ milestone: 'sprint-7' }) // closeTask

    // The scope never moved, yet every navigation ran the callback — three full,
    // non-silent board loads for opening and closing one card.
    expect(calls).toEqual([
      ['sprint-7', undefined],
      ['sprint-7', undefined],
      ['sprint-7', undefined],
    ])
  })

  it('per-key getters stay quiet while only ?task= moves', async () => {
    const { route, navigate } = makeRoute({ milestone: 'sprint-7' })
    const calls = []
    watch([() => route.query.milestone, () => route.query.archived], ([ms, archived]) =>
      calls.push([ms, archived]),
    )

    await navigate({ milestone: 'sprint-7', task: '252' })
    await navigate({ milestone: 'sprint-7', task: '252' })
    await navigate({ milestone: 'sprint-7' })

    expect(calls).toEqual([])
  })

  it('per-key getters still fire when the étape scope changes', async () => {
    const { route, navigate } = makeRoute({ milestone: 'sprint-7' })
    const calls = []
    watch([() => route.query.milestone, () => route.query.archived], ([ms, archived]) =>
      calls.push([ms, archived]),
    )

    await navigate({ milestone: 'sprint-8' })
    await navigate({}) // scope cleared from the chip

    expect(calls).toEqual([
      ['sprint-8', undefined],
      [undefined, undefined],
    ])
  })

  it('per-key getters still fire on entering and leaving the archive', async () => {
    const { route, navigate } = makeRoute({})
    const calls = []
    watch([() => route.query.milestone, () => route.query.archived], ([ms, archived]) =>
      calls.push([ms, archived]),
    )

    // archiveQuery drops the étape and sets archived=1 in one navigation: both sources
    // move, and the callback must run once, not twice.
    await navigate({ archived: '1' })
    await navigate({}) // withoutArchiveQuery

    expect(calls).toEqual([
      [undefined, '1'],
      [undefined, undefined],
    ])
  })

  it('the first getter still carries the étape, so the manual filter is cleared', async () => {
    const { route, navigate } = makeRoute({})
    const seen = []
    watch([() => route.query.milestone, () => route.query.archived], ([ms]) => seen.push(ms))

    await navigate({ milestone: 'sprint-9' })

    // KanbanBoard's callback is `([ms]) => { if (ms) filters.milestones = [] … }`.
    expect(seen).toEqual(['sprint-9'])
  })

  it('KanbanBoard passes the two scope keys as separate getters', () => {
    const sources = scopeWatchSources(board)
    expect(sources).toBeTruthy()
    expect(sources).toContain('() => route.query.milestone')
    expect(sources).toContain('() => route.query.archived')
    // Not one getter returning a pair — that is the shape that woke on every ?task=.
    expect(sources).not.toMatch(/\(\)\s*=>\s*\[\s*route\.query\.milestone/)
  })

  it('the board still reloads for the scope, and only for it', () => {
    const at = board.indexOf('route.query.milestone, () => route.query.archived')
    expect(at).toBeGreaterThan(-1)
    const body = board.slice(at, at + 400)
    expect(body).toContain('load(props.boardId)')
    // The open card lives in the same query and must not be a source here.
    expect(body.slice(0, body.indexOf('load(props.boardId)'))).not.toContain('route.query.task')
  })
})
