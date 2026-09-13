import { useFormat } from '@/composables/useFormat'

// Naive UI date-picker props + the compact due label, kept as a separate entry
// point so the 15+ components already calling it don't have to change.
//
// Since #2798 this is a thin adapter over useFormat: the formatting itself
// (locale, timezone, 12/24h, relative wording) lives in utils/format.js. New
// code should call useFormat() directly.
export function useDateLocale() {
  const { firstDayOfWeek, datePattern, dateTimePattern, formatDue, formatters } = useFormat()

  return {
    firstDayOfWeek,
    // The formatter set rides along with formatDue: a view that prints a due label
    // also decides whether it is red, and both have to read the date in the same
    // calendar (see utils/taskFilter).
    formatters,
    // Naive pickers still speak date-fns patterns; the preset is translated for
    // them, so picker and rendered text keep the same field order.
    dateFormat: datePattern,
    dateTimeFormat: dateTimePattern,
    formatDue,
  }
}
