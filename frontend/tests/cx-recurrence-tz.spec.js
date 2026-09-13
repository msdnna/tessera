// A recurrence rule has to carry the zone its calendar was drawn in (#2932).
//
// Every field of the rule names a calendar day — a weekday, a day of the month, a
// hand-picked date, "not on a weekend" — but the due date leaves the editor as an
// instant, and a freshly picked date defaults to *local* midnight. The backend used
// to re-read those days in its own zone (UTC), so for anyone east of UTC the whole
// series came out a day off the days the editor's own calendar highlighted: "every
// Monday" fired on Tuesdays, skip-weekends skipped Monday instead of Saturday.
//
// format.js already states the contract — anything the server evaluates against a
// clock has to be handed the resolved zone, not the blank. This pins that the rule
// the editor emits carries it.
//
// Which zone, though, is not free: it has to be the one the grid above the rule is
// drawn in. The whole editor builds its instants from LOCAL calendar fields
// (`new Date(y, m, d)`) and labels them with `timeZone: null` on purpose (#2798) —
// the day the user clicks is a *browser* day, and the due date leaves as browser
// midnight. Handing the server the profile zone instead made the rule name days off
// a calendar nobody drew: for a profile zone west of the browser's, browser midnight
// is still the previous day there, so `WithAnchor` read a monthly rule picked on the
// 6th as the 5th while the editor kept highlighting the 6th.
import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NMessageProvider } from 'naive-ui'

// The browser zone is pinned so "profile zone ≠ browser zone" is expressible at
// all; Europe/Moscow (UTC+3) sits east of the profile zones used below, which is
// what makes local midnight fall on the previous day over there.
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
const { useThemeStore } = await import('@/stores/theme')
const { resolvedTimezone } = await import('@/utils/format')

let wrapper = null

function mountWith(props) {
  wrapper = mount(
    { render: () => h(NMessageProvider, null, { default: () => h(DueEditor, props) }) },
    { attachTo: document.body },
  )
  return wrapper
}

const editor = (w) => w.findComponent(DueEditor)

// The last `apply` payload the editor emitted (due_date + start_date + recurrence).
function lastApply(w) {
  const events = editor(w).emitted('apply') || []
  return events.length ? events[events.length - 1][0] : null
}
function lastRule(w) {
  return lastApply(w)?.recurrence ?? null
}

// What the backend's `WithAnchor` derives from that payload: it reads the due
// instant in the rule's own zone (`due.In(r.loc())`, recur.go) and takes the day.
function anchorDay(apply) {
  return Number(
    new Intl.DateTimeFormat('en-GB', {
      timeZone: apply.recurrence.tz,
      day: 'numeric',
    }).format(new Date(apply.due_date)),
  )
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  // An un-unmounted component keeps its watchers on the shared theme store and
  // silently poisons the next spec's scheduler.
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('recurrence rule timezone', () => {
  it('carries the zone into the rule when profile and browser agree', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Europe/Moscow'
    const w = mountWith({ due: Date.UTC(2026, 3, 5, 21, 0), recurrence: null })
    await nextTick()
    editor(w).vm.setFreq('weekly')
    await nextTick()
    expect(lastRule(w)).toMatchObject({ freq: 'weekly', tz: 'Europe/Moscow' })
  })

  it('names the grid zone, not the profile zone, when the two differ', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Europe/London' // west of the browser's Europe/Moscow
    const w = mountWith({ due: Date.UTC(2026, 3, 5, 21, 0), recurrence: null })
    await nextTick()
    editor(w).vm.setFreq('weekly')
    await nextTick()
    // The weekday list the panel offers is read off the browser-local grid, so the
    // zone shipped with it has to be that grid's.
    expect(lastRule(w).tz).toBe('Europe/Moscow')
  })

  it('keeps a monthly anchor on the day that was clicked', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Europe/London'
    const w = mountWith({ due: null, recurrence: null })
    await nextTick()
    editor(w).vm.setFreq('monthly')
    await nextTick()
    editor(w).vm.onDayClick(new Date(2026, 6, 6)) // Mon 6 Jul 2026, as drawn
    await nextTick()
    expect(anchorDay(lastApply(w))).toBe(6)
  })

  it('falls back to the browser zone when no profile zone is set', async () => {
    const theme = useThemeStore()
    theme.timezone = ''
    const browser = resolvedTimezone('')
    const w = mountWith({ due: Date.UTC(2026, 3, 5, 21, 0), recurrence: null })
    await nextTick()
    editor(w).vm.setFreq('daily')
    await nextTick()
    // An environment with no Intl data resolves to '' — there the rule may omit it.
    if (browser) expect(lastRule(w).tz).toBe(browser)
    else expect(lastRule(w).tz).toBeUndefined()
  })

  it('re-stamps a rule loaded from elsewhere with the zone it is edited in', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Asia/Vladivostok'
    const w = mountWith({
      due: Date.UTC(2026, 3, 5, 14, 0),
      recurrence: { freq: 'weekly', interval: 1, weekdays: [1], tz: 'Asia/Vladivostok' },
    })
    await nextTick()
    editor(w).vm.toggleWeekday(3)
    await nextTick()
    const rule = lastRule(w)
    // The new weekday was picked off this browser's grid, so the list as a whole now
    // describes this browser's days — the stored zone has to follow it.
    expect(rule.weekdays).toEqual([1, 3])
    expect(rule.tz).toBe('Europe/Moscow')
  })
})
