import { computed } from 'vue'
import { i18n } from '@/i18n'
import { formatEstimate, formatEstimateFull, sumLaneEffort } from '@/utils/estimation'
import { columnName } from '@/utils/defaultNames'
import { tagLaneLabel, tagInPrefix } from '@/utils/tagGroups'

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
  prefixNames,
  estCfg,
  subtasksByParent,
  glMembers,
}) {
  // GitLab login → display name, from the workspace roster the board already holds.
  // Same source the card's assignee pills resolve their names through, so a lane and
  // the faces standing in it spell the person the same way.
  const glNameByLogin = computed(() => {
    const m = {}
    for (const g of glMembers?.value || []) {
      if (g?.gl_username) m[g.gl_username] = g.gl_name || g.gl_username
    }
    return m
  })

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
        // An assignee without a Tessera account is not «не назначено». The backend
        // splits the two: a GitLab member linked to a Tessera user becomes an
        // ordinary `task_assignees` row (so `assignee_ids` covers them), an unlinked
        // one lands in `task_gitlab_assignees` and reaches the board only as
        // `gitlab_assignee_logins`. Reading the first list alone swept every such
        // card into the unassigned lane — on a GitLab-synced board that is most of
        // them, and `assignee` is the grouping both charts OPEN with (defaultToolbar).
        // The same person is a first-class value everywhere else on the bar: the
        // composer offers them under «Исполнитель» as `gl:<login>` and filters by it
        // (utils/taskFilter matchesAssignees), and the card draws their face. The
        // lane key follows that same `gl:` convention (#2932).
        const login = id ? '' : (task.gitlab_assignee_logins || [])[0]
        if (login) {
          ensure(`gl:${login}`, glNameByLogin.value[login] || login).tasks.push(task)
        } else {
          const m = id ? membersMap.value[id] : null
          ensure(id || '∅', m?.name || t('board.chart.lane.unassigned')).tasks.push(task)
        }
      } else if (mode === 'tag') {
        // Respect the prefix when grouping by a tag namespace — by canonical
        // namespace, the same membership the board's own lanes ask for (utils/
        // tagGroups `tagInPrefix`), so the two surfaces show the same lanes.
        const ids = (task.tag_ids || []).filter((id) => {
          const tag = tagsMap.value[id]
          return !!tag && tagInPrefix(tag.name, tagPrefix.value)
        })
        const id = ids[0]
        const tag = id ? tagsMap.value[id] : null
        // Captioned the same way the board captions its tag columns (utils/tagGroups
        // `tagLaneLabel`): the stored "scope::value" is not a caption anywhere, and
        // least of all here — the lane name sits in a 224px sticky column and is
        // ellipsised, so under a narrowed prefix every lane read as the same long
        // namespace with the value that distinguishes them cut off.
        const label = tag
          ? tagLaneLabel(tag, { tagPrefix: tagPrefix.value, prefixNames: prefixNames?.value })
          : ''
        ensure(id || '∅', label || t('board.chart.lane.noTag'), tag?.color).tasks.push(task)
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

  // Effort total per lane, shown in the lane header. The unit comes from the
  // project's estimation config.
  //
  // Same rule as the kanban column's «Σ» (utils/estimation `sumLaneEffort`): a lane
  // holds top-level rows, their subtasks are drawn as sub-bars INSIDE the parent's
  // row (useChartRows `subBars`), so a task planned entirely in its children carried
  // no own estimate and counted as zero effort here.
  const laneTotal = (lane) => sumLaneEffort(lane.tasks, subtasksByParent?.value || {})
  function laneEffort(lane) {
    const total = laneTotal(lane)
    return total != null ? formatEstimate(total, estCfg.value) : ''
  }
  // Spelled-out lane total for the effort tooltip (a sum → no projected window).
  function laneEffortFull(lane) {
    const total = laneTotal(lane)
    return total != null ? formatEstimateFull(total, estCfg.value) : ''
  }

  return { lanes, laneEffort, laneEffortFull }
}
