import { ref, computed, onBeforeUnmount } from 'vue'
import { tasks as tasksApi } from '@/api'
import { formatEstimate, estimateToDays, estimateTooltip } from '@/utils/estimation'
import { useFormat } from '@/composables/useFormat'
import {
  DAY_MS,
  HOUR_MS,
  isAllDayMs,
  barSpan,
  anchorMs,
  xAt,
  parseDate as parse,
} from '@/utils/timeAxis'
import { SUB_STEP, SUB_TOP0 } from './useChartRows'

/**
 * Bar layout + drag-to-reschedule, shared by the Timeline and Gantt views.
 *
 * Both views draw the same bars from the same dates; only what they overlay on top
 * (a hover card / dependency arrows) differs. This owns:
 *   - `geom` / `ghostGeom` — where a bar and its estimate envelope sit on the axis,
 *     honouring an in-flight drag preview;
 *   - the pointer drag that moves a bar or resizes one edge, and the PATCH that
 *     persists it;
 *   - `renderRows` — the windowed rows with all of the above precomputed once per
 *     render (the templates used to call geom() six times per row).
 */
export function useChartBars({
  tier,
  range,
  dayW,
  estCfg,
  vwindow,
  subBars,
  rowHeight,
  findTask,
  onChanged,
}) {
  const { formatters } = useFormat()
  // During a drag we hold a transient preview so only the dragged bar re-renders.
  const drag = ref(null) // { id, mode, startX, baseStart, baseDue, hasStart, hasDue }
  const preview = ref(null) // { id, start, due }

  // Bar geometry for a task, honouring an active drag preview. In the hours tier a
  // timed start/due sits at its real clock time (see barSpan).
  function geom(t) {
    let s = parse(t.start_date)
    let d = parse(t.due_date)
    if (preview.value && preview.value.id === t.id) {
      s = preview.value.start
      d = preview.value.due
    }
    return barSpan({
      start: s,
      due: d,
      tier: tier.value,
      rangeStart: range.value.start,
      dayW: dayW.value,
    })
  }

  // Ghost "estimate" envelope: a dashed bar starting at the task's span start and
  // extending for as many calendar days as the estimate implies — so you can see at
  // a glance whether the planned start→due window matches the effort estimate.
  // Only meaningful for the time unit (estimateToDays returns null otherwise).
  function ghostGeom(t) {
    const days = estimateToDays(t.estimate, estCfg.value)
    if (days == null) return null
    let s = parse(t.start_date)
    let d = parse(t.due_date)
    if (preview.value && preview.value.id === t.id) {
      s = preview.value.start
      d = preview.value.due
    }
    if (s == null && d == null) return null
    const anchor = anchorMs(s ?? d, tier.value)
    // Frame the envelope with a 3px margin on the start/end too (matching the top/bottom inset).
    return {
      left: xAt(anchor, range.value.start, dayW.value) - 3,
      width: Math.max(dayW.value, days * dayW.value) + 6,
    }
  }

  // Tooltip on the ghost bar: full expansion + projected window (the clock label
  // it hangs off already marks it as an estimate, so no "Оценка:" prefix).
  function ghostTitle(t) {
    return estimateTooltip(t.start_date, t.estimate, estCfg.value, formatters.value)
  }

  // ── drag-to-reschedule (move whole bar / resize an edge) ──
  function onBarDown(e, t, mode) {
    if (e.button != null && e.button !== 0) return
    e.preventDefault()
    e.stopPropagation()
    const s = parse(t.start_date)
    const d = parse(t.due_date)
    drag.value = {
      id: t.id,
      mode,
      startX: e.clientX,
      baseStart: s,
      baseDue: d,
      hasStart: s != null,
      hasDue: d != null,
    }
    preview.value = { id: t.id, start: s, due: d }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  function onMove(e) {
    const g = drag.value
    if (!g) return
    // Snap to the hour when zoomed into the hours tier AND the edited endpoint is a
    // timed value; an all-day (UTC-midnight) endpoint keeps day snapping so it stays
    // all-day. Day snapping everywhere else.
    const base = g.mode === 'due' ? (g.baseDue ?? g.baseStart) : (g.baseStart ?? g.baseDue)
    const hourSnap = tier.value === 'hours' && base != null && !isAllDayMs(base)
    const unit = hourSnap ? HOUR_MS : DAY_MS
    const unitPx = hourSnap ? dayW.value / 24 : dayW.value
    const delta = Math.round((e.clientX - g.startX) / unitPx)
    if (delta === 0) {
      preview.value = { id: g.id, start: g.baseStart, due: g.baseDue }
      return
    }
    const shift = delta * unit
    let start = g.baseStart
    let due = g.baseDue
    if (g.mode === 'move') {
      // Move both ends; a one-ended task moves just that end.
      if (g.hasStart) start = g.baseStart + shift
      if (g.hasDue) due = g.baseDue + shift
    } else if (g.mode === 'start') {
      start = (g.baseStart ?? g.baseDue) + shift
      if (due != null && start > due) start = due
    } else if (g.mode === 'due') {
      due = (g.baseDue ?? g.baseStart) + shift
      if (start != null && due < start) due = start
    }
    preview.value = { id: g.id, start, due }
  }

  async function onUp() {
    const g = drag.value
    const p = preview.value
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    drag.value = null
    if (!g || !p) {
      preview.value = null
      return
    }
    const changed = p.start !== g.baseStart || p.due !== g.baseDue
    preview.value = null
    if (!changed) return
    const t = findTask(g.id)
    if (!t) return
    try {
      // Omit description — board tasks don't carry it; the backend preserves the
      // stored text on a full-replace that leaves description out.
      await tasksApi.update(t.id, {
        title: t.title,
        priority: t.priority || 0,
        due_date: p.due != null ? new Date(p.due).toISOString() : null,
        start_date: p.start != null ? new Date(p.start).toISOString() : null,
        recurrence: t.recurrence || null,
        completed: !!t.completed_at,
      })
      onChanged()
    } catch {
      onChanged()
    }
  }

  onBeforeUnmount(() => {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  })

  // Windowed rows with their geometry resolved once — lane rows pass through
  // untouched, task rows carry the bar, the ghost envelope and the sub-bars.
  const renderRows = computed(() =>
    vwindow.value.rows.map((r) => {
      if (r.t === 'lane') return r
      const ghost = ghostGeom(r.task)
      return {
        t: 'task',
        key: r.key,
        task: r.task,
        height: rowHeight(r.task),
        geom: geom(r.task),
        ghost,
        ghostEst: ghost ? formatEstimate(r.task.estimate, estCfg.value) : '',
        ghostTitle: ghost ? ghostTitle(r.task) : '',
        subs: subBars(r.task).map((s, i) => ({
          task: s,
          geom: geom(s),
          top: SUB_TOP0 + i * SUB_STEP,
        })),
      }
    }),
  )

  return { drag, preview, geom, ghostGeom, ghostTitle, onBarDown, renderRows }
}
