import { useFormat } from '@/composables/useFormat'

// Naive UI date-picker props + the compact due label, kept as a separate entry
// point so the 15+ components already calling it don't have to change.
//
// Since #2798 this is a thin adapter over useFormat: the formatting itself
// (locale, timezone, 12/24h, relative wording) lives in utils/format.js. New
// code should call useFormat() directly.
export function useDateLocale() {
  const { firstDayOfWeek, datePattern, dateTimePattern, formatDue } = useFormat()

  return {
    firstDayOfWeek,
    // Naive pickers still speak date-fns patterns; the preset is translated for
    // them, so picker and rendered text keep the same field order.
    dateFormat: datePattern,
    dateTimeFormat: dateTimePattern,
    formatDue,
  }
}
