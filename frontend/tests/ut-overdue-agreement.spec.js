// "Просрочено" has to mean one thing across the app. The board facet, the
// Gantt/Timeline counters and the server's home card all bucket a due date by its
// calendar day (workspace_tasks.go: `due < day_start`, taskFilter.matchesDue),
// but the red due pill on the card and the red date in the home list compared
// *instants* — so a task due today went red at 00:00 while every counter and the
// «Просрочено» filter said it was not overdue yet, and the «Сегодня» card counted
// it. These tests pin the card, the home list and the pure predicate to the same
// calendar-day rule.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'
import { matchesDue, isTaskOverdue } from '@/utils/taskFilter'

// Hoisted with the mock factory: `vi.mock` is lifted above the consts below, and
// the factory runs while the first component is being imported.
const FIX = vi.hoisted(() => {
  const NOW = new Date(2026, 7, 3, 12, 0, 0)
  const midnight = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).toISOString()
  return {
    NOW,
    midnight,
    atHour: (d, h) => new Date(d.getFullYear(), d.getMonth(), d.getDate(), h).toISOString(),
    dueToday: {
      id: 't1',
      board_id: 'b1',
      column_id: 'c1',
      title: 'Срок сегодня',
      number: 7,
      priority: 2,
      due_date: midnight(NOW),
    },
    dueYesterday: {
      id: 't2',
      board_id: 'b1',
      column_id: 'c1',
      title: 'Срок вчера',
      number: 8,
      priority: 2,
      due_date: midnight(new Date(2026, 7, 2)),
    },
  }
})

vi.mock('@/api', () => {
  const ok = (data) => Promise.resolve({ data })
  return {
    tasks: {
      update: vi.fn(() => ok({})),
      move: vi.fn(() => ok({})),
      setParent: vi.fn(() => ok({})),
      remove: vi.fn(),
      archive: vi.fn(),
      description: vi.fn(() => ok({ description: '' })),
      addTag: vi.fn(),
      removeTag: vi.fn(),
      addAssignee: vi.fn(),
      removeAssignee: vi.fn(),
      pinGitlabAssignee: vi.fn(),
      removeGitlabAssignee: vi.fn(),
      dueNotify: vi.fn(),
    },
    boards: { createTask: vi.fn(() => ok({})) },
    projects: { createTag: vi.fn(), milestones: () => ok([]) },
    users: { updatePrefs: vi.fn() },
    columns: { update: vi.fn(), remove: vi.fn() },
    workspaces: {
      summary: () => ok({ assigned: 1, active: 1, overdue: 0, due_today: 1, due_week: 1 }),
      tasks: () => ok([FIX.dueToday, FIX.dueYesterday]),
      tags: () => ok([]),
      tagPrefixes: () => ok([]),
      members: () => ok([]),
      milestones: () => ok([]),
    },
    gitlab: { listIntegrations: () => ok({ integrations: [] }) },
  }
})

// Noon of the reference day, so neither bound is a midnight edge case.
const { NOW, atHour, dueToday: DUE_TODAY, dueYesterday: DUE_YESTERDAY } = FIX

const COLS = [{ id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 }]
const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  RichContent: true,
  DueEditor: true,
  draggable: {
    template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>',
    props: ['list'],
  },
}

const mounted = []
const track = (w) => {
  mounted.push(w)
  return w
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.useFakeTimers()
  vi.setSystemTime(NOW)
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  document.body.innerHTML = ''
  vi.useRealTimers()
})

describe('the one overdue predicate', () => {
  it('starts the day after the due date, whatever time of day the due carries', () => {
    expect(isTaskOverdue(DUE_TODAY, NOW)).toBe(false)
    expect(isTaskOverdue(DUE_YESTERDAY, NOW)).toBe(true)
    // A due with a time still belongs to its calendar day: 10:00 today is not
    // overdue at 12:00, the same answer the «Просрочено» facet gives.
    const timed = { ...DUE_TODAY, due_date: atHour(NOW, 10) }
    expect(isTaskOverdue(timed, NOW)).toBe(false)
    expect(matchesDue(timed, 'overdue', NOW)).toBe(false)
    // Closed work is never overdue, and a task without a due date never is either.
    expect(isTaskOverdue({ ...DUE_YESTERDAY, completed_at: NOW.toISOString() }, NOW)).toBe(false)
    expect(isTaskOverdue({ id: 'x' }, NOW)).toBe(false)
  })

  it('agrees with the board facet on every case it is given', () => {
    for (const task of [DUE_TODAY, DUE_YESTERDAY, { ...DUE_TODAY, due_date: atHour(NOW, 10) }]) {
      expect(isTaskOverdue(task, NOW)).toBe(matchesDue(task, 'overdue', NOW))
    }
  })
})

describe('the card pill', () => {
  const mountCard = async (task) => {
    seedBoardStore({ columns: COLS, cardSize: 'large' })
    const TaskCard = (await import('@/components/TaskCard.vue')).default
    const w = track(mount(TaskCard, { props: { task }, global: { stubs } }))
    await w.vm.$nextTick()
    return w
  }
  const duePill = (w) => w.find('[data-tour="card-due"]')

  it('does not paint a task due today as overdue', async () => {
    const w = await mountCard(DUE_TODAY)
    const pill = duePill(w)
    expect(pill).toBeTruthy()
    // Before the fix the pill compared instants, so it went red at 00:00 — while
    // the «Просрочено» facet the user can click did not list this card at all.
    expect(pill.classes()).not.toContain('overdue')
  })

  it('still paints yesterday red', async () => {
    const w = await mountCard(DUE_YESTERDAY)
    expect(duePill(w).classes()).toContain('overdue')
  })
})

describe('the home list', () => {
  const mountHome = async () => {
    const { useWorkspacesStore } = await import('@/stores/workspaces')
    useWorkspacesStore().currentId = 'w1'
    const HomeView = (await import('@/views/HomeView.vue')).default
    const w = track(mount(HomeView, { global: { stubs } }))
    w.vm.filter = 'all' // the fixtures have no assignees; «Мои» would hide both
    await vi.waitFor(() => expect(w.findAll('.t-due').length).toBe(2))
    return w
  }

  it('reddens only the rows the «Просрочено» card counts', async () => {
    const w = await mountHome()
    const reds = w.findAll('.t-due').map((el) => el.classes().includes('overdue'))
    // Row order follows the mocked list: due today first, due yesterday second.
    expect(reds).toEqual([false, true])
  })
})
