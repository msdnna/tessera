// The board's stage selector and strip (#2994): the menu sorts active →
// planned → «Бэклог» / «Все задачи» (closed folded), picks by click and by B / A;
// the strip folds to a row with progress and the first line of the goals.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import * as naive from 'naive-ui'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})
const push = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
  useRoute: () => ({ params: {}, query: {}, path: '/' }),
}))
const metrics = vi.fn(() =>
  Promise.resolve({
    data: {
      totals: { tasks: 31, done: 18, open: 13, estimate: 5040, done_estimate: 3120 },
      scope: { at_start: 27, added: 6, removed: 2, now: 31 },
      signals: { overdue: 3 },
      milestone: {},
    },
  }),
)
vi.mock('@/api', () => ({ milestones: { metrics: (...a) => metrics(...a), update: vi.fn() } }))

const day = (off) => {
  const d = new Date()
  return new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate() + off)).toISOString()
}
const STAGES = [
  {
    id: 's42',
    slug: 'sprint-42',
    title: 'Спринт 42',
    state: 'active',
    start_date: day(-9),
    due_date: day(5),
    task_count: 31,
    done_count: 18,
  },
  {
    id: 's43',
    slug: 'sprint-43',
    title: 'Спринт 43',
    state: 'active',
    start_date: day(6),
    due_date: day(19),
    task_count: 12,
    done_count: 0,
  },
  {
    id: 's41',
    slug: 'sprint-41',
    title: 'Спринт 41',
    state: 'closed',
    task_count: 22,
    done_count: 19,
  },
]
const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
}

let wrapper = null
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('StageSelect', () => {
  async function open(scope = '') {
    const StageSelect = (await import('@/components/milestone/StageSelect.vue')).default
    wrapper = mount(StageSelect, {
      props: { stages: STAGES, scope },
      global: { stubs },
      attachTo: document.body,
    })
    await wrapper.find('[data-testid="stage-select"]').trigger('click')
    await nextTick()
    return wrapper
  }
  const menu = () => document.body.querySelector('[data-testid="stage-menu"]')

  it('shows the picked stage with its progress', async () => {
    const w = await open('sprint-42')
    expect(w.find('.ss-val').text()).toBe('Спринт 42')
    expect(w.find('.ss-pct').text()).toBe('58%')
  })

  it('lists active, then planned; closed stay folded', async () => {
    await open()
    const names = [...menu().querySelectorAll('[data-testid="stage-option"] .ss-row-n')].map(
      (e) => e.textContent,
    )
    expect(names).toEqual(['Спринт 42', 'Спринт 43'])
    expect(menu().textContent).not.toContain('Спринт 41')
    menu().querySelector('[data-testid="stage-show-closed"]').click()
    await nextTick()
    expect(menu().textContent).toContain('Спринт 41')
  })

  it('picks by click, and B / A pick the backlog and all tasks', async () => {
    const w = await open()
    menu().querySelector('[data-testid="stage-option"]').click()
    expect(w.emitted('pick')[0][0].id).toBe('s42')
    await w.find('[data-testid="stage-select"]').trigger('click')
    await nextTick()
    menu().dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyB', key: 'b', bubbles: true }))
    expect(w.emitted('pick')[1][0]).toBe('backlog')
    await w.find('[data-testid="stage-select"]').trigger('click')
    await nextTick()
    menu().dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyA', key: 'a', bubbles: true }))
    expect(w.emitted('pick')[2][0]).toBe('')
  })
})

describe('StageStrip', () => {
  const stage = {
    ...STAGES[0],
    description: '## Цели\n1. Довести v2 до всего приложения\n2. Синк без падений',
  }
  async function mountStrip(open = false) {
    const StageStrip = (await import('@/components/milestone/StageStrip.vue')).default
    wrapper = mount(StageStrip, {
      props: { stage, projectSlug: 'atlas', open },
      global: { stubs },
      attachTo: document.body,
    })
    await flushPromises()
    return wrapper
  }

  it('folded: progress and the first lines of the goals, without Markdown', async () => {
    const w = await mountStrip()
    expect(w.find('.sstrip-count').text()).toContain('18')
    expect(w.find('.sstrip-goal').text()).toBe(
      'Цели · Довести v2 до всего приложения · Синк без падений',
    )
    expect(metrics).not.toHaveBeenCalled()
  })

  it('unfolds on the chevron and reads the figures', async () => {
    const w = await mountStrip()
    await w.find('[data-testid="stage-strip-toggle"]').trigger('click')
    expect(w.emitted('update:open')[0]).toEqual([true])
    await w.setProps({ open: true })
    await flushPromises()
    expect(metrics).toHaveBeenCalledWith('s42')
    expect(w.find('.sstrip-body').text()).toContain('62%')
  })

  it('the name leads to the stage page', async () => {
    const w = await mountStrip()
    await w.find('.sstrip-name').trigger('click')
    expect(push.mock.calls[0][0].path).toBe('/project/atlas/stage/sprint-42')
  })
})
