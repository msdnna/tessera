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
  {
    key: 'delete',
    label: 'Удалить проект',
    danger: true,
    confirm: { title: 'Точно удалить?', sub: 'без возврата' },
  },
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
  menu().dispatchEvent(
    new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...extra }),
  )

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

  // The view list (#2987): section headings are not rows, a row captioned with
  // one digit answers to it, and the current row / note / lock are drawn.
  it('lists views under headings and picks one by its digit', async () => {
    const items = [
      { heading: 'Мои' },
      {
        key: 'v:1',
        label: 'Мой спринт',
        glyph: 'layout-kanban',
        active: true,
        note: 'изменено',
        noteWarn: true,
        caps: ['1'],
      },
      { heading: 'Общие доски' },
      { key: 'v:2', label: 'Релизный Гант', glyph: 'layout-gantt', locked: true, caps: ['2'] },
    ]
    const w = await open(items)
    expect(menu().querySelectorAll('.am-section')).toHaveLength(2)
    expect(menu().querySelector('[data-testid="action-menu-v:1"]').classList).toContain('current')
    expect(menu().querySelector('.am-note.warn').textContent).toBe('изменено')
    expect(menu().querySelector('[data-testid="action-menu-v:2"] .am-lock')).not.toBeNull()
    key('2')
    expect(w.emitted('select')).toEqual([['v:2']])
    w.unmount()
  })
})

// Submenus (#3111): a row with children opens a nested list beside the menu —
// → / ↵ on the row, ← / Esc back; a pick inside emits the child's key and
// closes everything. On a phone the nested list is the sheet's second screen.
describe('ActionMenu submenu', () => {
  const NESTED = [
    { key: 'open', label: 'Открыть', keys: 'card.open' },
    {
      key: 'priority',
      label: 'Приоритет',
      children: [
        { key: 'p:0', label: 'Нет', caps: ['0'] },
        { key: 'p:1', label: 'Срочно', dot: '#e0533d', caps: ['1'], checked: true },
        { key: 'p:2', label: 'Высокий', caps: ['2'] },
      ],
    },
    { key: 'complete', label: 'Выполнить' },
    {
      key: 'move',
      label: 'Переместить',
      children: [
        { key: 'm:todo', label: 'К работе' },
        { key: 'm:gone', label: 'Удалить', danger: true, confirm: { title: 'Точно?' } },
      ],
    },
  ]
  const sub = () => document.body.querySelector('[data-testid="action-menu-sub"]')

  it('→ opens the submenu on its first row, ↵ picks and closes all', async () => {
    const w = await open(NESTED)
    const row = menu().querySelector('[data-testid="action-menu-priority"]')
    expect(row.getAttribute('aria-haspopup')).toBe('menu')
    expect(row.querySelector('.am-chev')).not.toBeNull()
    key('ArrowDown')
    key('ArrowRight')
    await nextTick()
    expect(sub()).not.toBeNull()
    expect(row.getAttribute('aria-expanded')).toBe('true')
    expect(sub().querySelector('[data-testid="action-menu-p:0"]').classList).toContain('on')
    expect(sub().querySelector('[data-testid="action-menu-p:1"] .am-check')).not.toBeNull()
    key('ArrowDown')
    key('Enter')
    expect(w.emitted('select')).toEqual([['p:1']])
    expect(w.emitted('update:show')).toEqual([[false]])
    w.unmount()
  })

  it('← and Esc close the submenu and leave the cursor on its row', async () => {
    const w = await open(NESTED)
    key('ArrowDown')
    key('Enter')
    await nextTick()
    expect(sub()).not.toBeNull()
    key('ArrowLeft')
    await nextTick()
    expect(sub()).toBeNull()
    expect(menu().querySelector('[data-testid="action-menu-priority"]').classList).toContain('on')
    key('Enter')
    await nextTick()
    key('Escape')
    await nextTick()
    expect(sub()).toBeNull()
    expect(w.emitted('update:show')).toBeUndefined()
    key('Escape')
    expect(w.emitted('update:show')).toEqual([[false]])
    w.unmount()
  })

  it('a digit inside the submenu picks its row', async () => {
    const w = await open(NESTED)
    menu().querySelector('[data-testid="action-menu-priority"]').click()
    await nextTick()
    key('2')
    expect(w.emitted('select')).toEqual([['p:2']])
    w.unmount()
  })

  it('hover opens after a moment; passing over another row does not close it', async () => {
    vi.useFakeTimers()
    const w = await open(NESTED)
    const enter = (testid) =>
      menu().querySelector(`[data-testid="${testid}"]`).dispatchEvent(new MouseEvent('mouseenter'))
    enter('action-menu-priority')
    await nextTick()
    expect(sub()).toBeNull()
    vi.advanceTimersByTime(200)
    await nextTick()
    expect(sub()).not.toBeNull()
    // Crossing «Выполнить» on the way, then landing in the submenu.
    enter('action-menu-complete')
    vi.advanceTimersByTime(100)
    sub().dispatchEvent(new MouseEvent('mouseenter'))
    vi.advanceTimersByTime(500)
    await nextTick()
    expect(sub()).not.toBeNull()
    // Resting on another row closes it.
    enter('action-menu-open')
    vi.advanceTimersByTime(300)
    await nextTick()
    expect(sub()).toBeNull()
    w.unmount()
  })

  it('a two-step row inside the submenu arms first', async () => {
    const w = await open(NESTED)
    menu().querySelector('[data-testid="action-menu-move"]').click()
    await nextTick()
    sub().querySelector('[data-testid="action-menu-m:gone"]').click()
    await nextTick()
    expect(w.emitted('select')).toBeUndefined()
    sub().querySelector('[data-testid="action-menu-m:gone-armed"]').click()
    expect(w.emitted('select')).toEqual([['m:gone']])
    w.unmount()
  })

  it('on a phone the submenu replaces the sheet, with «Назад»', async () => {
    const saved = window.matchMedia
    window.matchMedia = vi.fn().mockReturnValue({
      matches: true,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
    try {
      const w = await open(NESTED)
      await nextTick()
      expect(menu().classList).toContain('sheet')
      menu().querySelector('[data-testid="action-menu-priority"]').click()
      await nextTick()
      expect(sub()).toBeNull()
      expect(menu().querySelector('[data-testid="action-menu-open"]')).toBeNull()
      expect(menu().querySelector('[data-testid="action-menu-p:2"]')).not.toBeNull()
      menu().querySelector('[data-testid="action-menu-back"]').click()
      await nextTick()
      expect(menu().querySelector('[data-testid="action-menu-open"]')).not.toBeNull()
      w.unmount()
    } finally {
      window.matchMedia = saved
    }
  })
})
