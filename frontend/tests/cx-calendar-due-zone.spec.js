// The calendar layout must put a task on the day the rest of the app says it is
// due (#2932).
//
// The month grid indexed tasks by `new Date(due).getDate()` — the BROWSER's
// calendar. Every other reading of a due date goes through the formatter set
// (utils/format), which has two rules the browser knows nothing about:
//
//   * a due carrying a time of day is read in the user's timezone preference;
//   * a pure UTC midnight is a date-only value (a GitLab issue due, an all-day
//     due) and is read in UTC in every zone — pitfall 7 of #2796.
//
// So the very same card could sit on the 15th in the calendar layout while its
// own due pill, the «Сегодня» facet and utils/taskFilter all said the 16th.
//
// Both rules are pinned here, at the level a user reads them off the screen:
// which grid cell the chip lands in.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api', () => ({
  tasks: {},
  getAccessToken: () => null,
}))

const BoardCalendarView = (await import('@/components/BoardCalendarView.vue')).default
const { useThemeStore } = await import('@/stores/theme')

// UTC+14, the furthest zone there is — far enough from any plausible runner to
// make "the profile's day" and "the device's day" provably different values.
const ZONE = 'Pacific/Kiritimati'

let wrapper = null

// The calendar opens on the user's current month, which in ZONE may already be
// the next one — so the fixture dates are built inside THAT month rather than
// inside the browser's.
function monthIn(zone) {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date())
  const get = (type) => Number(parts.find((p) => p.type === type).value)
  return { year: get('year'), month: get('month') - 1 }
}

function dayIn(zone, date) {
  return Number(new Intl.DateTimeFormat('en-US', { timeZone: zone, day: 'numeric' }).format(date))
}

function mountCalendar(tasks) {
  wrapper = mount(BoardCalendarView, { props: { tasks, statusColumns: [] } })
  return wrapper
}

// The cell of the displayed month carrying `day`. Trailing/leading cells of the
// neighbouring months are excluded, so a day in the middle of a month matches
// exactly one cell.
function cellFor(w, day) {
  const cells = w
    .findAll('.cal-cell')
    .filter((c) => !c.classes('out') && c.find('.cal-daynum').text() === String(day))
  expect(cells).toHaveLength(1)
  return cells[0]
}

function chipsOn(w, day) {
  return cellFor(w, day)
    .findAll('.cal-chip')
    .map((b) => b.text())
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

describe('BoardCalendarView due-date placement', () => {
  it('puts a timed due on the day the profile timezone sees, not the device', async () => {
    const { year, month } = monthIn(ZONE)
    // Midday UTC: the 15th for every runner west of UTC+12 (the guard below
    // says so out loud), already the 16th in ZONE.
    const due = new Date(Date.UTC(year, month, 15, 12, 0, 0))
    expect(dayIn(ZONE, due)).toBe(16)
    expect(due.getDate()).toBe(15) // guard: the device really does disagree

    const theme = useThemeStore()
    theme.language = 'ru'
    theme.timezone = ZONE
    const w = mountCalendar([
      { id: 't1', title: 'Сдать отчёт', due_date: due.toISOString(), priority: 0 },
    ])
    await nextTick()

    expect(chipsOn(w, 16)).toEqual(['Сдать отчёт'])
    expect(chipsOn(w, 15)).toEqual([])
  })

  it('keeps a date-only due on its UTC day whatever the profile timezone says', async () => {
    const { year, month } = monthIn(ZONE)
    // Pure UTC midnight — how a GitLab issue due and an all-day due are stored.
    // Reading it in the profile zone would move it to the 16th; reading it in
    // the browser's would move it to the 14th for anyone west of UTC. It is the
    // 15th for everyone, which is the whole point of the date-only rule.
    const due = new Date(Date.UTC(year, month, 15, 0, 0, 0))

    const theme = useThemeStore()
    theme.language = 'ru'
    theme.timezone = ZONE
    const w = mountCalendar([{ id: 't2', title: 'Релиз', due_date: due.toISOString() }])
    await nextTick()

    expect(chipsOn(w, 15)).toEqual(['Релиз'])
    expect(chipsOn(w, 16)).toEqual([])
    expect(chipsOn(w, 14)).toEqual([])
  })
})
