<script setup>
// The property pickers of the task modal (#3118, PackageJ «Модалка задачи:
// PropPicker у каждого свойства»): one controlled slot — `field` — opens the
// picker of that property at its row of «Свойства» (`anchor(field)`), under the
// row or, short of room, to the left of the panel. The modal keeps the task and
// does the writes (`ops`); this only wires the pickers to them, so the modal's
// template stays a list of rows.
import { computed } from 'vue'
import StatusPicker from './props/StatusPicker.vue'
import PriorityPicker from './props/PriorityPicker.vue'
import AssigneePicker from './props/AssigneePicker.vue'
import EstimatePicker from './props/EstimatePicker.vue'
import MilestonePicker from './props/MilestonePicker.vue'
import ParentPicker from './props/ParentPicker.vue'

const props = defineProps({
  // The open field ('status' | 'statusHead' | 'priority' | 'assignees' |
  // 'estimate' | 'milestone' | 'parent') or ''.
  field: { type: String, default: '' },
  anchor: { type: Function, required: true },
  // { task, columns, doneId, completed, counts, priority, members, assigned,
  //   gitlab, glAssigned, load, estimate, estCfg, rollup, milestones, parent,
  //   boardTasks, projectId, wsId, readonly }
  ctx: { type: Object, required: true },
  ops: { type: Object, required: true },
})
const emit = defineEmits(['update:field'])

const c = computed(() => props.ctx)
const open = (f) => props.field === f
function setOpen(f, on) {
  if (on) emit('update:field', f)
  else if (props.field === f) emit('update:field', '')
}
const at = (f) => () => props.anchor(f)
const ro = computed(() => c.value.readonly || '')
const isStatus = computed(() => open('status') || open('statusHead'))
const statusField = computed(() => (open('statusHead') ? 'statusHead' : 'status'))
</script>

<template>
  <StatusPicker
    :show="isStatus"
    :anchor="at(statusField)"
    :placement="statusField === 'statusHead' ? 'bottom-start' : 'below-or-side'"
    :columns="c.columns"
    :current-id="c.task?.column_id || ''"
    :done-id="c.doneId"
    :counts="c.counts"
    :completed="c.completed"
    :readonly="ro"
    testid="task-status-picker"
    @update:show="setOpen(statusField, $event)"
    @move="ops.move"
    @next="ops.next"
    @complete="ops.complete"
    @cycle="ops.cycle('status')"
  />
  <PriorityPicker
    :show="open('priority')"
    :anchor="at('priority')"
    :current="c.priority || 0"
    :readonly="ro"
    testid="task-priority-picker"
    @update:show="setOpen('priority', $event)"
    @set="ops.setPriority"
    @cycle="ops.cycle('priority')"
  />
  <AssigneePicker
    :show="open('assignees')"
    :anchor="at('assignees')"
    :members="c.members"
    :assigned="c.assigned"
    :gitlab="c.gitlab"
    :gl-assigned="c.glAssigned"
    :load="c.load"
    :readonly="ro"
    testid="task-assignee-picker"
    @update:show="setOpen('assignees', $event)"
    @toggle="ops.toggleAssignee"
    @toggle-gl="ops.toggleGl"
    @clear="ops.clearAssignees"
    @invite="ops.invite"
    @cycle="ops.cycle('assignees')"
  />
  <EstimatePicker
    :show="open('estimate')"
    :anchor="at('estimate')"
    :cfg="c.estCfg"
    :value="c.estimate"
    :rollup="c.rollup"
    :can-scale="!!c.projectId"
    :readonly="ro"
    testid="task-estimate-picker"
    @update:show="setOpen('estimate', $event)"
    @set="ops.setEstimate"
    @scale="ops.openScale"
  />
  <MilestonePicker
    :show="open('milestone')"
    :anchor="at('milestone')"
    :milestones="c.milestones"
    :current="c.task?.milestone_id || null"
    :project-id="c.projectId || ''"
    :readonly="ro"
    testid="task-milestone-picker"
    @update:show="setOpen('milestone', $event)"
    @set="ops.setMilestone"
    @created="ops.milestoneCreated"
  />
  <ParentPicker
    :show="open('parent')"
    :anchor="at('parent')"
    :task-id="c.task?.id || ''"
    :board-id="c.task?.board_id || ''"
    :project-id="c.projectId || ''"
    :ws-id="c.wsId || ''"
    :parent="c.parent"
    :board-tasks="c.boardTasks"
    :columns="c.columns"
    :done-id="c.doneId"
    :readonly="ro"
    testid="task-parent-picker"
    @update:show="setOpen('parent', $event)"
    @set="ops.setParent"
    @detach="ops.detach"
    @open="ops.openParent"
  />
</template>
