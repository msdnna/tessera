<script setup>
// The rail (v2, #2955): a 56px column left of the sidebar with a tile per
// workspace. On the web that is the whole story. In the desktop shell the
// rail also carries the servers the app is signed into (#2930): each server is
// a group — its head tile, then its workspaces — so a workspace on another
// server is one click away (the click switches server, which reloads the app,
// and lands in that workspace).
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NIcon, NDropdown, NModal, NCard, NInput, NButton, NTooltip, useMessage } from 'naive-ui'
import { AddOutline, HelpCircleOutline, LogInOutline } from '@vicons/ionicons5'
import BrandLogo from '@/components/BrandLogo.vue'
import ServerEditModal from '@/components/ServerEditModal.vue'
import ConfirmByName from '@/components/ConfirmByName.vue'
import RailTile from './RailTile.vue'
import WorkspaceAppearanceModal from './WorkspaceAppearanceModal.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useServersStore } from '@/stores/servers'
import { useNotificationsStore } from '@/stores/notifications'
import { useAuthStore } from '@/stores/auth'
import { useHelpStore } from '@/stores/help'
import { useServerBadges } from '@/composables/useServerBadges'
import { sSetFor } from '@/utils/serverStorage'
import { serverLabel } from '@/utils/servers'
import { workspaceName } from '@/utils/defaultNames'
import { initials } from '@/utils/initials'
import { humanizeError } from '@/utils/errors'
import { tileColor, tileLabel } from './workspaceTile'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const message = useMessage()
const ws = useWorkspacesStore()
const servers = useServersStore()
const notes = useNotificationsStore()
const auth = useAuthStore()
const help = useHelpStore()
useServerBadges()

const desktop = computed(() => servers.enabled && servers.list.length > 0)

// The active server's group draws from the live list; the cache keeps the
// others (and this one, for the login screen) drawn.
watch(
  () => ws.list,
  (list) => {
    if (servers.enabled && servers.activeId && list.length)
      servers.setWorkspaces(servers.activeId, list)
  },
)
function workspacesOf(server) {
  if (server.id === servers.activeId && ws.list.length) return ws.list
  return servers.workspaces[server.id] || []
}

// Unread notifications per workspace — the live feed (the last 50) grouped by
// where they came from. A dot, not a number: the bell has the count.
const unreadByWs = computed(() => {
  const out = {}
  for (const n of notes.items) if (!n.read_at && n.workspace_id) out[n.workspace_id] = true
  return out
})

function isCurrent(server, w) {
  return (!server || server.id === servers.activeId) && w.id === ws.currentId
}

async function pick(server, w) {
  if (server && server.id !== servers.activeId) {
    // Land in that workspace once the app has reloaded onto the other server.
    sSetFor('tessera_ws', server.id, w.id)
    servers.switchTo(server.id)
    return
  }
  if (!auth.isAuthenticated || w.id === ws.currentId) return
  await ws.selectWorkspace(w.id)
  // A board belongs to the workspace we just left.
  if (route.params.projectSlug || route.params.id) router.push('/')
}

// ── context menus ──
const menu = ref({ show: false, x: 0, y: 0, kind: '', target: null })
const menuOptions = computed(() => {
  const { kind, target } = menu.value
  if (kind === 'server') {
    return [
      { label: t('common.action.edit'), key: 'server-edit' },
      { label: t('shell.servers.remove'), key: 'server-remove' },
    ]
  }
  if (kind === 'ws' && target) {
    const role = target.my_role
    const out = []
    if (role === 'owner' || role === 'admin')
      out.push({ label: t('shell.rail.appearance'), key: 'appearance' })
    if (target.owner_id === auth.user?.id) {
      out.push({
        label: t('shell.workspace.delete'),
        key: 'delete',
        props: { style: 'color:#e0533d' },
      })
    }
    return out
  }
  return []
})
function openMenu(kind, target, e) {
  menu.value = { show: true, x: e.clientX, y: e.clientY, kind, target }
  if (!menuOptions.value.length) menu.value.show = false
}

const appearance = ref({ show: false, workspace: null })
const del = ref({ show: false, workspace: null })
const serverEdit = ref({ show: false, server: null })

