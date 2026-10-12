import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve, join, relative } from 'node:path'
import process from 'node:process'

// Regression test for #2932 — the board composer's sort chips fought the phone.
//
// Every vuedraggable list in the app holds the touch drag before it starts:
//
//   :delay="…" :touch-start-threshold="6" (+ :delay-on-touch-only="true")
//
// It is not decoration. SortableJS defaults to `delay: 0` and
// `touchStartThreshold: 0`, so on a touch screen a drag begins on the first pixel
// of movement and preventDefault's the scroll it was competing with. Every list
// here sits inside something scrollable (the board scroller, a column, the
// sidebar, the horizontally scrolling `.facets` bar), and in several of them the
// draggable element is also tappable — the sort chip flips its direction on
// click, a card opens. Without the trio the scroll can't get past the list and
// the tap is eaten by a drag that silently persists a reorder.
//
// The sort chips shipped without it (fixed here); this guard keeps the next
// draggable from doing the same. Sources are read off disk — the same trick as
// cx-scrollbars.spec.js: __dirname doesn't exist in ESM and import.meta.url isn't
// a file: URL under Vite. Vitest runs with frontend/ as cwd.
const root = process.cwd()
const srcDir = resolve(root, 'src')

function vueFiles(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...vueFiles(p))
    else if (name.endsWith('.vue')) out.push(p)
  }
  return out
}

// Slice out a `<draggable …>` opening tag. Quote-aware rather than a regex: the
// attribute values here hold `>` in expressions like `count > 0`, and a lazy
// `[^>]*` would cut the tag in half at the first one.
function openingTags(source, tag) {
  const tags = []
  const needle = '<' + tag
  let i = source.indexOf(needle)
  while (i !== -1) {
    let quote = null
    for (let j = i + needle.length; j < source.length; j++) {
      const ch = source[j]
      if (quote) {
        if (ch === quote) quote = null
      } else if (ch === '"' || ch === "'") {
        quote = ch
      } else if (ch === '>') {
        tags.push(source.slice(i, j + 1))
        break
      }
    }
    i = source.indexOf(needle, i + needle.length)
  }
  return tags
}

const draggables = vueFiles(srcDir).flatMap((file) =>
  openingTags(readFileSync(file, 'utf8'), 'draggable').map((tag) => ({
    file: relative(root, file),
    tag,
  })),
)

describe('vuedraggable touch settings (#2932)', () => {
  it('finds the draggable lists', () => {
    // A sanity floor: if the scanner silently stops matching, the assertions
    // below would pass over an empty list.
    expect(draggables.length).toBeGreaterThanOrEqual(8)
  })

  it('every <draggable> sets :touch-start-threshold', () => {
    const missing = draggables.filter((d) => !d.tag.includes(':touch-start-threshold='))
    expect(missing.map((d) => `${d.file}: ${d.tag.slice(0, 60)}…`)).toEqual([])
  })

  it('every <draggable> holds the touch drag for a non-zero delay', () => {
    // The value itself is per-list (the Eisenhower quadrants hold 120ms, a card's
    // subtasks 300ms, the rest 160ms) — what matters is that there IS a hold, so
    // a tap stays a tap.
    const bad = draggables.filter((d) => {
      const m = d.tag.match(/:delay="(\d+)"/)
      return !m || Number(m[1]) <= 0
    })
    expect(bad.map((d) => `${d.file}: ${d.tag.slice(0, 60)}…`)).toEqual([])
  })

  it('the hold is touch-only, except where a mouse hold is the point', () => {
    // `delay` without `delay-on-touch-only` makes the MOUSE wait too. Every board
    // and sidebar list wants the pointer drag to start at once, so all of them
    // pair the two. The one that doesn't is a card's subtask list, which holds a
    // deliberate 300ms on both pointers — it is nested inside a card that is
    // itself draggable, and the long hold is what keeps a stray press on a child
    // row from turning into a drag while reading the card.
    const exempt = 'src/components/task/TaskCardSubtasks.vue'
    const missing = draggables.filter(
      (d) => d.file !== exempt && !d.tag.includes(':delay-on-touch-only='),
    )
    expect(missing.map((d) => `${d.file}: ${d.tag.slice(0, 60)}…`)).toEqual([])
  })

  it('the board composer sort chips carry it (the list that regressed)', () => {
    const board = draggables.filter((d) => d.file.endsWith('KanbanBoard.vue'))
    const chips = board.find((d) => d.tag.includes('"sortLevels"'))
    expect(chips).toBeTruthy()
    expect(chips.tag).toContain(':delay="160"')
    expect(chips.tag).toContain(':delay-on-touch-only="true"')
    expect(chips.tag).toContain(':touch-start-threshold="6"')
  })
})
