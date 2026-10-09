// «Календарь» v2 (#2973 / #3019): «+N ещё» past three pills, the month / week
// mode handed to the board, the day cursor and the picked task on the keys, a
// pill dragged onto another day moving its due (date-only stays date-only).
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { handleKeydown, resetShortcuts } from '@/shortcuts/dispatcher'

vi.mock('@/api', () => ({ tasks: {}, getAccessToken: () => null }))

const BoardCalendarView = (await import('@/components/BoardCalendarView.vue')).default
const { useThemeStore } = await import('@/stores/theme')

function press(code, opts = {}) {
  handleKeydown(
    new KeyboardEvent('keydown', {
      code,
      key: opts.key ?? '',
      bubbles: true,
      cancelable: true,
      ...opts,
    }),
  )
}

// The month of «today» in UTC, so date-only dues land where the test expects.
const now = new Date()
const Y = now.getFullYear()
const M = now.getMonth()
const due = (d) => new Date(Date.UTC(Y, M, d)).toISOString()
const COLS = [
  { id: 'c1', name: 'К работе', color: '#0eb0a9', position: 1 },
  { id: 'c2', name: 'Готово', color: '#18a058', position: 2 },
]
const TASKS = [
  ...[1, 2, 3, 4, 5].map((i) => ({
    id: `t${i}`,
    title: `Задача ${i}`,
    column_id: 'c1',
    due_date: due(12),
  })),
  { id: 'u1', title: 'Без срока', column_id: 'c1', due_date: null },
]

let w = null
function mountCal(extra = {}) {
  const actions = {
    setDates: vi.fn(),
    create: vi.fn(),
    complete: vi.fn(),
    priority: vi.fn(),
    field: vi.fn(),
  }
  w = mount(BoardCalendarView, {
    props: { tasks: TASKS, statusColumns: COLS, doneColumnId: 'c2', actions, ...extra },
    attachTo: document.body,
  })
  return { w, actions }
}
const cellOf = (d) =>
  w
    .findAll('.cal-cell')
    .find((c) => !c.classes('out') && c.find('.cal-daynum').text() === String(d))

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  resetShortcuts()
  const theme = useThemeStore()
  theme.language = 'ru'
  theme.timezone = 'UTC'
})
afterEach(() => {
  w?.unmount()
  w = null
})

describe('month', () => {
  it('shows three pills and «+N ещё» for the rest of a crowded day', () => {
    mountCal()
    const cell = cellOf(12)
    expect(cell.findAll('.cal-chip')).toHaveLength(3)
    expect(cell.find('[data-testid="calendar-more"]').text()).toContain('+2')
  })

  it('«+N ещё» opens the whole day', async () => {
    mountCal()
    await cellOf(12).find('[data-testid="calendar-more"]').trigger('click')
    expect(w.findAll('[data-testid="calendar-day-list"] .cal-chip')).toHaveLength(5)
  })

  it('counts the undated tasks and lists them on a click', async () => {
    mountCal()
    const btn = w.find('[data-testid="calendar-undated"]')
    expect(btn.text()).toContain('1')
    await btn.trigger('click')
    expect(w.find('[data-testid="calendar-undated-list"]').text()).toContain('Без срока')
  })

  it('hands the mode to the board', async () => {
    mountCal()
    await w.find('[data-testid="calendar-mode-week"]').trigger('click')
    expect(w.emitted('update:mode')[0]).toEqual(['week'])
  })

  it('the week shows every task of a day', () => {
    mountCal({ mode: 'week' })
    expect(w.findAll('.cal-cell')).toHaveLength(7)
  })
})

describe('keyboard', () => {
  it('T puts the cursor on today, ↵ picks the first task, E completes it', async () => {
    const { actions } = mountCal({
      tasks: [{ id: 'x', title: 'Сегодня', column_id: 'c1', due_date: new Date().toISOString() }],
    })
    press('KeyT', { key: 't' })
    await nextTick()
    expect(w.find('.cal-cell.cursor').exists()).toBe(true)
    press('Enter', { key: 'Enter' })
    await nextTick()
    expect(w.find('.cp.picked').exists()).toBe(true)
    press('KeyE', { key: 'e' })
    expect(actions.complete).toHaveBeenCalled()
  })

  it('⇧→ on a picked task moves its due a day, date-only', async () => {
    const { actions } = mountCal({
      tasks: [{ id: 'x', title: 'Сдвиг', column_id: 'c1', due_date: due(12) }],
    })
    press('ArrowRight', { key: 'ArrowRight' }) // cursor appears on today
    await w.findAll('.cal-chip')[0].trigger('click') // picks it (and opens)
    press('ArrowDown', { key: 'ArrowDown' }) // keyboard back in use
    press('Enter', { key: 'Enter' })
    press('ArrowRight', { key: 'ArrowRight', shiftKey: true })
    await nextTick()
    const [task, dates] = actions.setDates.mock.calls.at(-1)
    expect(task.id).toBe('x')
    expect(dates.due_date).toBe(due(13))
  })
})

describe('drag', () => {
  it('a pill dropped on another day moves the due there', async () => {
    const { actions } = mountCal()
    const pill = cellOf(12).find('.cal-chip')
    const target = cellOf(20).element
    document.elementFromPoint = vi.fn(() => target)
    pill.element.dispatchEvent(
      new MouseEvent('pointerdown', { button: 0, clientX: 10, clientY: 10, bubbles: true }),
    )
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 60, clientY: 80 }))
    await nextTick()
    expect(w.find('.cal-cell.drop').exists()).toBe(true)
    expect(w.emitted('drag-state')[0]).toEqual([true])
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 60, clientY: 80 }))
    await nextTick()
    const [task, dates] = actions.setDates.mock.calls[0]
    expect(task.id).toBe('t1')
    expect(dates.due_date).toBe(new Date(Date.UTC(Y, M, 20)).toISOString())
    expect(w.emitted('drag-state')[1]).toEqual([false])
  })

  it('does not drag in the archive', () => {
    const { actions } = mountCal({ readonly: true })
    const pill = cellOf(12).find('.cal-chip')
    document.elementFromPoint = vi.fn(() => cellOf(20).element)
    pill.element.dispatchEvent(
      new MouseEvent('pointerdown', { button: 0, clientX: 10, clientY: 10, bubbles: true }),
    )
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 60, clientY: 80 }))
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 60, clientY: 80 }))
    expect(actions.setDates).not.toHaveBeenCalled()
  })
})
