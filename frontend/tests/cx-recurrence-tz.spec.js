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
// drawn in. The due picker (#3118) draws its grid in the reader's PROFILE zone —
// the zone every label of a due is read in — and a date-only due (a pure UTC
// midnight) is a UTC day. So a timed due ships the profile zone, a date-only one
// 'UTC' (the server reads the due in the rule's zone to find its day: a UTC
// midnight read in New York would be the evening before). The old DueEditor drew
// a browser-local grid and shipped the browser zone; with the grid gone, so is
// the mismatch between a profile zone and the browser's.
import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { nextTick } from 'vue'
import { loadPicker, mountPicker, lastApply as lastApplyOf, day } from './helpers/duePicker'
import { setActivePinia, createPinia } from 'pinia'

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

const DuePicker = await loadPicker()
const { useThemeStore } = await import('@/stores/theme')
const { resolvedTimezone } = await import('@/utils/format')

let wrapper = null

function mountWith(props) {
  wrapper = mountPicker(DuePicker, props)
  return wrapper
}

const model = (w) => w.findComponent(DuePicker).vm.model
const lastApply = (w) => lastApplyOf(w, DuePicker)
const lastRule = (w) => lastApply(w)?.recurrence ?? null

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
    model(w).setFreq('weekly')
    await nextTick()
    expect(lastRule(w)).toMatchObject({ freq: 'weekly', tz: 'Europe/Moscow' })
  })

  it('names the grid zone — the profile one — when profile and browser differ', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Europe/London' // west of the browser's Europe/Moscow
    const w = mountWith({ due: Date.UTC(2026, 3, 5, 12, 0), recurrence: null })
    await nextTick()
    model(w).setFreq('weekly')
    await nextTick()
    // The grid and every label of the due are read in the profile zone, so the
    // weekdays the panel offers are that zone's days.
    expect(lastRule(w).tz).toBe('Europe/London')
  })

  it('ships UTC for a date-only due — its day is a UTC day', async () => {
    const theme = useThemeStore()
    theme.timezone = 'America/New_York'
    const w = mountWith({ due: Date.UTC(2026, 3, 6), recurrence: null })
    await nextTick()
    model(w).setFreq('weekly')
    await nextTick()
    expect(lastRule(w).tz).toBe('UTC')
    expect(anchorDay(lastApply(w))).toBe(6)
  })

  it('keeps a monthly anchor on the day that was clicked', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Europe/London'
    const w = mountWith({ due: null, recurrence: null })
    await nextTick()
    model(w).setFreq('monthly')
    await nextTick()
    model(w).pickDay(day(2026, 6, 6)) // Mon 6 Jul 2026, as drawn
    await nextTick()
    expect(anchorDay(lastApply(w))).toBe(6)
  })

  it('falls back to the browser zone when no profile zone is set', async () => {
    const theme = useThemeStore()
    theme.timezone = ''
    const browser = resolvedTimezone('')
    const w = mountWith({ due: Date.UTC(2026, 3, 5, 21, 0), recurrence: null })
    await nextTick()
    model(w).setFreq('daily')
    await nextTick()
    // An environment with no Intl data resolves to '' — there the rule may omit it.
    if (browser) expect(lastRule(w).tz).toBe(browser)
    else expect(lastRule(w).tz).toBeUndefined()
  })

  it('re-stamps a rule loaded from elsewhere with the zone it is edited in', async () => {
    const theme = useThemeStore()
    theme.timezone = '' // the browser's Europe/Moscow
    const w = mountWith({
      due: Date.UTC(2026, 3, 5, 14, 0),
      recurrence: { freq: 'weekly', interval: 1, weekdays: [1], tz: 'Asia/Vladivostok' },
    })
    await nextTick()
    model(w).toggleWeekday(3)
    await nextTick()
    const rule = lastRule(w)
    // The new weekday was picked off this reader's grid, so the list as a whole now
    // describes this reader's days — the stored zone has to follow it.
    expect(rule.weekdays).toEqual([1, 3])
    expect(rule.tz).toBe('Europe/Moscow')
  })
})
