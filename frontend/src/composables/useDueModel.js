// The editing model behind the due picker (v2, #3118): one task's start + due +
// recurrence rule, edited in wall terms and committed together.
//
// Every change commits at once (as the old DueEditor did): `onApply` gets the
// whole { due_date, start_date, recurrence } — one PUT, so the rule never travels
// without the dates it counts from and never gets dropped by a date edit. An
// endpoint the reader did not touch goes back exactly as it came (the same ISO
// string), so re-sending never moves it.
//
// The calendar is the reader's: days are read in the profile timezone (UTC for a
// date-only due), and the rule carries the zone its days were read in
// (`recur.Rule.TZ`) — 'UTC' for a date-only due, the profile's for a timed one.
import { computed, reactive, ref, watch } from 'vue'
import { useThemeStore } from '@/stores/theme'
import { useFormat } from '@/composables/useFormat'
import { resolvedTimezone } from '@/utils/format'
import { triggerColumnMissing, dueDateMissing } from '@/utils/recurrence'
import {
  coordsOf,
  dayKey,
  epochOf,
  nextDay,
  occurrenceDayKeys,
  presetDay,
  readEndpoint,
  ruleSummary,
  ruleZone,
  sameDay,
  wallLabel,
  wallNow,
  withStoredAnchor,
  writeEndpoint,
} from '@/utils/dueModel'

const toIso = (v) => {
  if (v === null || v === undefined || v === '') return null
  const ms = typeof v === 'number' ? v : Date.parse(v)
  return Number.isNaN(ms) ? null : new Date(ms).toISOString()
}

/**
 * @param {() => { due, start, recurrence, columns }} source  the task as stored
 * @param {(patch) => void} onApply
 */
