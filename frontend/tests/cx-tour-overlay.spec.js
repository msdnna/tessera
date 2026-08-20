import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { nextTick } from 'vue'

const apiMock = vi.hoisted(() => ({
  acknowledgements: { ack: vi.fn(() => Promise.resolve()), list: vi.fn() },
}))
vi.mock('@/api', () => apiMock)

import TourOverlay from '@/components/TourOverlay.vue'
import { useTourStore } from '@/stores/tour'

// jsdom lays nothing out, so anchors get a hand-made box — the overlay treats a
// 0×0 element as "not there yet" (a collapsed drawer, a display:none section).
function anchor(key, box = { left: 100, top: 60, width: 40, height: 24 }) {
  const el = document.createElement('button')
  el.setAttribute('data-tour', key)
  el.getBoundingClientRect = () => ({
    ...box,
    right: box.left + box.width,
    bottom: box.top + box.height,
  })
  document.body.appendChild(el)
  return el
}

// A stand-in for an open naive picker (calendar/priority/tags popover): the
// overlay treats any visible .n-popover as a panel it must not dim or advance
// over (#2753 rework).
function panel(box = { left: 200, top: 200, width: 220, height: 260 }) {
  const el = document.createElement('div')
  el.className = 'n-popover'
  el.getBoundingClientRect = () => ({
    ...box,
    right: box.left + box.width,
    bottom: box.top + box.height,
  })
  document.body.appendChild(el)
  return el
}

const INFO = { id: 'workspaces', anchor: 'ws-switch', title: 'Пространства', body: 'Тут они' }
const ACTION = {
  id: 'open-menu',
  anchor: 'proj-add',
  title: 'Создать проект',
  body: 'Нажмите +',
  mode: 'action',
  advanceOn: { click: 'proj-add' },
}

let wrapper = null

async function render() {
  wrapper = mount(TourOverlay)
  await nextTick()
  await nextTick()
  return wrapper
}

const pop = () => document.querySelector('[data-testid="tour-pop"]')

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  setActivePinia(createPinia())
  document.body.innerHTML = ''
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
  localStorage.clear()
})

