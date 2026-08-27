import { computed } from 'vue'
import { i18n } from '@/i18n'
import { formatEstimate, formatEstimateFull, sumEstimates } from '@/utils/estimation'
import { columnName } from '@/utils/defaultNames'

// Swimlane grouping for the Timeline and Gantt board views, driven by the shared
// composer-bar's groupMode ('status' | 'tag' (+ prefix) | 'assignee' | 'none') —
// neither view owns a grouping control of its own.
//
// `source` is the already-ordered list of scheduled tasks, NOT props.tasks: the
// Timeline feeds it the incoming order (which already reflects the composer's sort),
// the Gantt feeds it the dependency-DFS order when «Авто» is on. Lane membership is
// the only thing computed here; ordering stays the caller's decision.
// Called from a component's setup but also directly from its own spec, so the
// translator comes from `i18n.global` rather than `useI18n()` (#2799).
const t = (key) => i18n.global.t(key)

export function useChartLanes({
  source,
  statusColumns,
  membersMap,
  tagsMap,
  groupMode,
  tagPrefix,
  estCfg,
}) {
  const lanes = computed(() => {
    const mode = groupMode.value
    const buckets = new Map()
    const ensure = (key, label, color) => {
      if (!buckets.has(key)) buckets.set(key, { key, label, color, tasks: [] })
      return buckets.get(key)
    }
    // For 'status' grouping, seed lanes in column order so empty columns still show
    // and the lane order matches the board.
    if (mode === 'status') {
      for (const col of statusColumns.value) ensure(col.id, columnName(col), col.color)
    }
    for (const task of source.value) {
      if (mode === 'assignee') {
        const id = (task.assignee_ids || [])[0]
        const m = id ? membersMap.value[id] : null
        ensure(id || '∅', m?.name || t('board.chart.lane.unassigned')).tasks.push(task)
      } else if (mode === 'tag') {
        // Respect the prefix when grouping by a tag namespace.
        const ids = (task.tag_ids || []).filter((id) => {
          const tag = tagsMap.value[id]
          return tag && (!tagPrefix.value || (tag.name || '').startsWith(tagPrefix.value))
        })
        const id = ids[0]
        const tag = id ? tagsMap.value[id] : null
        ensure(id || '∅', tag?.name || t('board.chart.lane.noTag'), tag?.color).tasks.push(task)
      } else if (mode === 'status') {
        const col = statusColumns.value.find((c) => c.id === task.column_id)
        ensure(task.column_id || '∅', col?.name || '—', col?.color).tasks.push(task)
      } else {
        ensure('all', t('board.chart.lane.all')).tasks.push(task)
      }
    }
    const arr = [...buckets.values()].filter((l) => l.tasks.length || mode === 'status')
    // Lane tasks keep `source`'s order — re-sorting by start here would override the
    // composer's sort (e.g. «Сорт: Статус») and the Gantt's «Авто» order.
    if (mode !== 'status') arr.sort((a, b) => (a.key === '∅' ? 1 : 0) - (b.key === '∅' ? 1 : 0))
    return arr
  })

  // Effort total per lane (sum of estimates), shown in the lane header. The unit
  // comes from the project's estimation config.
  function laneEffort(lane) {
    const total = sumEstimates(lane.tasks)
    return total != null ? formatEstimate(total, estCfg.value) : ''
  }
  // Spelled-out lane total for the effort tooltip (a sum → no projected window).
  function laneEffortFull(lane) {
    const total = sumEstimates(lane.tasks)
    return total != null ? formatEstimateFull(total, estCfg.value) : ''
  }

  return { lanes, laneEffort, laneEffortFull }
}
