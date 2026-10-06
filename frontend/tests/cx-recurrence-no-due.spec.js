// A recurrence on a task with no due date never fires — not once, on any trigger.
//
// The server advances a rule from the due date and from nothing else:
// `recurAdvance` (backend/handlers/recurrence.go) returns a no-op for
// `t.DueDate == nil` before it even compares the trigger, so neither ticking the
// task («При завершении») nor dragging it into the trigger column («При переходе
// в колонку») moves anything, and the schedule worker's query filters
// `due_date IS NOT NULL` besides. The rule is nevertheless accepted and stored —
// the editor commits it the moment a frequency is picked, `recurrenceToStore`
// canonicalises it without ever looking at the due date — and the card draws its
// repeat badge. «Каждый понедельник» on a task nobody dated looks live and is not.
//
// The same class as the dead trigger column (cx-recurrence-dead-column-trigger),
// and harder to notice: there the grid at least had a date to refuse to
// highlight; here it is empty for the innocent reason that there is nothing to
// count from, which reads as «ещё ничего не выбрано» (#2932).
import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NMessageProvider } from 'naive-ui'
import { dueDateMissing } from '@/utils/recurrence'

const TZ_BEFORE = process.env.TZ
process.env.TZ = 'Europe/Moscow'
afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

vi.mock('@/api', () => ({
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  getAccessToken: () => null,
  projects: { milestones: vi.fn(() => Promise.resolve({ data: [] })) },
  milestones: {},
  gitlab: { projectConfig: vi.fn(() => Promise.reject(new Error('no integration'))) },
}))

const DueEditor = (await import('@/components/DueEditor.vue')).default

const TODO = '11111111-1111-1111-1111-111111111111'
const COLUMNS = [{ id: TODO, name: 'К работе' }]
const DUE = new Date(2026, 6, 10, 9, 0).getTime() // Fri 10 Jul 2026
const RULE = { freq: 'daily', interval: 1, trigger: 'complete', tz: 'Europe/Moscow' }

let wrapper = null

function mountWith(props) {
  wrapper = mount(
    { render: () => h(NMessageProvider, null, { default: () => h(DueEditor, props) }) },
    { attachTo: document.body },
  )
  return wrapper
}

const editor = (w) => w.findComponent(DueEditor)
const warning = (w) => w.find('.de-warn')

// Day numbers the grid highlights as *upcoming* occurrences in the month in view.
function highlighted(w) {
  return w
    .findAll('.de-day')
    .filter((b) => b.classes('occ') && !b.classes('out'))
    .map((b) => Number(b.text()))
    .sort((a, b) => a - b)
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('dueDateMissing', () => {
  it('flags a rule on a task with no due date', () => {
    expect(dueDateMissing(RULE, null)).toBe(true)
    expect(dueDateMissing(RULE, undefined)).toBe(true)
    expect(dueDateMissing({ freq: 'custom', dates: ['2026-07-10'] }, null)).toBe(true)
  })

  it('stays quiet once there is a due date', () => {
    expect(dueDateMissing(RULE, DUE)).toBe(false)
    // The epoch is a date like any other — a task due 01.01.1970 is absurd, not
    // undated, and treating it as unset would hide the warning's opposite error.
    expect(dueDateMissing(RULE, 0)).toBe(false)
  })

  it('stays quiet when there is no rule at all', () => {
    expect(dueDateMissing(null, null)).toBe(false)
    expect(dueDateMissing({}, null)).toBe(false)
    expect(dueDateMissing({ freq: '' }, null)).toBe(false)
  })
})

describe('the editor says why an undated repeat will not fire', () => {
  it('warns when the rule is opened on a task with no due date', async () => {
    const w = mountWith({ due: null, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    expect(warning(w).exists()).toBe(true)
    expect(warning(w).text()).not.toMatch(/^task\.recur\./) // a missing key falls back to its path
    expect(highlighted(w)).toEqual([])
  })

  it('stays quiet when a frequency is picked on an undated task', async () => {
    // `setFreq` anchors an unset due to today at 00:00 precisely so the fresh rule
    // has something to advance from — the one path into the dead state that is
    // already closed. Warning here would be crying wolf at the editor's own fix.
    const w = mountWith({ due: null, columns: COLUMNS, recurrence: null })
    await nextTick()
    expect(warning(w).exists()).toBe(false) // no rule yet: nothing to warn about
    editor(w).vm.setFreq('weekly')
    await nextTick()
    expect(warning(w).exists()).toBe(false)
    expect(editor(w).vm.localDue).not.toBe(null)
  })

  it('warns for every trigger, not just the clock-driven one', async () => {
    for (const trigger of ['complete', 'column', 'schedule']) {
      const w = mountWith({
        due: null,
        columns: COLUMNS,
        recurrence: { ...RULE, trigger, trigger_column: trigger === 'column' ? TODO : undefined },
      })
      await nextTick()
      expect(warning(w).exists()).toBe(true)
      w.unmount()
    }
    wrapper = null
  })

  it('clears once a date is picked in the grid', async () => {
    const w = mountWith({ due: null, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    const cell = w.findAll('.de-day').find((b) => !b.classes('out') && b.text() === '10')
    await cell.trigger('click')
    await nextTick()
    expect(warning(w).exists()).toBe(false)
    expect(highlighted(w).length).toBeGreaterThan(0)
  })

  it('appears when «Очистить срок» is pressed under a live rule', async () => {
    // The way a user actually reaches the dead state: the rule is set (and the
    // editor gave it a date of its own), then the date is cleared and the repeat
    // stays on the card, silently dead.
    const w = mountWith({ due: DUE, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    expect(warning(w).exists()).toBe(false)
    const clear = w.findAll('.de-link').find((b) => b.text() === 'Очистить срок')
    await clear.trigger('click')
    await nextTick()
    expect(warning(w).exists()).toBe(true)
    expect(highlighted(w)).toEqual([])
  })
})

// ── negative control: a dated rule keeps its preview and says nothing ──
describe('a dated repeat is untouched', () => {
  it('previews the daily series with no warning', async () => {
    const w = mountWith({ due: DUE, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    expect(warning(w).exists()).toBe(false)
    // 24 daily occurrences from Fri 10 Jul — the rest of July, then August.
    expect(highlighted(w)).toEqual(Array.from({ length: 21 }, (_, i) => 11 + i))
  })
})
