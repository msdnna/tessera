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
import { NIcon, NDropdown, NTooltip, NPopconfirm } from 'naive-ui'
import { AddOutline, HelpCircleOutline, LogInOutline } from '@vicons/ionicons5'
import BrandLogo from '@/components/BrandLogo.vue'
import ServerFormPopover from './ServerFormPopover.vue'
import RailTile from './RailTile.vue'
import RailAppearance from './RailAppearance.vue'
import VersionBadge from '@/components/VersionBadge.vue'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useServersStore } from '@/stores/servers'
import { useNotificationsStore } from '@/stores/notifications'
import { useAuthStore } from '@/stores/auth'
import { useHelpStore } from '@/stores/help'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useWorkspaceMenu } from '@/composables/useWorkspaceMenu'
import { useShortcuts } from '@/composables/useShortcuts'
import { handleRowKeys } from '@/composables/useRowKeys'
import { useServerBadges } from '@/composables/useServerBadges'
import { sSetFor } from '@/utils/serverStorage'
import { serverLabel } from '@/utils/servers'
import { workspaceName } from '@/utils/defaultNames'
import { initials } from '@/utils/initials'
import { tileColor, tileLabel } from './workspaceTile'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const ws = useWorkspacesStore()
const servers = useServersStore()
const notes = useNotificationsStore()
const auth = useAuthStore()
const help = useHelpStore()
const dialogs = useWsDialogsStore()
const whatsNew = useWhatsNewStore()
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
// A workspace tile carries the workspace menu (useWorkspaceMenu): the full one
// on the current workspace — with the sidebar collapsed this is where members
// and integrations are reached — and appearance / delete on the others.
const menu = ref({ show: false, x: 0, y: 0, kind: '', target: null })
const railEl = ref(null)
const tileEl = (id) => railEl.value?.querySelector(`[data-ws="${CSS.escape(id)}"]`) || null
const wsMenu = useWorkspaceMenu(
  () => (menu.value.kind === 'ws' ? menu.value.target : null),
  () => (menu.value.target ? tileEl(menu.value.target.id) : null),
)
const menuOptions = computed(() => {
  const { kind } = menu.value
  if (kind === 'server') {
    return [
      { label: t('common.action.edit'), key: 'server-edit' },
      { label: t('shell.servers.remove'), key: 'server-remove' },
    ]
  }
  if (kind === 'ws') return wsMenu.options.value
  return []
})
function openMenu(kind, target, e) {
  menu.value = { show: true, x: e.clientX, y: e.clientY, kind, target }
  if (!menuOptions.value.length) menu.value.show = false
}

// The server form opens at the door that asked: the server tile, or «+».
const serverEdit = ref({ show: false, server: null, anchor: null })

// Removing a profile purges its namespace — the refresh token included — and, if
// it was the last one, puts a fresh profile pointed at the public default in its
// place (servers.remove). A slip one item below «Изменить» would sign the user out
// AND erase a self-hosted server's address with no undo, so the menu never calls
// it straight off: it opens a confirm where the menu stood (#2932).
const removeAsk = ref({ show: false, x: 0, y: 0, id: '', name: '' })
function askRemove(server) {
  const { x, y } = menu.value
  removeAsk.value = {
    show: true,
    x,
    y,
    id: server.id,
    name: server.name || serverLabel(server.url),
  }
}
function doRemove() {
  const { id } = removeAsk.value
  removeAsk.value = { ...removeAsk.value, show: false, id: '' }
  if (id) servers.remove(id)
}

function onMenu(key) {
  const { kind, target } = menu.value
  if (key === 'integrations') return // a submenu head
  menu.value.show = false
  if (key === 'server-edit')
    serverEdit.value = {
      show: true,
      server: target,
      anchor: railEl.value?.querySelector(`[data-server="${CSS.escape(target.id)}"]`) || null,
    }
  else if (key === 'server-remove') askRemove(target)
  else if (kind === 'ws') wsMenu.select(key)
}
const addBtn = ref(null)
function openCreate() {
  dialogs.open('create', null, addBtn.value)
}
function onAdd(key) {
  if (key === 'server-add') serverEdit.value = { show: true, server: null, anchor: addBtn.value }
  else openCreate()
}

// ── «+»: a new workspace; on desktop also a new server ──
const addOptions = computed(() => [
  { label: t('shell.workspace.create'), key: 'create' },
  { label: t('shell.servers.addTitle'), key: 'server-add' },
])

