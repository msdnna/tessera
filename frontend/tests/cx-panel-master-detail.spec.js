import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

// Master/detail on a phone for the two panels of #2893, round 3.
//
// These live in vitest rather than in the mobile e2e tier for one reason: the
// GitLab sync journal sits behind a GitLab link, and only a global admin with a
// live GitLab instance can create one — the e2e account never sees the panel at
// all. Two rounds of this task shipped that panel with "проверить смогу только
// вашими глазами" attached. What breaks there is not CSS but *which pane is
// mounted*, and that is plain template logic: it can be asserted here without a
// stand, so it is.
//
// The background-jobs modal is checked live as well (panels.spec.js), behind its
// own admin gate. It is repeated here because the two panels now share one
// behaviour and a regression in either should be visible without a stand.

const api = {
  gitlab: {
    syncRuns: vi.fn(),
    syncRunActions: vi.fn(),
    syncActionDetail: vi.fn(),
    retryWriteback: vi.fn(),
  },
  admin: { jobs: vi.fn(), runJob: vi.fn(), cancelJob: vi.fn() },
}
vi.mock('@/api', () => api)

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// The journal subscribes to workspace events; nothing here drives them.
vi.mock('@/composables/useRealtime', () => ({ useRealtime: () => ({ on: () => () => {} }) }))

// Stub keys are Naive's internal component names, not the `N…` aliases — a
// wrong key silently renders the real component (see cx-task-tabs.spec.js).
const stubs = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub"><slot /></button>' },
  Tooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /></div>' },
  // The modal renders into a teleport by default; the stub keeps its body in the
  // wrapper so the panes are findable.
  Modal: { template: '<div class="n-modal-stub"><slot name="header" /><slot /></div>' },
}

// shown reads what `v-show` actually writes — the element's own inline display.
//
// Not `wrapper.isVisible()`: that goes through jsdom's `getComputedStyle`, which
// here reports the *first* value it computed for an element and keeps reporting
// it after `v-show` flips. A pane hidden at mount and revealed later reads as
// hidden forever, and — the way that bit this spec — a pane visible at mount and
// hidden later reads as visible forever, so the assertion that matters most
// (the run list must LEAVE when the diff opens) would have passed unconditionally.
function shown(w, selector) {
  const el = w.find(selector)
  expect(el.exists(), `нет элемента ${selector}`).toBe(true)
  return el.element.style.display !== 'none'
}

// setViewport stubs matchMedia the way useResponsive reads it: one `matches`
// flag, plus the listener pair it registers on mount.
function setViewport({ mobile }) {
  window.matchMedia = vi.fn().mockReturnValue({
    matches: mobile,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
}

const RUN = {
  id: 'r1',
  kind: 'push',
  trigger: 'manual',
  status: 'ok',
  started_at: '2026-09-07T10:00:00Z',
  finished_at: '2026-09-07T10:00:20Z',
  created_count: 1,
  updated_count: 0,
  failed_count: 0,
}
const ACTION = {
  id: 'a1',
  op: 'update',
  status: 'ok',
  direction: 'push',
  summary: 'Задача #12 → GitLab',
  has_detail: false,
  detail: {},
}

const wrappers = []
// Every mount is unmounted: a leaked one keeps Vue's scheduler holding a dead
// component and the *next* spec in the file goes red for no reason of its own.
afterEach(() => {
  while (wrappers.length) wrappers.pop().unmount()
})

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  api.gitlab.syncRuns.mockResolvedValue({ data: [RUN] })
  api.gitlab.syncRunActions.mockResolvedValue({
    data: { items: [ACTION], has_more: false, next_after_seq: null },
  })
  api.admin.jobs.mockResolvedValue({
    data: [
      { key: 'j1', kind: 'worker', name: 'gitlab_sync', status: 'idle' },
      { key: 'j2', kind: 'worker', name: 'reminders', status: 'idle' },
    ],
  })
})

async function mountJournal() {
  const C = (await import('@/components/GitLabJournalPanel.vue')).default
  const w = mount(C, { props: { wsId: 'ws1' }, global: { stubs } })
  wrappers.push(w)
  await flushPromises()
  return w
}

