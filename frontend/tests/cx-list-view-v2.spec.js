// «Список» v2 (#2973 / #3020): groups as tinted trays that fold, rows picked by
// the keyboard (and only drawn as picked once the keyboard is in use), the
// board's task actions behind E / P 0–4 / ⌘←→ / ⌘↑↓, rename in place, the
// field columns resized from their captions.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { handleKeydown, resetShortcuts } from '@/shortcuts/dispatcher'

vi.mock('@/api', () => ({
  tasks: { update: vi.fn(), move: vi.fn(), remove: vi.fn(), archive: vi.fn() },
  boards: {},
}))

const { default: BoardListView } = await import('@/components/BoardListView.vue')

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

const COLS = [
  { key: 'c1', name: 'К работе', rawName: 'К работе', color: '#0eb0a9', status: { id: 'c1' } },
  { key: 'c2', name: 'Готово', rawName: 'Готово', color: '#18a058', status: { id: 'c2' } },
]
const LISTS = {
  c1: [
    { id: 't1', number: 1, title: 'Первая', priority: 3 },
    { id: 't2', number: 2, title: 'Вторая', priority: 0 },
    { id: 't3', number: 3, title: 'Третья', priority: 1 },
  ],
  c2: [{ id: 't4', number: 4, title: 'Готовая', completed_at: '2026-10-01T10:00:00Z' }],
}

let wrappers = []
function mountList(extra = {}) {
  const actions = {
    complete: vi.fn(),
    priority: vi.fn(),
    shift: vi.fn(() => Promise.resolve()),
    reorder: vi.fn(() => Promise.resolve()),
    rename: vi.fn(() => Promise.resolve(true)),
    create: vi.fn(() => Promise.resolve()),
    field: vi.fn(),
  }
  const w = mount(BoardListView, {
    props: {
      columns: COLS,
      statusColumns: [{ id: 'c1' }, { id: 'c2' }],
      lists: LISTS,
      groupMode: 'status',
      doneColumnId: 'c2',
      canReorder: true,
      actions,
      ...extra,
    },
    global: { stubs: { TagPill: true, UserAvatar: true, TesseraIcon: true } },
    attachTo: document.body,
  })
  wrappers.push(w)
  return { w, actions }
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetShortcuts()
})
afterEach(() => {
  wrappers.forEach((w) => w.unmount())
  wrappers = []
})

describe('groups', () => {
  it('draws every lane with its rows, tinted with the lane colour', () => {
    const { w } = mountList()
    const heads = w.findAll('[data-testid="list-group"]')
    expect(heads.map((h) => h.attributes('data-group'))).toEqual(['К работе', 'Готово'])
    expect(w.findAll('[data-testid="list-row"]')).toHaveLength(4)
    expect(w.find('.lv-tray').attributes('style')).toContain('--gc: #0eb0a9')
  })

  it('a click on the header asks the board to fold the lane', async () => {
    const { w } = mountList()
    await w.findAll('[data-testid="list-group"]')[0].trigger('click')
    expect(w.emitted('fold')[0][0].key).toBe('c1')
  })

  it('a folded lane keeps its header and loses its rows', () => {
    const { w } = mountList({ isFolded: (d) => d.key === 'c1' })
    expect(w.findAll('[data-testid="list-row"]')).toHaveLength(1)
    expect(w.findAll('[data-testid="list-group"]')).toHaveLength(2)
  })
})

describe('keyboard', () => {
  it('walks rows with the arrows and shows the ring only then', async () => {
    const { w } = mountList()
    expect(w.find('.lr.cursor').exists()).toBe(false)
    press('ArrowDown', { key: 'ArrowDown' })
    await nextTick()
    const cur = w.find('.lr.cursor')
    expect(cur.attributes('data-task-id')).toBe('t1')
    expect(w.find('[data-testid="list-row-hint"]').exists()).toBe(true)
  })

  it('E completes the picked row; ⇧↓ grows the pick and E takes them all', async () => {
    const { actions } = mountList()
    press('ArrowDown', { key: 'ArrowDown' })
    press('KeyE', { key: 'e' })
    expect(actions.complete).toHaveBeenLastCalledWith([LISTS.c1[0]])
    press('ArrowDown', { key: 'ArrowDown', shiftKey: true })
    press('KeyE', { key: 'e' })
    expect(actions.complete.mock.calls.at(-1)[0].map((t) => t.id)).toEqual(['t1', 't2'])
  })

  it('⌘↓ re-orders inside the column, ⌘→ moves to the next lane', async () => {
    const { actions } = mountList()
    press('ArrowDown', { key: 'ArrowDown' })
    press('ArrowDown', { key: 'ArrowDown', ctrlKey: true })
    expect(actions.reorder).toHaveBeenCalledWith({
      task: LISTS.c1[0],
      laneKey: 'c1',
      beforeId: 't2',
      afterId: 't3',
    })
    press('ArrowRight', { key: 'ArrowRight', ctrlKey: true })
    expect(actions.shift.mock.calls[0][1]).toBe('right')
  })

  it('does not re-order where the list does not show the stored order', () => {
    const { actions } = mountList({ canReorder: false })
    press('ArrowDown', { key: 'ArrowDown' })
    press('ArrowDown', { key: 'ArrowDown', ctrlKey: true })
    expect(actions.reorder).not.toHaveBeenCalled()
  })

  it('← folds the group of the row, the cursor lands on its header', async () => {
    const { w } = mountList()
    press('ArrowDown', { key: 'ArrowDown' })
    press('ArrowLeft', { key: 'ArrowLeft' })
    expect(w.emitted('fold')[0][0].key).toBe('c1')
  })

  it('F2 renames in place and the board gets the new title', async () => {
    const { w, actions } = mountList()
    press('ArrowDown', { key: 'ArrowDown' })
    press('F2', { key: 'F2' })
    await nextTick()
    const input = w.find('[data-testid="list-row-rename"] input')
    expect(input.exists()).toBe(true)
    await input.setValue('Первая — новое имя')
    await input.trigger('keydown', { key: 'Enter' })
    await nextTick()
    expect(actions.rename).toHaveBeenCalledWith(LISTS.c1[0], 'Первая — новое имя')
  })

  it('stays silent in the archive', () => {
    const { actions } = mountList({ readonly: true })
    press('ArrowDown', { key: 'ArrowDown' })
    press('KeyE', { key: 'e' })
    expect(actions.complete).not.toHaveBeenCalled()
  })
})

describe('field columns', () => {
  it('a drag on the caption edge asks for the new width', async () => {
    const { w } = mountList()
    const grip = w.find('[data-testid="list-col-due"] .lv-grip')
    grip.element.dispatchEvent(new MouseEvent('pointerdown', { clientX: 100, bubbles: true }))
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 140 }))
    window.dispatchEvent(new MouseEvent('pointerup'))
    expect(w.emitted('resize')[0]).toEqual(['due', 132])
  })
})
