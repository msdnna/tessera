import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import {
  tasks as tasksApi,
  boards as boardsApi,
  projects as projectsApi,
  workspaces as wsApi,
} from '@/api'
import { useBoardViewStore } from '@/stores/boardView'
import { usePaletteStore } from '@/stores/palette'
import { useFormat } from '@/composables/useFormat'
import { rank } from '@/utils/fuzzy'
import { priorityLabel, priorityOptions } from '@/utils/priority'
import { columnName } from '@/utils/defaultNames'
import { columnStatusName } from '@/utils/columnStatus'
import { sortedColumns, columnTail } from '@/utils/status'
import { taskBasePatch } from '@/utils/taskPatch'
import { taskLink } from '@/utils/taskLink'
import { copyText } from '@/utils/clipboard'
import { humanizeError } from '@/utils/errors'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { ICONS } from './icons'

export const FIELDS = ['status', 'assignees', 'priority', 'due', 'tags', 'subtask']

const TAG_COLORS = ['#7c5cff', '#2f80ed', '#0eb0a9', '#18a058', '#f0a020', '#e0533d', '#eb2f96']

// «12.10», «12.10.2026», «2026-10-12» → local midnight of that day, or null.
export function parseDay(text, now = new Date()) {
  const s = (text || '').trim()
  let y, m, d
  let hit = /^(\d{1,2})[./](\d{1,2})(?:[./](\d{2,4}))?$/.exec(s)
  if (hit) {
    d = +hit[1]
    m = +hit[2]
    y = hit[3] ? +hit[3] : now.getFullYear()
    if (y < 100) y += 2000
  } else if ((hit = /^(\d{4})-(\d{1,2})-(\d{1,2})$/.exec(s))) {
    y = +hit[1]
    m = +hit[2]
    d = +hit[3]
  } else return null
  const date = new Date(y, m - 1, d)
  if (date.getMonth() !== m - 1 || date.getDate() !== d) return null
  return date
}

function dayFromNow(days, now = new Date()) {
  return new Date(now.getFullYear(), now.getMonth(), now.getDate() + days)
}

// Monday of next week (the «next week» of a due-date picker).
export function nextMonday(now = new Date()) {
  const dow = (now.getDay() + 6) % 7 // Mon = 0
  return dayFromNow(7 - dow, now)
}

