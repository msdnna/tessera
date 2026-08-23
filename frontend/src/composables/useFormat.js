import { computed } from 'vue'
import { useThemeStore } from '@/stores/theme'
import {
  createFormatters,
  datePattern,
  dateTimePattern,
  datePresetSamples,
  timePattern,
} from '@/utils/format'

// Store-bound view over utils/format: one place where the user's language,
// timezone, 12/24h and date-preset preferences meet Intl (#2798, stage 2 of #2796).
//
// Components should import this, never utils/format directly — the pure module
// exists for tests and for the theme store, which cannot depend on itself.
//
// Every returned function reads `f.value` when it is CALLED, not when it is
// destructured, so a render that calls formatDate() tracks the preference refs
// and re-runs when the user changes a setting mid-session.
export function useFormat() {
  const theme = useThemeStore()

  const prefs = computed(() => ({
    language: theme.language,
    timezone: theme.timezone,
    timeFormat: theme.timeFormat,
    dateFormat: theme.dateFormat,
    weekStart: theme.weekStart,
  }))

  const f = computed(() => createFormatters(prefs.value))

  return {
    // The whole formatter set, for pure helpers in utils/ that render dates but
    // must not depend on Pinia (taskFeed, milestones, estimation): the component
    // hands them `formatters` and they stay testable with a synthetic one.
    formatters: f,
    locale: computed(() => f.value.locale),
    timeZone: computed(() => f.value.timeZone),

    formatDate: (value, options) => f.value.formatDate(value, options),
    formatTime: (value, options) => f.value.formatTime(value, options),
    formatDateTime: (value, options) => f.value.formatDateTime(value, options),
    formatNumber: (value, options) => f.value.formatNumber(value, options),
    formatList: (items, options) => f.value.formatList(items, options),
    formatRelative: (value, options) => f.value.formatRelative(value, options),
    formatDue: (value, options) => f.value.formatDue(value, options),

    // date-fns patterns for Naive UI pickers, which format and parse by pattern
    // rather than by Intl options.
    datePattern: computed(() => datePattern(prefs.value)),
    timePattern: computed(() => timePattern(prefs.value)),
    dateTimePattern: computed(() => dateTimePattern(prefs.value)),
    // Naive firstDayOfWeek: 0 = Monday … 6 = Sunday. Our week_start: 1 = Mon, 0 = Sun.
    firstDayOfWeek: computed(() => (theme.weekStart === 0 ? 6 : theme.weekStart - 1)),
    // Labelled samples for the settings picker, rendered in the current language.
    datePresetOptions: computed(() =>
      datePresetSamples(prefs.value).map((o) => ({ label: o.label, value: o.value })),
    ),
  }
}
