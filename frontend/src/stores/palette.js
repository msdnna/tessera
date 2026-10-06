import { defineStore } from 'pinia'
import { ref } from 'vue'
import { sGet, sSet } from '@/utils/serverStorage'

const RECENT_KEY = 'tessera_palette_recent'
const RECENT_MAX = 5

function readRecent() {
  try {
    const v = JSON.parse(sGet(RECENT_KEY) || '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

// The ⌘K palette (v2, #2957). One instance for the app (CommandPalette in
// AppLayout); anything can open it — the shortcut, the header field, a card's
// S/A/P/D/L keys — through this store.
//
// `scope` is null for the results list, or `{ taskId, field }` once the palette
// is narrowed to one task: `field` null lists the task's actions, a field name
// ('status' | 'assignees' | 'priority' | 'due' | 'tags' | 'subtask') lists that
// field's options. `direct` marks a palette opened straight onto a field from a
// card shortcut — applying a value then closes it instead of stepping back.
export const usePaletteStore = defineStore('palette', () => {
  const open = ref(false)
  const query = ref('')
  const scope = ref(null)
  // The section picked with the pills above the list. A prefix typed into the
  // query (# @ >) wins over it; «Доски и этапы» has no prefix, so it lives here.
  const section = ref('all')
  const recent = ref(readRecent())

  function show(q = '') {
    query.value = q
    scope.value = null
    section.value = 'all'
    open.value = true
  }

  function toggle() {
    if (open.value) hide()
    else show()
  }

  function openTask(taskId, field = null) {
    query.value = ''
    scope.value = { taskId, field, direct: !!field }
    open.value = true
  }

  function hide() {
    open.value = false
    scope.value = null
    query.value = ''
  }

  // Tasks opened from the palette, newest first — the empty-query list.
  function pushRecent(task) {
    if (!task?.id) return
    const slim = {
      id: task.id,
      number: task.number,
      title: task.title,
      board_id: task.board_id,
      board_slug: task.board_slug,
      project_slug: task.project_slug,
      board_name: task.board_name,
      project_name: task.project_name,
      column_name: task.column_name,
      column_name_key: task.column_name_key,
      column_color: task.column_color,
      completed_at: task.completed_at,
    }
    recent.value = [slim, ...recent.value.filter((r) => r.id !== task.id)].slice(0, RECENT_MAX)
    sSet(RECENT_KEY, JSON.stringify(recent.value))
  }

  return { open, query, scope, section, recent, show, toggle, openTask, hide, pushRecent }
})
