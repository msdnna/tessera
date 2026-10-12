import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'

// The «Начало работы» checklist and the spotlight queue (#3000, #3070): what
// counts as done, with the tour or without it, and which hint is offered when.

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

import {
  useOnboardingStore,
  ITEM_PREFIX,
  CHECKLIST_HIDDEN,
  itemsOfEvent,
} from '@/stores/onboarding'
import { useHintsStore, MAX_PER_SESSION } from '@/stores/hints'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useTourStore } from '@/stores/tour'
import { setTourEventSink, tourEvent } from '@/utils/tourEvents'

async function withAcks(keys) {
  apiMock.acknowledgements.list.mockResolvedValue({
    data: keys.map((key) => ({ key, ack_at: '2026-10-06T10:00:00Z' })),
  })
  await useWhatsNewStore().load()
}
const acked = () => apiMock.acknowledgements.ack.mock.calls.map((c) => c[0])

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  vi.clearAllMocks()
  setActivePinia(createPinia())
  useTourStore().doneMs = 0
})
afterEach(() => setTourEventSink(null))

describe('checklist (onboarding store)', () => {
  it('counts a result done on the user’s own, and says so', async () => {
    await withAcks([])
    const ob = useOnboardingStore()
    expect(ob.isDone('basics.project')).toBe(false)
    await ob.mark('basics.project')
    expect(acked()).toContain(ITEM_PREFIX + 'basics.project:self')
    expect(ob.isDone('basics.project')).toBe(true)
    expect(ob.isSelf('basics.project')).toBe(true)
    expect(ob.basics).toMatchObject({ done: 1, total: 6, complete: false })
  })

  it('counts the same result as the tour’s while a chapter runs', async () => {
    await withAcks([])
    const ob = useOnboardingStore()
    useTourStore().start('basics')
    ob.note('board.created')
    await Promise.resolve()
    expect(acked()).toContain(ITEM_PREFIX + 'basics.board')
    expect(ob.isSelf('basics.board')).toBe(false)
  })

  it('takes the user’s own socket events and ignores colleagues’', async () => {
    await withAcks([])
    const ob = useOnboardingStore()
    ob.onSocket({ type: 'project.created', actor: 'someone-else' }, 'me')
    ob.onSocket({ type: 'project.created' }, 'me') // no actor — a worker, a sync
    expect(acked()).toEqual([])
    ob.onSocket({ type: 'task.moved', actor: 'me' }, 'me')
    expect(acked()).toContain(ITEM_PREFIX + 'basics.move:self')
  })

  it('maps the client events onto results', () => {
    expect(itemsOfEvent('task.tagged')).toEqual(['task.tags', 'basics.fields'])
    expect(itemsOfEvent('palette.opened')).toEqual(['keys.palette'])
    expect(itemsOfEvent('nothing')).toEqual([])
  })

  it('hears the events components report through tourEvent()', async () => {
    await withAcks([])
    const ob = useOnboardingStore()
    setTourEventSink((n) => ob.note(n))
    tourEvent('inbox.captured')
    expect(acked()).toContain(ITEM_PREFIX + 'inbox.capture:self')
    // With no sink (tests, before the shell is up) it is a no-op.
    setTourEventSink(null)
    expect(() => tourEvent('inbox.captured')).not.toThrow()
  })

  it('treats a v1 guide walked to the end as «Основы» passed', async () => {
    await withAcks(['getstarted:done'])
    const ob = useOnboardingStore()
    expect(ob.basics.complete).toBe(true)
    expect(ob.isDone('basics.open')).toBe(true)
  })

  it('counts a chapter walked to the end as complete', async () => {
    await withAcks(['getstarted:ch:inbox'])
    const ob = useOnboardingStore()
    expect(ob.optional.find((c) => c.id === 'inbox').complete).toBe(true)
    expect(ob.chaptersDone).toBe(1)
  })

  it('locks the GitLab chapter until a GitLab project is bound', async () => {
    await withAcks([])
    const ob = useOnboardingStore()
    expect(ob.optional.find((c) => c.id === 'gitlab').locked).toBe(true)
    ob.gitlabBound = true
    expect(ob.optional.find((c) => c.id === 'gitlab').locked).toBe(false)
  })

  it('hides for good and comes back', async () => {
    await withAcks([])
    const ob = useOnboardingStore()
    await ob.hide()
    expect(ob.hidden).toBe(true)
    await ob.unhide()
    expect(ob.hidden).toBe(false)
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ key: CHECKLIST_HIDDEN })
  })

  it('«Начать с начала» clears the tour’s marks, keeps the card hidden, restarts', async () => {
    await withAcks([ITEM_PREFIX + 'basics.task', 'getstarted:done', CHECKLIST_HIDDEN])
    const ob = useOnboardingStore()
    await ob.restart()
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ prefix: 'getstarted:' })
    expect(ob.isDone('basics.task')).toBe(false)
    expect(ob.hidden).toBe(true)
    expect(useTourStore().chapter).toBe('basics')
  })
})

