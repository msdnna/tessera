import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — a collapsed column couldn't be re-opened by tapping
// it on a phone.
//
// A collapsed column renders as a 44px tile. Its header (and with it the collapse
// chevron) is hidden, so `.col-strip` — which carries the expand click — is the
// only way back out of a collapse; that is exactly the constraint #2893 settled on
// when it kept the strip on mobile instead of dropping it.
//
// The catch is that the strip was content-sized: chevron, a vertical title capped
// at 260px, a count. On the desktop that matches the tile (`.col` is
// `align-self: flex-start`, so the column is as tall as its content), which is why
// this never showed there. The mobile block stretches `.col` to the full board
// height, and the strip kept its own height — most of the tall tile became dead
// area that advertised `cursor: pointer` and swallowed the tap. The drag drop
// overlay had always covered the whole tile (`inset: 0`), so the two disagreed.
//
// This locks the invariant: whatever element holds the expand click must fill the
// collapsed tile. Sources are read off disk (same trick as cx-scrollbars.spec.js):
// __dirname doesn't exist in ESM and import.meta.url isn't a file: URL under Vite.
// Vitest runs with frontend/ as cwd.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

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

describe('collapsed column strip (#2932)', () => {
  it('hangs the expand click on the strip, not on some inner label', () => {
    // The click lives on `.col-strip` itself — the element the CSS below stretches.
    // If it ever moves onto a child, that child is the tap target and the rule
    // guarded here stops meaning anything.
    const strip = board.slice(board.indexOf('class="col-strip"'))
    expect(strip.slice(0, 200)).toContain('@click="toggleCollapse(dcol)"')
  })

  it('stretches the strip to the collapsed tile so the whole tile is tappable', () => {
    const css = ruleBody(board, '.col-strip')
    expect(css).toBeTruthy()
    // `flex: 1 …` — the strip grows into the tile. Basis stays `auto` so the
    // desktop tile, which is sized by this very content, is unchanged.
    expect(css).toMatch(/flex:\s*1\b/)
    expect(css).not.toMatch(/flex:\s*1\s+0\s+0/)
  })

  it('keeps the collapsed tile full height on a phone', () => {
    // The stretch above is only load-bearing because mobile grows `.col`; if that
    // ever goes back to content height the strip has nothing to grow into.
    // Several mobile blocks live in this file; anchor on the one that lays the
    // columns out (its own comment says so).
    const mobile = board.slice(board.indexOf('/* Mobile: snap each column to the start'))
    expect(mobile.slice(0, 900)).toContain('align-self: stretch')
  })

  it('still hides the header on a collapsed column', () => {
    // The reason the strip is the only exit: no header, no collapse button.
    const hidden = board.slice(board.indexOf('.col.collapsed :deep(.col-head)'))
    expect(hidden.slice(0, 200)).toContain('display: none')
  })
})