describe('TourOverlay', () => {
  it('renders nothing while no tour is running', async () => {
    await render()
    expect(pop()).toBe(null)
  })

  it('renders nothing until the anchor exists in the DOM', async () => {
    const tour = useTourStore()
    tour.start([INFO])
    await render()
    expect(pop()).toBe(null)
  })

  it('shows «Понятно» and «Пропустить» on an info step', async () => {
    anchor('ws-switch')
    const tour = useTourStore()
    tour.start([INFO])
    await render()

    expect(pop()).not.toBe(null)
    expect(pop().textContent).toContain('Пространства')
    expect(document.querySelector('[data-testid="tour-next"]')).not.toBe(null)
    expect(document.querySelector('[data-testid="tour-skip"]')).not.toBe(null)
  })

  it('shows only «Пропустить» on an action step — the user has to act', async () => {
    anchor('proj-add')
    const tour = useTourStore()
    tour.start([ACTION])
    await render()

    expect(document.querySelector('[data-testid="tour-next"]')).toBe(null)
    expect(document.querySelector('[data-testid="tour-skip"]')).not.toBe(null)
  })

  it('«Понятно» advances, «Пропустить» ends the guide', async () => {
    anchor('ws-switch')
    anchor('proj-add')
    const tour = useTourStore()
    tour.start([INFO, ACTION])
    await render()

    document.querySelector('[data-testid="tour-next"]').click()
    await nextTick()
    expect(tour.current.id).toBe('open-menu')

    document.querySelector('[data-testid="tour-skip"]').click()
    await nextTick()
    expect(tour.active).toBe(false)
    expect(pop()).toBe(null)
  })

  it('advances an action step when the user clicks the real anchor', async () => {
    anchor('proj-add')
    const tour = useTourStore()
    tour.start([ACTION, INFO])
    await render()

    document.querySelector('[data-tour="proj-add"]').click()
    await nextTick()
    expect(tour.current.id).toBe('workspaces')
  })

  it('cuts a hole in the mask around every anchor of the step', async () => {
    anchor('card-priority', { left: 10, top: 10, width: 30, height: 20 })
    anchor('card-due', { left: 60, top: 10, width: 30, height: 20 })
    const tour = useTourStore()
    tour.start([{ id: 'card', anchor: 'card-priority', extra: ['card-due'], title: 'Поля' }])
    await render()

    // A <mask> with one black hole-rect per anchor (overlap-safe, unlike an
    // evenodd path), over a single dimming rect.
    const holes = document.querySelectorAll('#tr-hole rect[fill="black"]')
    expect(holes).toHaveLength(2)
    expect(document.querySelector('.tr-dim')).not.toBe(null)
  })

  it('omits the mask when the step opts out', async () => {
    anchor('ws-switch')
    const tour = useTourStore()
    tour.start([{ ...INFO, mask: false }])
    await render()
    expect(document.querySelector('.tr-mask')).toBe(null)
    expect(pop()).not.toBe(null)
  })

  it('draws one arrow per anchor', async () => {
    anchor('card-priority', { left: 10, top: 10, width: 30, height: 20 })
    anchor('card-due', { left: 60, top: 10, width: 30, height: 20 })
    const tour = useTourStore()
    tour.start([{ id: 'card', anchor: 'card-priority', extra: ['card-due'], title: 'Поля' }])
    await render()
    expect(document.querySelectorAll('.tr-arrows .tr-path')).toHaveLength(2)
  })

  it('skips a step whose anchor never appears instead of hanging on it', async () => {
    vi.useFakeTimers()
    try {
      anchor('proj-add')
      const tour = useTourStore()
      // First step points at something that is not on this screen at all.
      tour.start([{ id: 'ghost', anchor: 'nowhere', title: 'Нет якоря' }, ACTION])
      await render()
      expect(pop()).toBe(null)

      await vi.advanceTimersByTimeAsync(9000)
      await nextTick()
      expect(tour.current.id).toBe('open-menu')
    } finally {
      vi.useRealTimers()
    }
  })

  it('advances a step when the entity it asked for appears', async () => {
    anchor('board-name')
    const tour = useTourStore()
    tour.start([
      {
        id: 'create-board',
        anchor: 'board-name',
        mode: 'action',
        advanceOn: { count: 'board-row' },
      },
      INFO,
    ])
    await render()
    expect(tour.current.id).toBe('create-board')

    anchor('board-row')
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    expect(tour.current.id).toBe('workspaces')
  })

  it('advances a set-step when the field starts carrying a value', async () => {
    // The overlay feeds advanceOn.set through the same counter as .count, so a
    // marker attribute appearing on the field is what ends the step (#2759).
    const el = anchor('tm-due')
    const tour = useTourStore()
    tour.start([
      {
        id: 'tm-due',
        anchor: 'tm-due',
        mode: 'action',
        advanceOn: { set: '[data-tour="tm-due"][data-tour-set]' },
      },
      INFO,
    ])
    anchor('ws-switch')
    await render()
    expect(tour.current.id).toBe('tm-due')

    el.setAttribute('data-tour-set', '')
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    expect(tour.current.id).toBe('workspaces')
  })

  it('holds a set-step until the open picker is closed', async () => {
    // #2753 rework: picking a date filled the field, so the guide jumped to the
    // next step while the calendar was still open. Now it waits for the picker
    // to close first.
    const el = anchor('tm-due')
    const p = panel()
    const tour = useTourStore()
    tour.start([
      {
        id: 'tm-due',
        anchor: 'tm-due',
        mode: 'action',
        advanceOn: { set: '[data-tour="tm-due"][data-tour-set]' },
      },
      INFO,
    ])
    anchor('ws-switch')
    await render()
    expect(tour.current.id).toBe('tm-due')

    // Field filled, but the picker is still open → stay put.
    el.setAttribute('data-tour-set', '')
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    expect(tour.current.id).toBe('tm-due')

    // Picker closed → advance.
    p.remove()
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    expect(tour.current.id).toBe('workspaces')
  })

  it('cuts the open picker panel out of the mask too', async () => {
    anchor('tm-due')
    panel()
    const tour = useTourStore()
    tour.start([{ id: 'tm-due', anchor: 'tm-due', title: 'Срок', mode: 'action' }])
    await render()
    // One hole for the field anchor, one for the open panel.
    expect(document.querySelectorAll('#tr-hole rect[fill="black"]')).toHaveLength(2)
  })

  it('flags the elements it points at, so hover-only buttons stay visible', async () => {
    const el = anchor('board-add')
    const tour = useTourStore()
    tour.start([{ id: 'add-board', anchor: 'board-add', title: 'Доска', mode: 'action' }])
    await render()
    expect(el.hasAttribute('data-tour-active')).toBe(true)

    await tour.skip()
    await nextTick()
    expect(el.hasAttribute('data-tour-active')).toBe(false)
  })

  it('picks up an anchor that appears later (dropdown, modal)', async () => {
    const tour = useTourStore()
    tour.start([{ id: 'late', anchor: 'menu-project', title: 'Проект' }])
    await render()
    expect(pop()).toBe(null)

    anchor('menu-project')
    // The MutationObserver re-resolves on the next frame.
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    await nextTick()
    expect(pop()).not.toBe(null)
  })
})
