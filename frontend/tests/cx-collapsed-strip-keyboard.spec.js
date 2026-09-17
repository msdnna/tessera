import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — a collapsed column was a dead end for the keyboard,
// and the one place its name is unreadable was the one place that never said it.
//
// Collapsing is reachable from the keyboard: `.col-collapse` in ColumnHeader is an
// n-button, so Tab + Enter folds a column. Collapsing then hides the whole header
// (`.col.collapsed :deep(.col-head) { display: none }`), and by #2893 the strip is
// the ONLY way back out — but the strip was a `<div>` with a click handler, which
// never enters the tab order. Folded from the keyboard, a column stayed folded.
//
// The caption had the mirror problem: `.strip-title` is rotated and clipped
// (`max-height` + ellipsis), while the strip's own `title` read a bare «expand the
// column». Under TAG grouping the column name IS a tag («сложность::высокая») —
// exactly the case `.col-title` in ColumnHeader carries its own `title` for.
//
// Sources are read off disk (same trick as cx-collapsed-strip-tap.spec.js):
// __dirname doesn't exist in ESM and import.meta.url isn't a file: URL under Vite.
// Vitest runs with frontend/ as cwd.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')
const ru = JSON.parse(readFileSync(resolve(root, 'src/locales/ru/board.json'), 'utf8'))
const en = JSON.parse(readFileSync(resolve(root, 'src/locales/en/board.json'), 'utf8'))

// Body of a top-level CSS rule, by brace matching from its selector.
function ruleBody(source, selector) {
  const start = source.indexOf('\n' + selector + ' {')
  if (start === -1) return null
  const open = source.indexOf('{', start)
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i)
  }
  return null
}

// The strip's opening tag, from its element name through the closing `>`.
const stripTag = (() => {
  const at = board.indexOf('class="col-strip"')
  const open = board.lastIndexOf('<', at)
  return board.slice(open, board.indexOf('>', at) + 1)
})()

describe('collapsed column strip — keyboard + caption (#2932)', () => {
  it('is a real button, so a keyboard-folded column can be unfolded again', () => {
    expect(stripTag.startsWith('<button')).toBe(true)
    // Inside a form-less board a bare button would still default to `submit`;
    // and without the explicit type a future wrapper form would reload the page.
    expect(stripTag).toContain('type="button"')
  })

  it('closes the tag it opens, so the count chip stays inside the strip', () => {
    const at = board.indexOf('class="col-strip"')
    const after = board.slice(at, at + 900)
    expect(after).toContain('</button>')
    // …and not the old div closer, which would leave the markup unbalanced.
    expect(after.slice(0, after.indexOf('</button>'))).not.toContain('</div>')
  })

  it('keeps the expand click on the strip itself', () => {
    // Same invariant cx-collapsed-strip-tap.spec.js locks: the stretched element
    // is the one that carries the click. Re-checked here because this spec moved
    // the element's tag name.
    expect(stripTag).toContain('@click="toggleCollapse(dcol)"')
  })

  it('resets the button chrome so the strip still looks like a strip', () => {
    const css = ruleBody(board, '.col-strip')
    expect(css).toBeTruthy()
    expect(css).toMatch(/border:\s*0/)
    expect(css).toMatch(/background:\s*none/)
    // A button doesn't inherit the page font, and — unlike the block-level div it
    // replaced — doesn't stretch to the tile on its own.
    expect(css).toMatch(/font:\s*inherit/)
    expect(css).toMatch(/width:\s*100%/)
    // The stretch guarded by cx-collapsed-strip-tap.spec.js must survive.
    expect(css).toMatch(/flex:\s*1\b/)
  })

  it('shows a focus ring, since the strip is now a tab stop', () => {
    expect(ruleBody(board, '.col-strip:focus-visible')).toMatch(/outline:/)
  })

  it('names the column in the strip tooltip', () => {
    expect(stripTag).toContain(':title="stripTitle(dcol)"')
    expect(board).toContain("t('board.column.expandTitle', { name: dcol.name })")
  })

  it('has the interpolated tooltip in both locales and no stale bare key', () => {
    for (const loc of [ru, en]) {
      expect(loc.column.expandTitle).toContain('{name}')
      // The old key said only «expand the column»; leaving it behind invites the
      // next edit to reach for it again.
      expect(loc.column.expand).toBeUndefined()
    }
  })
})