function onMenu(key) {
  const target = menu.value.target
  menu.value.show = false
  if (key === 'appearance') appearance.value = { show: true, workspace: target }
  else if (key === 'delete') {
    if (ws.list.length <= 1) message.warning(t('shell.workspace.deleteLast'))
    else del.value = { show: true, workspace: target }
  } else if (key === 'server-edit') serverEdit.value = { show: true, server: target }
  else if (key === 'server-remove') servers.remove(target.id)
  else if (key === 'create') createModal.value = { show: true, name: '' }
  else if (key === 'server-add') serverEdit.value = { show: true, server: null }
}

async function deleteWorkspace() {
  const w = del.value.workspace
  try {
    await ws.removeWorkspace(w.id)
    message.success(t('shell.workspace.deleted'))
    router.push('/')
  } catch (e) {
    message.error(humanizeError(e))
  }
}

// ── «+»: a new workspace; on desktop also a new server ──
const createModal = ref({ show: false, name: '' })
const addOptions = computed(() => [
  { label: t('shell.workspace.create'), key: 'create' },
  { label: t('shell.servers.addTitle'), key: 'server-add' },
])
async function createWorkspace() {
  const name = createModal.value.name.trim()
  if (!name) return
  try {
    await ws.createWorkspace(name)
    createModal.value.show = false
    if (route.params.projectSlug || route.params.id) router.push('/')
  } catch (e) {
    message.error(humanizeError(e))
  }
}

function onServerSave(patch) {
  if (serverEdit.value.server) servers.update(serverEdit.value.server.id, patch)
  else servers.add(patch)
}
function onServerRemove(id) {
  serverEdit.value.show = false
  servers.remove(id)
}

function serverBadge(s) {
  return s.id === servers.activeId ? 0 : servers.badges[s.id] || 0
}
const serverName = (s) => s.name || serverLabel(s.url)
</script>

