// #2850: the column header shows `cards (total with subtasks)`. Two halves are
// pinned here — the counting itself, and the chip that renders it. The counting is
// where it can go wrong quietly: the board's subtask map is flat and holds every
// nesting level, so a shallow count would under-report, and a parent_id chain that
// loops back on itself would hang the render.
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { countWithSubtasks } from '@/utils/boardCounts'

// useMessage() throws without an <n-message-provider>; keep the rest of naive-ui
// intact and stub only that, as the other component specs do.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const { default: ColumnHeader } = await import('@/components/ColumnHeader.vue')

const task = (id) => ({ id, title: id })

describe('countWithSubtasks', () => {
  it('counts only the cards when they have no children', () => {
    expect(countWithSubtasks([task('a'), task('b')], {})).toEqual({ tasks: 2, total: 2 })
  })

  it('is zero for an empty column, and tolerates a missing map', () => {
    expect(countWithSubtasks([], {})).toEqual({ tasks: 0, total: 0 })
    expect(countWithSubtasks(undefined, undefined)).toEqual({ tasks: 0, total: 0 })
  })

  it('walks every nesting level, not just the direct children', () => {
    // a → a1 → a11, plus a2; b → b1. Cards: 2, tree: 6.
    const subs = {
      a: [task('a1'), task('a2')],
      a1: [task('a11')],
      b: [task('b1')],
    }
    expect(countWithSubtasks([task('a'), task('b')], subs)).toEqual({ tasks: 2, total: 6 })
  })

  it('ignores children of a parent that is not in this column', () => {
    // The map is board-wide: another column's parent must not leak into this count.
    const subs = { a: [task('a1')], z: [task('z1'), task('z2')] }
    expect(countWithSubtasks([task('a')], subs)).toEqual({ tasks: 1, total: 2 })
  })

  it('honours a narrowed child list and keeps descending below it', () => {
    // The caller hands in one merged map: the composer's narrowed list for the
    // parents it filtered (a keeps only a1), the raw board map for the levels
    // below (a1 → a11), which the filtered map never carries.
    const subs = { a: [task('a1')], a1: [task('a11')] }
    expect(countWithSubtasks([task('a')], subs)).toEqual({ tasks: 1, total: 3 })
  })

  it('terminates on a parent chain that loops back on itself', () => {
    const subs = { a: [task('a1')], a1: [task('a')] }
    expect(countWithSubtasks([task('a')], subs)).toEqual({ tasks: 1, total: 2 })
  })
})

const DCOL = { key: 'c1', name: 'К работе', rawName: 'К работе' }
const mountHead = (props) =>
  mount(ColumnHeader, { props: { dcol: DCOL, ...props }, global: { stubs: { TesseraIcon: true } } })

describe('ColumnHeader count chip', () => {
  it('renders the total in brackets after the card count', () => {
    const w = mountHead({ count: 5, total: 12 })
    expect(w.get('[data-testid="column-count"]').text()).toBe('5 (12)')
    w.unmount()
  })

  it('drops the brackets when the cards have no subtasks', () => {
    // `5 (5)` would be noise, not information.
    const w = mountHead({ count: 5, total: 5 })
    const chip = w.get('[data-testid="column-count"]')
    expect(chip.text()).toBe('5')
    expect(chip.find('.count-sub').exists()).toBe(false)
    w.unmount()
  })

  it('explains both numbers in the hover title', () => {
    const w = mountHead({ count: 5, total: 12 })
    const title = w.get('[data-testid="column-count"]').attributes('title')
    expect(title).toContain('5')
    expect(title).toContain('12')
    w.unmount()

    const flat = mountHead({ count: 5, total: 5 })
    const flatTitle = flat.get('[data-testid="column-count"]').attributes('title')
    expect(flatTitle).toContain('5')
    expect(flatTitle).not.toContain('12')
    flat.unmount()
  })
})
