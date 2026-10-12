// The Eisenhower thresholds are the board's settings now (#3018): urgent = due
// within N days (or overdue), important = priority from M up; defaults ≤ 3 days
// and ≥ high. A change of either re-sorts the grid.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import draggable from 'vuedraggable'

vi.mock('@/api', () => ({
  tasks: { eisenhower: vi.fn(() => Promise.resolve({})) },
}))

const { default: BoardMatrixView } = await import('@/components/BoardMatrixView.vue')

const day = 24 * 3600 * 1000
const iso = (d) => new Date(Date.now() + d * day).toISOString()
const TASKS = [
  { id: 'hi-soon', title: 'a', priority: 3, due_date: iso(2) },
  { id: 'mid-soon', title: 'b', priority: 2, due_date: iso(2) },
  { id: 'hi-week', title: 'c', priority: 3, due_date: iso(6) },
]

beforeEach(() => setActivePinia(createPinia()))

const ids = (w) => w.findAllComponents(draggable).map((d) => d.props('list').map((x) => x.id))

describe('BoardMatrixView thresholds', () => {
  it('defaults to ≤ 3 days and ≥ high', () => {
    const w = mount(BoardMatrixView, {
      props: { tasks: TASKS },
      global: { stubs: { TaskCard: true, TesseraIcon: true } },
    })
    // quadrants: 0 urgent+important, 1 important, 2 urgent, 3 neither
    expect(ids(w)).toEqual([['hi-soon'], ['hi-week'], ['mid-soon'], []])
    w.unmount()
  })

  it('follows a wider window and a lower importance floor', async () => {
    const w = mount(BoardMatrixView, {
      props: { tasks: TASKS },
      global: { stubs: { TaskCard: true, TesseraIcon: true } },
    })
    await w.setProps({ urgentWindowDays: 7, importantPriority: 2 })
    expect(ids(w)).toEqual([['hi-soon', 'mid-soon', 'hi-week'], [], [], []])
    w.unmount()
  })
})
