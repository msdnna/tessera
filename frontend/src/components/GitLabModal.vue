<script setup>
// «GitLab» — a large window (v2, #2975, mockup GitlabWin) over the workspace's
// GitLab bindings: Подключения · Правила лейблов · Обратная запись · Журнал, and
// the conflicts window as a door. The sync status and «Синхронизировать» (⇧S)
// sit in the header so every section sees them.
//
// The form state and the server calls live in gitlab/useGitlabForm (unchanged
// logic); the sections inject it. A binding is saved as one PUT of the whole
// form, so the window keeps an explicit save — «Изменения не сохранены ·
// Сбросить · Сохранить ⌘S» shows up only once something was edited; the
// account, the webhook, a sync and deleting a binding act at once, as before.
import { computed, nextTick, provide, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  LinkOutline,
  PricetagsOutline,
  CloudUploadOutline,
  ReaderOutline,
  GitMergeOutline,
  SyncOutline,
  ChevronDownOutline,
  CloudDownloadOutline,
  AddOutline,
  LogoGitlab,
  TimeOutline,
} from '@vicons/ionicons5'
import { useConflictsStore } from '@/stores/conflicts'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { workspaceName } from '@/utils/defaultNames'
import { useGitlabForm, GITLAB_FORM } from './gitlab/useGitlabForm'
import LargeWindow from './kit/LargeWindow.vue'
import ActionMenu from './kit/ActionMenu.vue'
import ShortcutKeys from './ShortcutKeys.vue'
import Kbd from './Kbd.vue'
import { cap } from '@/utils/keyCaps'
import GlConnectionsTab from './gitlab/GlConnectionsTab.vue'
import GlRulesTab from './gitlab/GlRulesTab.vue'
import GlWritebackTab from './gitlab/GlWritebackTab.vue'
import GitLabJournalPanel from './GitLabJournalPanel.vue'
import './kit/kit.css'
import './gitlab/gitlab-window.css'

const props = defineProps({
  show: { type: Boolean, default: false },
  wsId: { type: String, default: null },
})
// No 'synced' emit — a background sync reports itself over the workspace socket
// (`integration.sync`), which every client picks up, not just the starter.
const emit = defineEmits(['update:show'])

const { t } = useI18n()
const { isMobile } = useResponsive()
const conflicts = useConflictsStore()
const section = ref('connections')
const winRef = ref(null)
const paneRef = ref(null)
const journalRef = ref(null)

const f = reactive(
  useGitlabForm({
    wsId: () => props.wsId,
    // A sync was started: its run shows up live in the journal.
    onSyncStarted: () => {
      section.value = 'journal'
      nextTick(() => journalRef.value?.reload())
    },
  }),
)
provide(GITLAB_FORM, f)

const conflictTotal = computed(() => Math.max(f.conflictCount, conflicts.count))
const navItems = computed(() => [
  {
    key: 'connections',
    icon: LinkOutline,
    label: t('gitlab.window.nav.connections'),
    count: f.integrations.length || null,
  },
  {
    key: 'rules',
    icon: PricetagsOutline,
    label: t('gitlab.window.nav.rules'),
    count: f.rules.length || null,
  },
  {
    key: 'writeback',
    icon: CloudUploadOutline,
    label: t('gitlab.window.nav.writeback'),
    count: f.wbEnabled ? f.bindings.length || null : null,
  },
  { key: 'journal', icon: ReaderOutline, label: t('gitlab.window.nav.journal') },
  { heading: t('gitlab.window.nav.more'), desktop: true },
  {
    key: 'conflicts',
    out: true,
    icon: GitMergeOutline,
    label: t('gitlab.window.nav.conflicts'),
    count: conflictTotal.value || null,
    tone: conflictTotal.value ? 'danger' : '',
    onClick: () => conflicts.openResolver(),
  },
])

