// The column header's menu and rename (v2 interaction model, #2969).
//
// The delete is no longer the header's to perform: it happens at once and the
// board offers the undo (KanbanBoard knows where the cards go and shows the
// toast), so the header only emits `delete`. The board's last column can't be
// deleted — its cards would have nowhere to move. Renaming happens in place.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const remove = vi.fn()
const update = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({ columns: { remove: (...a) => remove(...a), update: (...a) => update(...a) } }))

const { default: ColumnHeader } = await import('@/components/ColumnHeader.vue')

const DCOL = { key: 'c1', name: 'К работе', rawName: 'К работе' }
const mountHead = (props = {}) =>
  mount(ColumnHeader, {
    props: { dcol: DCOL, editable: true, count: 3, ...props },
    global: { stubs: { TesseraIcon: true } },
    attachTo: document.body,
  })
const row = (key) => document.body.querySelector(`[data-testid="column-actions-${key}"]`)

describe('ColumnHeader menu', () => {
  beforeEach(() => {
    remove.mockClear()
    update.mockClear()
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('a right click opens the menu; «Удалить» only asks the board', async () => {
    const w = mountHead()
    await w.get('.col-head').trigger('contextmenu', { clientX: 10, clientY: 10 })
    await nextTick()
    row('delete').click()
    expect(w.emitted('delete')).toHaveLength(1)
    expect(remove).not.toHaveBeenCalled()
    w.unmount()
  })

  it('the last column offers the delete greyed out, with the reason', async () => {
    const w = mountHead({ canDelete: false })
    await w.get('.col-menu').trigger('click')
    await nextTick()
    expect(row('delete').getAttribute('aria-disabled')).toBe('true')
    expect(row('delete').getAttribute('title')).toContain('Последнюю колонку')
    row('delete').click()
    expect(w.emitted('delete')).toBeUndefined()
    w.unmount()
  })

  it('the colour row and the done switch report their values', async () => {
    const w = mountHead({ isDone: false })
    await w.get('.col-menu').trigger('click')
    await nextTick()
    row('done').click()
    expect(w.emitted('set-done')).toEqual([['c1']])
    w.unmount()
  })
})

describe('ColumnHeader rename', () => {
  beforeEach(() => update.mockClear())
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('double click renames in place; ↵ saves', async () => {
    const w = mountHead()
    await w.get('.col-title').trigger('dblclick')
    const input = w.get('input')
    await input.setValue('В работе')
    await input.trigger('keydown', { key: 'Enter' })
    await nextTick()
    expect(update).toHaveBeenCalledWith('c1', { name: 'В работе', color: '' })
    w.unmount()
  })

  it('an empty name is refused on ↵ and dropped on click-away', async () => {
    const w = mountHead()
    await w.get('.col-title').trigger('dblclick')
    const input = w.get('input')
    await input.setValue('  ')
    await input.trigger('keydown', { key: 'Enter' })
    await nextTick()
    expect(w.text()).toContain('Название не может быть пустым')
    expect(w.find('input').exists()).toBe(true)
    await w.get('input').setValue('')
    await w.get('input').trigger('blur')
    await nextTick()
    expect(w.find('input').exists()).toBe(false)
    expect(update).not.toHaveBeenCalled()
    w.unmount()
  })

  it('Tab saves and hands the rename to the next column', async () => {
    const w = mountHead()
    await w.get('.col-title').trigger('dblclick')
    await w.get('input').setValue('Бэклог')
    await w.get('input').trigger('keydown', { key: 'Tab' })
    await nextTick()
    await nextTick()
    expect(w.emitted('rename-next')).toHaveLength(1)
    w.unmount()
  })

  it('opens when the board asks (F2, Tab from the previous header)', async () => {
    const w = mountHead()
    await w.setProps({ renameRequest: { key: 'c1', n: 1 } })
    expect(w.find('input').exists()).toBe(true)
    w.unmount()
  })
})
