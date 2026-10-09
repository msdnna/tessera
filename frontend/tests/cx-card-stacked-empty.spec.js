// Stack mode («Поля в столбик») draws every visible field as its own full-width
// row even when it is unset — that empty row IS the field's only trigger on the
// card (see the comment above the pills in TaskCardPills). The row-level guard
// `hasAnyPill` decides whether the whole block renders, so it has to agree with
// the per-pill conditions: it used to ignore `stackFields` and a card with
// nothing set at all («Заполненные поля» off) lost the block entirely — priority,
// срок, теги and исполнители became unreachable from the board on exactly the
// cards that need them, while a card with a single tag showed all six rows.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

vi.mock('@/api', () => ({
  tasks: {
    update: vi.fn(() => Promise.resolve({ data: {} })),
    move: vi.fn(() => Promise.resolve({ data: {} })),
    setParent: vi.fn(() => Promise.resolve({ data: {} })),
    remove: vi.fn(),
    archive: vi.fn(),
    description: vi.fn(() => Promise.resolve({ data: { description: '' } })),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    addAssignee: vi.fn(),
    removeAssignee: vi.fn(),
    pinGitlabAssignee: vi.fn(),
    removeGitlabAssignee: vi.fn(),
    dueNotify: vi.fn(),
  },
  boards: { createTask: vi.fn(() => Promise.resolve({ data: {} })) },
  projects: { createTag: vi.fn() },
  users: { updatePrefs: vi.fn() },
  columns: { update: vi.fn(), remove: vi.fn() },
}))

const COLS = [{ id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 }]
// A card with nothing pillable on it: no priority, no due date, no tags, no
// assignees, no estimate, no milestone, no description.
const BARE = { id: 't1', board_id: 'b1', column_id: 'c1', title: 'Пустая', number: 11 }
const WITH_PRIORITY = { ...BARE, priority: 2 }

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  RichContent: true,
  DuePicker: true,
  draggable: {
    template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>',
    props: ['list'],
  },
}

const mountCard = async (task, seed) => {
  seedBoardStore({ columns: COLS, cardSize: 'large', ...seed })
  const TaskCard = (await import('@/components/TaskCard.vue')).default
  const w = mount(TaskCard, { props: { task }, global: { stubs } })
  await w.vm.$nextTick()
  return w
}

describe('stack mode on a card with nothing set', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
  })

  it('keeps the pill block — empty rows are the only triggers there', async () => {
    const w = await mountCard(BARE, { stackFields: true, showEmpty: false })
    expect(w.find('.pills').exists()).toBe(true)
    expect(w.find('.pills').classes()).toContain('stacked')
  })

  it('draws the unset rows, so every field stays clickable', async () => {
    const w = await mountCard(BARE, { stackFields: true, showEmpty: false })
    // priority / срок / оценка / веха / теги render an «—» placeholder in stack
    // mode; the people row carries its own add-icon instead.
    expect(w.findAll('.sf-empty').length).toBeGreaterThanOrEqual(4)
    expect(w.find('.assignee-pill').exists()).toBe(true)
  })

  it('shows the same rows whether or not one field happens to be set', async () => {
    const empty = await mountCard(BARE, { stackFields: true, showEmpty: false })
    const filled = await mountCard(WITH_PRIORITY, { stackFields: true, showEmpty: false })
    expect(empty.findAll('.pill').length).toBe(filled.findAll('.pill').length)
  })

  it('still drops the block without stack mode (nothing to show, nothing drawn)', async () => {
    const w = await mountCard(BARE, { stackFields: false, showEmpty: false })
    expect(w.find('.pills').exists()).toBe(false)
  })

  it('leaves the block alone when the placeholders are wanted anyway', async () => {
    const w = await mountCard(BARE, { stackFields: false, showEmpty: true })
    expect(w.find('.pills').exists()).toBe(true)
  })
})