// ── the binding switcher (top of the column) and the sync menu ──
const menu = ref('') // '' | 'binding' | 'sync'
const menuAnchor = ref(null)
const menuShow = computed({
  get: () => !!menu.value,
  set: (v) => {
    if (!v) menu.value = ''
  },
})
function openMenu(which, el) {
  menuAnchor.value = el
  menu.value = which
}
const boardLabel = (id) => f.boardOptions.find((o) => o.value === id)?.label || ''
const menuItems = computed(() => {
  if (menu.value === 'sync')
    return [
      {
        key: 'full',
        label: t('gitlab.modal.sync.full'),
        icon: CloudDownloadOutline,
        disabled: f.syncing || !f.currentId,
      },
      { key: 'journal', label: t('gitlab.modal.sync.journal'), icon: TimeOutline },
    ]
  return [
    { heading: t('gitlab.window.bindingsHeading'), aside: String(f.integrations.length) },
    ...f.integrations.map((b) => ({
      key: b.id,
      label: f.bindingLabel(b),
      note: boardLabel(b.board_id),
      icon: LogoGitlab,
      active: b.id === f.currentId,
    })),
    { gap: true },
    {
      key: '__new',
      label: t('gitlab.window.newConnection'),
      icon: AddOutline,
      disabled: !f.isAdmin,
      disabledTitle: t('gitlab.modal.binding.adminOnly'),
    },
  ]
})
function onMenu(key) {
  if (menu.value === 'sync') {
    if (key === 'full') f.syncNow('full')
    else if (key === 'journal') section.value = 'journal'
    return
  }
  if (key === '__new') {
    f.newBinding()
    section.value = 'connections'
  } else f.selectBinding(key)
}
const switcherLabel = computed(() =>
  f.currentBinding ? f.bindingLabel(f.currentBinding) : t('gitlab.window.newConnection'),
)

// The status in the header: the current binding's last pull.
const statusText = computed(() =>
  f.lastSynced
    ? t('gitlab.window.syncedAgo', { ago: f.ago(f.lastSynced) })
    : t('gitlab.modal.sync.never'),
)

async function save() {
  if (!f.isAdmin || f.saving) return
  await f.save()
}

function paneEscape() {
  if (menu.value) {
    menu.value = ''
    return true
  }
  return !!paneRef.value?.escape?.()
}
const topmost = () => !!winRef.value?.topmost()
useShortcuts(
  {
    'gitlab.sync': () => f.currentId && !f.syncing && f.syncNow(),
    'gitlab.save': () => f.dirty && save(),
    'win.new': () => paneRef.value?.addNew?.(),
    'journal.retry': () => section.value === 'journal' && journalRef.value?.retry(),
    'journal.resolve': () => section.value === 'journal' && journalRef.value?.resolve(),
  },
  { when: topmost },
)

watch(
  () => [props.show, props.wsId],
  ([show]) => {
    if (show) f.open()
    else menu.value = ''
  },
)

const journalSchedule = computed(() => {
  const o = f.intervalOptions.find((x) => x.value === Number(f.intervalSec))
  const hook = f.webhookEnabled ? t('gitlab.window.webhookPlus') : ''
  return [hook, o?.label].filter(Boolean).join(' · ')
})
</script>

