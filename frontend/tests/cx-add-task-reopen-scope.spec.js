import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — a landed «＋ задача» dragged the open input back into its
// own column.
//
// `submitAddTask` awaits the POST and the reload that follows it, and then re-opened the
// add gesture unconditionally: `addingInColumn.value = dcol.key` + `focusTaskInput()`,
// under a comment claiming `load()` had reset the refs. It has not — nothing in `load()`
// touches `addingInColumn` — so on the ordinary path the write changed nothing and only
// the focus needed restoring. In a race it changed everything: the input commits on blur
// as well as on Enter, so while the request was on the wire the reader could have closed
// the gesture (Esc, or a click away) or opened «＋ задача» on ANOTHER column. The
// resolving create then pulled the input back here and took the focus with it — and
// `newTaskTitle` is one shared ref, so the title they had already started typing in the
// other column travelled with it and the next Enter filed the card in the lane they had
// just left. The failure path (restoreFailedTitle, cx-add-task-restore-on-error) already
// refuses to write into an input the reader has moved on from; this is the same rule on
// the success side.
//
// The first half plays the gesture against a model of the component's three refs; the
// second pins the board's own wiring, since KanbanBoard is never mounted in this suite
// (it pulls the whole board stack) — the same split cx-sprint-scope-new-card uses.
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

// The add gesture as the component holds it: which column's input is open, the one
// shared title ref behind it, and where the focus went. `reopen` is the rule under
// test — 'guarded' is what the board does now, 'always' is what it did before.
function gesture(reopen) {
  const addingInColumn = ref(null)
  const newTaskTitle = ref('')
  const focused = ref(null)
  const created = []

  const focusTaskInput = () => (focused.value = addingInColumn.value)
  const startAddTask = (key) => {
    addingInColumn.value = key
    newTaskTitle.value = ''
    focusTaskInput()
  }
  const cancelAddTask = () => {
    addingInColumn.value = null
    newTaskTitle.value = ''
    focused.value = null
  }
  // submitAddTask, split at its only await: the part that runs on Enter/blur, and the
  // part that runs when the create and the reload come back.
  const submit = (key) => {
    const title = newTaskTitle.value.trim()
    if (!title) {
      cancelAddTask()
      return null
    }
    newTaskTitle.value = '' // cleared BEFORE the await (see cx-add-task-restore-on-error)
    return () => {
      created.push({ key, title })
      if (reopen === 'always') {
        addingInColumn.value = key
        focusTaskInput()
      } else if (addingInColumn.value === key) {
        focusTaskInput()
      }
    }
  }
  return { addingInColumn, newTaskTitle, focused, created, startAddTask, cancelAddTask, submit }
}

