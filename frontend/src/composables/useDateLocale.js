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

  // The Monday-anchored epoch-day of the week a given epoch-day belongs to, using
  // the user's week start (0 = Sun, 1 = Mon). Two dates in the same week share it.
  function weekStartDay(epochDay, firstDay) {
    const dow = new Date(epochDay * 86400000).getUTCDay() // 0 = Sun … 6 = Sat
    const offset = (dow - firstDay + 7) % 7
    return epochDay - offset
  }

  // relativeDay returns "Сегодня"/"Завтра"/"Вчера" (or the localized weekday when
  // the date is elsewhere in the current week), else '' to fall back to an
  // absolute date. y/mo/day are the due's calendar components.
  const cap = (s) => (s ? s.charAt(0).toUpperCase() + s.slice(1) : s)
  // `long` = capitalised words + full weekday names (used outside pills, where the
  // lowercase abbreviated form would clash with capitalised siblings like priority).
  function relativeDay(y, mo, day, now, locale, long = false) {
    const dueEpoch = Math.round(Date.UTC(y, mo, day) / 86400000)
    const todayEpoch = Math.round(
      Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) / 86400000,
    )
    const diff = dueEpoch - todayEpoch
    const en = locale.startsWith('en')
    if (diff === 0) return en ? 'Today' : long ? 'Сегодня' : 'сегодня'
    if (diff === 1) return en ? 'Tomorrow' : long ? 'Завтра' : 'завтра'
    if (diff === -1) return en ? 'Yesterday' : long ? 'Вчера' : 'вчера'
    const firstDay = theme.weekStart === 0 ? 0 : 1
    if (
      Math.abs(diff) <= 6 &&
      weekStartDay(dueEpoch, firstDay) === weekStartDay(todayEpoch, firstDay)
    ) {
      // Short lowercase weekday in pills ("пн"); full capitalised weekday elsewhere
      // ("Понедельник") so it doesn't read as an abbreviation next to other fields.
      const wd = new Date(dueEpoch * 86400000).toLocaleDateString(locale, {
        weekday: long ? 'long' : 'short',
        timeZone: 'UTC',
      })
      return long ? cap(wd) : wd
    }
    return ''
  }

  // formatDue renders a task due date compactly. Near dates read as relative
  // shorthand ("Завтра", or a weekday within the current week); otherwise day +
  // month (+ year when not the current one). The time is appended only when the
  // due carries a real time-of-day — so date-only tasks (and legacy 00:00 rows)
  // stay terse while timed ones show the hour. Honours the 12h/24h preference.
  function formatDue(dateStr, { long = false } = {}) {
    if (!dateStr) return ''
    const d = new Date(dateStr)
    const locale = theme.language === 'en' ? 'en-GB' : 'ru-RU'
    const now = new Date()
    // A pure UTC-midnight value is a date-only due (e.g. a GitLab issue/milestone
    // date — no time of day). Read its calendar day in UTC so the local-tz offset
    // doesn't push it to "03:00". Manual dues anchor to local midnight (a non-zero
    // UTC time), so their calendar day and time come from the local components.
    const dateOnly = d.getUTCHours() === 0 && d.getUTCMinutes() === 0 && d.getUTCSeconds() === 0
    const y = dateOnly ? d.getUTCFullYear() : d.getFullYear()
    const mo = dateOnly ? d.getUTCMonth() : d.getMonth()
    const day = dateOnly ? d.getUTCDate() : d.getDate()

    let time = ''
    if (!dateOnly && (d.getHours() !== 0 || d.getMinutes() !== 0)) {
      time = d.toLocaleTimeString(locale, {
        hour: '2-digit',
        minute: '2-digit',
        hour12: theme.timeFormat === '12h',
      })
    }

    const rel = relativeDay(y, mo, day, now, locale, long)
    if (rel) return time ? `${rel}, ${time}` : rel

    const o = dateOnly
      ? { day: '2-digit', month: 'short', timeZone: 'UTC' }
      : { day: '2-digit', month: 'short' }
    if (y !== now.getFullYear()) o.year = 'numeric'
    const dateLabel = cap(d.toLocaleDateString(locale, o))
    return time ? `${dateLabel}, ${time}` : dateLabel
  }

  return { firstDayOfWeek, dateFormat, dateTimeFormat, formatDue }
}
