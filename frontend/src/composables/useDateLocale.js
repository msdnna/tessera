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
  return { firstDayOfWeek, dateFormat, dateTimeFormat }
}