describe('a landed «＋ задача» stays where the reader is (#2932)', () => {
  it('keeps the input open and focused for the next title', () => {
    // The ordinary path: nothing else happened while the POST was in flight, so rapid
    // entry must survive the reload that re-rendered the column.
    const g = gesture('guarded')
    g.startAddTask('col-a')
    g.newTaskTitle.value = 'первая'
    const land = g.submit('col-a')
    g.focused.value = null // the reload re-rendered the column and dropped the focus
    land()
    expect(g.addingInColumn.value).toBe('col-a')
    expect(g.focused.value).toBe('col-a')
    expect(g.created).toEqual([{ key: 'col-a', title: 'первая' }])
  })

  it('does not file the next title into the column the reader left', () => {
    // Enter in column A, then «＋ задача» on column B while the POST is on the wire:
    // the click blurs A's input first (an empty commit, i.e. a cancel), then opens B's.
    const g = gesture('guarded')
    g.startAddTask('col-a')
    g.newTaskTitle.value = 'первая'
    const land = g.submit('col-a')
    g.submit('col-a') // the blur that the click on B's button fires first
    g.startAddTask('col-b')
    g.newTaskTitle.value = 'втор'
    land()
    expect(g.addingInColumn.value).toBe('col-b')
    expect(g.focused.value).toBe('col-b')
    expect(g.newTaskTitle.value).toBe('втор')
  })

  it('is exactly what the unguarded re-open got wrong', () => {
    // The same sequence against the old rule, so the guard above cannot quietly start
    // passing for another reason: the input jumps back to A carrying B's half-typed
    // title, and the next Enter would create it in A.
    const g = gesture('always')
    g.startAddTask('col-a')
    g.newTaskTitle.value = 'первая'
    const land = g.submit('col-a')
    g.submit('col-a')
    g.startAddTask('col-b')
    g.newTaskTitle.value = 'втор'
    land()
    expect(g.addingInColumn.value).toBe('col-a')
    expect(g.newTaskTitle.value).toBe('втор')
  })

  it('leaves a closed gesture closed', () => {
    // Enter, then a click away: the blur commits an empty title, which is a cancel.
    // Re-opening would put an input the reader dismissed back on screen and pull the
    // focus out of whatever they clicked on.
    const g = gesture('guarded')
    g.startAddTask('col-a')
    g.newTaskTitle.value = 'первая'
    const land = g.submit('col-a')
    g.submit('col-a')
    land()
    expect(g.addingInColumn.value).toBe(null)
    expect(g.focused.value).toBe(null)
  })

  it('still creates the card in the column it was typed into', () => {
    // The guard is about where the INPUT ends up; the card itself is bound to the
    // column captured at submit time and must not follow the reader.
    const g = gesture('guarded')
    g.startAddTask('col-a')
    g.newTaskTitle.value = 'первая'
    const land = g.submit('col-a')
    g.submit('col-a')
    g.startAddTask('col-b')
    land()
    expect(g.created).toEqual([{ key: 'col-a', title: 'первая' }])
  })
})

describe('KanbanBoard wires the success path through that rule (#2932)', () => {
  it('re-opens through reopenForRapidEntry, after the reload', () => {
    const body = fnBody(board, 'submitAddTask')
    expect(body).toBeTruthy()
    const reload = body.indexOf('await load(props.boardId)')
    const reopen = body.indexOf('reopenForRapidEntry(dcol)')
    expect(reload).toBeGreaterThan(-1)
    expect(reopen).toBeGreaterThan(reload)
  })

  it('no longer writes addingInColumn back from the success path', () => {
    // The write is the bug: it is what moved the open input to a column the reader had
    // already left.
    const body = fnBody(board, 'submitAddTask')
    expect(body).not.toMatch(/addingInColumn\.value\s*=/)
  })

  it('refocuses only the column whose input is still open', () => {
    const body = fnBody(board, 'reopenForRapidEntry')
    expect(body).toBeTruthy()
    const guard = body.indexOf('if (addingInColumn.value !== dcol.key) return')
    const focus = body.indexOf('focusTaskInput()')
    expect(guard).toBeGreaterThan(-1)
    expect(focus).toBeGreaterThan(guard)
  })

  it('leaves the failure path guarding the same way', () => {
    // The two halves state one rule; if restoreFailedTitle loses its guard the success
    // side alone is not enough.
    const body = fnBody(board, 'restoreFailedTitle')
    expect(body).toMatch(
      /if \(addingInColumn\.value !== null && addingInColumn\.value !== dcol\.key\) return/,
    )
  })

  it('still commits on both Enter and blur', () => {
    // Both bindings are what makes the race reachable at all; without them this guard
    // would be passing for the wrong reason.
    const tile = board.slice(board.indexOf('v-if="addingInColumn === dcol.key"'))
    // kit/QuickAddField (#3112): ↵ is its `submit`, the click away its `blur`.
    const input = tile.slice(0, 800)
    expect(input).toContain('@submit="submitAddTask(dcol)"')
    expect(input).toContain('@blur="submitAddTask(dcol)"')
  })
})
