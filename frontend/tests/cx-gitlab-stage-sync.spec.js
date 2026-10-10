// GitLab stages both ways (#3010): a stage of a binding that writes to GitLab
// is editable, one that does not stays read-only; a refused write shows on the
// stage strip as «Описание не синхронизировано с GitLab · ↵ повторить» and the
// retry re-sends it; a conflict opens the conflicts window on the stage, where
// the decision goes through the stage's own endpoint.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import * as naive from 'naive-ui'

const warning = vi.fn()
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning }),
  }
})
vi.mock('@/composables/useMessage', () => ({
  useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning }),
}))
const push = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
  useRoute: () => ({ params: {}, query: {}, path: '/' }),
}))
const api = {
  milestones: {
    metrics: vi.fn(() => Promise.resolve({ data: { totals: {}, milestone: {} } })),
    update: vi.fn(),
    retryGitlab: vi.fn(),
    resolveConflict: vi.fn(),
  },
  gitlab: { conflicts: vi.fn(), resolveConflict: vi.fn() },
  tasks: { get: vi.fn() },
}
vi.mock('@/api', () => api)

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
}

const GL_STAGE = {
  id: 's42',
  slug: 'sprint-42',
  title: 'Спринт 42',
  state: 'active',
  description: 'Цели спринта',
  task_count: 3,
  done_count: 1,
  gl_global_id: 'gid://gitlab/Milestone/41',
  gl_url: 'https://gitlab.example/g/p/-/milestones/4',
  gl_iid: 4,
  gl_writable: true,
  gl_push_error: '',
  gl_push_fields: [],
  gl_conflict: false,
}

let wrapper = null
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

describe('utils/milestones · GitLab stage state', () => {
  it('editable unless GitLab-linked without writing', async () => {
    const m = await import('@/utils/milestones')
    expect(m.stageEditable({ id: 'n' })).toBe(true)
    expect(m.stageEditable(GL_STAGE)).toBe(true)
    expect(m.stageEditable({ ...GL_STAGE, gl_writable: false })).toBe(false)
    expect(m.stageUnsynced(GL_STAGE)).toBe(false)
    expect(m.stageUnsynced({ ...GL_STAGE, gl_push_error: 'http 500' })).toBe(true)
    expect(m.stageConflict({ ...GL_STAGE, gl_conflict: true })).toBe(true)
    expect(m.unsyncedKey({ gl_push_fields: ['description'] })).toBe(
      'milestones.gitlab.unsyncedGoals',
    )
    expect(m.unsyncedKey({ gl_push_fields: ['title', 'description'] })).toBe(
      'milestones.gitlab.unsynced',
    )
  })
})

describe('StageStrip · GitLab write-back', () => {
  async function mountStrip(stage) {
    const StageStrip = (await import('@/components/milestone/StageStrip.vue')).default
    wrapper = mount(StageStrip, {
      props: { stage, projectSlug: 'atlas' },
      global: { stubs },
      attachTo: document.body,
    })
    await flushPromises()
    return wrapper
  }

  it('a refused goals write replaces the goal line, and «повторить» re-sends it', async () => {
    api.milestones.retryGitlab.mockResolvedValue({
      data: { ...GL_STAGE, gl_sync: { status: 'ok' } },
    })
    const w = await mountStrip({
      ...GL_STAGE,
      gl_push_error: 'gitlab: http 500',
      gl_push_fields: ['description'],
    })
    expect(w.find('.sstrip-err').text()).toContain('Описание не синхронизировано с GitLab')
    await w.find('[data-testid="stage-strip-retry"]').trigger('click')
    await flushPromises()
    expect(api.milestones.retryGitlab).toHaveBeenCalledWith('s42')
    expect(w.emitted('changed')).toHaveLength(1)
  })

  it('a conflict opens the conflicts window on the stage', async () => {
    const { useConflictsStore } = await import('@/stores/conflicts')
    const store = useConflictsStore()
    const w = await mountStrip({ ...GL_STAGE, gl_conflict: true })
    expect(w.find('.sstrip-err').text()).toContain('Конфликт с GitLab')
    await w.find('[data-testid="stage-strip-retry"]').trigger('click')
    expect(store.resolverOpen).toBe(true)
    expect(store.focusMilestoneId).toBe('s42')
    expect(api.milestones.retryGitlab).not.toHaveBeenCalled()
  })

  it('a synced stage shows its goals, no mark', async () => {
    const w = await mountStrip(GL_STAGE)
    expect(w.find('.sstrip-err').exists()).toBe(false)
    expect(w.find('.sstrip-goal').text()).toBe('Цели спринта')
  })
})

