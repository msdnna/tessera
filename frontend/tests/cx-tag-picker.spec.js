// The v2 tag picker (#3113, mockups TagPicker / PackageC): the keys and the
// rows. The picker only says what to do — `toggle`, `create`, `manage` — the
// modal and the card do it (their side is in cx-tag-create-existing-name).
//
// Pinned here:
//   • the task's tags first («Теги задачи», never filtered), then everything
//     else («Все теги», or «Найдено» once something is typed), best match first;
//   • «Создать тег «…»» only when no tag is called that;
//   • ↑↓ walk the rows, ↵ toggles the one under the cursor, Space toggles once
//     the arrows have picked a row (before that it types — names have spaces),
//     ⌘↵ asks the host to create, Esc closes the picker and nothing else;
//   • a tag under a GitLab rule prefix is never offered, and on the task it is
//     shown locked — a click doesn't take it off.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import TagPicker from '@/components/task/TagPicker.vue'

const TAGS = [
  { id: 'fe', name: 'app::frontend', color: '#7c5cff', task_count: 48 },
  { id: 'des', name: 'design', color: '#eb2f96', task_count: 17 },
  { id: 'fl', name: 'frontend-legacy', color: '#9aa0aa', task_count: 3 },
  { id: 'bk', name: 'backend', color: '#2f80ed', task_count: 9 },
  { id: 'st', name: 'status::done', color: '#18a058' },
]

let w = null
async function open(props = {}) {
  w = mount(TagPicker, {
    props: {
      show: true,
      anchor: () => document.body,
      tags: TAGS,
      selected: ['fe', 'des'],
      lockedPrefixes: new Set(['status::']),
      create: vi.fn(() => Promise.resolve(true)),
      ...props,
    },
    attachTo: document.body,
  })
  for (let i = 0; i < 4; i++) await w.vm.$nextTick()
  return w
}
const input = () => document.querySelector('[data-testid="tag-picker-search"]')
const rowsText = () =>
  [...document.querySelectorAll('[data-testid^="tag-picker-row-"]')].map((r) =>
    r.getAttribute('data-testid').replace('tag-picker-row-', ''),
  )
const heads = () => [...document.querySelectorAll('.tp-head-t')].map((n) => n.textContent.trim())
async function type(text) {
  input().value = text
  input().dispatchEvent(new Event('input', { bubbles: true }))
  await w.vm.$nextTick()
  await w.vm.$nextTick()
}
async function key(k, extra = {}) {
  const e = new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...extra })
  input().dispatchEvent(e)
  await w.vm.$nextTick()
  await w.vm.$nextTick()
  return e
}

beforeEach(() => {
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})
afterEach(() => {
  w?.unmount()
  w = null
  document.body.innerHTML = ''
})

describe('TagPicker — rows', () => {
  it('the task’s tags first, then the rest; GitLab-rule tags are never offered', async () => {
    await open()
    expect(heads()).toEqual(['Теги задачи', 'Все теги'])
    expect(rowsText()).toEqual(['fe', 'des', 'bk', 'fl'])
    expect(document.activeElement).toBe(input())
  })

  it('typing filters the rest (not the task’s own), best match first', async () => {
    await open()
    await type('front')
    expect(heads()).toEqual(['Теги задачи', 'Найдено'])
    expect(rowsText()).toEqual(['fe', 'des', 'fl'])
  })

  it('«Создать тег» shows only when no tag is called that', async () => {
    await open()
    await type('backend')
    expect(document.querySelector('[data-testid="tag-picker-create"]')).toBeNull()
    await type('дизайн-система')
    expect(document.querySelector('[data-testid="tag-picker-create"]').textContent).toContain(
      'дизайн-система',
    )
  })

  it('a locked tag on the task stays on: no toggle on click', async () => {
    await open({ selected: ['st', 'des'] })
    const row = document.querySelector('[data-testid="tag-picker-row-st"]')
    expect(row.classList.contains('locked')).toBe(true)
    row.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(w.emitted('toggle')).toBeUndefined()
  })
})

describe('TagPicker — keys', () => {
  it('↵ toggles the best match of what was typed', async () => {
    await open()
    await type('back')
    await key('Enter')
    expect(w.emitted('toggle')).toEqual([['bk']])
  })

  it('↵ on a name only the task’s own tags answer to never takes it off', async () => {
    const create = vi.fn(() => Promise.resolve(true))
    await open({ create })
    await type('design')
    await key('Enter')
    expect(w.emitted('toggle')).toBeUndefined()
    expect(create).toHaveBeenCalledWith('design')
  })

  it('↵ on «Создать» creates, ⌘↵ creates from anywhere', async () => {
    const create = vi.fn(() => Promise.resolve(true))
    await open({ create })
    await type('новый тег')
    await key('Enter')
    expect(create).toHaveBeenCalledWith('новый тег')
    await type('ещё один')
    await key('Enter', { ctrlKey: true })
    expect(create).toHaveBeenLastCalledWith('ещё один')
  })

  it('Space types until the arrows pick a row, then toggles it', async () => {
    await open()
    const typed = await key(' ')
    expect(typed.defaultPrevented).toBe(false)
    expect(w.emitted('toggle')).toBeUndefined()
    await key('ArrowDown') // fe → des
    const sp = await key(' ')
    expect(sp.defaultPrevented).toBe(true)
    expect(w.emitted('toggle')).toEqual([['des']])
  })

  it('↓ walks past «Создать» to «Управлять тегами…», ↵ there hands over the query', async () => {
    await open()
    await type('fr')
    // rows: fe, des (the task's), fl (found, under the cursor), create, manage
    await key('ArrowDown')
    await key('ArrowDown')
    await key('Enter')
    expect(w.emitted('manage')).toEqual([['fr']])
    expect(w.emitted('update:show')).toEqual([[false]])
  })

  it('↑ from the top wraps to the last row', async () => {
    await open()
    await key('ArrowUp')
    await key('Enter')
    // The last row is «Управлять тегами…».
    expect(w.emitted('manage')).toEqual([['']])
  })

  it('Esc closes the picker and stops there', async () => {
    await open()
    const outer = vi.fn()
    document.addEventListener('keydown', outer)
    await key('Escape')
    document.removeEventListener('keydown', outer)
    expect(w.emitted('update:show')).toEqual([[false]])
    expect(outer).not.toHaveBeenCalled()
  })
})
