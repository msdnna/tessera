// ActionMenu — the v2 row menu (#2969): keys per row, and the two-step danger
// item that arms on the first press and fires on the second.
import { describe, it, expect, afterEach, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import ActionMenu from '@/components/kit/ActionMenu.vue'

const ITEMS = [
  { key: 'rename', label: 'Переименовать', keys: 'row.rename' },
  { key: 'soon', label: 'Архивировать', stub: 3005 },
  { gap: true },
  { key: 'delete', label: 'Удалить проект', danger: true, confirm: { title: 'Точно удалить?', sub: 'без возврата' } },
]

async function open(items = ITEMS) {
  const w = mount(ActionMenu, {
    props: { show: false, items, anchor: () => ({ x: 10, y: 10 }) },
    attachTo: document.body,
  })
  await w.setProps({ show: true })
  await nextTick()
  return w
}
const menu = () => document.body.querySelector('[data-testid="action-menu"]')
const key = (k, extra = {}) =>
  menu().dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...extra }))

afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('ActionMenu', () => {
  it('the danger item needs a second press', async () => {
    const w = await open()
    menu().querySelector('[data-testid="action-menu-delete"]').click()
    await nextTick()
    expect(w.emitted('select')).toBeUndefined()
    expect(menu().querySelector('[data-testid="action-menu-delete-armed"]')).not.toBeNull()
    key('Enter')
    expect(w.emitted('select')).toEqual([['delete']])
    w.unmount()
  })

  it('disarms after four seconds of nothing', async () => {
    vi.useFakeTimers()
    const w = await open()
    menu().querySelector('[data-testid="action-menu-delete"]').click()
    await nextTick()
    vi.advanceTimersByTime(4100)
    await nextTick()
    expect(menu().querySelector('[data-testid="action-menu-delete-armed"]')).toBeNull()
    w.unmount()
  })

  it('a row answers to its own key inside the menu', async () => {
    const w = await open()
    key('F2', { code: 'F2' })
    expect(w.emitted('select')).toEqual([['rename']])
    w.unmount()
  })

  it('a stub row is shown but does nothing', async () => {
    const w = await open()
    const row = menu().querySelector('[data-testid="action-menu-soon"]')
    expect(row.getAttribute('title')).toContain('3005')
    row.click()
    expect(w.emitted('select')).toBeUndefined()
    w.unmount()
  })

  it('Esc disarms first, then closes', async () => {
    const w = await open()
    menu().querySelector('[data-testid="action-menu-delete"]').click()
    await nextTick()
    key('Escape')
    await nextTick()
    expect(w.emitted('update:show')).toBeUndefined()
    key('Escape')
    expect(w.emitted('update:show')).toEqual([[false]])
    w.unmount()
  })
})
