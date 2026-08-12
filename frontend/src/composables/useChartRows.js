import { ref, computed, onMounted, nextTick, watch } from 'vue'

// Row geometry + virtualization shared by the Timeline and Gantt board views.
//
// The body renders one DOM row per lane-header and per task; on a board with 200+
// tasks that (with per-row gridlines + bars) dominates the cost of every zoom and
// scroll. We window it: only rows intersecting the viewport (plus a margin) render,
// with top/bottom spacers that preserve the total scroll height. Bars are positioned
// by pure axis math, so windowing changes nothing about their geometry — and the
// Gantt's arrow SVG stays full-height, so dependencies to off-screen tasks still
// draw correctly.
export const ROW_H = 36 // task row height
// Subtask sub-bars stack below the parent bar within the same row; each adds
// SUB_STEP to the row height. SUB_TOP0 sits just under the 24px parent bar (the
// 14px sub-bar height lives in the .tl-subbar CSS).
export const SUB_STEP = 18 // sub-bar height (14) + gap (4)
export const SUB_TOP0 = 34 // first sub-bar top (parent bar bottom 30 + 4px gap)
export const VMARGIN = 600 // px of off-screen rows kept rendered above/below the viewport

export function useChartRows({
  lanes,
  tasks,
  subtasksByParent,
  scrollY,
  viewH,
  bodyTop,
  bodyEl,
  // Initial lane-header height, used until the real one is measured on mount. The
  // two views seed it differently (see the callers) — the measured value wins.
  laneH0 = 30,
}) {
  // Scheduled subtasks of a task (those with a start or due), drawn as sub-bars.
  function subBars(t) {
    const subs = subtasksByParent.value[t.id]
    if (!subs || !subs.length) return []
    return subs.filter((s) => s.start_date || s.due_date)
  }
  // Row height grows by SUB_STEP per scheduled subtask (36 when none).
  function rowHeight(t) {
    return ROW_H + SUB_STEP * subBars(t).length
  }
  // Resolve a task by id across top-level tasks AND subtasks — the drag-reschedule
  // save step needs it, since a dragged sub-bar's task lives in subtasksByParent.
  function findTask(id) {
    const top = tasks.value.find((x) => x.id === id)
    if (top) return top
    for (const arr of Object.values(subtasksByParent.value)) {
      const s = arr.find((x) => x.id === id)
      if (s) return s
    }
    return null
  }

  // The lane-header row is content-driven, so measure its rendered height instead of
  // assuming a constant — that's what keeps the Gantt's SVG arrow geometry exact.
  // Styling is uniform across lanes, so one measurement covers all of them.
  const laneH = ref(laneH0)
  function measureLaneH() {
    const h = bodyEl.value?.querySelector('.tl-lanehead')?.offsetHeight
    if (h) laneH.value = h
  }

  // Flat ordered list of visual rows (lane header, then its task rows), matching the
  // document flow so spacer heights stay exact.
  const flatRows = computed(() => {
    const out = []
    for (const lane of lanes.value) {
      out.push({ t: 'lane', key: `L${lane.key}`, lane })
      for (const task of lane.tasks) out.push({ t: 'task', key: task.id, task })
    }
    return out
  })
  const rowH = (r) => (r.t === 'lane' ? laneH.value : rowHeight(r.task))
  const rowLayout = computed(() => {
    const rows = flatRows.value
    const tops = new Array(rows.length)
    let y = 0
    for (let i = 0; i < rows.length; i++) {
      tops[i] = y
      y += rowH(rows[i])
    }
    return { tops, height: y }
  })
  const vwindow = computed(() => {
    const rows = flatRows.value
    const { tops, height } = rowLayout.value
    const n = rows.length
    if (!n) return { rows: [], top: 0, bottom: 0 }
    const lo = scrollY.value - bodyTop.value - VMARGIN
    const hi = scrollY.value - bodyTop.value + (viewH.value || 800) + VMARGIN
    let start = 0
    while (start < n && tops[start] + rowH(rows[start]) < lo) start++
    if (start >= n) return { rows: [], top: height, bottom: 0 }
    let end = start
    while (end < n && tops[end] < hi) end++
    if (end <= start) end = Math.min(n, start + 1)
    const last = end - 1
    return {
      rows: rows.slice(start, end),
      top: tops[start],
      bottom: height - (tops[last] + rowH(rows[last])),
    }
  })

  onMounted(() => nextTick(measureLaneH))
  watch(lanes, () => nextTick(measureLaneH))

  return {
    subBars,
    rowHeight,
    findTask,
    laneH,
    measureLaneH,
    flatRows,
    rowH,
    rowLayout,
    vwindow,
  }
}
