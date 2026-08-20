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
import { sidebarDragging } from '@/composables/useSidebarDnd'
import { boardDragging } from '@/composables/useBoardDragScroll'

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

// A stand-in for an overlay surface the user opened mid-tour (a modal/drawer):
// while one is on screen the dimming mask is switched off entirely (#2753).
function surface(cls = 'n-modal', box = { left: 400, top: 200, width: 400, height: 300 }) {
  const el = document.createElement('div')
  el.className = cls
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
  // Module-level flags — a test that leaves one set would fade the next one out.
  sidebarDragging.value = false
  boardDragging.value = false
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

  it('cuts around the group the guide created, not the first one in the tree', async () => {
    // #2778 rework: `cut` went to the DOM with its {group} token unexpanded-ish
    // — it never went through resolve() at all — so a workspace that already had
    // groups got the bright hole (and the arrow's clearance) on someone else's.
    const groupNode = (id, box) => {
      const node = document.createElement('div')
      node.setAttribute('data-tour-group', id)
      const row = document.createElement('div')
      row.setAttribute('data-tour', 'group-row')
      row.getBoundingClientRect = () => ({
        ...box,
        right: box.left + box.width,
        bottom: box.top + box.height,
      })
      node.appendChild(row)
      document.body.appendChild(node)
      return row
    }
    groupNode('g-old', { left: 10, top: 10, width: 120, height: 24 })
    groupNode('g-new', { left: 10, top: 300, width: 120, height: 24 })
    anchor('project-row', { left: 10, top: 500, width: 120, height: 24 })

    const tour = useTourStore()
    tour.start([
      {
        id: 'dnd-project',
        anchor: 'project-row',
        cut: ['[data-tour-group="{group}"] [data-tour="group-row"]'],
        title: 'Перетащите проект',
        mode: 'action',
      },
    ])
    tour.noteCreated({ groupId: 'g-new' })
    await render()

    // One hole for the step's own anchor, one for the cut — and the cut sits on
    // the created group's row (y 300), not on the pre-existing one (y 10).
    const holes = [...document.querySelectorAll('#tr-hole rect[fill="black"]')]
    expect(holes).toHaveLength(2)
    expect(holes.map((h) => h.getAttribute('y'))).toContain('294')
    expect(holes.map((h) => h.getAttribute('y'))).not.toContain('4')
  })

  it('omits the mask when the step opts out', async () => {
    anchor('ws-switch')
    const tour = useTourStore()
    tour.start([{ ...INFO, mask: false }])
    await render()
    expect(document.querySelector('.tr-mask')).toBe(null)
    expect(pop()).not.toBe(null)
  })

  it('switches the mask off while an off-script modal/drawer is open (no double-dim)', async () => {
    // The surface holds none of the step's anchors → the user opened it off the
    // tour (create-workspace), so the tour must not dim it.
    anchor('ws-switch')
    surface('n-modal') // default box is far from the anchor
    const tour = useTourStore()
    tour.start([INFO])
    await render()
    // Popover/arrows still show, but the page is not dimmed by the tour.
    expect(pop()).not.toBe(null)
    expect(document.querySelector('.tr-mask')).toBe(null)
  })

  it('keeps the mask (with the row highlighted) for a guided modal', async () => {
    // The surface *contains* the step's anchor (the project/task modal the step
    // points into) → dim it, but cut a hole around the action row.
    anchor('project-name', { left: 100, top: 100, width: 200, height: 30 })
    surface('n-modal', { left: 40, top: 40, width: 500, height: 400 }) // wraps the field
    const tour = useTourStore()
    tour.start([
      { id: 'project-create', anchor: 'project-name', title: 'Назовите', mode: 'action' },
    ])
    await render()
    expect(document.querySelector('.tr-mask')).not.toBe(null)
    // one hole for the field row
    expect(document.querySelectorAll('#tr-hole rect[fill="black"]')).toHaveLength(1)
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

  it('advances a moved-step once the tracked element lands in another container', async () => {
    // #2778: the overlay reports where the card currently lives (the column's
    // data-column-name), the store compares it with the address on step entry.
    const from = document.createElement('div')
    from.setAttribute('data-column-name', 'К работе')
    const to = document.createElement('div')
    to.setAttribute('data-column-name', 'В процессе')
    document.body.append(from, to)
    const card = document.createElement('div')
    card.setAttribute('data-testid', 'task-card')
    from.appendChild(card)
    anchor('board-composer') // something for the arrow to point at

    const tour = useTourStore()
    tour.start([
      {
        id: 'dnd-card',
        anchor: 'board-composer',
        mode: 'action',
        advanceOn: {
          moved: {
            el: '[data-testid="task-card"]',
            within: '[data-column-name]',
            by: 'data-column-name',
          },
        },
      },
      INFO,
    ])
    anchor('ws-switch')
    await render()
    expect(tour.current.id).toBe('dnd-card')

    // Mid-drag: SortableJS has the node out of any list for a frame.
    card.remove()
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    expect(tour.current.id).toBe('dnd-card')

    to.appendChild(card)
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    expect(tour.current.id).toBe('workspaces')
  })

  it('survives the board re-rendering the card it tracks after the drop', async () => {
    // The board reloads and rebuilds its card list right after a drop, so the
    // tracked node is replaced by a different element in the target column. The
    // address is read off the container's attribute, not from a node reference.
    const from = document.createElement('div')
    from.setAttribute('data-column-name', 'К работе')
    const to = document.createElement('div')
    to.setAttribute('data-column-name', 'В процессе')
    document.body.append(from, to)
    const card = document.createElement('div')
    card.setAttribute('data-testid', 'task-card')
    from.appendChild(card)
    anchor('board-composer')

    const tour = useTourStore()
    tour.start([
      {
        id: 'dnd-card',
        anchor: 'board-composer',
        mode: 'action',
        advanceOn: {
          moved: {
            el: '[data-testid="task-card"]',
            within: '[data-column-name]',
            by: 'data-column-name',
          },
        },
      },
      INFO,
    ])
    anchor('ws-switch')
    await render()

    card.remove()
    const rebuilt = document.createElement('div')
    rebuilt.setAttribute('data-testid', 'task-card')
    to.appendChild(rebuilt)
    await new Promise((r) => requestAnimationFrame(r))
    await nextTick()
    expect(tour.current.id).toBe('workspaces')
  })

  it('fades out while a drag is in progress, on either surface', async () => {
    // #2778: the layer never blocked the pointer, but an arrow over the drag
    // ghost and a mask shading the target both read as "the guide is in the way".
    anchor('ws-switch')
    const tour = useTourStore()
    tour.start([INFO])
    await render()
    const layer = () => document.querySelector('.tr-layer')
    expect(layer().classList.contains('dragging')).toBe(false)

    boardDragging.value = true
    await nextTick()
    expect(layer().classList.contains('dragging')).toBe(true)

    boardDragging.value = false
    sidebarDragging.value = true
    await nextTick()
    expect(layer().classList.contains('dragging')).toBe(true)

    sidebarDragging.value = false
    await nextTick()
    expect(layer().classList.contains('dragging')).toBe(false)
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
