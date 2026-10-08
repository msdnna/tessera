import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — «＋ колонка» could create the same column twice.
//
// The inline name input commits on BOTH `@keyup.enter` and `@blur`, and it stays
// mounted with the typed name in it for the whole round-trip: `newColumnName` was
// cleared only AFTER `await boards.createColumn(...)` came back. So the gesture was
// re-entrant — a second Enter on a slow link, or clicking away right after the first
// one, entered `submitAddColumn` again while the first POST was still on the wire,
// read the same non-empty name and fired a second create. The server does not dedupe
// (handlers/boards.go CreateColumn appends at MaxColumnPosition unconditionally), so
// the board came back with two identical columns — and only the last one carried the
// expand override from the auto-collapse fix, so on a board with «сворачивать пустые
// колонки» on, the twin showed up as a nameless 44px strip.
//
// The fix is an explicit in-flight flag rather than submitAddTask's trick of clearing
// the input synchronously before awaiting: that would silently throw the typed name
// away whenever the request fails.
//
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// guard reads the source the way cx-new-column-not-collapsed.spec.js does. Vitest runs
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

describe('column create is not re-entrant (#2932)', () => {
  it('bails out while a create is already on the wire', () => {
    const body = fnBody(board, 'submitAddColumn')
    expect(body).toBeTruthy()
    // The early return has to be the FIRST thing the function does — ahead of reading
    // the name, so the second Enter cannot slip past it.
    const guard = body.indexOf('if (columnInFlight.value) return')
    const read = body.indexOf('newColumnName.value.trim()')
    expect(guard).toBeGreaterThan(-1)
    expect(read).toBeGreaterThan(-1)
    expect(guard).toBeLessThan(read)
  })

  it('raises the flag before awaiting the POST', () => {
    const body = fnBody(board, 'submitAddColumn')
    const raise = body.indexOf('columnInFlight.value = true')
    const post = body.indexOf('await boards.createColumn(')
    expect(raise).toBeGreaterThan(-1)
    expect(post).toBeGreaterThan(-1)
    expect(raise).toBeLessThan(post)
  })

  it('lowers the flag on the failure path too', () => {
    // Without `finally` a failed create would leave the flag up and the «＋ колонка»
    // input dead for the rest of the session — the create gesture lost entirely,
    // which is worse than the duplicate it replaces.
    const body = fnBody(board, 'submitAddColumn')
    expect(body).toMatch(/\}\s*finally\s*\{\s*columnInFlight\.value = false\s*\}/)
  })

  it('keeps the typed name in the input when the create fails', () => {
    // The whole reason for an explicit flag instead of submitAddTask's synchronous
    // clear: the name is only wiped on the success path.
    const body = fnBody(board, 'submitAddColumn')
    const clear = body.indexOf("newColumnName.value = ''")
    const post = body.indexOf('await boards.createColumn(')
    const katch = body.indexOf('} catch')
    expect(clear).toBeGreaterThan(post)
    expect(clear).toBeLessThan(katch)
  })

  it('still commits on both Enter and blur', () => {
    // If either binding ever goes away the race is gone, but so is a commit path the
    // reader relies on — this test would then be passing for the wrong reason.
    // The field is kit/InlineEdit (#3112): its one `commit` is ↵ and the click
    // away alike (`{ via: 'enter' | 'blur' }`).
    const tile = board.slice(board.indexOf('<div v-if="showAddColumn"'))
    const input = tile.slice(0, tile.indexOf('/>') + 2)
    expect(input).toContain('<InlineEdit')
    expect(input).toContain('@commit="submitAddColumn"')
  })

  it('shows the input as busy while the create is in flight', () => {
    const tile = board.slice(board.indexOf('<div v-if="showAddColumn"'))
    expect(tile.slice(0, 600)).toContain(':busy="columnInFlight"')
  })

  it('still lets the next column be created after one succeeds', () => {
    // The flag must not outlive the request: startAddColumn reopens the input, and a
    // flag left up would make the very next «＋ колонка» a no-op.
    const start = fnBody(board, 'startAddColumn')
    expect(start).toBeTruthy()
    expect(start).not.toContain('columnInFlight')
    const body = fnBody(board, 'submitAddColumn')
    expect(body.match(/columnInFlight\.value = false/g) || []).toHaveLength(1)
  })

  it('leaves submitAddTask its own (synchronous) protection', () => {
    // The sibling gesture is immune for a different reason — it clears its input
    // before awaiting, so a second Enter reads an empty title and cancels. Pinned so
    // a later refactor toward «keep the text for rapid entry» doesn't reintroduce the
    // duplicate one column over.
    const body = fnBody(board, 'submitAddTask')
    expect(body).toBeTruthy()
    const clear = body.indexOf("newTaskTitle.value = ''")
    const post = body.indexOf('await boards.createTask(')
    expect(clear).toBeGreaterThan(-1)
    expect(post).toBeGreaterThan(-1)
    expect(clear).toBeLessThan(post)
  })
})
