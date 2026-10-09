<script setup>
// «Родитель · U» (#3118, mockups PropPicker f=parent / parentSet): which task
// this one is a subtask of. A search over the board and the rest of the
// project — «Эта доска», «Недавние», «Другие доски проекта»; the task itself
// and its own subtasks are never offered (a cycle). With a parent set, it leads
// the list with «Открепить» (⌫) under it. Attaching to a task on another board
// moves this one there (the server adopts the parent's board).
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { SearchOutline } from '@vicons/ionicons5'
import PropPicker from '../../kit/PropPicker.vue'
import { workspaces as wsApi, projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { hasModKey } from '@/utils/keys'
import {
  descendantsOf,
  footerFor,
  parentItems,
  pushRecent,
  readRecent,
  RECENT_PARENTS_KEY,
  WIDTHS,
} from './propItems'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'below-or-side' },
  taskId: { type: String, default: '' },
  boardId: { type: String, default: '' },
  projectId: { type: String, default: '' },
  wsId: { type: String, default: '' },
  // The current parent ({ id, title, number, column_* }) or null.
  parent: { type: Object, default: null },
  // Top-level tasks of the board (the kanban's own list when it has it).
  boardTasks: { type: Array, default: () => [] },
  columns: { type: Array, default: () => [] },
  doneId: { type: String, default: null },
  readonly: { type: String, default: '' },
  keyId: { type: String, default: 'task.parent' },
  testid: { type: String, default: 'parent-picker' },
})
const emit = defineEmits(['update:show', 'set', 'detach', 'open'])
const { t } = useI18n()
const wsStore = useWorkspacesStore()

const query = ref('')
const recent = ref([])
// Every task of the project (subtasks too — they can be parents, and they are
// what the descendants are computed from), fetched once per open.
const projectTasks = ref([])
const loading = ref(false)
let seq = 0
async function load() {
  const mine = ++seq
  if (!props.wsId) return
  loading.value = !projectTasks.value.length
  try {
    const [all, boards] = await Promise.all([
      wsApi.tasks(props.wsId, { include_subtasks: 1 }),
      props.projectId
        ? wsStore.boardsByProject[props.projectId] ||
          projApi.boards(props.projectId).then((r) => r.data || [])
        : [],
    ])
    if (mine !== seq) return
    const ids = new Set((boards || []).map((b) => b.id))
    if (props.boardId) ids.add(props.boardId)
    projectTasks.value = (all.data || []).filter((x) => ids.has(x.board_id))
  } catch {
    if (mine === seq) projectTasks.value = []
  } finally {
    if (mine === seq) loading.value = false
  }
}
watch(
  () => props.show,
  (on) => {
    if (!on) return
    query.value = ''
    recent.value = readRecent(RECENT_PARENTS_KEY)
    load()
  },
)
watch(
  () => props.taskId,
  () => (projectTasks.value = []),
)

const excluded = computed(() => descendantsOf(props.taskId, projectTasks.value))
const built = computed(() =>
  parentItems({
    t,
    query: query.value,
    boardId: props.boardId,
    boardTasks: props.boardTasks,
    projectTasks: projectTasks.value,
    columns: props.columns,
    doneId: props.doneId,
    parent: props.parent,
    excluded: excluded.value,
    recent: recent.value,
    ops: {
      set: (id) => {
        recent.value = pushRecent(RECENT_PARENTS_KEY, id)
        emit('set', id)
      },
      detach: () => emit('detach'),
      open: (id) => emit('open', id),
    },
  }),
)
const footer = computed(() => {
  if (props.readonly) return footerFor('readonly', t)
  return footerFor(props.parent ? 'parentSet' : 'parent', t)
})

function onKey(e, api) {
  if (props.readonly || hasModKey(e)) return false
  if (e.key === 'Backspace' && !api.query && props.parent) {
    e.preventDefault()
    emit('detach')
    api.close()
    return true
  }
  return false
}
</script>

<template>
  <PropPicker
    v-model:query="query"
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :width="WIDTHS.parent"
    :title="t('props.parent.title')"
    :header="false"
    :key-id="keyId"
    search
    :placeholder="t('props.parent.search')"
    :items="built.items"
    :cursor="built.cursor"
    :loading="loading && !boardTasks.length"
    :empty="{ icon: SearchOutline, title: t('props.parent.nothing') }"
    :footer="footer"
    :readonly="readonly"
    :on-key="onKey"
    :testid="testid"
    @update:show="emit('update:show', $event)"
  />
</template>
