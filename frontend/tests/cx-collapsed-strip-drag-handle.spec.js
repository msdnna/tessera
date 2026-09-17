import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — a collapsed column could not be reordered.
//
// The board's column draggable takes `handle: '.col-drag'`, and ColumnHeader stamps
// that class on the status glyph and the caption. Both live inside `.col-head`,
// which `.col.collapsed :deep(.col-head) { display: none }` removes outright — so a
// folded column had no handle anywhere in its DOM and stayed put. With the board
// setting «collapse empty columns» on, that is EVERY empty column, i.e. exactly the
// ones being arranged while a board is still being set up.
//
// The fix hangs the handle on the strip (the only element a collapsed column still
// renders) under the same condition the draggable itself is enabled by.
//
// Sources are read off disk (same trick as cx-collapsed-strip-keyboard.spec.js):
// __dirname doesn't exist in ESM and import.meta.url isn't a file: URL under Vite.
// Vitest runs with frontend/ as cwd.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')
const header = readFileSync(resolve(root, 'src/components/ColumnHeader.vue'), 'utf8')

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

// The column draggable's opening tag — the one bound to `colModel`.
const colDraggableTag = (() => {
  const at = board.indexOf(':list="colModel"')
  const open = board.lastIndexOf('<draggable', at)
  return board.slice(open, board.indexOf('>', at) + 1)
})()

describe('collapsed column strip — drag handle (#2932)', () => {
  it('carries the handle class the column draggable actually looks for', () => {
    // If these two ever disagree the strip gets a cosmetic class and the column
    // stays undraggable — which is the bug this spec is about.
    expect(colDraggableTag).toContain('handle=".col-drag"')
    expect(stripTag).toContain("'col-drag': canReorderColumns")
  })

  it('offers the handle only where reordering is possible at all', () => {
    // Status grouping only (tag/milestone lanes have no stored column order) and
    // never in the archive, which is read-only.
    expect(board).toContain(
      "const canReorderColumns = computed(() => groupMode.value === 'status' && !archivedMode.value)",
    )
  })

  it('gates the draggable on the same computed, so the two cannot drift', () => {
    expect(colDraggableTag).toContain(':disabled="!canReorderColumns"')
    // The predicate used to be spelled out inline here; leaving the copy behind is
    // what would let the strip advertise a grip the draggable refuses to honour.
    expect(colDraggableTag).not.toContain("groupMode !== 'status'")
  })

  it('shows a grab hand, and beats the strip’s own pointer cursor', () => {
    const css = ruleBody(board, '.col-strip.col-drag')
    expect(css).toMatch(/cursor:\s*grab/)
    // One class would tie with `.col-strip { cursor: pointer }` and lose on source
    // order; ColumnHeader's identical `.col-drag` rule is scoped to that component
    // and never reaches this button.
    expect(ruleBody(board, '.col-drag')).toBeNull()
    expect(ruleBody(board, '.col-strip')).toMatch(/cursor:\s*pointer/)
  })

  it('keeps the strip a real button that still expands the column', () => {
    // The handle must not cost the keyboard path (#2932) or the tap target (#2893).
    expect(stripTag.startsWith('<button')).toBe(true)
    expect(stripTag).toContain('type="button"')
    expect(stripTag).toContain('@click="toggleCollapse(dcol)"')
  })

  it('leaves the expanded column’s handle where it was', () => {
    // The strip only replaces a handle that is hidden; an expanded column keeps
    // dragging by its glyph and caption, and the archive keeps neither.
    expect(header).toContain("'col-drag': canEdit")
    expect(header).toContain('const canEdit = computed(() => props.editable && !props.readonly)')
  })
})
