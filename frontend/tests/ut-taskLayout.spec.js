// Task presentation modes (#2716). The rule that matters is the narrow-screen
// override: a 560px side panel on a phone is a fullscreen sheet with worse
// ergonomics, so the saved preference is only honoured when it can mean something.
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  TASK_LAYOUTS,
  dismissesSidebar,
  effectiveTaskLayout,
  loadTaskLayout,
  saveTaskLayout,
} from '@/utils/taskLayout'

describe('effectiveTaskLayout', () => {
  it('passes through the three known layouts on a wide screen', () => {
    for (const l of TASK_LAYOUTS) expect(effectiveTaskLayout(l, false)).toBe(l)
  })

  it('falls back to the modal for unknown or missing values', () => {
    expect(effectiveTaskLayout('drawer', false)).toBe('modal')
    expect(effectiveTaskLayout(null, false)).toBe('modal')
    expect(effectiveTaskLayout(undefined, false)).toBe('modal')
  })

  it('overrides the saved layout on narrow screens', () => {
    expect(effectiveTaskLayout('sidebar', true)).toBe('modal')
    expect(effectiveTaskLayout('fullscreen', true)).toBe('modal')
  })
})

describe('taskLayout persistence', () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => vi.restoreAllMocks())

  it('round-trips a saved layout', () => {
    saveTaskLayout('sidebar')
    expect(loadTaskLayout()).toBe('sidebar')
  })

  it('defaults to the modal when nothing is stored', () => {
    expect(loadTaskLayout()).toBe('modal')
  })

  it('ignores a junk value already in storage', () => {
    localStorage.setItem('tessera_task_layout', 'drawer')
    expect(loadTaskLayout()).toBe('modal')
  })

  it('refuses to persist an unknown layout', () => {
    saveTaskLayout('sidebar')
    saveTaskLayout('drawer')
    expect(loadTaskLayout()).toBe('sidebar')
  })

  it('survives disabled storage instead of throwing', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    expect(loadTaskLayout()).toBe('modal')
    expect(() => saveTaskLayout('sidebar')).not.toThrow()
  })
})

// The panel has no mask, so nothing dismisses it for free — this predicate is the
// whole dismissal policy, and the interesting half is what it must NOT close on.
describe('dismissesSidebar', () => {
  let card = null
  beforeEach(() => {
    document.body.innerHTML = ''
    card = document.createElement('div')
    card.className = 'tm-card tm-sidebar'
    card.innerHTML = '<button id="inside">x</button>'
    document.body.appendChild(card)
  })

  const add = (html) => {
    const host = document.createElement('div')
    host.innerHTML = html
    document.body.appendChild(host)
    return host.firstElementChild
  }

  it('dismisses on a click that lands on empty space', () => {
    expect(dismissesSidebar(add('<div id="board-bg"></div>'), card)).toBe(true)
  })

  it('keeps the panel when the click is inside it', () => {
    expect(dismissesSidebar(card.querySelector('#inside'), card)).toBe(false)
    expect(dismissesSidebar(card, card)).toBe(false)
  })

  it('keeps the panel when another task card is clicked — that re-points it', () => {
    const inner = add('<div data-testid="task-card"><span id="t">Заголовок</span></div>')
    expect(dismissesSidebar(inner.querySelector('#t'), card)).toBe(false)
  })

  it.each([
    ['a popover / dropdown / date picker', 'v-binder-follower-container'],
    ['a nested modal', 'n-modal-container'],
    ['a drawer', 'n-drawer-container'],
    ['a message', 'n-message-container'],
    ['a notification', 'n-notification-container'],
    ['the image preview', 'n-image-preview-container'],
  ])('keeps the panel when the click lands in %s', (_label, cls) => {
    const el = add(`<div class="${cls}"><button id="opt">Полный экран</button></div>`)
    expect(dismissesSidebar(el.querySelector('#opt'), card)).toBe(false)
  })

  // #2807: the tour used to die here. Its buttons live in a layer teleported to
  // <body>, so a mousedown on "Понятно" closed the panel, the step's anchor went with
  // it, the layer unmounted — and the click that advances the tour never fired, so the
  // step never changed, in the store or in localStorage.
  it('keeps the panel when a tour button is pressed', () => {
    const layer = add(
      '<div class="tr-layer"><div class="tr-pop">' +
        '<button class="tr-btn" data-testid="tour-next">Понятно</button>' +
        '<button class="tr-skip" data-testid="tour-skip">Пропустить</button>' +
        '</div></div>',
    )
    expect(dismissesSidebar(layer.querySelector('[data-testid="tour-next"]'), card)).toBe(false)
    expect(dismissesSidebar(layer.querySelector('[data-testid="tour-skip"]'), card)).toBe(false)
  })

  it('does not throw on a target that is not an element', () => {
    expect(dismissesSidebar(null, card)).toBe(false)
    expect(dismissesSidebar(document, card)).toBe(false)
  })
})
