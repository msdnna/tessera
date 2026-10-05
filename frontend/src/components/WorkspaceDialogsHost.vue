<script setup>
// One home for the workspace dialogs (v2). Their doors — the workspace menu in
// the sidebar header, the rail tile's menu, the footer «⋯» — only call
// `wsDialogs.open(name)`; this host renders the modal that name stands for.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NModal, NCard, NInput, NButton, useMessage } from 'naive-ui'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { DEFAULT_ESTIMATION } from '@/utils/estimation'
import { workspaceName } from '@/utils/defaultNames'
import { humanizeError } from '@/utils/errors'
import MembersModal from './MembersModal.vue'
import GitLabModal from './GitLabModal.vue'
import EstimationModal from './EstimationModal.vue'
import WorkspaceCommandsModal from './WorkspaceCommandsModal.vue'
import BackgroundJobsModal from './BackgroundJobsModal.vue'
import WorkspaceAppearanceModal from './rail/WorkspaceAppearanceModal.vue'
import ConfirmByName from './ConfirmByName.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const message = useMessage()
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
for (const s of [members, gitlab, estimation, commands, jobs])
  useOverlayBack(s, () => (s.value = false))

const target = computed(() => dialogs.target || ws.current)

const newName = ref('')
watch(create, (open) => open && (newName.value = ''))
async function createWorkspace() {
  const name = newName.value.trim()
  if (!name) return
  try {
    await ws.createWorkspace(name)
    create.value = false
    // A board belongs to the workspace we just left.
    if (route.params.projectSlug || route.params.id) router.push('/')
  } catch (e) {
    message.error(humanizeError(e))
  }
}

async function deleteWorkspace() {
  try {
    await ws.removeWorkspace(target.value.id)
    message.success(t('shell.workspace.deleted'))
    router.push('/') // the tree fully changed — land on Home
  } catch (e) {
    message.error(humanizeError(e))
  }
}
</script>

<template>
  <MembersModal v-model:show="members" :ws-id="ws.currentId" />
  <GitLabModal v-model:show="gitlab" :ws-id="ws.currentId" />
  <EstimationModal
    v-model:show="estimation"
    scope="workspace"
    :target-id="ws.currentId"
    :name="workspaceName(ws.current)"
    :value="ws.current?.estimation || null"
    :inherited="DEFAULT_ESTIMATION"
  />
  <WorkspaceCommandsModal v-model:show="commands" :workspace-id="ws.currentId" />
  <BackgroundJobsModal v-if="auth.isAdmin" v-model:show="jobs" />
  <WorkspaceAppearanceModal v-model:show="appearance" :workspace="target" />

  <n-modal v-model:show="create">
    <n-card
      :title="t('shell.workspace.create')"
      style="max-width: 360px"
      role="dialog"
      :bordered="false"
    >
      <n-input
        v-model:value="newName"
        :placeholder="t('shell.workspace.namePlaceholder')"
        autofocus
        @keyup.enter="createWorkspace"
      />
      <template #footer>
        <div class="wd-actions">
          <n-button size="small" @click="create = false">{{ t('common.action.cancel') }}</n-button>
          <n-button
            type="primary"
            size="small"
            :disabled="!newName.trim()"
            @click="createWorkspace"
          >
            {{ t('common.action.create') }}
          </n-button>
        </div>
      </template>
    </n-card>
  </n-modal>

  <ConfirmByName
    v-model:show="del"
    :name="workspaceName(target)"
    :title="t('shell.workspace.delete')"
    :message="t('shell.workspace.deleteMessage')"
    @confirm="deleteWorkspace"
  />
</template>

<style scoped>
.wd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
