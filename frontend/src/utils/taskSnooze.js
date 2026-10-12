// «Отложить» a task (#3108, PackageG MGHome — swipe left on «Мои задачи»). A task
// has no «snoozed» state of its own, so putting it off means moving its due date:
// to tomorrow, the day after, the coming Monday or a week from today. A due with
// a clock keeps its clock; a date-only due (or none) stays date-only — the same
// kinds the due picker writes (utils/dueModel).
import { addDays, coordsOf, readEndpoint, wallNow, writeEndpoint } from './dueModel'

export const TASK_SNOOZE_KEYS = ['tomorrow', 'dayAfter', 'monday', 'week']

// The days the options lead to, counted from today's wall date in the profile.
export function taskSnoozeDays(fmt, now = new Date()) {
  const today = coordsOf(wallNow(fmt, now))
  const dow = new Date(today.year, today.month, today.day).getDay()
  const toMonday = (8 - dow) % 7 || 7
  return [
    { key: 'tomorrow', day: addDays(today, 1) },
    { key: 'dayAfter', day: addDays(today, 2) },
    { key: 'monday', day: addDays(today, toMonday) },
    { key: 'week', day: addDays(today, 7) },
  ]
}

// The new due_date for a task put off to `day`.
export function snoozedDue(dueIso, day, fmt, tz) {
  const time = readEndpoint(dueIso, fmt)?.time || null
  return writeEndpoint(day, time, tz)
}
