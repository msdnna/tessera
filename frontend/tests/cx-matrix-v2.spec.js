// The matrix v2 (#2973): the thresholds in the axis captions, the selection on
// the keys, ⌘ + arrow pins the card into that quadrant, ⌫ gives a pinned card
// back to its automatic place — the override is the task's own
// `eisenhower_quadrant` (migration 0028), no new storage.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { handleKeydown, resetShortcuts } from '@/shortcuts/dispatcher'

const eisenhower = vi.fn(() => Promise.resolve({}))
vi.mock('@/api', () => ({ tasks: { eisenhower: (...a) => eisenhower(...a) } }))

const { default: BoardMatrixView } = await import('@/components/BoardMatrixView.vue')

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
const day = 24 * 3600 * 1000
const iso = (d) => new Date(Date.now() + d * day).toISOString()
const TASKS = [
  { id: 'a', title: 'a', priority: 4, due_date: iso(1) },
  { id: 'b', title: 'b', priority: 1, due_date: null, eisenhower_quadrant: 1 },
]

let w = null
function mountMatrix(props = {}) {
  const actions = { complete: vi.fn(), priority: vi.fn(), field: vi.fn() }
  w = mount(BoardMatrixView, {
    props: { tasks: TASKS, actions, ...props },
    global: {
      stubs: {
        TaskCard: {
          props: ['selected'],
          template: '<div class="tc" :class="{ sel: selected }" />',
        },
        TesseraIcon: true,
      },
    },
    attachTo: document.body,
  })
  return { w, actions }
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetShortcuts()
  eisenhower.mockClear()
})
afterEach(() => {
  w?.unmount()
  w = null
})

describe('BoardMatrixView v2', () => {
  it('names the thresholds on the axes', async () => {
    mountMatrix({ urgentWindowDays: 7, importantPriority: 4 })
    const text = w.text()
    expect(text).toContain('7 дней')
    expect(text.toLowerCase()).toContain('≥ срочный')
  })

  it('marks a card placed by hand, and its mark gives it back', async () => {
    mountMatrix()
    const pins = w.findAll('[data-testid="matrix-pin"]')
    expect(pins).toHaveLength(1)
    await pins[0].trigger('click')
    expect(eisenhower).toHaveBeenCalledWith('b', null)
  })

  it('selects with the arrows and pins with ⌘ + arrow', async () => {
    const { actions } = mountMatrix()
    press('ArrowDown', { key: 'ArrowDown' })
    await nextTick()
    expect(w.find('.tc.sel').exists()).toBe(true)
    press('KeyE', { key: 'e' })
    expect(actions.complete).toHaveBeenCalledWith([TASKS[0]])
    press('ArrowDown', { key: 'ArrowDown', ctrlKey: true })
    expect(eisenhower).toHaveBeenCalledWith('a', 2)
  })

  it('⌫ unpins the selected card placed by hand', async () => {
    mountMatrix()
    press('ArrowDown', { key: 'ArrowDown' }) // a, in «Сделать»
    press('ArrowRight', { key: 'ArrowRight' }) // b, in «Запланировать»
    press('Backspace', { key: 'Backspace' })
    expect(eisenhower).toHaveBeenCalledWith('b', null)
  })
})
