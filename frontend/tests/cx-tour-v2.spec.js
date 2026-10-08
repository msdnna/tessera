import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { nextTick } from 'vue'

// The v2 surfaces of the tour (#3000): the checklist on «Мои задачи», the
// spotlight card and Settings → «Подсказки».

vi.hoisted(() => {
  globalThis.__APP_VERSION__ = '1.0.0'
})
const apiMock = vi.hoisted(() => ({
  acknowledgements: {
    ack: vi.fn(() => Promise.resolve()),
    list: vi.fn(() => Promise.resolve({ data: [] })),
    forget: vi.fn(() => Promise.resolve({ data: { deleted: 1 } })),
  },
}))
vi.mock('@/api', () => apiMock)
const accountMock = vi.hoisted(() => ({ isBrandNewAccount: vi.fn(() => false) }))
vi.mock('@/utils/account', () => accountMock)
vi.mock('@/data/whatsNew', () => ({ WHATS_NEW: [] }))

import TourChecklist from '@/components/tour/TourChecklist.vue'
import SpotlightHint from '@/components/tour/SpotlightHint.vue'
import SettingsHints from '@/components/settings/SettingsHints.vue'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useTourStore } from '@/stores/tour'
import { useHintsStore } from '@/stores/hints'

async function withAcks(keys) {
  apiMock.acknowledgements.list.mockResolvedValue({
    data: keys.map((key) => ({ key, ack_at: '2026-10-06T10:00:00Z' })),
  })
  await useWhatsNewStore().load()
}
const acked = () => apiMock.acknowledgements.ack.mock.calls.map((c) => c[0])
// An element with a real-looking box (jsdom lays nothing out).
function el(attrs, box = { left: 900, top: 10, width: 40, height: 30 }) {
  const e = document.createElement('button')
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v)
  e.getBoundingClientRect = () => ({
    ...box,
    right: box.left + box.width,
    bottom: box.top + box.height,
  })
  document.body.appendChild(e)
  return e
}

let wrapper = null
beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  vi.clearAllMocks()
  setActivePinia(createPinia())
  useTourStore().doneMs = 0
  document.body.innerHTML = ''
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
  vi.useRealTimers()
})

describe('TourChecklist', () => {
  it('lists «Основы» as results, the next one with «Продолжить»', async () => {
    await withAcks(['getstarted:item:basics.project', 'getstarted:item:basics.board:self'])
    wrapper = mount(TourChecklist)
    await nextTick()
    expect(wrapper.attributes('data-state')).toBe('run')
    const rows = wrapper.findAll('[data-testid="tour-checklist-item"]')
    expect(rows).toHaveLength(6)
    expect(rows[0].attributes('data-done')).toBe('tour')
    expect(rows[1].attributes('data-done')).toBe('self')
    expect(rows[1].text()).toContain('сделано без тура')
    expect(rows[2].classes()).toContain('cur')
    expect(wrapper.text()).toContain('Основы · 2 из 6')
  })

  it('starts the chapter from the step that counts a row', async () => {
    await withAcks([])
    wrapper = mount(TourChecklist)
    await wrapper.find('[data-item="basics.task"]').trigger('click')
    await Promise.resolve()
    const tour = useTourStore()
    expect(tour.chapter).toBe('basics')
    expect(tour.current.id).toBe('task-create')
  })

  it('starts «Создать проект» at «+», not inside a form that isn’t open', async () => {
    await withAcks([])
    wrapper = mount(TourChecklist)
    await wrapper.find('[data-item="basics.project"]').trigger('click')
    await Promise.resolve()
    expect(useTourStore().current.id).toBe('tree-add')
  })

  it('starts an optional chapter, but not a locked one', async () => {
    await withAcks([])
    wrapper = mount(TourChecklist)
    await wrapper.find('[data-chapter="gitlab"]').trigger('click')
    await Promise.resolve()
    expect(useTourStore().active).toBe(false)
    await wrapper.find('[data-chapter="inbox"]').trigger('click')
    await Promise.resolve()
    expect(useTourStore().chapter).toBe('inbox')
  })

  it('folds by default once «Основы» are behind, and remembers the choice', async () => {
    await withAcks(['getstarted:done'])
    wrapper = mount(TourChecklist)
    expect(wrapper.attributes('data-state')).toBe('min')
    await wrapper.find('[data-testid="tour-checklist-fold"]').trigger('click')
    expect(wrapper.attributes('data-state')).toBe('run')
    expect(localStorage.getItem('tessera_checklist_min')).toBe('0')
  })

  it('turns green when every chapter is passed', async () => {
    const all = ['basics', 'task', 'inbox', 'stages', 'keys', 'tags', 'projects']
    await withAcks(all.map((c) => `getstarted:ch:${c}`))
    wrapper = mount(TourChecklist)
    expect(wrapper.attributes('data-state')).toBe('done')
    await wrapper.find('[data-testid="tour-checklist-hide"]').trigger('click')
    expect(acked()).toContain('getstarted:checklist:hidden')
  })
})