<template>
  <div class="ar" data-testid="app-rail">
    <div class="ar-logo"><BrandLogo :height="30" :mark="true" :wordmark="false" /></div>

    <div class="ar-list" data-tour="ws-switch">
      <!-- Web: one server, the one serving this page — just its workspaces. -->
      <template v-if="!desktop">
        <RailTile
          v-for="w in ws.list"
          :key="w.id"
          :label="tileLabel(w)"
          :color="tileColor(w)"
          :icon="w.icon"
          :icon-mode="w.icon_mode"
          :active="isCurrent(null, w)"
          :dot="!!unreadByWs[w.id] && !isCurrent(null, w)"
          :data-ws="w.id"
          @pick="pick(null, w)"
          @menu="openMenu('ws', w, $event)"
        >
          <template #tip>{{ workspaceName(w) }}</template>
        </RailTile>
      </template>

      <!-- Desktop: a group per server — its head, then its workspaces. -->
      <template v-else>
        <div
          v-for="s in servers.list"
          :key="s.id"
          class="ar-group"
          :class="{ current: s.id === servers.activeId }"
          :data-server="s.id"
        >
          <RailTile
            small
            :label="initials(serverName(s))"
            :color="s.color"
            :icon="s.icon"
            :active="s.id === servers.activeId"
            :muted="!!servers.signedOut[s.id]"
            :badge="serverBadge(s)"
            @pick="servers.switchTo(s.id)"
            @menu="openMenu('server', s, $event)"
          >
            <n-icon v-if="servers.signedOut[s.id]" class="ar-out" :component="LogInOutline" />
            <template #tip>
              <div class="ar-tip-name">{{ serverName(s) }}</div>
              <div class="ar-tip-sub">{{ s.url }}</div>
              <div v-if="servers.signedOut[s.id]" class="ar-tip-sub">
                {{ t('shell.servers.signedOut') }}
              </div>
            </template>
          </RailTile>
          <RailTile
            v-for="w in workspacesOf(s)"
            :key="`${s.id}:${w.id}`"
            :label="tileLabel(w)"
            :color="tileColor(w)"
            :icon="w.icon"
            :icon-mode="w.icon_mode"
            :active="isCurrent(s, w)"
            :muted="!!servers.signedOut[s.id]"
            :dot="s.id === servers.activeId && !!unreadByWs[w.id] && !isCurrent(s, w)"
            :data-ws="w.id"
            @pick="pick(s, w)"
            @menu="s.id === servers.activeId && openMenu('ws', w, $event)"
          >
            <template #tip>
              <div class="ar-tip-name">{{ workspaceName(w) }}</div>
              <div class="ar-tip-sub">{{ serverName(s) }}</div>
            </template>
          </RailTile>
        </div>
      </template>

      <n-dropdown
        v-if="desktop"
        trigger="click"
        placement="right-start"
        :options="addOptions"
        @select="onMenu"
      >
        <button
          type="button"
          class="ar-add"
          data-testid="rail-add"
          :aria-label="t('shell.rail.add')"
        >
          <n-icon :component="AddOutline" :size="18" />
        </button>
      </n-dropdown>
      <n-tooltip v-else-if="auth.isAuthenticated" placement="right" :delay="300">
        <template #trigger>
          <button
            type="button"
            class="ar-add"
            data-testid="rail-add"
            :aria-label="t('shell.workspace.create')"
            @click="createModal = { show: true, name: '' }"
          >
            <n-icon :component="AddOutline" :size="18" />
          </button>
        </template>
        {{ t('shell.workspace.create') }}
      </n-tooltip>
    </div>

    <n-tooltip v-if="auth.isAuthenticated" placement="right" :delay="300">
      <template #trigger>
        <button
          type="button"
          class="ar-help"
          :aria-label="t('shell.user.helpCenter')"
          @click="help.openCenter()"
        >
          <n-icon :component="HelpCircleOutline" :size="18" />
        </button>
      </template>
      {{ t('shell.user.helpCenter') }}
    </n-tooltip>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="menu.show"
      :x="menu.x"
      :y="menu.y"
      :options="menuOptions"
      @select="onMenu"
      @clickoutside="menu.show = false"
    />

    <n-modal v-model:show="createModal.show">
      <n-card
        :title="t('shell.workspace.create')"
        style="max-width: 360px"
        role="dialog"
        :bordered="false"
      >
        <n-input
          v-model:value="createModal.name"
          :placeholder="t('shell.workspace.namePlaceholder')"
          autofocus
          @keyup.enter="createWorkspace"
        />
        <template #footer>
          <div class="ar-modal-actions">
            <n-button size="small" @click="createModal.show = false">{{
              t('common.action.cancel')
            }}</n-button>
            <n-button
              type="primary"
              size="small"
              :disabled="!createModal.name.trim()"
              @click="createWorkspace"
            >
              {{ t('common.action.create') }}
            </n-button>
          </div>
        </template>
      </n-card>
    </n-modal>

    <WorkspaceAppearanceModal v-model:show="appearance.show" :workspace="appearance.workspace" />
    <ConfirmByName
      v-model:show="del.show"
      :name="workspaceName(del.workspace)"
      :title="t('shell.workspace.delete')"
      :message="t('shell.workspace.deleteMessage')"
      @confirm="deleteWorkspace"
    />
    <ServerEditModal
      v-if="servers.enabled"
      v-model:show="serverEdit.show"
      :server="serverEdit.server"
      @save="onServerSave"
      @remove="onServerRemove"
    />
  </div>
</template>

<style scoped>
.ar {
  box-sizing: border-box;
  width: 56px;
  height: 100%;
  flex: none;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 12px 0;
  background: var(--t-rail);
}
.ar-logo {
  flex: none;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 4px;
}
/* The tiles scroll when there are more than fit; the logo and help stay put. */
.ar-list {
  flex: 1;
  min-height: 0;
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  overflow-y: auto;
  scrollbar-width: none;
}
.ar-list::-webkit-scrollbar {
  display: none;
}
.ar-group {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding-bottom: 6px;
}
.ar-group + .ar-group {
  padding-top: 6px;
}
.ar-out {
  position: absolute;
  right: -2px;
  bottom: -3px;
  font-size: 12px;
  color: var(--t-text2);
}
.ar-add,
.ar-help {
  flex: none;
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: 11px;
  color: var(--t-text3);
  cursor: pointer;
}
.ar-add {
  background: var(--t-ctl);
}
.ar-help {
  border-radius: 10px;
  background: transparent;
}
.ar-add:hover,
.ar-help:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
.ar-tip-name {
  font-weight: 600;
}
.ar-tip-sub {
  font-size: 12px;
  opacity: 0.75;
}
.ar-modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
