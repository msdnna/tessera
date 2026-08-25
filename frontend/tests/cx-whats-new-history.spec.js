import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ru from '@/locales/ru'

// The version stamp in the footer opens the same modal as an update does, but
// showing the whole changelog and acknowledging nothing (#2812).

// The build stamps this in; under vitest it is absent, and the store would then
// treat every release as newer than this build and show nothing.
vi.hoisted(() => {
  globalThis.__APP_VERSION__ = '1.0.0'
})

const apiMock = vi.hoisted(() => ({
  acknowledgements: {
    ack: vi.fn(() => Promise.resolve()),
    list: vi.fn(() => Promise.resolve({ data: [] })),
  },
  meta: { version: vi.fn(() => Promise.resolve({ data: { api: '0.100.0' } })) },
}))
vi.mock('@/api', () => apiMock)

const accountMock = vi.hoisted(() => ({ isBrandNewAccount: vi.fn(() => false) }))
vi.mock('@/utils/account', () => accountMock)

// Three releases, two of them above the acked baseline — so "the history" and
// "what you just updated into" are different lists whatever the real data file
// currently holds.
vi.mock('@/data/whatsNew', () => ({
  WHATS_NEW: [
    { version: '0.9.0', titleKey: 'whatsNew.demo.title', itemKeys: ['whatsNew.demo.item1'] },
    { version: '0.8.0', titleKey: 'whatsNew.demo.title', itemKeys: ['whatsNew.demo.item1'] },
    {
      version: '0.7.0',
      titleKey: 'whatsNew.demo.title',
      itemKeys: ['whatsNew.demo.item1'],
      spotlight: {
        navKey: 'notes',
        titleKey: 'whatsNew.demo.title',
        bodyKey: 'whatsNew.demo.item1',
      },
    },
  ],
}))

import { useWhatsNewStore } from '@/stores/whatsNew'
import { WHATS_NEW } from '@/data/whatsNew'
import WhatsNewModal from '@/components/WhatsNewModal.vue'
import VersionBadge from '@/components/VersionBadge.vue'

// Mounted components are unmounted in afterEach: a mount left open in this suite
// keeps its scheduler around and the next test's re-render never lands.
let wrapper = null

function mountWith(component) {
  const i18n = createI18n({
    legacy: false,
    locale: 'ru',
    messages: { ru: { ...ru, whatsNew: { demo: { title: 'Демо', item1: 'пункт' } } } },
  })
  wrapper = mount(component, { global: { plugins: [i18n] }, attachTo: document.body })
  return wrapper
}

// n-modal teleports its card out of the wrapper, so a single tick on the
// wrapper does not repaint it — let the whole render queue drain.
const drain = () => new Promise((r) => setTimeout(r, 0))
const modalCard = () => document.querySelector('[data-testid="whats-new-modal"]')

beforeEach(() => {
  vi.clearAllMocks()
  apiMock.acknowledgements.list.mockResolvedValue({ data: [] })
  accountMock.isBrandNewAccount.mockReturnValue(false)
  setActivePinia(createPinia())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

describe('changelog history mode', () => {
  it('shows every curated release, newest first, past acks and the build version', async () => {
    const store = useWhatsNewStore()
    // Baselined at the newest release: the update queue is empty…
    apiMock.acknowledgements.list.mockResolvedValue({ data: [{ key: 'whatsnew:web:0.9.0' }] })
    await store.load()
    expect(store.pending).toEqual([])
    expect(store.entries).toEqual([])

    // …and the history still lists all of them.
    store.openHistory()
    expect(store.isHistory).toBe(true)
    expect(store.entries.map((e) => e.version)).toEqual(['0.9.0', '0.8.0', '0.7.0'])
    expect(store.entries).toHaveLength(WHATS_NEW.length)
  })

  it('acknowledges nothing when the history is closed', async () => {
    const store = useWhatsNewStore()
    apiMock.acknowledgements.list.mockResolvedValue({ data: [{ key: 'whatsnew:web:0.7.0' }] })
    await store.load()
    const queued = store.pending.map((e) => e.version)

    store.openHistory()
    store.closeHistory()

    expect(store.historyOpen).toBe(false)
    expect(apiMock.acknowledgements.ack).not.toHaveBeenCalled()
    // The update the user has not seen yet is still waiting for them.
    expect(store.pending.map((e) => e.version)).toEqual(queued)
  })

  it('lets a pending update win over a manual open', async () => {
    const store = useWhatsNewStore()
    await store.load()
    expect(store.pending).toHaveLength(3)

    store.openHistory()
    // The update modal is already on screen — and it is the one that writes acks.
    expect(store.isHistory).toBe(false)
    expect(store.entries).toEqual(store.pending)
  })

  it('holds the spotlight arrows back while the history is open', async () => {
    const store = useWhatsNewStore()
    await store.load()
    await store.dismissModal()
    expect(store.currentSpotlight).not.toBe(null)

    store.openHistory()
    expect(store.currentSpotlight).toBe(null)
    store.closeHistory()
    expect(store.currentSpotlight).not.toBe(null)
  })
})

describe('WhatsNewModal', () => {
  it('renders the history wording and closes without an ack', async () => {
    const store = useWhatsNewStore()
    apiMock.acknowledgements.list.mockResolvedValue({ data: [{ key: 'whatsnew:web:0.9.0' }] })
    await store.load()
    store.openHistory()

    mountWith(WhatsNewModal)
    await drain()
    const card = modalCard()
    expect(card).not.toBe(null)
    expect(card.textContent).toContain(ru.app.whatsNew.historyTitle)
    expect(card.textContent).toContain(ru.app.whatsNew.historySub)
    expect(card.textContent).toContain(ru.app.whatsNew.close)
    expect(card.querySelectorAll('.wn-rel')).toHaveLength(WHATS_NEW.length)

    card.querySelector('button').click()
    await drain()
    expect(store.historyOpen).toBe(false)
    expect(apiMock.acknowledgements.ack).not.toHaveBeenCalled()
  })

  it('keeps the update wording and still acknowledges on dismiss', async () => {
    const store = useWhatsNewStore()
    apiMock.acknowledgements.list.mockResolvedValue({ data: [{ key: 'whatsnew:web:0.7.0' }] })
    await store.load()

    mountWith(WhatsNewModal)
    await drain()
    const card = modalCard()
    expect(card.textContent).toContain(ru.app.whatsNew.title)
    expect(card.textContent).toContain(ru.app.whatsNew.gotIt)
    expect(card.querySelectorAll('.wn-rel')).toHaveLength(2)

    card.querySelector('button').click()
    await drain()
    expect(apiMock.acknowledgements.ack).toHaveBeenCalled()
  })
})

describe('VersionBadge', () => {
  it('opens the history on click and on Enter', async () => {
    const store = useWhatsNewStore()
    const w = mountWith(VersionBadge)
    const row = w.find('[data-testid="version-badge"]')
    expect(row.exists()).toBe(true)

    await row.trigger('click')
    expect(store.historyOpen).toBe(true)

    store.closeHistory()
    await row.trigger('keydown.enter')
    expect(store.historyOpen).toBe(true)
  })
})