function onServerSave(patch) {
  if (serverEdit.value.server) servers.update(serverEdit.value.server.id, patch)
  else servers.add(patch)
}
// The form's own «Удалить сервер» takes two presses (ServerFormPopover), so by
// the time it reaches here the answer is already yes.
function onServerRemove(id) {
  serverEdit.value.show = false
  servers.remove(id)
}

function serverBadge(s) {
  return s.id === servers.activeId ? 0 : servers.badges[s.id] || 0
}
const serverName = (s) => s.name || serverLabel(s.url)

// ── keyboard (#2969): G W gets onto the rail, ↑/↓ walk the workspace tiles,
// ⇧N opens «Новое пространство», I the focused tile's «Оформление».
function tiles() {
  return [...(railEl.value?.querySelectorAll('[data-ws]') || [])].filter((el) => el.offsetParent !== null)
}
function focusRail() {
  const list = tiles()
  const cur = list.find((el) => el.getAttribute('aria-current') === 'true') || list[0]
  cur?.focus()
}
useShortcuts({ 'nav.goRail': focusRail })
function onRailKey(e) {
  const list = tiles()
  const i = list.indexOf(document.activeElement)
  if (i < 0) return
  if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault()
    list[Math.min(list.length - 1, Math.max(0, i + (e.key === 'ArrowDown' ? 1 : -1)))].focus()
    return
  }
  const id = list[i].getAttribute('data-ws')
  const w = ws.list.find((x) => x.id === id)
  handleRowKeys(e, {
    'row.new': openCreate,
    'row.icon': () => {
      if (w && ['owner', 'admin'].includes(w.my_role))
        dialogs.open('appearance', w.id === ws.currentId ? null : w, list[i])
    },
  })
}
</script>

<template>
  <div ref="railEl" class="ar" data-testid="app-rail">
    <!-- The logo carries the build versions (v2): they used to be a line in the
         working area's corner, under the FAB. A click opens «Что нового», as the
         line did (#2812). -->
    <n-tooltip placement="right" :delay="300">
      <template #trigger>
        <button
          type="button"
          class="ar-logo"
          data-testid="rail-logo"
          :aria-label="t('app.whatsNew.historyTitle')"
          @click="whatsNew.openHistory()"
        >
          <BrandLogo :height="30" :mark="true" :wordmark="false" />
        </button>
      </template>
      <VersionBadge mode="block" />
      <div class="ar-tip-sub ar-ver-hint">{{ t('app.whatsNew.historyTitle') }}</div>
    </n-tooltip>

    <div class="ar-list" data-tour="ws-switch" @keydown="onRailKey">
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
        @select="onAdd"
      >
        <button
          ref="addBtn"
          type="button"
          class="ar-add"
          :class="{ open: dialogs.current === 'create' }"
          data-testid="rail-add"
          :aria-label="t('shell.rail.add')"
        >
          <n-icon :component="AddOutline" :size="18" />
        </button>
      </n-dropdown>
      <n-tooltip v-else-if="auth.isAuthenticated" placement="right" :delay="300">
        <template #trigger>
          <button
            ref="addBtn"
            type="button"
            class="ar-add"
            :class="{ open: dialogs.current === 'create' }"
            data-testid="rail-add"
            :aria-label="t('shell.workspace.create')"
            @click="openCreate"
          >
            <n-icon :component="AddOutline" :size="18" />
          </button>
        </template>
        {{ t('shell.workspace.create') }}
      </n-tooltip>
    </div>

    <RailAppearance />
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

    <n-popconfirm
      :show="removeAsk.show"
      :x="removeAsk.x"
      :y="removeAsk.y"
      trigger="manual"
      :positive-button-props="{ type: 'error', 'data-testid': 'server-remove-confirm' }"
      :positive-text="t('shell.servers.remove')"
      @positive-click="doRemove"
      @negative-click="removeAsk.show = false"
      @clickoutside="removeAsk.show = false"
    >
      <template #trigger><span /></template>
      {{ t('shell.servers.removeConfirm', { name: removeAsk.name }) }}
    </n-popconfirm>

    <ServerFormPopover
      v-if="servers.enabled"
      v-model:show="serverEdit.show"
      :server="serverEdit.server"
      :anchor="() => serverEdit.anchor || addBtn"
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
  padding: 0 4px;
  border: none;
  border-radius: 10px;
  background: transparent;
  cursor: pointer;
}
.ar-logo:hover {
  background: var(--t-hover);
}
.ar-ver-hint {
  margin-top: 6px;
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
.ar-add.open {
  background: var(--t-active);
  color: var(--t-primary);
}
.ar-tip-name {
  font-weight: 600;
}
.ar-tip-sub {
  font-size: 12px;
  opacity: 0.75;
}
</style>