describe('SpotlightHint', () => {
  it('shows the first eligible hint whose element is on screen, skipping missing ones', async () => {
    vi.useFakeTimers()
    await withAcks(['whatschanged:v2', 'whatschanged:later'])
    // «inbox» is first in the queue, but its button isn't on this screen:
    // the next one is shown instead of the queue stalling (#3000).
    el({ 'data-testid': 'quick-fab-button' })
    wrapper = mount(SpotlightHint)
    await vi.advanceTimersByTimeAsync(800)
    const card = document.querySelector('[data-testid="spotlight-card"]')
    expect(card?.dataset.hint).toBe('capture')
    expect(card.textContent).toContain('Новое в v2')
  })

  it('«Понятно» retires the hint; a click on the target too', async () => {
    vi.useFakeTimers()
    await withAcks(['whatschanged:v2', 'whatschanged:later'])
    const fab = el({ 'data-testid': 'quick-fab-button' })
    el({ 'data-testid': 'board-customize-btn' }, { left: 500, top: 60, width: 30, height: 30 })
    wrapper = mount(SpotlightHint)
    await vi.advanceTimersByTimeAsync(800)
    document.querySelector('[data-testid="spotlight-ok"]').click()
    await nextTick()
    expect(acked()).toContain('spotlight:capture')
    await vi.advanceTimersByTimeAsync(800)
    expect(document.querySelector('[data-testid="spotlight-card"]')?.dataset.hint).toBe(
      'view-options',
    )
    document.querySelector('[data-testid="board-customize-btn"]').click()
    await nextTick()
    expect(acked()).toContain('spotlight:view-options')
    expect(fab).toBeTruthy()
  })

  it('stays away while the tour runs or a window is open', async () => {
    vi.useFakeTimers()
    await withAcks(['whatschanged:v2', 'whatschanged:later'])
    el({ 'data-testid': 'quick-fab-button' })
    useTourStore().start('keys')
    wrapper = mount(SpotlightHint)
    await vi.advanceTimersByTimeAsync(800)
    expect(document.querySelector('[data-testid="spotlight-card"]')).toBe(null)
    useTourStore().stop()
    const modal = document.createElement('div')
    modal.setAttribute('role', 'dialog')
    modal.setAttribute('aria-modal', 'true')
    document.body.appendChild(modal)
    await vi.advanceTimersByTimeAsync(800)
    expect(document.querySelector('[data-testid="spotlight-card"]')).toBe(null)
    modal.remove()
    await vi.advanceTimersByTimeAsync(800)
    expect(document.querySelector('[data-testid="spotlight-card"]')).not.toBe(null)
  })

  it('shows no more than two hints a session', async () => {
    vi.useFakeTimers()
    await withAcks(['whatschanged:v2', 'whatschanged:later'])
    const hints = useHintsStore()
    hints.shown({ key: 'a' })
    hints.shown({ key: 'b' })
    el({ 'data-testid': 'quick-fab-button' })
    wrapper = mount(SpotlightHint)
    await vi.advanceTimersByTimeAsync(800)
    expect(document.querySelector('[data-testid="spotlight-card"]')).toBe(null)
  })
})

describe('SettingsHints', () => {
  it('lists every hint with its state and brings a shown one back', async () => {
    await withAcks(['spotlight:inbox', 'whatschanged:v2', 'whatschanged:later'])
    wrapper = mount(SettingsHints)
    const rows = wrapper.findAll('[data-testid="settings-hints-row"]')
    expect(rows.length).toBe(10)
    const inbox = wrapper.find('[data-hint="inbox"]')
    expect(inbox.attributes('data-state')).toBe('shown')
    expect(inbox.text()).toContain('показана')
    expect(wrapper.find('[data-hint="stage-strip"]').attributes('data-state')).toBe('wait')
    await inbox.find('[data-testid="settings-hints-again"]').trigger('click')
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ key: 'spotlight:inbox' })
  })

  it('«Показать подсказки заново» and «Начать с начала»', async () => {
    await withAcks(['getstarted:done'])
    wrapper = mount(SettingsHints)
    await wrapper.find('[data-testid="settings-hints-reset"]').trigger('click')
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ prefix: 'spotlight:' })
    await wrapper.find('[data-testid="settings-hints-restart"]').trigger('click')
    await Promise.resolve()
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ prefix: 'getstarted:' })
    expect(useTourStore().chapter).toBe('basics')
  })
})
