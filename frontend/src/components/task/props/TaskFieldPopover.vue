<script setup>
// A property picker over a board task (#3118, PackageJ «Карточка на доске: тот
// же компонент у пиллов»): the pills of a card and the cells of the «Список»
// open the same PropPicker the task modal does, at the pill or the cell. The
// board's context (people, stages, the GitLab roster, the estimation config)
// comes from the board store; the write goes straight to the API and the board
// is told (`changed`) either way — a refusal included, so it re-reads.
//
// One instance serves a whole card or a whole list: `field` says which picker
// is open ('priority' | 'assignees' | 'estimate' | 'milestone'), `anchor` where.
import { computed } from 'vue'
import { useMessage } from 'naive-ui'
import { storeToRefs } from 'pinia'
import { tasks as tasksApi } from '@/api'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { taskBasePatch } from '@/utils/taskPatch'
import { glAssigneesOf, glPickerMembers } from '@/utils/assignees'
import { sumEstimates } from '@/utils/estimation'
import { tourEvent } from '@/utils/tourEvents'
import PriorityPicker from './PriorityPicker.vue'
import AssigneePicker from './AssigneePicker.vue'
import EstimatePicker from './EstimatePicker.vue'
import MilestonePicker from './MilestonePicker.vue'

const props = defineProps({
  task: { type: Object, default: null },
  field: { type: String, default: '' },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'bottom-start' },
  // Children of the task — the «Σ подзадач» of the estimate.
  subtasks: { type: Array, default: () => [] },
  readonly: { type: Boolean, default: false },
  testid: { type: String, default: 'card' },
})
const emit = defineEmits(['update:field', 'changed', 'cycle'])
const message = useMessage()

const bv = useBoardViewStore()
const { membersList: members, gitlabMembersList: gitlabMembers, milestonesList } = storeToRefs(bv)
const wsStore = useWorkspacesStore()
const projectWindow = useProjectWindowStore()
const wsDialogs = useWsDialogsStore()

const open = (f) => !!props.task && props.field === f
function setOpen(f, on) {
  if (on) emit('update:field', f)
  else if (props.field === f) emit('update:field', '')
}

async function run(fn) {
  try {
    await fn()
    tourEvent('task.fields')
  } catch (e) {
    message.error(e.message)
  }
  emit('changed')
}
const patch = (p) =>
  run(() => tasksApi.update(props.task.id, { ...taskBasePatch(props.task), ...p }))

// ── people ──
const assigned = computed(() => props.task?.assignee_ids || [])
const glLogins = computed(() => props.task?.gitlab_assignee_logins || [])
const glList = computed(() =>
  props.task
    ? glPickerMembers(gitlabMembers.value, glAssigneesOf(props.task, gitlabMembers.value))
    : [],
)
const toggle = (uid) =>
  run(() =>
    assigned.value.includes(uid)
      ? tasksApi.removeAssignee(props.task.id, uid)
      : tasksApi.addAssignee(props.task.id, uid),
  )
const toggleGl = (m) =>
  run(() =>
    glLogins.value.includes(m.gl_username)
      ? tasksApi.removeGitlabAssignee(props.task.id, m.gl_username)
      : tasksApi.pinGitlabAssignee(props.task.id, {
          gl_username: m.gl_username,
          gl_name: m.gl_name,
          gl_avatar_url: m.gl_avatar_url,
        }),
  )
const clearAll = () =>
  run(() =>
    Promise.all([
      ...assigned.value.map((uid) => tasksApi.removeAssignee(props.task.id, uid)),
      ...glLogins.value.map((l) => tasksApi.removeGitlabAssignee(props.task.id, l)),
    ]),
  )
function invite(email) {
  emit('update:field', '')
  wsDialogs.draftEmail = email
  wsDialogs.open('members')
}

// ── estimate and stage ──
const estCfg = computed(() => wsStore.estimationFor(bv.projectId))
const rollup = computed(() => sumEstimates(props.subtasks))
function openScale() {
  emit('update:field', '')
  if (bv.projectId) projectWindow.open(bv.projectId, 'estimation')
}
const setMilestone = (id) =>
  run(() =>
    id ? tasksApi.setMilestone(props.task.id, id) : tasksApi.clearMilestone(props.task.id),
  )
function milestoneCreated(m) {
  bv.milestonesMap[m.id] = m
}
</script>

<template>
  <template v-if="task && !readonly">
    <PriorityPicker
      :show="open('priority')"
      :anchor="anchor"
      :placement="placement"
      :current="task.priority || 0"
      key-id="card.priorityPick"
      :testid="`${testid}-priority-picker`"
      @update:show="setOpen('priority', $event)"
      @set="(p) => patch({ priority: p })"
      @cycle="emit('cycle', 'priority')"
    />
    <AssigneePicker
      :show="open('assignees')"
      :anchor="anchor"
      :placement="placement"
      :members="members"
      :assigned="assigned"
      :gitlab="glList"
      :gl-assigned="glLogins"
      :load="bv.taskStats.boardId === task.board_id ? bv.taskStats.load : null"
      key-id="card.assignees"
      :testid="`${testid}-assignee-picker`"
      @update:show="setOpen('assignees', $event)"
      @toggle="toggle"
      @toggle-gl="toggleGl"
      @clear="clearAll"
      @invite="invite"
      @cycle="emit('cycle', 'assignees')"
    />
    <EstimatePicker
      :show="open('estimate')"
      :anchor="anchor"
      :placement="placement"
      :cfg="estCfg"
      :value="task.estimate ?? null"
      :rollup="rollup"
      :can-scale="!!bv.projectId"
      key-id=""
      :testid="`${testid}-estimate-picker`"
      @update:show="setOpen('estimate', $event)"
      @set="(v) => patch({ estimate: v && v > 0 ? v : null })"
      @scale="openScale"
    />
    <MilestonePicker
      :show="open('milestone')"
      :anchor="anchor"
      :placement="placement"
      :milestones="milestonesList"
      :current="task.milestone_id || null"
      :project-id="bv.projectId || ''"
      key-id=""
      :testid="`${testid}-milestone-picker`"
      @update:show="setOpen('milestone', $event)"
      @set="setMilestone"
      @created="milestoneCreated"
    />
  </template>
</template>