describe('spotlight queue (hints store)', () => {
  it('offers nothing «new» to an account that never saw the v2 card', async () => {
    await withAcks(['whatsnew:web:0.9.0', 'whatschanged:v2'])
    const h = useHintsStore()
    const keys = h.queue.map((x) => x.key)
    // Only what isn't tied to «Позже»: contextual hints and the rail.
    expect(keys).not.toContain('inbox')
    expect(keys).toContain('rail')
    expect(keys).toContain('stage-strip')
  })

  it('puts the mini-tour’s points in the queue after «Позже», «new» first', async () => {
    await withAcks(['whatschanged:v2', 'whatschanged:later'])
    const h = useHintsStore()
    const kinds = h.queue.map((x) => x.kind)
    expect(h.queue.map((x) => x.key)).toContain('inbox')
    expect(kinds.indexOf('ctx')).toBeGreaterThan(kinds.lastIndexOf('new'))
  })

  it('drops a dismissed hint and a switched-off kind', async () => {
    await withAcks(['whatschanged:v2', 'whatschanged:later', 'spotlight:inbox', 'hints:off:ctx'])
    const h = useHintsStore()
    const keys = h.queue.map((x) => x.key)
    expect(keys).not.toContain('inbox')
    expect(keys).not.toContain('stage-strip')
    expect(h.ctxOn).toBe(false)
  })

  it('shows at most two per session; one already shown does not spend twice', async () => {
    await withAcks(['whatschanged:v2', 'whatschanged:later'])
    const h = useHintsStore()
    const [a, b, c] = h.queue
    expect(h.canShow(a.key)).toBe(true)
    h.shown(a)
    h.shown(a)
    expect(h.budgetLeft).toBe(MAX_PER_SESSION - 1)
    h.shown(b)
    expect(h.canShow(c.key)).toBe(false)
    expect(h.canShow(a.key)).toBe(true)
  })

  it('«Понятно» acks spotlight:<key>; a release hint goes through What’s New', async () => {
    await withAcks(['whatschanged:v2', 'whatschanged:later'])
    const h = useHintsStore()
    await h.dismiss(h.queue.find((x) => x.key === 'palette'))
    expect(acked()).toContain('spotlight:palette')
  })

  it('«Показать снова» brings one back, even outside its rules', async () => {
    await withAcks(['spotlight:inbox'])
    const h = useHintsStore()
    expect(h.status('inbox').state).toBe('shown')
    await h.showAgain('inbox')
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ key: 'spotlight:inbox' })
    expect(h.queue.map((x) => x.key)).toContain('inbox')
    expect(h.status('inbox').state).toBe('queue')
  })

  it('«Показать подсказки заново» clears the namespace and the session budget', async () => {
    await withAcks(['spotlight:inbox', 'spotlight:rail'])
    const h = useHintsStore()
    h.shown({ key: 'x' })
    h.shown({ key: 'y' })
    await h.resetAll()
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ prefix: 'spotlight:' })
    expect(h.budgetLeft).toBe(MAX_PER_SESSION)
    expect(h.queue.length).toBeGreaterThan(5)
  })

  it('switches a kind off and on through hints:off:<kind>', async () => {
    await withAcks([])
    const h = useHintsStore()
    await h.setKind('new', false)
    expect(acked()).toContain('hints:off:new')
    expect(h.newOn).toBe(false)
    await h.setKind('new', true)
    expect(apiMock.acknowledgements.forget).toHaveBeenCalledWith({ key: 'hints:off:new' })
  })

  it('walking the mini-tour marks its hints shown', async () => {
    await withAcks([])
    const t = useTourStore()
    t.start('whatschanged')
    t.next()
    await new Promise((r) => setTimeout(r, 0))
    expect(acked()).toContain('spotlight:inbox')
  })

  it('switching «new» off also silences «Что нового»', async () => {
    apiMock.acknowledgements.list.mockResolvedValue({
      data: [{ key: 'hints:off:new' }, { key: 'getstarted:done' }],
    })
    const wn = useWhatsNewStore()
    await wn.load()
    expect(wn.showV2Intro).toBe(false)
  })
})
