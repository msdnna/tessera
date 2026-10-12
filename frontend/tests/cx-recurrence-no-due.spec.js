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
import { nextTick } from 'vue'
import { loadPicker, mountPicker, highlighted, deadWarning } from './helpers/duePicker'
import { setActivePinia, createPinia } from 'pinia'
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

const DuePicker = await loadPicker()

const TODO = '11111111-1111-1111-1111-111111111111'
const COLUMNS = [{ id: TODO, name: 'К работе' }]
const DUE = new Date(2026, 6, 10, 9, 0).getTime() // Fri 10 Jul 2026
const RULE = { freq: 'daily', interval: 1, trigger: 'complete', tz: 'Europe/Moscow' }

let wrapper = null

function mountWith(props) {
  wrapper = mountPicker(DuePicker, props)
  return wrapper
}

const model = (w) => w.findComponent(DuePicker).vm.model
const warning = (w) => deadWarning(w, DuePicker)

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

describe('the picker says why an undated repeat will not fire', () => {
  it('warns when the rule is opened on a task with no due date', async () => {
    const w = mountWith({ due: null, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    const text = await warning(w)
    expect(text).toBeTruthy()
    expect(text).not.toMatch(/^task\./) // a missing key falls back to its path
    expect(highlighted()).toEqual([])
  })

  it('stays quiet when a frequency is picked on an undated task', async () => {
    // `setFreq` anchors an unset due to today at 00:00 precisely so the fresh rule
    // has something to advance from — the one path into the dead state that is
    // already closed. Warning here would be crying wolf at the editor's own fix.
    const w = mountWith({ due: null, columns: COLUMNS, recurrence: null })
    await nextTick()
    expect(await warning(w)).toBe(null) // no rule yet: nothing to warn about
    model(w).setFreq('weekly')
    await nextTick()
    expect(await warning(w)).toBe(null)
    expect(model(w).due).not.toBe(null)
  })

  it('warns for every trigger, not just the clock-driven one', async () => {
    for (const trigger of ['complete', 'column', 'schedule']) {
      const w = mountWith({
        due: null,
        columns: COLUMNS,
        recurrence: { ...RULE, trigger, trigger_column: trigger === 'column' ? TODO : undefined },
      })
      await nextTick()
      expect(await warning(w)).toBeTruthy()
      w.unmount()
    }
    wrapper = null
  })

  it('clears once a date is picked in the grid', async () => {
    const w = mountWith({ due: null, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    const cell = [...document.querySelectorAll('[data-testid="due-cal"] .dc-day')].find(
      (b) => !b.classList.contains('out') && b.textContent.trim() === '10',
    )
    cell.click()
    await nextTick()
    expect(await warning(w)).toBe(null)
    expect(highlighted().length).toBeGreaterThan(0)
  })

  it('appears when «Очистить срок» is pressed under a live rule', async () => {
    // The way a user actually reaches the dead state: the rule is set (and the
    // editor gave it a date of its own), then the date is cleared and the repeat
    // stays on the card, silently dead.
    const w = mountWith({ due: DUE, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    expect(await warning(w)).toBe(null)
    document.querySelector('[data-testid="due-clear"]').click()
    await nextTick()
    expect(await warning(w)).toBeTruthy()
    expect(highlighted()).toEqual([])
  })
})

// ── negative control: a dated rule keeps its preview and says nothing ──
describe('a dated repeat is untouched', () => {
  it('previews the daily series with no warning', async () => {
    const w = mountWith({ due: DUE, columns: COLUMNS, recurrence: RULE })
    await nextTick()
    expect(await warning(w)).toBe(null)
    // 24 daily occurrences from Fri 10 Jul — the rest of July, then August.
    expect(highlighted()).toEqual(Array.from({ length: 21 }, (_, i) => 11 + i))
  })
})
