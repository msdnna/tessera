import { useI18n } from 'vue-i18n'
import { useFormat } from './useFormat'
import { distance, recordState } from '@/utils/inbox'

// Labels of «Входящие» records (#2972), in the reader's calendar and language.
export function useInboxFormat() {
  const { t } = useI18n()
  const { formatDate, formatTime, formatDue, formatRelative } = useFormat()

  // The capture's date chip: «завтра, ср 7 окт. · 10:00».
  function whenLong(at, now = new Date()) {
    if (!at) return ''
    const rel = formatRelative(at, { now, dateOnly: false })
    const day = formatDate(at, { weekday: 'short', day: 'numeric', month: 'short' })
    const time = formatTime(at)
    return `${rel ? `${rel}, ` : ''}${day} · ${time}`
  }

  // The time on a row: today's — the clock; later — «ср, 14:00» / «12 окт.».
  function whenShort(at, now = new Date()) {
    if (!at) return ''
    const d = new Date(at)
    const sameDay =
      d.getFullYear() === now.getFullYear() &&
      d.getMonth() === now.getMonth() &&
      d.getDate() === now.getDate()
    if (sameDay) return formatTime(at)
    const label = formatDue(at, { now })
    // Beyond this week the day alone reads better than «12 окт., 09:00».
    return label.includes(',') && !formatRelative(at, { now, dateOnly: false })
      ? label.split(',')[0]
      : label
  }

  // «40 мин назад» / «через 1 ч 20 м».
  function ago(at, now = new Date()) {
    const d = distance(at, now)
    const dir = d.past ? 'past' : 'future'
    if (d.key === 'now') return t('inbox.dist.now')
    return t(`inbox.dist.${dir}.${d.key}`, { n: d.n, h: d.h, m: d.m })
  }

  // The line under the time: how far a fired or upcoming reminder is.
  function hint(r, now = new Date()) {
    const st = recordState(r, now)
    if (st === 'fired') return ago(r.remind_at, now)
    if (st === 'upcoming') {
      const d = distance(r.remind_at, now)
      return d.key === 'min' || d.key === 'hm' || d.key === 'h' ? ago(r.remind_at, now) : ''
    }
    return ''
  }

  // A note's age on the right: «вчера», «пн», «12 окт.».
  function noteWhen(n, now = new Date()) {
    return (
      formatRelative(n.updated_at, { now, dateOnly: false }) ||
      formatDate(n.updated_at, { day: 'numeric', month: 'short' })
    )
  }

  return { whenLong, whenShort, ago, hint, noteWhen, formatTime }
}
