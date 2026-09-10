<script setup>
// Desktop server hotbar (#2930): a narrow rail of the Tessera servers this app
// is signed into, with the active one marked and an unread badge per server —
// so a notification arriving on another instance is visible from here.
//
// Renders only inside the Tauri shell. On the web there is exactly one server
// (the origin serving this page), so the whole rail would be a column with one
// tile in it and nothing to switch to.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTooltip, NDropdown, NIcon } from 'naive-ui'
import { AddOutline, LogInOutline } from '@vicons/ionicons5'
import ServerEditModal from './ServerEditModal.vue'
import ProjectIcon from './ProjectIcon.vue'
import { useServersStore } from '@/stores/servers'
import { useNotificationsStore } from '@/stores/notifications'
import { useServerBadges } from '@/composables/useServerBadges'
import { initials } from '@/utils/initials'
import { hueGrad } from '@/utils/gradient'
import { serverLabel } from '@/utils/servers'

const { t } = useI18n()
const servers = useServersStore()
const notes = useNotificationsStore()
useServerBadges()

const editing = ref(null) // profile being edited, or null when adding
const showEdit = ref(false)

const menuServerId = ref('')
const menuAt = ref({ x: 0, y: 0 })
const showMenu = ref(false)
const menuOptions = computed(() => [
  { label: t('common.action.edit'), key: 'edit' },
  { label: t('shell.servers.remove'), key: 'remove' },
])

// The active server's badge is the live bell count — it is already maintained
// over the websocket, so the poller skips it (see useServerBadges).
function badgeFor(server) {
  if (server.id === servers.activeId) return notes.unread
  return servers.badges[server.id] || 0
}

function tileLabel(server) {
  return initials(server.name || serverLabel(server.url))
}

function openAdd() {
  editing.value = null
  showEdit.value = true
}

function openMenu(server, e) {
  menuServerId.value = server.id
  menuAt.value = { x: e.clientX, y: e.clientY }
  showMenu.value = true
}

function onMenu(key) {
  showMenu.value = false
  const server = servers.list.find((s) => s.id === menuServerId.value)
  if (!server) return
  if (key === 'edit') {
    editing.value = server
    showEdit.value = true
  } else if (key === 'remove') {
    servers.remove(server.id)
  }
}

function onSave(patch) {
  if (editing.value) servers.update(editing.value.id, patch)
  else servers.add(patch)
  editing.value = null
}

function onRemove(id) {
  showEdit.value = false
  editing.value = null
  servers.remove(id)
}
</script>

<template>
  <div v-if="servers.enabled" class="sr-rail" data-testid="server-rail">
    <n-tooltip v-for="s in servers.list" :key="s.id" placement="right" :delay="300">
      <template #trigger>
        <button
          class="sr-tile"
          :class="{ active: s.id === servers.activeId, out: servers.signedOut[s.id] }"
          type="button"
          :aria-label="s.name || serverLabel(s.url)"
          @click="servers.switchTo(s.id)"
          @contextmenu.prevent="openMenu(s, $event)"
        >
          <span class="sr-mark" />
          <span class="sr-face" :style="{ backgroundImage: hueGrad(s.color) }">
            <ProjectIcon v-if="s.icon" :icon="s.icon" :initials="tileLabel(s)" :size="20" />
            <template v-else>{{ tileLabel(s) }}</template>
          </span>
          <span v-if="badgeFor(s) > 0" class="sr-badge">{{
            badgeFor(s) > 99 ? '99+' : badgeFor(s)
          }}</span>
          <n-icon v-else-if="servers.signedOut[s.id]" class="sr-out" :component="LogInOutline" />
        </button>
      </template>
      <div class="sr-tip">
        <div class="sr-tip-name">{{ s.name || serverLabel(s.url) }}</div>
        <div class="sr-tip-url">{{ s.url }}</div>
        <div v-if="servers.signedOut[s.id]" class="sr-tip-url">
          {{ t('shell.servers.signedOut') }}
        </div>
      </div>
    </n-tooltip>

    <n-tooltip placement="right" :delay="300">
      <template #trigger>
        <button
          class="sr-tile sr-add"
          type="button"
          :aria-label="t('shell.servers.addTitle')"
          data-testid="server-rail-add"
          @click="openAdd"
        >
          <span class="sr-face sr-face-add"><n-icon :component="AddOutline" /></span>
        </button>
      </template>
      {{ t('shell.servers.addTitle') }}
    </n-tooltip>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="showMenu"
      :x="menuAt.x"
      :y="menuAt.y"
      :options="menuOptions"
      @select="onMenu"
      @clickoutside="showMenu = false"
    />

    <server-edit-modal
      v-model:show="showEdit"
      :server="editing"
      @save="onSave"
      @remove="onRemove"
    />
  </div>
</template>

<style scoped>
.sr-rail {
  /* border-box: the app has no global reset, so under the default content-box the
     10px vertical padding would add onto height:100% and push the rail past the
     shell — a page-level scrollbar that nudged the whole window down (#2930). */
  box-sizing: border-box;
  width: 56px;
  flex: none;
  /* Fill the flex parent (app-shell, itself 100vh) rather than re-asserting 100vh:
     equal to the sibling layout's height, and immune to any host chrome offset. */
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 10px 0;
  overflow-y: auto;
  background: var(--t-surface-alt);
  border-right: 1px solid var(--t-border);
}
.sr-tile {
  position: relative;
  width: 44px;
  height: 40px;
  flex: none;
  padding: 0;
  border: 0;
  background: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}
/* Active marker: a short bar in the gutter left of the tile, Lens-style. It
   grows on hover for the inactive ones, so the rail answers "which of these can
   I click" before the tooltip does. */
.sr-mark {
  position: absolute;
  left: 0;
  width: 3px;
  height: 8px;
  border-radius: 0 3px 3px 0;
  background: var(--t-text3);
  opacity: 0;
  transition:
    height 0.15s ease,
    opacity 0.15s ease;
}
.sr-tile:hover .sr-mark {
  opacity: 0.6;
  height: 16px;
}
.sr-tile.active .sr-mark {
  opacity: 1;
  height: 24px;
  background: var(--t-accent-grad-vert);
}
.sr-face {
  width: 36px;
  height: 36px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 13px;
  color: #fff;
  overflow: hidden;
  transition:
    border-radius 0.15s ease,
    transform 0.15s ease;
}
.sr-tile:hover .sr-face,
.sr-tile.active .sr-face {
  border-radius: 10px;
}
/* A server whose session ended is drawn faded — the tile still switches to it,
   which is how you get to its login screen. */
.sr-tile.out .sr-face {
  opacity: 0.45;
}
.sr-face-add {
  background: none;
  border: 1px dashed var(--t-border);
  color: var(--t-text3);
}
.sr-badge {
  position: absolute;
  top: -1px;
  right: 0;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: #d03050;
  color: #fff;
  font-size: 10px;
  line-height: 16px;
  font-weight: 700;
  text-align: center;
  box-shadow: 0 0 0 2px var(--t-surface-alt);
}
.sr-out {
  position: absolute;
  top: -1px;
  right: 0;
  font-size: 12px;
  color: var(--t-text3);
  background: var(--t-surface-alt);
  border-radius: 50%;
}
.sr-tip-name {
  font-weight: 600;
}
.sr-tip-url {
  font-size: 12px;
  opacity: 0.75;
}
</style>
