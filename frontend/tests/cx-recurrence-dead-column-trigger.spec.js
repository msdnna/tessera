// A column-triggered recurrence only ever advances when the task is moved into
// the column named in `trigger_column`: the server compares the two uuids
// verbatim (`rule.TriggerColumn == columnID.String()`, backend/handlers/task_ops.go)
// and nothing else fires the rule. So a rule whose trigger column is unset — the
// select opens on its placeholder, and picking the event alone already commits —
// or points at a column that is no longer on the board matches nothing, ever.
//
// It is still stored, still drawn as a live repeat, and the editor's calendar
// still highlighted its upcoming occurrences: the same «the preview promises days
// the server will never fire on» the skip-weekends and anchor fixes were about
// (utils/recurrence.js), except here *no* day is ever going to come (#2932).
import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NMessageProvider } from 'naive-ui'
import { triggerColumnMissing } from '@/utils/recurrence'

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
const DOING = '22222222-2222-2222-2222-222222222222'
const GONE = '99999999-9999-9999-9999-999999999999'
const COLUMNS = [
  { id: TODO, name: 'К работе' },
  { id: DOING, name: 'В процессе' },
]
const DUE = new Date(2026, 6, 10, 9, 0).getTime() // Fri 10 Jul 2026

let wrapper = null

function mountWith(props) {
  wrapper = mount(
    { render: () => h(NMessageProvider, null, { default: () => h(DueEditor, props) }) },
    { attachTo: document.body },
  )
  return wrapper
}

const editor = (w) => w.findComponent(DueEditor)

// Day numbers the grid highlights as *upcoming* occurrences in the month in view.
function highlighted(w) {
  return w
    .findAll('.de-day')
    .filter((b) => b.classes('occ') && !b.classes('out'))
    .map((b) => Number(b.text()))
    .sort((a, b) => a - b)
}

const warning = (w) => w.find('.de-warn')

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('triggerColumnMissing', () => {
  it('flags a column rule with no column named', () => {
    expect(triggerColumnMissing({ freq: 'daily', trigger: 'column' }, [TODO])).toBe(true)
    expect(triggerColumnMissing({ freq: 'daily', trigger: 'column', trigger_column: '' }, [])).toBe(
      true,
    )
  })

  it('flags a column that is not on the board any more', () => {
    expect(
      triggerColumnMissing({ freq: 'daily', trigger: 'column', trigger_column: GONE }, [
        TODO,
        DOING,
      ]),
    ).toBe(true)
  })

  it('stays quiet for a column that is on the board', () => {
    expect(
      triggerColumnMissing({ freq: 'daily', trigger: 'column', trigger_column: TODO }, [
        TODO,
        DOING,
      ]),
    ).toBe(false)
  })

  it('stays quiet for the other triggers and for no rule at all', () => {
    expect(triggerColumnMissing({ freq: 'daily', trigger: 'complete' }, [])).toBe(false)
    expect(triggerColumnMissing({ freq: 'daily', trigger: 'schedule' }, [])).toBe(false)
    expect(triggerColumnMissing({ freq: 'daily' }, [])).toBe(false)
    expect(triggerColumnMissing(null, [TODO])).toBe(false)
    expect(triggerColumnMissing({}, [TODO])).toBe(false)
  })

  it('does not cry wolf when the column list is unknown', () => {
    // The editor can be opened without a column list (`columns` defaults to []).
    // "I cannot see the board" is not "the column is gone" — only an unnamed
    // column is knowable then.
    expect(
      triggerColumnMissing({ freq: 'daily', trigger: 'column', trigger_column: TODO }, []),
    ).toBe(false)
    expect(
      triggerColumnMissing({ freq: 'daily', trigger: 'column', trigger_column: TODO }, undefined),
    ).toBe(false)
  })
})

describe('the editor does not preview a rule that can never fire', () => {
  it('highlights nothing for a column rule with no column picked', async () => {
    const w = mountWith({
      due: DUE,
      columns: COLUMNS,
      recurrence: { freq: 'daily', interval: 1, trigger: 'column', tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(highlighted(w)).toEqual([])
  })

  it('says why, instead of leaving the panel looking complete', async () => {
    const w = mountWith({
      due: DUE,
      columns: COLUMNS,
      recurrence: { freq: 'daily', interval: 1, trigger: 'column', tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(warning(w).exists()).toBe(true)
    expect(warning(w).text()).not.toMatch(/^task\.recur\./) // missing key falls back to the path
  })

  it('warns about a trigger column that has been deleted from the board', async () => {
    const w = mountWith({
      due: DUE,
      columns: COLUMNS,
      recurrence: {
        freq: 'daily',
        interval: 1,
        trigger: 'column',
        trigger_column: GONE,
        tz: 'Europe/Moscow',
      },
    })
    await nextTick()
    expect(highlighted(w)).toEqual([])
    expect(warning(w).exists()).toBe(true)
  })

  it('goes dark the moment the event is switched to «в колонку»', async () => {
    // Picking the event commits the rule straight away — there is no Save to
    // withhold — so this is the state a user actually lands in.
    const w = mountWith({
      due: DUE,
      columns: COLUMNS,
      recurrence: { freq: 'daily', interval: 1, trigger: 'complete', tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(highlighted(w).length).toBeGreaterThan(0)
    editor(w).vm.trigger = 'column'
    await nextTick()
    expect(highlighted(w)).toEqual([])
    expect(warning(w).exists()).toBe(true)
  })

  it('lights back up once a column is picked', async () => {
    const w = mountWith({
      due: DUE,
      columns: COLUMNS,
      recurrence: { freq: 'daily', interval: 1, trigger: 'column', tz: 'Europe/Moscow' },
    })
    await nextTick()
    editor(w).vm.triggerColumn = DOING
    await nextTick()
    // 24 daily occurrences from Fri 10 Jul — the rest of July, then August.
    expect(highlighted(w)).toEqual(Array.from({ length: 21 }, (_, i) => 11 + i))
    expect(warning(w).exists()).toBe(false)
  })
})

// ── negative controls: every rule that CAN fire keeps its preview ──
describe('rules that do fire are untouched', () => {
  it('previews a column rule aimed at a live column', async () => {
    const w = mountWith({
      due: DUE,
      columns: COLUMNS,
      recurrence: {
        freq: 'daily',
        interval: 1,
        trigger: 'column',
        trigger_column: TODO,
        tz: 'Europe/Moscow',
      },
    })
    await nextTick()
    expect(highlighted(w).length).toBeGreaterThan(0)
    expect(warning(w).exists()).toBe(false)
  })

  it('previews completion- and schedule-triggered rules', async () => {
    for (const trigger of ['complete', 'schedule']) {
      const w = mountWith({
        due: DUE,
        columns: COLUMNS,
        recurrence: { freq: 'weekly', interval: 1, trigger, tz: 'Europe/Moscow' },
      })
      await nextTick()
      expect(highlighted(w)).toEqual([17, 24, 31])
      expect(warning(w).exists()).toBe(false)
      w.unmount()
      wrapper = null
    }
  })

  it('keeps previewing when the editor was opened without a column list', async () => {
    const w = mountWith({
      due: DUE,
      recurrence: {
        freq: 'daily',
        interval: 1,
        trigger: 'column',
        trigger_column: TODO,
        tz: 'Europe/Moscow',
      },
    })
    await nextTick()
    expect(highlighted(w).length).toBeGreaterThan(0)
    expect(warning(w).exists()).toBe(false)
  })
})
