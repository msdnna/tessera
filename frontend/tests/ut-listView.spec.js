// «Список» (#3020): the pure half — the geometry the windowed rendering reads,
// the keyboard cursor and its selection, the neighbours of a ⌘↑/↓ step.
import { describe, it, expect } from 'vitest'
import {
  listLayout,
  trayWindow,
  spotTop,
  navOrder,
  stepCursor,
  rangeBetween,
  reorderNeighbours,
  tagsThatFit,
  spotKey,
} from '@/utils/listView'
import { listConfig, LIST_WIDTHS, LIST_WIDTH_MIN, LIST_WIDTH_MAX } from '@/utils/boardViews'

const M = { head: 38, headGap: 6, row: 40, gap: 2, pad: 4, empty: 32 }
const tasks = (n, p = 't') => Array.from({ length: n }, (_, i) => ({ id: `${p}${i}` }))
const groups = [{ key: 'a' }, { key: 'b' }, { key: 'c' }]
const lists = { a: tasks(3, 'a'), b: [], c: tasks(500, 'c') }

describe('listLayout', () => {
  it('stacks headers and trays with exact heights', () => {
    const l = listLayout({ groups, lists, folded: () => false, m: M })
    const [a, b, c] = l.blocks
    expect(a.headTop).toBe(6)
    expect(a.tray).toEqual({ top: 44, height: 4 + 3 * 40 + 2 * 2 + 4, addTop: 4 + 3 * 42 })
    // An empty group keeps a short tray that says so.
    expect(b.tray.height).toBe(4 + 32 + 4)
    expect(c.tray.height).toBe(8 + 500 * 40 + 499 * 2)
    expect(l.height).toBe(c.tray.top + c.tray.height)
  })

  it('a folded group is a header alone', () => {
    const l = listLayout({ groups, lists, folded: (k) => k === 'a', m: M })
    expect(l.blocks[0].tray).toBeNull()
    expect(l.blocks[1].headTop).toBe(6 + 38 + 6)
  })

  it('makes room for the inline «new task» row', () => {
    const l = listLayout({
      groups,
      lists,
      folded: () => false,
      extra: (k) => (k === 'b' ? 40 : 0),
      m: M,
    })
    expect(l.blocks[1].tray.height).toBe(4 + 40 + 4)
    expect(l.blocks[1].tray.addTop).toBe(4)
  })
})

describe('trayWindow', () => {
  it('mounts only the rows that meet the viewport', () => {
    const l = listLayout({ groups, lists, folded: () => false, m: M })
    const c = l.blocks[2]
    const lo = c.tray.top + 4 + 200 * 42
    const [i0, i1] = trayWindow(c, lo, lo + 800, M)
    expect(i0).toBe(200)
    expect(i1 - i0).toBeLessThan(25)
    expect(trayWindow(l.blocks[0], 100000, 101000, M)).toEqual([0, 0])
  })
})

describe('spotTop', () => {
  it('finds a row inside its tray, a header on its own', () => {
    const l = listLayout({ groups, lists, folded: () => false, m: M })
    expect(spotTop(l, { g: 'a', id: 'a1' }, lists, M)).toEqual({ top: 44 + 4 + 42, height: 40 })
    expect(spotTop(l, { g: 'b', id: null }, lists, M).height).toBe(38)
  })
})

describe('cursor and selection', () => {
  const small = { a: tasks(2, 'a'), b: tasks(1, 'b') }
  const order = navOrder([{ key: 'a' }, { key: 'b' }], small, (k) => k === 'b')

  it('reads headers and the rows of open groups only', () => {
    expect(order.map(spotKey)).toEqual(['a::', 'a::a0', 'a::a1', 'b::'])
  })

  it('starts on the first row and stops at the ends', () => {
    expect(stepCursor(order, null, 1)).toEqual({ g: 'a', id: 'a0' })
    expect(stepCursor(order, { g: 'b', id: null }, 1)).toEqual({ g: 'b', id: null })
    expect(stepCursor(order, { g: 'a', id: 'a0' }, -1)).toEqual({ g: 'a', id: null })
  })

  it('selects the rows between anchor and cursor, never a header', () => {
    expect(rangeBetween(order, { g: 'a', id: 'a1' }, { g: 'a', id: null })).toEqual([
      { g: 'a', id: 'a0' },
      { g: 'a', id: 'a1' },
    ])
  })
})

describe('reorderNeighbours', () => {
  const list = tasks(4)
  it('names the rows the step lands between', () => {
    expect(reorderNeighbours(list, 't2', -1)).toEqual({ beforeId: 't0', afterId: 't1' })
    expect(reorderNeighbours(list, 't1', -1)).toEqual({ beforeId: null, afterId: 't0' })
    expect(reorderNeighbours(list, 't1', 1)).toEqual({ beforeId: 't2', afterId: 't3' })
    expect(reorderNeighbours(list, 't2', 1)).toEqual({ beforeId: 't3', afterId: null })
  })
  it('refuses past either end', () => {
    expect(reorderNeighbours(list, 't0', -1)).toBeNull()
    expect(reorderNeighbours(list, 't3', 1)).toBeNull()
  })
})

describe('field columns', () => {
  it('fits at least one tag pill', () => {
    expect(tagsThatFit(150, 3)).toBe(1)
    expect(tagsThatFit(260, 3)).toBe(2)
    expect(tagsThatFit(60, 0)).toBe(0)
  })
  it('keeps widths inside bounds, defaults for the rest', () => {
    const c = listConfig({ widths: { due: 10, tags: 9999, people: 'x' } })
    expect(c.widths.due).toBe(LIST_WIDTH_MIN)
    expect(c.widths.tags).toBe(LIST_WIDTH_MAX)
    expect(c.widths.people).toBe(LIST_WIDTHS.people)
    expect(listConfig().widths).toEqual(LIST_WIDTHS)
  })
})
