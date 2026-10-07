<script setup>
// One home for the workspace dialogs (v2). Their doors — the workspace menu in
// the sidebar header, the rail tile's menu, the footer «⋯» — only call
// `wsDialogs.open(name)`; this host renders the modal that name stands for.
import { computed } from 'vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { DEFAULT_ESTIMATION } from '@/utils/estimation'
import { workspaceName } from '@/utils/defaultNames'
import MembersModal from './MembersModal.vue'
import GitLabModal from './GitLabModal.vue'
import EstimationPopover from './EstimationPopover.vue'
import WorkspaceCommandsModal from './WorkspaceCommandsModal.vue'
import BackgroundJobsModal from './BackgroundJobsModal.vue'
import WorkspaceFormPopover from './rail/WorkspaceFormPopover.vue'
import WorkspaceDeletePopover from './rail/WorkspaceDeletePopover.vue'

const ws = useWorkspacesStore()
const auth = useAuthStore()
const dialogs = useWsDialogsStore()

// A v-model per dialog over the single `current` slot.
function slot(name) {
  return computed({
    get: () => dialogs.current === name,
    set: (v) => {
      if (v) dialogs.open(name, dialogs.target)
      else if (dialogs.current === name) dialogs.close()
    },
  })
}
const members = slot('members')
const gitlab = slot('gitlab')
const estimation = slot('estimation')
const commands = slot('commands')
const jobs = slot('jobs')
const appearance = slot('appearance')
const create = slot('create')
const del = slot('delete')

// Browser Back closes these modals instead of leaving the board.
for (const s of [members, gitlab, commands, jobs]) useOverlayBack(s, () => (s.value = false))

const target = computed(() => dialogs.target || ws.current)

// The small three — create, appearance, delete — are popover forms at the door
// that opened them (#2969): a rail tile or «+» opens them to its right, the
// sidebar header under itself.
const anchorEl = () => dialogs.anchor || document.querySelector('[data-testid="ws-head"]')
const onRail = computed(() => !!dialogs.anchor?.closest?.('[data-testid="app-rail"]'))
const placement = computed(() => (onRail.value ? 'right-start' : 'bottom-start'))
const offset = computed(() => (onRail.value ? { x: 8, y: -8 } : { x: 0, y: 6 }))
</script>

<template>
  <MembersModal v-model:show="members" :ws-id="ws.currentId" />
  <GitLabModal v-model:show="gitlab" :ws-id="ws.currentId" />
  <EstimationPopover
    v-model:show="estimation"
    :anchor="anchorEl"
    :placement="placement"
    :offset="offset"
    scope="workspace"
    :target-id="ws.currentId"
    :name="workspaceName(ws.current)"
    :value="ws.current?.estimation || null"
    :inherited="DEFAULT_ESTIMATION"
  />
  <WorkspaceCommandsModal v-model:show="commands" :workspace-id="ws.currentId" />
  <BackgroundJobsModal v-if="auth.isAdmin" v-model:show="jobs" />
  <WorkspaceFormPopover
    v-model:show="create"
    kind="create"
    :anchor="anchorEl"
    :placement="placement"
    :offset="offset"
  />
  <WorkspaceFormPopover
    v-model:show="appearance"
    kind="appearance"
    :workspace="target"
    :anchor="anchorEl"
    :placement="placement"
    :offset="offset"
  />
  <WorkspaceDeletePopover
    v-model:show="del"
    :workspace="target"
    :anchor="anchorEl"
    :placement="placement"
    :offset="offset"
  />
</template>