// The palette narrowed to one task (v2, #2957): the list of its fields with
// their current values, and each field's options. Card shortcuts (S/A/P/D/L,
// ⇧N) open the palette straight onto one of these lists.
export function usePaletteTask() {
  const { t } = useI18n()
  const message = useMessage()
  const palette = usePaletteStore()
  const boardView = useBoardViewStore()
  const { formatDate } = useFormat()

  const task = ref(null)
  const columns = ref([])
  const members = ref([])
  const tags = ref([])
  const boardInfo = ref(null)
  const loading = ref(false)
  const busy = ref(false)

  const field = computed(() => palette.scope?.field || null)

  let seq = 0
  watch(
    () => palette.scope?.taskId,
    async (id) => {
      const mine = ++seq
      if (!id) {
        task.value = null
        return
      }
      loading.value = true
      try {
        const detail = (await tasksApi.get(id)).data
        const b = (await boardsApi.get(detail.board_id)).data
        const [cols, mem, tg] = await Promise.all([
          boardsApi.columns(detail.board_id),
          wsApi.members(b.workspace_id),
          projectsApi.tags(b.project_id),
        ])
        if (mine !== seq) return
        task.value = detail
        boardInfo.value = b
        columns.value = sortedColumns(cols.data || [])
        members.value = mem.data || []
        tags.value = tg.data || []
      } catch (e) {
        if (mine === seq) {
          message.error(humanizeError(e))
          palette.hide()
        }
      } finally {
        if (mine === seq) loading.value = false
      }
    },
    { immediate: true },
  )

  async function reloadTask() {
    if (!task.value) return
    task.value = (await tasksApi.get(task.value.id)).data
  }

  // Runs one change, reports failure, and tells the palette what to do next.
  async function apply(fn, next = 'close') {
    if (busy.value) return 'stay'
    busy.value = true
    try {
      await fn()
      return next
    } catch (e) {
      message.error(humanizeError(e))
      return 'stay'
    } finally {
      busy.value = false
    }
  }

  // After a value is applied: a palette opened straight onto the field (from a
  // card key) has done its job; one reached via Tab steps back to the actions.
  const afterSingle = () => (palette.scope?.direct ? 'close' : 'back')

  const doneColumnId = computed(() => boardInfo.value?.done_column_id || null)
  const currentColumn = computed(() => columns.value.find((c) => c.id === task.value?.column_id))

  function statusOf(c, i) {
    const isDone = doneColumnId.value ? c.id === doneColumnId.value : i === columns.value.length - 1
    return {
      name: columnStatusName({ isDone, first: i === 0, name: c.name, nameKey: c.name_key }),
      color: c.color || 'var(--t-text3)',
    }
  }

  // The task's own status glyph, for the chip in the input row.
  const status = computed(() => {
    const i = columns.value.findIndex((c) => c.id === task.value?.column_id)
    return i >= 0 ? statusOf(columns.value[i], i) : null
  })

  async function boardTasks(boardId) {
    if (boardView.boardId === boardId && boardView.taskSource) return boardView.taskSource()
    return (await boardsApi.tasks(boardId)).data || []
  }

  const actionRows = computed(() => {
    const tk = task.value
    if (!tk) return []
    const names = (tk.assignees || []).map((a) => a.name || a.email)
    const tagNames = (tk.tags || []).map((x) => x.name)
    const open = (f) => () => {
      palette.scope = { ...palette.scope, field: f }
      palette.query = ''
      return 'stay'
    }
    return [
      {
        key: 'status',
        icon: ICONS.SwapHorizontalOutline,
        title: t('shell.palette.task.status'),
        sub: t('shell.palette.task.now', { value: columnName(currentColumn.value) || '—' }),
        keys: 'card.status',
        field: 'status',
        run: open('status'),
      },
      {
        key: 'assignees',
        icon: ICONS.PersonAddOutline,
        title: t('shell.palette.task.assignees'),
        sub: names.length ? names.join(', ') : t('shell.palette.task.nobody'),
        keys: 'card.assignees',
        field: 'assignees',
        run: open('assignees'),
      },
      {
        key: 'priority',
        icon: ICONS.FlagOutline,
        title: t('shell.palette.task.priority'),
        sub: priorityLabel(tk.priority),
        keys: 'card.priorityPick',
        field: 'priority',
        run: open('priority'),
      },
      {
        key: 'due',
        icon: ICONS.CalendarClearOutline,
        title: t('shell.palette.task.due'),
        sub: tk.due_date
          ? formatDate(tk.due_date, { day: 'numeric', month: 'short' })
          : t('shell.palette.task.noDue'),
        keys: 'card.due',
        field: 'due',
        run: open('due'),
      },
      {
        key: 'tags',
        icon: ICONS.PricetagOutline,
        title: t('shell.palette.task.tags'),
        sub: tagNames.length ? tagNames.join(', ') : t('shell.palette.task.noTags'),
        keys: 'card.tags',
        field: 'tags',
        run: open('tags'),
      },
      {
        key: 'subtask',
        icon: ICONS.GitBranchOutline,
        title: t('shell.palette.task.subtask'),
        keys: 'card.subtask',
        field: 'subtask',
        run: open('subtask'),
      },
      {
        key: 'copy',
        group: 'more',
        icon: ICONS.LinkOutline,
        title: t('shell.palette.task.copyLink'),
        keys: 'palette.copyLink',
        run: () => copyLink(),
      },
      {
        key: 'newTab',
        group: 'more',
        icon: ICONS.OpenOutline,
        title: t('shell.palette.task.newTab'),
        keys: 'palette.newTab',
        run: () => openNewTab(),
      },
    ]
  })

  async function copyLink() {
    const url = taskLink(task.value)
    if (!url) return 'stay'
    await copyText(url)
    message.success(t('shell.palette.task.copied'))
    return 'close'
  }

  function openNewTab() {
    const url = taskLink(task.value)
    if (url) window.open(url, '_blank', 'noopener')
    return 'close'
  }

  function statusRows() {
    const tk = task.value
    return columns.value.map((c, i) => ({
      key: `col:${c.id}`,
      status: statusOf(c, i),
      title: columnName(c),
      checked: c.id === tk.column_id,
      run: () =>
        apply(async () => {
          if (c.id === tk.column_id) return
          const tail = columnTail(await boardTasks(tk.board_id), c.id, tk.id)
          await tasksApi.move(tk.id, { column_id: c.id, before_id: tail, after_id: null })
        }, afterSingle()),
    }))
  }

  function assigneeRows() {
    const tk = task.value
    const on = new Set((tk.assignees || []).map((a) => a.id))
    return members.value.map((m) => ({
      key: `as:${m.user_id}`,
      avatar: m.user_id,
      title: m.name || m.email,
      sub: m.email,
      checked: on.has(m.user_id),
      search: [m.name, m.email],
      run: () =>
        apply(async () => {
          if (on.has(m.user_id)) await tasksApi.removeAssignee(tk.id, m.user_id)
          else await tasksApi.addAssignee(tk.id, m.user_id)
          await reloadTask()
        }, 'stay'),
    }))
  }

  function priorityRows() {
    const tk = task.value
    return priorityOptions()
      .slice()
      .reverse()
      .map((o) => ({
        key: `prio:${o.value}`,
        dot: o.value ? PRIORITY_COLORS[o.value] : null,
        icon: o.value ? null : ICONS.FlagOutline,
        title: o.label,
        checked: (tk.priority || 0) === o.value,
        keysCaps: [String(o.value)],
        run: () => setPriority(o.value),
      }))
  }

  function setPriority(value) {
    const tk = task.value
    return apply(
      () => tasksApi.update(tk.id, { ...taskBasePatch(tk), priority: value }),
      afterSingle(),
    )
  }

  function dueRows(query) {
    const tk = task.value
    const fmt = (d) => formatDate(d, { weekday: 'short', day: 'numeric', month: 'short' })
    const set = (d) =>
      apply(
        () =>
          tasksApi.update(tk.id, {
            ...taskBasePatch(tk),
            due_date: d ? d.toISOString() : null,
          }),
        afterSingle(),
      )
    const typed = parseDay(query)
    const rows = []
    if (typed) {
      rows.push({
        key: 'due:typed',
        icon: ICONS.CalendarClearOutline,
        title: fmt(typed),
        run: () => set(typed),
      })
    }
    const presets = [
      ['today', dayFromNow(0)],
      ['tomorrow', dayFromNow(1)],
      ['nextWeek', nextMonday()],
    ]
    for (const [k, d] of presets) {
      rows.push({
        key: `due:${k}`,
        icon: ICONS.CalendarClearOutline,
        title: t(`shell.palette.due.${k}`),
        value: fmt(d),
        search: [t(`shell.palette.due.${k}`)],
        run: () => set(d),
      })
    }
    if (tk.due_date) {
      rows.push({
        key: 'due:none',
        icon: ICONS.CalendarClearOutline,
        title: t('shell.palette.due.none'),
        search: [t('shell.palette.due.none')],
        run: () => set(null),
      })
    }
    return { rows, typed: !!typed }
  }

  function tagRows(query) {
    const tk = task.value
    const on = new Set((tk.tags || []).map((x) => x.id))
    const rows = tags.value.map((tag) => ({
      key: `tag:${tag.id}`,
      dot: tag.color,
      title: tag.name,
      checked: on.has(tag.id),
      search: [tag.name],
      run: () =>
        apply(async () => {
          if (on.has(tag.id)) await tasksApi.removeTag(tk.id, tag.id)
          else await tasksApi.addTag(tk.id, tag.id)
          await reloadTask()
        }, 'stay'),
    }))
    const q = (query || '').trim()
    const exists = tags.value.some((tag) => tag.name.toLowerCase() === q.toLowerCase())
    const create =
      q && !exists
        ? {
            key: 'tag:new',
            icon: ICONS.AddOutline,
            title: t('shell.palette.task.createTag', { name: q }),
            pinned: true,
            run: () =>
              apply(async () => {
                const res = await projectsApi.createTag(boardInfo.value.project_id, {
                  name: q,
                  color: TAG_COLORS[Math.floor(Math.random() * TAG_COLORS.length)],
                })
                tags.value = [...tags.value, res.data]
                await tasksApi.addTag(tk.id, res.data.id)
                await reloadTask()
                palette.query = ''
              }, 'stay'),
          }
        : null
    return { rows, create }
  }

  function subtaskRows(query) {
    const tk = task.value
    const title = (query || '').trim()
    return [
      {
        key: 'sub:new',
        icon: ICONS.GitBranchOutline,
        title: title || t('shell.palette.task.subtaskPlaceholder'),
        muted: !title,
        disabled: !title,
        pinned: true,
        run: () =>
          apply(async () => {
            await boardsApi.createTask(tk.board_id, {
              column_id: tk.column_id,
              parent_id: tk.id,
              title,
            })
            message.success(t('shell.palette.task.subtaskCreated'))
          }, 'close'),
      },
    ]
  }

  // The rows to show for the current scope and query, already grouped.
  function groupsFor(query) {
    if (!task.value) return []
    const q = (query || '').trim()
    const f = field.value
    if (!f) {
      const rows = rank(actionRows.value, (r) => [r.title], q)
      const main = rows.filter((r) => r.group !== 'more')
      const more = rows.filter((r) => r.group === 'more')
      return [
        {
          id: 'actions',
          title: t('shell.palette.groups.actions'),
          rows: main.map((r) => ({ ...r, hl: q })),
        },
        {
          id: 'more',
          title: t('shell.palette.groups.more'),
          rows: more.map((r) => ({ ...r, hl: q })),
        },
      ].filter((g) => g.rows.length)
    }
    const title = t(`shell.palette.task.${f}`)
    const filter = (rows) =>
      q ? rank(rows, (r) => r.search || [r.title], q).map((r) => ({ ...r, hl: q })) : rows
    let rows
    if (f === 'status') rows = filter(statusRows())
    else if (f === 'assignees') rows = filter(assigneeRows())
    else if (f === 'priority') rows = filter(priorityRows())
    else if (f === 'due') {
      const { rows: all, typed } = dueRows(q)
      rows = typed ? all : filter(all)
    } else if (f === 'tags') {
      const { rows: all, create } = tagRows(q)
      rows = [...filter(all), ...(create ? [create] : [])]
    } else rows = subtaskRows(q)
    return [{ id: f, title, rows }]
  }

  return { task, status, loading, busy, field, groupsFor, copyLink, openNewTab, setPriority }
}