// openAction expands the single run and picks its single action.
async function openAction(w) {
  await w.find('.j-run-head').trigger('click')
  await flushPromises()
  await w.find('.j-action').trigger('click')
  await flushPromises()
}

describe('GitLabJournalPanel.vue — журнал синка на телефоне', () => {
  it('показывает по одному экрану за раз и возвращает назад', async () => {
    setViewport({ mobile: true })
    const w = await mountJournal()

    const runs = () => shown(w, '[data-testid="journal-runs"]')
    const detail = () => shown(w, '[data-testid="journal-detail"]')

    // Arrival: the run list owns the screen. The detail pane must be hidden
    // outright rather than showing «Выберите действие» — that placeholder is a
    // prompt for a second pane, and on a phone there is no second pane to fill.
    expect(runs()).toBe(true)
    expect(detail()).toBe(false)

    await openAction(w)

    // Picking an action swaps the screens, and the diff arrives with a way back.
    expect(runs()).toBe(false)
    expect(detail()).toBe(true)
    expect(w.find('.j-back').exists()).toBe(true)

    await w.find('.j-back').trigger('click')
    await flushPromises()
    expect(runs()).toBe(true)
    expect(detail()).toBe(false)
    // The run stays expanded: `v-show` keeps the list mounted, so coming back
    // lands where the user left rather than at a collapsed list.
    expect(w.find('.j-action').exists()).toBe(true)
  })

  it('на десктопе обе панели остаются на экране одновременно', async () => {
    setViewport({ mobile: false })
    const w = await mountJournal()

    expect(shown(w, '[data-testid="journal-runs"]')).toBe(true)
    // Side by side the placeholder is the point — it labels the empty pane.
    expect(shown(w, '[data-testid="journal-detail"]')).toBe(true)

    await openAction(w)
    expect(shown(w, '[data-testid="journal-runs"]')).toBe(true)
    expect(shown(w, '[data-testid="journal-detail"]')).toBe(true)
    // No back button on the desktop: there is nothing to go back to.
    expect(w.find('.j-back').exists()).toBe(false)
  })
})

describe('BackgroundJobsModal.vue — фоновые задачи на телефоне', () => {
  // The modal loads on the false→true edge of `show`, so mounting with show:true
  // would leave the list empty and every assertion below vacuous.
  async function mountJobs() {
    const C = (await import('@/components/BackgroundJobsModal.vue')).default
    const w = mount(C, { props: { show: false }, global: { stubs } })
    wrappers.push(w)
    await w.setProps({ show: true })
    await flushPromises()
    expect(w.findAll('.bj-row').length, 'список заданий пуст — мок не доехал').toBe(2)
    return w
  }

  it('открывается на списке, а не на первой задаче', async () => {
    setViewport({ mobile: true })
    const w = await mountJobs()

    // Pre-selecting the first row is a convenience for the side-by-side layout.
    // On a phone it would open the detail screen unasked — and since load() runs
    // on a poll, it would also drag the user back into it a beat after they
    // tapped "back". So: list on arrival, detail hidden.
    expect(shown(w, '[data-testid="jobs-list"]')).toBe(true)
    expect(shown(w, '[data-testid="jobs-detail"]')).toBe(false)

    await w.findAll('.bj-row')[1].trigger('click')
    await flushPromises()
    expect(shown(w, '[data-testid="jobs-list"]')).toBe(false)
    expect(shown(w, '[data-testid="jobs-detail"]')).toBe(true)

    await w.find('.bj-back').trigger('click')
    await flushPromises()
    expect(shown(w, '[data-testid="jobs-list"]')).toBe(true)
    expect(shown(w, '[data-testid="jobs-detail"]')).toBe(false)
  })

  it('на десктопе первая задача по-прежнему выбирается сама', async () => {
    setViewport({ mobile: false })
    const w = await mountJobs()

    expect(shown(w, '[data-testid="jobs-list"]')).toBe(true)
    expect(shown(w, '[data-testid="jobs-detail"]')).toBe(true)
    // The convenience the phone gives up is kept where the second pane exists.
    expect(w.find('.bj-detail-name').exists()).toBe(true)
    expect(w.find('.bj-back').exists()).toBe(false)
  })
})
