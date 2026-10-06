import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { collapseKey, NO_LANE } from '@/utils/boardLists'

// Regression test for #2932 — the «Без тегов» lane arrived collapsed in a grouping
// the reader had never folded it in.
//
// Tag grouping can be narrowed to one namespace («Тег · Сложность»): `groupTags` is
// then only that prefix's tags, so the catch-all lane holds «cards carrying no
// сложность:: tag» — on a real board most of it. Un-narrowed, the same lane holds
// «cards with no tag at all». Different buckets, and the collapse overrides live in
// ONE per-board blob that switching the narrowing does not reset — so folding away
// the noisy lane under one prefix drew the other prefix's catch-all as a 44px strip,
// and expanding it cancelled auto-collapse of an empty catch-all somewhere else.
//
// The fix threads `tagPrefix` into `collapseKey`. This file pins the board's half of
// it (the key it asks for); the function's own table is in ut-boardLists.spec.js.
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// guard reads the source the way cx-new-column-not-collapsed.spec.js does.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

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

describe('tag-prefix scoping of the catch-all collapse (#2932)', () => {
  it('asks for the key with the active tag narrowing', () => {
    const body = fnBody(board, 'colCollapseKey')
    expect(body).toBeTruthy()
    expect(body).toMatch(/collapseKey\(\s*groupMode\.value\s*,\s*dcol\.key\s*,\s*tagPrefix\.value/)
  })

  it('routes every override through that one key', () => {
    // toggleCollapse / isColCollapsed / the autoCollapseEmpty watcher must not build
    // a key of their own — a second spelling would leak again on its own path.
    for (const name of ['isColCollapsed', 'toggleCollapse']) {
      expect(fnBody(board, name)).toContain('colCollapseKey(')
    }
    const watcher = board.slice(board.indexOf('watch(autoCollapseEmpty,'))
    expect(watcher.slice(0, 400)).toContain('colCollapseKey(dcol)')
  })

  it('separates the buckets a prefix switch swaps', () => {
    // What the board does: fold the lane, then flip the grouping chip's narrowing.
    const colCollapse = {}
    colCollapse[collapseKey('tag', NO_LANE, 'сложность')] = true
    expect(collapseKey('tag', NO_LANE, 'команда') in colCollapse).toBe(false)
    expect(collapseKey('tag', NO_LANE, '') in colCollapse).toBe(false)
  })

  it('leaves the un-narrowed key as it was on disk', () => {
    // colCollapse is persisted per board (snapshotToolbar → localStorage / saved
    // views), so the common mode's key has to stay byte-identical.
    expect(collapseKey('tag', NO_LANE, '')).toBe(`tag:${NO_LANE}`)
  })

  it('keeps a real tag lane keyed by its id', () => {
    // A tag id means the same lane under every narrowing; prefixing it would throw
    // away collapses readers already have.
    expect(collapseKey('tag', 'tag-1', 'сложность')).toBe('tag-1')
  })
})
