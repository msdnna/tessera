import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { collapseKey } from '@/utils/boardLists'

// Regression test for #2932 — a freshly created column came back as a 44px strip.
//
// «Сворачивать пустые колонки» (autoCollapseEmpty, «Настроить вид» → Колонки) folds
// every lane holding no cards. A column is born empty, so the moment `submitAddColumn`
// reloaded the board the brand-new column matched the auto rule and was drawn
// collapsed: `.col.collapsed` hides the header, the card list AND the «＋ задача»
// button, so the reader typed a name and got back a nameless strip they had to click
// open before they could use it — the create gesture undone by a view setting.
//
// The fix writes the same explicit `false` override a manual expand writes, keyed by
// the id the API returns, before the reload draws the column. The inverse case was
// already handled the other way round (flipping the toggle ON clears the overrides of
// empty columns), so the setting still wins when it is the reader who asks for it.
//
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// guard reads the source the way cx-collapsed-strip-tap.spec.js does. Vitest runs with
// frontend/ as cwd; __dirname doesn't exist in ESM.
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

describe('new column survives auto-collapse (#2932)', () => {
  it('marks the created column expanded from the API response', () => {
    const body = fnBody(board, 'submitAddColumn')
    expect(body).toBeTruthy()
    // The id has to come from what the POST returned — a guessed or stale key would
    // land in a slot no column reads.
    expect(body).toMatch(/const\s*\{\s*data\s*\}\s*=\s*await boards\.createColumn\(/)
    expect(body).toMatch(/colCollapse\[[^\]]*data\.id[^\]]*\]\s*=\s*false/)
  })

  it('writes the override before the reload that draws the column', () => {
    // After the reload the column has already been rendered once — collapsed. The
    // write has to precede it or the strip still flashes.
    const body = fnBody(board, 'submitAddColumn')
    const write = body.indexOf('colCollapse[')
    const reload = body.indexOf('await load(')
    expect(write).toBeGreaterThan(-1)
    expect(reload).toBeGreaterThan(-1)
    expect(write).toBeLessThan(reload)
  })

  it('stores it under the key the collapse rule reads', () => {
    // colCollapseKey delegates to this; for a status column (the only grouping that
    // can create one — see showAddColumn) the key is the column id itself, so the
    // override lands in the very slot isColCollapsed looks up.
    expect(collapseKey('status', 'col-123')).toBe('col-123')
    const body = fnBody(board, 'submitAddColumn')
    expect(body).toContain('colCollapseKey(')
  })

  it('still lets an explicit override beat the auto rule', () => {
    // The whole fix rests on this precedence: if the auto rule ever starts winning,
    // the write above becomes a no-op.
    const body = fnBody(board, 'isColCollapsed')
    expect(body).toBeTruthy()
    expect(body).toMatch(/if\s*\(k in colCollapse\)\s*return colCollapse\[k\]/)
    expect(body).toMatch(/return autoCollapseEmpty\.value && colCount\(dcol\) === 0/)
  })

  it('still lets turning the setting ON collapse the new column too', () => {
    // Asked for explicitly, «сворачивать пустые колонки» must beat our override —
    // that watcher is what clears it (and what keeps the toggle from reading as dead).
    const watcher = board.slice(board.indexOf('watch(autoCollapseEmpty,'))
    expect(watcher.slice(0, 400)).toMatch(
      /colCount\(dcol\) === 0 && k in colCollapse\)\s*delete colCollapse\[k\]/,
    )
  })

  it('still hides the header and the add button on a collapsed column', () => {
    // Why the strip was more than cosmetic: a collapsed column offers no way to add
    // a card and no caption to read.
    const hidden = board.slice(board.indexOf('.col.collapsed :deep(.col-head)'))
    const rule = hidden.slice(0, hidden.indexOf('}'))
    expect(rule).toContain('.col.collapsed .add-task') // v2 name of the add button
    expect(rule).toContain('.col.collapsed .drop')
  })
})
