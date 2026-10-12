<script setup>
// One home for the workspace dialogs (v2). Their doors — the workspace menu in
// the sidebar header, the rail tile's menu, the footer «⋯» — only call
// `wsDialogs.open(name)`; this host renders the modal that name stands for.
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from '@/composables/useMessage'
import { useGitlabStore } from '@/stores/gitlab'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useShortcuts } from '@/composables/useShortcuts'
import { DEFAULT_ESTIMATION } from '@/utils/estimation'
import { workspaceName } from '@/utils/defaultNames'
import MembersModal from './MembersModal.vue'
import GitLabModal from './GitLabModal.vue'
import EstimationPopover from './EstimationPopover.vue'
import WorkspaceCommandsModal from './WorkspaceCommandsModal.vue'
import BackgroundJobsModal from './BackgroundJobsModal.vue'
import ImportWindow from './import/ImportWindow.vue'
import WorkspaceFormPopover from './rail/WorkspaceFormPopover.vue'
import WorkspaceDeletePopover from './rail/WorkspaceDeletePopover.vue'

const ws = useWorkspacesStore()
const auth = useAuthStore()
const dialogs = useWsDialogsStore()
const { t } = useI18n()

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

// Browser Back closes the large windows instead of leaving the board — they
// take care of it themselves (kit/LargeWindow, #2975).

// G J — background jobs (#2975, JobsWin), for administrators: nobody else has
// the window.
useShortcuts({ 'nav.goJobs': () => auth.isAdmin && dialogs.open('jobs') })

// Back from GitLab's consent page (#3083): the connect flow returns to the page
// it started on with ?gitlab_oauth=connected (or the reason it failed). Reopen
// the GitLab window where it was left, say how it went, drop the parameter.
const route = useRoute()
const router = useRouter()
const message = useMessage()
const gl = useGitlabStore()
watch(
  () => route.query.gitlab_oauth,
  async (result) => {
    if (!result) return
    const query = { ...route.query }
    delete query.gitlab_oauth
    router.replace({ path: route.path, query, hash: route.hash })
    await gl.load()
    if (result === 'connected') message.success(t('gitlab.window.oauthResult.connected'))
    else message.error(t('gitlab.window.oauthResult.failed', { reason: String(result) }))
    dialogs.open('gitlab')
  },
  { immediate: true },
)

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
  <!-- Task import onto a board (#3068): opened through stores/boardTransfer. -->
  <ImportWindow />
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