<template>
  <LargeWindow
    ref="winRef"
    v-model:section="section"
    :show="show"
    :crumbs="[workspaceName(f.ws.current), t('gitlab.window.crumb')]"
    title="GitLab"
    :icon="LogoGitlab"
    tone="#fc6d26"
    :items="navItems"
    :escape="paneEscape"
    testid="gitlab-window"
    tab-testid="gitlab-tab"
    @update:show="emit('update:show', $event)"
  >
    <template #aside>
      <span v-if="!isMobile" class="gw-status" :class="{ never: !f.lastSynced }">
        <span class="gw-dot" />
        {{ statusText }}
      </span>
      <span class="gw-sync">
        <button
          type="button"
          class="kit-btn"
          :disabled="!f.currentId || f.syncing"
          :title="f.currentId ? '' : t('gitlab.modal.sync.saveFirst')"
          data-testid="gitlab-sync"
          @click="f.syncNow()"
        >
          <n-icon :component="SyncOutline" :size="15" :class="{ spin: f.syncing }" />
          <span v-if="!isMobile">{{ t('gitlab.modal.sync.run') }}</span>
          <ShortcutKeys v-if="!isMobile" id="gitlab.sync" joined />
        </button>
        <button
          type="button"
          class="kit-btn gw-caret"
          :aria-label="t('common.action.more')"
          data-testid="gitlab-sync-menu"
          @click="openMenu('sync', $event.currentTarget)"
        >
          <n-icon :component="ChevronDownOutline" :size="14" />
        </button>
      </span>
    </template>

    <template #nav-top>
      <button
        type="button"
        class="gw-switch"
        data-testid="gitlab-binding-switch"
        @click="openMenu('binding', $event.currentTarget)"
      >
        <n-icon :component="LogoGitlab" :size="15" class="gw-switch-ic" />
        <span class="gw-switch-l">{{ switcherLabel }}</span>
        <n-icon :component="ChevronDownOutline" :size="14" />
      </button>
    </template>
    <template #nav-foot>
      <div class="lw-foot-row">
        {{ f.isAdmin ? t('gitlab.window.footSave') : t('gitlab.window.footReadOnly') }}
      </div>
      <div class="lw-foot-row">
        <span class="gw-foot-keys"
          ><Kbd>{{ cap(1) }}</Kbd
          ><span>–</span><Kbd>{{ cap(4) }}</Kbd></span
        >
        <span>{{ t('common.window.sections') }}</span>
      </div>
    </template>

    <div class="gw-pane">
      <GlConnectionsTab v-if="section === 'connections'" ref="paneRef" />
      <GlRulesTab v-else-if="section === 'rules'" ref="paneRef" />
      <GlWritebackTab v-else-if="section === 'writeback'" ref="paneRef" />
      <div v-else class="glw" data-testid="gitlab-journal">
        <div class="glw-title">
          <h2>{{ t('gitlab.window.nav.journal') }}</h2>
          <span class="glw-note">{{ t('gitlab.window.journalNote') }}</span>
        </div>
        <GitLabJournalPanel
          ref="journalRef"
          class="gw-journal"
          :ws-id="wsId"
          :integrations="f.integrations"
          :schedule="journalSchedule"
        />
      </div>

      <!-- The binding is one PUT of the whole form — the bar appears once the
           form differs from what the server has. -->
      <div v-if="f.dirty && f.isAdmin" class="gw-dirty" data-testid="gitlab-dirty">
        <span class="gw-dirty-dot" />
        <span class="gw-dirty-t">{{
          f.currentId ? t('gitlab.window.unsaved') : t('gitlab.window.unsavedNew')
        }}</span>
        <span class="glw-grow" />
        <button type="button" class="kit-btn small" @click="f.resetForm()">
          {{ t('gitlab.window.reset') }}
        </button>
        <button
          type="button"
          class="kit-btn small primary"
          :disabled="f.saving"
          data-testid="gitlab-save"
          @click="save"
        >
          {{ f.currentId ? t('gitlab.modal.save') : t('gitlab.window.connect') }}
          <ShortcutKeys v-if="!isMobile" id="gitlab.save" joined />
        </button>
      </div>
    </div>

    <ActionMenu
      v-model:show="menuShow"
      :anchor="() => menuAnchor"
      :placement="menu === 'sync' ? 'bottom-end' : 'bottom-start'"
      :offset="{ x: 0, y: 6 }"
      :items="menuItems"
      :width="menu === 'sync' ? 260 : 340"
      testid="gitlab-menu"
      @select="onMenu"
    />
  </LargeWindow>
</template>

<style scoped>
.gw-pane {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}
.gw-pane > :first-child {
  flex: 1;
  min-height: 0;
}
.gw-journal {
  flex: 1;
  min-height: 0;
}
.gw-status {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
  margin-right: 4px;
}
.gw-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #2fa86b;
}
.gw-status.never .gw-dot {
  background: var(--t-text3);
}
.gw-sync {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}
.gw-sync .kit-btn:first-child {
  border-radius: 10px 0 0 10px;
}
.gw-caret {
  border-radius: 0 10px 10px 0 !important;
  padding: 0 8px;
}
.spin {
  animation: gw-spin 0.9s linear infinite;
}
@keyframes gw-spin {
  to {
    transform: rotate(360deg);
  }
}
.gw-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 38px;
  margin-bottom: 8px;
  padding: 0 10px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  cursor: pointer;
  flex: none;
  min-width: 0;
}
.gw-switch:hover {
  background: var(--t-hover);
}
.gw-switch-ic {
  color: #fc6d26;
  flex: none;
}
.gw-switch-l {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lw-mobile .gw-switch {
  margin-bottom: 0;
  max-width: 200px;
}
.gw-foot-keys {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
/* The unsaved-changes bar at the foot of the pane. */
.gw-dirty {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 8px 16px 2px 0;
  padding: 8px 10px 8px 14px;
  border-radius: 14px;
  background: var(--t-sub);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--t-primary) 40%, transparent);
  font-size: 13px;
}
.lw-mobile .gw-dirty {
  margin-right: 0;
}
.gw-dirty-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--t-accent-grad);
}
.gw-dirty-t {
  color: var(--t-text2);
}
</style>
