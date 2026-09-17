import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — a failed «＋ задача» threw the typed title away.
//
// `submitAddTask` clears `newTaskTitle` synchronously, BEFORE awaiting the POST. That
// is deliberate (it keeps the input open for rapid entry and is what makes the gesture
// non-re-entrant — see cx-add-column-double-submit.spec.js), but it also means the
// title is already gone when the request comes back with an error: the reader got a
// toast, no card, and an empty input with nothing to retry from. The sibling
// `submitAddColumn` explicitly keeps its name in the input on that path; this one had
// no counterpart at all. Worse, the input still commits on `@blur`, so one stray click
// after the failure closed the add gesture silently.
//
// The fix restores the title on the failure path only — and only into an input the
// reader hasn't moved on from (they may have typed the next task, or opened another
// column's input, where this title would be filed in the wrong lane).
//
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// guard reads the source the way cx-add-column-double-submit.spec.js does. Vitest runs
// with frontend/ as cwd; __dirname doesn't exist in ESM.
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

describe('a failed task create keeps the typed title (#2932)', () => {
  it('restores the title from submitAddTask’s catch', () => {
    const body = fnBody(board, 'submitAddTask')
    expect(body).toBeTruthy()
    const katch = body.indexOf('} catch')
    const restore = body.indexOf('restoreFailedTitle(')
    expect(katch).toBeGreaterThan(-1)
    expect(restore).toBeGreaterThan(katch)
  })

  it('passes the title that was read before the input was cleared', () => {
    // `newTaskTitle` is empty by then, so the restore has to carry the local `title`
    // (the normalized one that was actually sent) — reading the ref back would restore
    // an empty string and change nothing.
    const body = fnBody(board, 'submitAddTask')
    expect(body).toMatch(/restoreFailedTitle\(dcol, title\)/)
  })

  it('does not restore over text the reader has since typed', () => {
    const body = fnBody(board, 'restoreFailedTitle')
    expect(body).toBeTruthy()
    const guard = body.indexOf('if (newTaskTitle.value) return')
    const write = body.indexOf('newTaskTitle.value = title')
    expect(guard).toBeGreaterThan(-1)
    expect(write).toBeGreaterThan(guard)
  })

  it('does not drop the title into another column’s open input', () => {
    // Filing it in the wrong lane would be worse than losing it: the reader would
    // commit a title they didn't retype, in a column they didn't mean.
    const body = fnBody(board, 'restoreFailedTitle')
    const guard = body.indexOf(
      'if (addingInColumn.value !== null && addingInColumn.value !== dcol.key) return',
    )
    const write = body.indexOf('newTaskTitle.value = title')
    expect(guard).toBeGreaterThan(-1)
    expect(guard).toBeLessThan(write)
  })

  it('reopens the input on the column the title was meant for, focused', () => {
    // The blur commit can have closed the add gesture (`addingInColumn === null`)
    // while the POST was on the wire; restoring into a closed input would be a no-op
    // the reader never sees.
    const body = fnBody(board, 'restoreFailedTitle')
    const write = body.indexOf('newTaskTitle.value = title')
    const open = body.indexOf('addingInColumn.value = dcol.key')
    const focus = body.indexOf('focusTaskInput()')
    expect(open).toBeGreaterThan(write)
    expect(focus).toBeGreaterThan(open)
  })

  it('leaves the success path clearing the input before the await', () => {
    // The synchronous clear is the anti-duplicate guard for this gesture — restoring on
    // error must not tempt a later refactor into holding the title across the request.
    const body = fnBody(board, 'submitAddTask')
    const clear = body.indexOf("newTaskTitle.value = ''")
    const post = body.indexOf('await boards.createTask(')
    expect(clear).toBeGreaterThan(-1)
    expect(clear).toBeLessThan(post)
  })

  it('does not restore on the empty-title bail-out', () => {
    // An empty submit (the second Enter, or a blur with nothing typed) is a cancel, not
    // a failure: it must not resurrect anything.
    const body = fnBody(board, 'submitAddTask')
    const bail = body.indexOf('cancelAddTask()')
    const restore = body.indexOf('restoreFailedTitle(')
    expect(bail).toBeGreaterThan(-1)
    expect(bail).toBeLessThan(restore)
  })

  it('still commits on both Enter and blur', () => {
    // Both bindings are why a lost title is unrecoverable in the first place; if either
    // disappears this guard would be passing for the wrong reason.
    const tile = board.slice(
      board.indexOf('<div\n                  v-if="addingInColumn === dcol.key"'),
    )
    const input = tile.slice(0, tile.indexOf('</n-input>') + 1 || 800)
    expect(input).toContain('@keydown.enter.exact.prevent="submitAddTask(dcol)"')
    expect(input).toContain('@blur="submitAddTask(dcol)"')
  })
})
