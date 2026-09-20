// «Не по выходным» must not rewrite weekdays the person picked by hand.
//
// The two settings answer the same question, and when they disagreed the skip
// won — and cost the rule its interval doing it. `applySkip` pushed the chosen
// Saturday to Monday, and the server anchors the next step on the *pushed* day:
// one Mon-started week further on, where `nextWeekly` finds that Saturday five
// days out rather than a fortnight. From the second occurrence «каждые 2 недели
// по субботам» was plain weekly Mondays, while the editor's weekday chips kept
// showing Saturday.
//
// The server now drops the flag for a weekly rule that names its weekdays
// (`recur.normalized`, backend/internal/recur/recur.go); this grid mirrors that,
// so the preview shows the series that will actually fire (#2932).
import { describe, it, expect } from 'vitest'
import { nextOccurrence } from '@/utils/recurrence'

const DAY = 24 * 60 * 60 * 1000
const sat = () => new Date(2026, 2, 7, 9, 0, 0) // Saturday 2026-03-07
const fri = () => new Date(2026, 2, 6, 9, 0, 0) // Friday 2026-03-06

describe('weekly + skip weekends', () => {
  it('keeps the chosen weekend day and the interval', () => {
    const rule = { freq: 'weekly', interval: 2, weekdays: [6], skip_weekends: true }
    let cur = sat()
    for (let i = 0; i < 4; i += 1) {
      const next = nextOccurrence(rule, cur)
      expect(next.getDay()).toBe(6)
      expect(next - cur).toBe(14 * DAY)
      cur = next
    }
  })

  it('still pushes off the weekend when the day is derived, not chosen', () => {
    const next = nextOccurrence({ freq: 'weekly', interval: 1, skip_weekends: true }, sat())
    expect(next.getDay()).toBe(1) // Monday
  })

  it('leaves a non-weekend weekday set exactly where it was', () => {
    const rule = { freq: 'weekly', interval: 2, weekdays: [5], skip_weekends: true }
    const next = nextOccurrence(rule, fri())
    expect(next.getDay()).toBe(5)
    expect(next - fri()).toBe(14 * DAY)
  })

  it('leaves daily alone — there is no weekday set to contradict', () => {
    const next = nextOccurrence({ freq: 'daily', interval: 1, skip_weekends: true }, fri())
    expect(next.getDay()).toBe(1) // Monday
  })
})