export function useDueModel(source, onApply) {
  const theme = useThemeStore()
  const { formatters } = useFormat()
  const fmt = () => formatters.value
  // The zone a clock is written in — the profile's, else the browser's.
  const zone = () => resolvedTimezone(theme.timezone)
  const weekStart = computed(() => (theme.weekStart === 0 ? 0 : 1))

  const due = ref(toIso(source().due))
  const start = ref(toIso(source().start))
  const target = ref('due') // 'start' | 'due'

  const dueEp = computed(() => readEndpoint(due.value, fmt()))
  const startEp = computed(() => readEndpoint(start.value, fmt()))
  const activeEp = computed(() => (target.value === 'start' ? startEp.value : dueEp.value))
  const today = () => coordsOf(wallNow(fmt()))

  // The month the grid shows.
  const view = ref(null)
  function navTo(day) {
    const d = day || today()
    view.value = { year: d.year, month: d.month }
  }
  navTo(dueEp.value?.day)

  watch(
    () => source().due,
    (v) => {
      due.value = toIso(v)
      if (target.value === 'due' && dueEp.value) navTo(dueEp.value.day)
    },
  )
  watch(
    () => source().start,
    (v) => {
      start.value = toIso(v)
      if (target.value === 'start' && startEp.value) navTo(startEp.value.day)
    },
  )

  // ── recurrence rule (local mirror of the stored one) ──
  const freq = ref('')
  const interval = ref(1)
  const weekdays = ref([]) // 0 = Sunday … 6
  const dates = ref([]) // 'YYYY-MM-DD'
  const trigger = ref('complete')
  const triggerColumn = ref(null)
  const targetColumn = ref('')
  const createNew = ref(false)
  const recurForever = ref(true)
  const skipWeekends = ref(false)
  // Hand-picked weekdays already say which days, so the server drops the skip
  // for such a rule (recur.normalized) — a box that unticks itself on reopen.
  const skipWeekendsMoot = computed(() => freq.value === 'weekly' && weekdays.value.length > 0)
  const skipWeekendsOffered = computed(
    () => (freq.value === 'daily' || freq.value === 'weekly') && !skipWeekendsMoot.value,
  )

  watch(
    () => source().recurrence,
    (r) => {
      freq.value = r?.freq || ''
      interval.value = r?.interval || 1
      weekdays.value = r?.weekdays ? [...r.weekdays] : []
      dates.value = r?.dates ? [...r.dates] : []
      trigger.value = r?.trigger || 'complete'
      triggerColumn.value = r?.trigger_column || null
      targetColumn.value = r?.target_column || ''
      createNew.value = !!r?.create_new
      recurForever.value = !r?.once
      skipWeekends.value = !!r?.skip_weekends
    },
    { immediate: true },
  )

  function buildRule() {
    if (!freq.value) return null
    const r = { freq: freq.value, interval: Math.max(1, Math.round(interval.value || 1)) }
    if (freq.value === 'weekly' && weekdays.value.length)
      r.weekdays = [...weekdays.value].sort((a, b) => a - b)
    if (freq.value === 'custom') r.dates = [...dates.value].sort()
    if (trigger.value && trigger.value !== 'complete') r.trigger = trigger.value
    if (trigger.value === 'column' && triggerColumn.value) r.trigger_column = triggerColumn.value
    if (targetColumn.value) r.target_column = targetColumn.value
    if (createNew.value) r.create_new = true
    if (!recurForever.value) r.once = true
    if (skipWeekends.value && skipWeekendsOffered.value) r.skip_weekends = true
    // Every field above names a calendar day, and the server re-reads the due in
    // this zone to find it (recur.go) — the zone this picker drew its grid in.
    const tz = ruleZone(due.value, zone())
    if (tz) r.tz = tz
    return r
  }
  const rule = computed(() => buildRule())

  // The anchor the server keeps for a monthly/yearly rule survives an edit that
  // leaves the due DAY alone (`sameRuleDay`, handlers/tasks.go) — a clock nudge
  // is not a reschedule.
  const dueDayMoved = computed(() => {
    const a = dueEp.value
    const b = readEndpoint(toIso(source().due), fmt())
    if (!a || !b) return !a !== !b
    return !sameDay(a.day, b.day)
  })
  const previewRule = computed(() =>
    withStoredAnchor(rule.value, source().recurrence, dueDayMoved.value),
  )
  // A rule that can never fire: a column trigger with no (live) column, or no
  // due date to count from. The grid previews nothing for it and the panel says why.
  const deadTriggerColumn = computed(() =>
    triggerColumnMissing(
      previewRule.value,
      (source().columns || []).map((c) => c.id),
    ),
  )
  const deadNoDue = computed(() => dueDateMissing(previewRule.value, due.value))
  const occKeys = computed(() =>
    deadTriggerColumn.value || deadNoDue.value
      ? new Set()
      : occurrenceDayKeys(previewRule.value, dueEp.value, 24),
  )
  // The custom list as picked — not the same claim as `occKeys` (a date the
  // series has walked past, or past the end of a one-off rule, never fires).
  const pickedKeys = computed(() => (freq.value === 'custom' ? new Set(dates.value) : new Set()))
  const next = computed(() =>
    deadTriggerColumn.value || deadNoDue.value ? null : nextDay(previewRule.value, dueEp.value),
  )
  const summary = computed(() =>
    ruleSummary(previewRule.value, {
      fmt: fmt(),
      weekStart: weekStart.value,
      columns: source().columns || [],
      due: dueEp.value,
    }),
  )
  const nextLabel = computed(() =>
    next.value ? wallLabel({ day: next.value, time: null }, fmt()) : '',
  )

  // ── commits ──
  function commit() {
    onApply({ due_date: due.value, start_date: start.value, recurrence: buildRule() })
  }
  function writeActive(ep) {
    const iso = ep ? writeEndpoint(ep.day, ep.time, zone()) : null
    if (target.value === 'start') start.value = iso
    else due.value = iso
    if (ep) navTo(ep.day)
    commit()
  }
  function setTarget(which) {
    target.value = which
    const ep = which === 'start' ? startEp.value : dueEp.value
    if (ep) navTo(ep.day)
  }
  // A day keeps the clock the endpoint already had.
  function pickDay(day) {
    writeActive({ day, time: activeEp.value?.time || null })
  }
  function setTime(time) {
    const day = activeEp.value?.day || today()
    writeActive({ day, time: time || null })
  }
  function clearActive() {
    writeActive(null)
  }
  function applyPreset(key) {
    if (key === 'none') return clearActive()
    pickDay(presetDay(key, today()))
  }
  // A line read by quickParse.parseDue. A date without a clock keeps the clock
  // the endpoint had — like a preset or a click in the grid.
  function applyParsed(res) {
    if (!res?.ok) return false
    if (res.clear) {
      clearActive()
      return true
    }
    const day = res.day ? coordsOf(res.day) : activeEp.value?.day || today()
    writeActive({ day, time: res.time || activeEp.value?.time || null })
    return true
  }

  function setFreq(v) {
    freq.value = v || ''
    // A repeat counts from the due date — a task without one gets today.
    if (v && !due.value) due.value = writeEndpoint(today(), null, zone())
    commit()
  }
  const setter = (r) => (v) => {
    r.value = v
    commit()
  }
  function setInterval(n) {
    interval.value = Math.min(99, Math.max(1, Math.round(Number(n) || 1)))
    commit()
  }
  function toggleWeekday(d) {
    const set = new Set(weekdays.value)
    if (set.has(d)) set.delete(d)
    else set.add(d)
    weekdays.value = [...set].sort((a, b) => a - b)
    commit()
  }
  // «По датам»: the dates are ticked in the repeat panel's own grid; the due
  // follows the earliest of them, keeping its clock.
  function toggleCustomDate(day) {
    const k = dayKey(day)
    const set = new Set(dates.value)
    if (set.has(k)) set.delete(k)
    else set.add(k)
    dates.value = [...set].sort()
    if (dates.value.length) {
      const [y, m, d] = dates.value[0].split('-').map(Number)
      due.value = writeEndpoint(
        { year: y, month: m - 1, day: d },
        dueEp.value?.time || null,
        zone(),
      )
    }
    commit()
  }
  function removeRule() {
    freq.value = ''
    commit()
  }

  // Days strictly between start and due (the bar's span).
  const span = computed(() => {
    if (!startEp.value || !dueEp.value) return null
    const a = epochOf(startEp.value.day)
    const b = epochOf(dueEp.value.day)
    return { lo: Math.min(a, b), hi: Math.max(a, b), days: Math.abs(b - a) }
  })

  return reactive({
    due,
    start,
    target,
    view,
    dueEp,
    startEp,
    activeEp,
    weekStart,
    freq,
    interval,
    weekdays,
    dates,
    trigger,
    triggerColumn,
    targetColumn,
    createNew,
    recurForever,
    skipWeekends,
    skipWeekendsMoot,
    skipWeekendsOffered,
    rule,
    previewRule,
    deadTriggerColumn,
    deadNoDue,
    occKeys,
    pickedKeys,
    next,
    nextLabel,
    summary,
    span,
    today,
    navTo,
    setTarget,
    pickDay,
    setTime,
    clearActive,
    applyPreset,
    applyParsed,
    setFreq,
    setInterval,
    toggleWeekday,
    toggleCustomDate,
    removeRule,
    setTrigger: setter(trigger),
    setTriggerColumn: setter(triggerColumn),
    setTargetColumn: setter(targetColumn),
    setCreateNew: setter(createNew),
    setForever: setter(recurForever),
    setSkipWeekends: setter(skipWeekends),
    commit,
  })
}
