import { ref, computed, watch, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useFormat } from '@/composables/useFormat'
import { useShortcuts } from '@/composables/useShortcuts'
import { columnStatusName } from '@/utils/columnStatus'
import { sortedColumns } from '@/utils/status'
import { DAY_MS, startOfDay, parseDate, spanOf, workingDays } from '@/utils/timeAxis'

// The v2 half of the Timeline and Gantt views (#2959), shared by both:
//   - what a row shows beyond its bar — the status glyph and colour, the subtask
//     progress, how overdue it is, its people, the selected bar's date tooltip;
//   - the lane summary bar («Релиз 0.150 · 0 из 5» and its span);
//   - the selected bar and the keys of the chart (↑↓ select, ←/→ scroll, +/− zoom,
//     T today, ⇧←/⇧→ shift the dates, ↵ open, Esc clear);
//   - the controller the board's sub-toolbar drives (ChartControls).
export function useChartView({
  props,
  timeline,
  bars,
  rows,
  leftW,
  leftCollapsed,
  toggleLeft,
  // Owned by the view: useChartRows reads it to fold lanes.
  collapsedLanes,
  counters = computed(() => []),
  hint = computed(() => ''),
  onOpen,
}) {
  const { t } = useI18n()
  const { formatDate } = useFormat()

  // ── status: the board header's glyph in the column's colour ──
  const cols = computed(() => sortedColumns(props.statusColumns || []))
  function statusOf(task) {
    const col = cols.value.find((c) => c.id === task.column_id)
    if (!col) return { icon: task.completed_at ? 'status-done' : 'status-todo', color: '' }
    const list = cols.value
    const done = props.doneColumnId
      ? col.id === props.doneColumnId
      : col.id === list[list.length - 1]?.id
    return {
      icon: columnStatusName({
        isDone: done || !!task.completed_at,
        first: col.id === list[0]?.id,
        name: col.name,
        nameKey: col.name_key,
      }),
      color: col.color || '',
    }
  }

  // Subtask progress (done/all children); a completed task is full, a task
  // without children has no progress line at all.
  function progressOf(task) {
    if (task.completed_at) return 1
    const subs = props.subtasksByParent?.[task.id]
    if (!subs?.length) return null
    return subs.filter((s) => s.completed_at).length / subs.length
  }

  function overdueDays(task) {
    if (!task.due_date || task.completed_at) return 0
    const due = startOfDay(parseDate(task.due_date))
    const today = timeline.todayMs
    return due < today ? Math.round((today - due) / DAY_MS) : 0
  }

  function peopleOf(task) {
    return (task.assignee_ids || [])
      .map((id) => props.membersMap?.[id])
      .filter(Boolean)
      .slice(0, 3)
  }

  const dm = (ms) => formatDate(ms, { day: 'numeric', month: 'short', timeZone: null })
  function rangeLabel(task) {
    const { a, b } = spanOf(task)
    if (a == null) return ''
    const days = workingDays(a, b)
    const range = a === b ? dm(a) : `${dm(a)} – ${dm(b)}`
    return { range, days: t('board.chart.workDays', days, { named: { n: days } }) }
  }

  // Rows as the template draws them: the windowed geometry plus the v2 meta.
  const viewRows = computed(() =>
    bars.renderRows.value.map((r) => {
      if (r.t !== 'task') return r
      const over = overdueDays(r.task)
      return {
        ...r,
        status: statusOf(r.task),
        progress: progressOf(r.task),
        overdue: over,
        overdueText: over ? t('board.chart.overdueBy', over, { named: { n: over } }) : '',
        people: peopleOf(r.task),
      }
    }),
  )

  // ── lanes: folded set + the summary bar across the group's span ──
  function toggleLane(key) {
    const next = new Set(collapsedLanes.value)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    collapsedLanes.value = next
  }
  function laneSummary(lane) {
    let lo = Infinity
    let hi = -Infinity
    for (const task of lane.tasks) {
      const g = bars.geom(task)
      if (!g) continue
      lo = Math.min(lo, g.left)
      hi = Math.max(hi, g.left + g.width)
    }
    if (!Number.isFinite(lo)) return null
    const total = lane.tasks.length
    return { left: lo, width: hi - lo, pct: total ? (lane.done || 0) / total : 0 }
  }

  // ── selection ──
  const selectedId = ref(null)
  const taskIds = computed(() =>
    rows.flatRows.value.filter((r) => r.t === 'task').map((r) => r.key),
  )
  watch(taskIds, (ids) => {
    if (selectedId.value && !ids.includes(selectedId.value)) selectedId.value = null
  })
  function select(id) {
    selectedId.value = id
  }
  // Bring a row into view: vertically from the row layout (it may be windowed
  // out of the DOM), horizontally so its bar's start shows.
  function reveal(id) {
    const el = timeline.scrollEl.value
    if (!el) return
    const i = rows.flatRows.value.findIndex((r) => r.key === id)
    if (i < 0) return
    const top = rows.rowLayout.value.tops[i] + timeline.bodyTop.value
    const h = rows.rowH(rows.flatRows.value[i])
    const head = timeline.bodyTop.value
    if (top - head < el.scrollTop) el.scrollTop = top - head
    else if (top + h > el.scrollTop + el.clientHeight) el.scrollTop = top + h - el.clientHeight
    const task = rows.findTask(id)
    const g = task && bars.geom(task)
    if (g) {
      const lw = leftW.value
      const x = g.left + lw
      if (x - lw < el.scrollLeft || x > el.scrollLeft + el.clientWidth - 80)
        timeline.animateScrollLeft(Math.max(0, x - lw - 3 * timeline.dayW.value))
    }
  }
  function move(dir) {
    const ids = taskIds.value
    if (!ids.length) return
    const i = ids.indexOf(selectedId.value)
    const next =
      i < 0 ? (dir > 0 ? 0 : ids.length - 1) : Math.max(0, Math.min(ids.length - 1, i + dir))
    selectedId.value = ids[next]
    reveal(ids[next])
  }
  const selectedTask = computed(() =>
    selectedId.value ? rows.findTask(selectedId.value) || null : null,
  )
  const selectedLabel = computed(() => (selectedTask.value ? rangeLabel(selectedTask.value) : null))

  useShortcuts({
    'chart.today': () => timeline.centerToday(),
    'chart.left': () => timeline.scrollByDays(-7),
    'chart.right': () => timeline.scrollByDays(7),
    'chart.up': () => move(-1),
    'chart.down': () => move(1),
    'chart.zoomIn': () => timeline.zoomIn(),
    'chart.zoomInShift': () => timeline.zoomIn(),
    'chart.zoomOut': () => timeline.zoomOut(),
  })
  useShortcuts(
    {
      'chart.open': () => onOpen(selectedId.value),
      'chart.clear': () => (selectedId.value = null),
      'chart.shiftLeft': () => bars.shiftDays(selectedTask.value, -1),
      'chart.shiftRight': () => bars.shiftDays(selectedTask.value, 1),
    },
    { when: () => !!selectedTask.value },
  )

  // ── the controller the sub-toolbar drives ──
  const ctl = reactive({
    scale: timeline.scale,
    setScale: timeline.setScale,
    canZoomIn: computed(() => timeline.zoomIdx.value < timeline.ZOOM.length - 1),
    canZoomOut: computed(() => timeline.zoomIdx.value > 0),
    zoomIn: () => timeline.zoomIn(),
    zoomOut: () => timeline.zoomOut(),
    today: () => timeline.centerToday(),
    leftCollapsed,
    toggleLeft,
    counters,
    hint,
  })

  return {
    statusOf,
    viewRows,
    toggleLane,
    laneSummary,
    selectedId,
    select,
    selectedLabel,
    ctl,
  }
}
