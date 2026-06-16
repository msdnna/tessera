import { computed } from 'vue'
import { useThemeStore } from '@/stores/theme'

// Maps the user's localizing preferences onto Naive UI date-picker props so
// calendars follow the chosen week-start and date/time format.
export function useDateLocale() {
  const theme = useThemeStore()
  // Naive firstDayOfWeek: 0 = Monday … 6 = Sunday. Our week_start: 1 = Mon, 0 = Sun.
  const firstDayOfWeek = computed(() => (theme.weekStart === 0 ? 6 : theme.weekStart - 1))
  const dateFormat = computed(() => theme.dateFormat || 'dd.MM.yyyy')
  const dateTimeFormat = computed(
    () => `${dateFormat.value} ${theme.timeFormat === '12h' ? 'hh:mm a' : 'HH:mm'}`,
  )

  // formatDue renders a task due date compactly: day + month (+ year when not the
  // current one), and the time only when it isn't midnight — so date-only tasks
  // (and legacy rows, which default to 00:00) stay terse while timed ones show the
  // hour. Honours the user's 12h/24h preference.
  function formatDue(dateStr) {
    if (!dateStr) return ''
    const d = new Date(dateStr)
    const locale = theme.language === 'en' ? 'en-GB' : 'ru-RU'
    const opts = { day: '2-digit', month: 'short' }
    if (d.getFullYear() !== new Date().getFullYear()) opts.year = 'numeric'
    if (d.getHours() !== 0 || d.getMinutes() !== 0) {
      opts.hour = '2-digit'
      opts.minute = '2-digit'
      opts.hour12 = theme.timeFormat === '12h'
      return d.toLocaleString(locale, opts)
    }
    return d.toLocaleDateString(locale, opts)
  }

  return { firstDayOfWeek, dateFormat, dateTimeFormat, formatDue }
}
