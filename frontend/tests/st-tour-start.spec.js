import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'

// When the guide opens itself and who is allowed to see it (#2760), plus the
// hand-off with the What's-New store — the two overlays share the same acks and
// must never be on screen at once.

// The build stamps this in; under vitest it is absent, and the changelog store
// would then treat every release as "newer than this build" and show nothing.
vi.hoisted(() => {
  globalThis.__APP_VERSION__ = '1.0.0'
})

const apiMock = vi.hoisted(() => ({
  acknowledgements: {
    ack: vi.fn(() => Promise.resolve()),
    list: vi.fn(() => Promise.resolve({ data: [] })),
  },
}))
vi.mock('@/api', () => apiMock)

const accountMock = vi.hoisted(() => ({ isBrandNewAccount: vi.fn(() => true) }))
vi.mock('@/utils/account', () => accountMock)

// A single release with a spotlight, so "what the changelog wants to show" is
// predictable regardless of what the real data file currently holds.
vi.mock('@/data/whatsNew', () => ({
  WHATS_NEW: [
    {
      version: '0.9.0',
      title: 'Релиз',
      items: ['что-то новое'],
      spotlight: { navKey: 'notes', text: 'Заметки переехали' },
    },
  ],
}))

import { useTourStore, TOUR_PREFIX, TOUR_SKIPPED, TOUR_STEP_KEY } from '@/stores/tour'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { GET_STARTED } from '@/data/getStarted'

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  accountMock.isBrandNewAccount.mockReturnValue(true)
  apiMock.acknowledgements.list.mockResolvedValue({ data: [] })
  setActivePinia(createPinia())
})
afterEach(() => localStorage.clear())

describe('tour autostart', () => {
  it('opens the guide for a brand-new account with no getstarted ack', () => {
    const t = useTourStore()
    expect(t.autoStart({ acked: new Set(['whatsnew:0.1.0']) })).toBe(true)
    expect(t.active).toBe(true)
    expect(t.current.id).toBe(GET_STARTED[0].id)
  })

  it('stays away from accounts that predate this build', () => {
    accountMock.isBrandNewAccount.mockReturnValue(false)
    const t = useTourStore()
    expect(t.autoStart({ acked: new Set() })).toBe(false)
    expect(t.active).toBe(false)
  })

  it.each([TOUR_SKIPPED, TOUR_PREFIX + 'done'])('never runs twice (%s)', (key) => {
    const t = useTourStore()
    expect(t.autoStart({ acked: new Set([key]) })).toBe(false)
    expect(t.active).toBe(false)
  })

  it('is silent on mobile and writes no ack there', () => {
    const t = useTourStore()
    expect(t.autoStart({ acked: new Set(), mobile: true })).toBe(false)
    expect(t.active).toBe(false)
    // The account keeps its right to the guide on a desktop: nothing was written.
    expect(apiMock.acknowledgements.ack).not.toHaveBeenCalled()
  })

  it('accepts the acks as a plain array too', () => {
    const t = useTourStore()
    expect(t.autoStart({ acked: [TOUR_SKIPPED] })).toBe(false)
  })

  it('picks up where a reload interrupted it', () => {
    localStorage.setItem(TOUR_STEP_KEY, 'card-open')
    const t = useTourStore()
    t.autoStart({ acked: new Set() })
    expect(t.current.id).toBe('card-open')
  })

  it('does not restart a guide that is already on screen', () => {
    const t = useTourStore()
    t.autoStart({ acked: new Set() })
    t.next()
    const at = t.current.id
    expect(t.autoStart({ acked: new Set() })).toBe(false)
    expect(t.current.id).toBe(at)
  })

  it('startGuide re-runs it from the top whatever the acks say', () => {
    const t = useTourStore()
    expect(t.startGuide()).toBe(true)
    expect(t.current.id).toBe(GET_STARTED[0].id)
  })
})

describe("hand-off with What's New", () => {
  it('holds the changelog modal and spotlights back while the guide runs', async () => {
    // A returning user: they have a changelog to catch up on, and can still
    // start the guide by hand from the sidebar footer.
    accountMock.isBrandNewAccount.mockReturnValue(false)
    const wn = useWhatsNewStore()
    const t = useTourStore()
    await wn.load()
    expect(wn.pending).toHaveLength(1)

    t.startGuide()
    expect(wn.pending).toEqual([])

    // Nothing was consumed — the changelog comes back once the guide is over.
    await t.skip()
    expect(wn.pending).toHaveLength(1)

    // Same for the spotlight hints, which queue up behind the modal.
    await wn.dismissModal()
    expect(wn.currentSpotlight).not.toBe(null)
    t.startGuide()
    expect(wn.currentSpotlight).toBe(null)
    await t.skip()
    expect(wn.currentSpotlight).not.toBe(null)
  })

  it('reports a failed ack load so the guide is not offered blindly', async () => {
    apiMock.acknowledgements.list.mockRejectedValueOnce(new Error('offline'))
    const wn = useWhatsNewStore()
    expect(await wn.load()).toBe(false)
    // …and a successful one reports true, which is what unlocks the autostart.
    expect(await wn.load()).toBe(true)
  })

  it('baselines a brand-new account silently instead of showing the changelog', async () => {
    const wn = useWhatsNewStore()
    expect(await wn.load()).toBe(true)
    expect(wn.pending).toEqual([])
  })
})
