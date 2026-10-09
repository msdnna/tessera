import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — a late answer published the board you had left.
//
// `load()` fans out ten requests and, after awaiting them, ASSIGNS: `board`, `columns`,
// `allTasks`, `subtasksByParent`, and — loudest of all — `boardViewStore.setContext(id,
// wsId, projectId)`, the identity every child scopes its own work by (tag creation,
// estimation config). Nothing between the await and those writes asks whether the answer
// is still the one the reader is waiting for.
//
// It is not a hypothetical: KanbanBoard is NOT remounted when you switch boards (the
// comment above `loadViews` says so outright — that is why the loaded saved view has to
// be dropped by hand), and the two in-place scope switches, `?milestone=` and
// `?archived=1`, reload the very same component through the same function. So the
// classic race applies in three flavours:
//
//   • click board B while A is still loading → A answers last and takes the screen,
//     while `props.boardId` and the URL still say B;
//   • toggle the archive → the ordinary answer lands after the archive's and the
//     read-only view comes back full of live cards (which `taskQuery` is careful to
//     scope precisely so it cannot happen server-side);
//   • switch étape → the previous sprint's cards under the new sprint's chip.
//
// `setContext` makes it worse than a wrong picture: a card created afterwards is filed
// against the stale project id. The same class was settled for TaskModal, HomeView,
// MilestonesView, NotesView, BoardView and DocumentsView in this task; this was the
// last loader on the board surface without a generation.
//
// The partial re-fetches (realtime `task.*`/`column.*` bursts) share the fate: they too
// write into the shared refs after an await, and one in flight across a board switch
// lands on the new board's screen.
//
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// guards are read out of the source the way cx-add-task-restore-on-error.spec.js and
// cx-board-query-watch-scope.spec.js do. Vitest runs with frontend/ as cwd; __dirname
// doesn't exist in ESM.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

// Body of a function declaration, by brace matching from its `function <name>(`.
// The parameter list has to be stepped over by paren depth first: `load(id, { silent
// = false } = {})` destructures, so the first `{` after the name is an argument's,
// not the body's — matching from it silently yields ` silent = false ` as the body,
// and every assertion then passes or fails for the wrong reason.
function fnBody(source, name) {
  const start = source.indexOf(`function ${name}(`)
  if (start === -1) return null
  let parens = 0
  let i = source.indexOf('(', start)
  for (; i < source.length; i++) {
    if (source[i] === '(') parens++
    else if (source[i] === ')' && --parens === 0) break
  }
  const open = source.indexOf('{', i)
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i)
  }
  return null
}

// Index of the first stale-answer bailout in a body, or -1.
function guardAt(body, from = 0) {
  return body.indexOf('mine !== loadSeq', from)
}

describe('a late board answer does not take the screen (#2932)', () => {
  it('load() claims a generation before it awaits anything', () => {
    const body = fnBody(board, 'load')
    expect(body).toBeTruthy()
    const claim = body.indexOf('++loadSeq')
    const firstAwait = body.indexOf('await')
    expect(claim).toBeGreaterThan(-1)
    expect(claim).toBeLessThan(firstAwait)
  })

  it('drops a superseded answer before publishing the board', () => {
    const body = fnBody(board, 'load')
    const guard = guardAt(body)
    expect(guard).toBeGreaterThan(-1)
    // Every write of the shared board state comes after the bailout.
    for (const write of [
      'board.value = b.data',
      'columns.value = c.data',
      'boardViewStore.setContext(',
      'allTasks.value = (t.data',
      'subtasksByParent.value = byParent',
    ]) {
      const at = body.indexOf(write)
      expect(at, write).toBeGreaterThan(guard)
    }
  })

  it('a superseded answer neither clears the spinner nor toasts', () => {
    // Both belong to the request that raised them: the abandoned board's 404 must not
    // shout over the board now on screen, and its `finally` must not uncover a board
    // that is still loading.
    const body = fnBody(board, 'load')
    const katch = body.indexOf('} catch')
    const toast = body.indexOf('message.error(e.message)', katch)
    expect(guardAt(body, katch)).toBeGreaterThan(-1)
    expect(guardAt(body, katch)).toBeLessThan(toast)
    const fin = body.indexOf('} finally')
    expect(body.slice(fin)).toMatch(/mine === loadSeq[\s\S]*loading\.value = false/)
  })

  it('fetchTasks drops a page fetched for another board or scope', () => {
    // The realtime tasks-only refetch, same shared refs. It reads the generation
    // WITHOUT bumping it: it must not invalidate the full load it may be running
    // alongside, only notice one that started later.
    const body = fnBody(board, 'fetchTasks')
    expect(body).toBeTruthy()
    expect(body).toMatch(/const mine = loadSeq(?!\s*\+)/)
    const guard = guardAt(body)
    expect(guard).toBeGreaterThan(-1)
    expect(body.indexOf('allTasks.value = (t.data')).toBeGreaterThan(guard)
    expect(body.indexOf('subtasksByParent.value = byParent')).toBeGreaterThan(guard)
  })

  it('the columns and board partial re-fetches guard their own writes', () => {
    const partials = board.slice(
      board.indexOf('const partialFetch = {'),
      board.indexOf('function schedulePartial('),
    )
    expect(partials).toBeTruthy()
    for (const write of ['columns.value = ', 'board.value = ']) {
      const at = partials.indexOf(write)
      expect(at, write).toBeGreaterThan(-1)
      expect(guardAt(partials), write).toBeGreaterThan(-1)
      expect(partials.lastIndexOf('loadSeq', at), write).toBeGreaterThan(-1)
    }
  })

  it('loadWorkspaceMeta does not refill the maps for a board left behind', () => {
    // It is awaited from inside load() and also scheduled on its own by `meta` events,
    // so it carries the same read-only generation as fetchTasks — and it ends in
    // setContext, the identity the whole board scopes by.
    const body = fnBody(board, 'loadWorkspaceMeta')
    expect(body).toBeTruthy()
    expect(body).toMatch(/const mine = loadSeq(?!\s*\+)/)
    const guard = guardAt(body)
    expect(guard).toBeGreaterThan(-1)
    expect(body.indexOf('boardViewStore.refill(')).toBeGreaterThan(guard)
    expect(body.indexOf('boardViewStore.setContext(')).toBeGreaterThan(guard)
  })

  it('switching boards invalidates whatever is in flight', () => {
    // The `props.boardId` watcher reaches load() via `await load(id)`, which bumps the
    // generation itself — the assertion here is that nothing on that path publishes
    // state before the bump, i.e. the watcher does not assign board data of its own.
    const at = board.indexOf('() => props.boardId,')
    expect(at).toBeGreaterThan(-1)
    const body = board.slice(at, board.indexOf('watch(', at + 10))
    // The watcher hands the id to openBoard (#3017), which awaits load(id) first.
    expect(body).toMatch(/await openBoard\(id\)/)
    expect(body).not.toMatch(/allTasks\.value =|columns\.value =|board\.value =/)
    const open = board.slice(board.indexOf('async function openBoard('))
    expect(open.slice(0, open.indexOf('\n}'))).toMatch(/await load\(id\)/)
  })
})
