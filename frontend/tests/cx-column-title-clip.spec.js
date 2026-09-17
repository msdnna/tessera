import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — a long column caption pushed the column header's
// controls out of the column.
//
// `.col-title` is `flex: 1`, and a flex item's `min-width` defaults to `auto`: it
// will not shrink below its longest word. Ordinary status names ("В процессе")
// wrap and nobody noticed, but in tag-grouping mode — the board's headline
// feature — the caption IS a tag name, and with the raw prefix shown it is a
// single unbreakable token («сложность::высокая»). The header then grew past the
// fixed `.col` width (an expanded column does not clip its overflow) and carried
// the count chip, the collapse chevron and the «…» menu out over the neighbouring
// column, where they overlap its cards and take the clicks. In a tag lane there is
// no «…» menu at all, so that chevron is the only control that folds the lane.
//
// The rest of the product already clips this caption: the collapsed strip
// (`.strip-title` in KanbanBoard.vue) and the Android lane title (maxLines = 1).
//
// Sources are read off disk (same trick as cx-collapsed-strip-tap.spec.js):
// __dirname doesn't exist in ESM and import.meta.url isn't a file: URL under Vite.
// Vitest runs with frontend/ as cwd.
const root = process.cwd()
const header = readFileSync(resolve(root, 'src/components/ColumnHeader.vue'), 'utf8')
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

describe('column caption stays inside its column (#2932)', () => {
  const title = ruleBody(header, '.col-title')

  it('lets the caption shrink past its longest word', () => {
    expect(title).toBeTruthy()
    // Without this the whole fix is inert: `overflow: hidden` on an item that
    // never shrinks clips nothing, because the item is as wide as its content.
    expect(title).toMatch(/min-width:\s*0\b/)
  })

  it('clips the overflow to one line instead of spilling', () => {
    expect(title).toMatch(/overflow:\s*hidden\b/)
    expect(title).toMatch(/text-overflow:\s*ellipsis\b/)
    expect(title).toMatch(/white-space:\s*nowrap\b/)
  })

  it('keeps the full caption reachable once it is clipped', () => {
    // An ellipsis that hides the tag name with no way to read it would trade one
    // bug for another — the element that gets clipped carries the tooltip.
    const span = header.slice(header.indexOf('class="col-title"'))
    expect(span.slice(0, 200)).toContain(':title="dcol.name"')
  })

  it('does not leave the same rule sitting uselessly in the parent', () => {
    // `.col-title` is a non-root child of ColumnHeader, so a copy of this rule in
    // KanbanBoard's scoped block never matches — it only invites an edit that
    // silently does nothing. `.col-head` is the component's root and may stay.
    expect(ruleBody(board, '.col-title')).toBeNull()
    expect(ruleBody(board, '.col-head')).toBeTruthy()
  })
})