describe('useStageSync · the answer of a write', () => {
  it('conflict → the window opens on the stage; failed → a warning', async () => {
    const { useStageSync } = await import('@/composables/useStageSync')
    const { useConflictsStore } = await import('@/stores/conflicts')
    api.gitlab.conflicts.mockResolvedValue({ data: [] })
    let sync
    const Host = {
      setup() {
        sync = useStageSync()
        return () => null
      },
    }
    wrapper = mount(Host)
    expect(
      sync.report({ id: 's42', gl_sync: { status: 'conflict', fields: ['description'] } }),
    ).toBe('conflict')
    const store = useConflictsStore()
    expect(store.resolverOpen).toBe(true)
    expect(store.focusMilestoneId).toBe('s42')
    sync.report({ id: 's42', gl_sync: { status: 'failed', error: 'http 403' } })
    expect(warning).toHaveBeenCalledTimes(2)
    expect(String(warning.mock.calls[1][0])).toContain('http 403')
    expect(sync.report({ id: 's42' })).toBe('')
  })
})

describe('ConflictResolverModal · a stage conflict', () => {
  const STAGE_CONFLICT = {
    id: 'm1',
    entity: 'milestone',
    milestone_id: 'm1',
    milestone_title: 'Спринт 42',
    milestone_slug: 'sprint-42',
    project_slug: 'atlas',
    task_id: null,
    change_kind: 'milestone',
    gl_iid: 4,
    fields: [{ field: 'description', base: 'A', ours: 'A + B', theirs: 'A + C' }],
  }
  const TASK_CONFLICT = {
    id: 'c1',
    entity: 'task',
    task_id: 't1',
    task_number: 7,
    task_title: 'Экспорт',
    change_kind: 'state',
    fields: [{ field: 'state', base: 'opened', ours: 'closed', theirs: 'opened' }],
  }
  const lwStubs = {
    Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
    Tooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /></div>' },
    Modal: { template: '<div class="n-modal-stub"><slot /></div>' },
  }

  async function open(props = {}) {
    api.gitlab.conflicts.mockResolvedValue({ data: [TASK_CONFLICT, STAGE_CONFLICT] })
    const C = (await import('@/components/ConflictResolverModal.vue')).default
    wrapper = mount(C, {
      props: { show: false, wsId: 'ws1', ...props },
      global: { stubs: lwStubs },
    })
    await wrapper.setProps({ show: true })
    await flushPromises()
    return wrapper
  }

  it('lists the stage in the queue and pre-selects it from the stage', async () => {
    const w = await open({ focusMilestoneId: 'm1' })
    const items = w.findAll('[data-testid="conflict-item"]')
    expect(items).toHaveLength(2)
    expect(items[1].text()).toContain('Спринт 42')
    expect(items[1].classes()).toContain('on')
    expect(w.find('.cr-title').text()).toContain('Спринт 42')
    expect(w.find('[data-testid="conflict-open-task"]').text()).toContain('Открыть этап')
  })

  it('the decision goes to the stage endpoint, not the task one', async () => {
    api.milestones.resolveConflict.mockResolvedValue({ data: { gl_sync: { status: 'ok' } } })
    const w = await open({ focusMilestoneId: 'm1' })
    await w.find('[data-testid="conflict-pick-theirs"]').trigger('click')
    await w.find('[data-testid="conflict-apply"]').trigger('click')
    await flushPromises()
    expect(api.milestones.resolveConflict).toHaveBeenCalledWith('m1', { resolution: 'theirs' })
    expect(api.gitlab.resolveConflict).not.toHaveBeenCalled()
  })

  it('«Открыть этап» leads to the stage page', async () => {
    const w = await open({ focusMilestoneId: 'm1' })
    await w.find('[data-testid="conflict-open-task"]').trigger('click')
    // The window unwinds its history entry first (≤ 400 ms without a popstate).
    await new Promise((r) => setTimeout(r, 450))
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/project/atlas/stage/sprint-42')
  })
})
