<script setup>
// The «Проект» window (v2, #2971 / #2989, mockup ProjectWin): a large window —
// tier 2 of the interaction model, like the task modal — where a project is set
// up. Tabs in a sunken column on the left: Общее · Теги · Этапы · Оценка, and the
// workspace windows that also concern it (Участники, GitLab) as outside links.
// Every field saves at once; Esc closes what is open inside first, then the
// window, and focus goes back to the door it was opened from.
//
// One instance lives in AppLayout and follows stores/projectWindow.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { CAPS, cap } from '@/utils/keyCaps'
import { NModal, NIcon } from 'naive-ui'
import {
  CloseOutline,
  InformationCircleOutline,
  PricetagsOutline,
  TrailSignOutline,
  TimeOutline,
  PeopleOutline,
  GitBranchOutline,
  OpenOutline,
} from '@vicons/ionicons5'
import { useProjectWindowStore, TABS } from '@/stores/projectWindow'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useBoardViewStore } from '@/stores/boardView'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { workspaces as wsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import { isTypingTarget } from '@/utils/keys'
import ProjectIcon from '../ProjectIcon.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
import Kbd from '../Kbd.vue'
import ProjectGeneralTab from './ProjectGeneralTab.vue'
import ProjectTagsTab from './ProjectTagsTab.vue'
import ProjectStagesTab from './ProjectStagesTab.vue'
import ProjectEstimationTab from './ProjectEstimationTab.vue'

const { t } = useI18n()
const router = useRouter()
const win = useProjectWindowStore()
const ws = useWorkspacesStore()
const dialogs = useWsDialogsStore()
const boardView = useBoardViewStore()
const { isMobile } = useResponsive()

const show = computed({
  get: () => !!win.projectId,
  set: (v) => {
    if (!v) close()
  },
})
useOverlayBack(show, () => close())

const project = computed(() => ws.projects.find((p) => p.id === win.projectId) || null)
// The project vanished (deleted here or elsewhere) — nothing left to set up.
watch(
  () => !!win.projectId && !project.value && ws.projects.length > 0,
  (gone) => {
    if (gone) win.close()
  },
)

const initials = computed(() => (project.value?.name || '?').trim().slice(0, 2).toUpperCase())
const iconMode = computed(() => project.value?.icon_mode === 'icon')
const boxStyle = computed(() =>
  iconMode.value || project.value?.color === 'transparent'
    ? { background: 'transparent' }
    : { background: hueGrad(project.value?.color || 'var(--t-primary)') },
)
const glyphColor = computed(() =>
  iconMode.value ? project.value?.color || 'var(--t-primary)' : '',
)

// Counts on the tabs: the tabs report theirs as they load.
const counts = ref({ tags: null, stages: null, members: null })
function setCount(key, n) {
  counts.value = { ...counts.value, [key]: n }
}
watch(
  () => win.projectId,
  async (id) => {
    counts.value = { tags: null, stages: null, members: null }
    changed.value = false
    if (!id || !ws.currentId) return
    try {
      const { data } = await wsApi.members(ws.currentId)
      setCount('members', (data || []).length)
    } catch {
      // the count is decoration
    }
  },
)

const TAB_ICONS = {
  general: InformationCircleOutline,
  tags: PricetagsOutline,
  stages: TrailSignOutline,
  estimation: TimeOutline,
}
const tabs = computed(() =>
  TABS.map((key, i) => ({
    key,
    icon: TAB_ICONS[key],
    label: t(`project.window.tab.${key}`),
    count: key === 'tags' ? counts.value.tags : key === 'stages' ? counts.value.stages : null,
    digit: String(i + 1),
  })),
)

// Focus goes to the first row of the tab — not on a phone, where a focused
// field would pull the keyboard up over the window.
function focusPane() {
  if (!isMobile.value) nextTick(() => paneRef.value?.focusFirst?.())
}
function pickTab(key) {
  win.tab = key
  focusPane()
}

// Something was saved: the open board of this project reloads its tags,
// stages and estimate unit when the window closes.
const changed = ref(false)
function markChanged() {
  changed.value = true
}

function close() {
  if (changed.value && boardView.active && boardView.projectId === win.projectId) {
    boardView.bumpReload()
  }
  changed.value = false
  win.close()
}

// Leave for a page (a stage, #2993). Closing from the UI takes back the history
// entry Back would close the window with (useOverlayBack), and that `back()`
// lands after a push made at once — undoing it. So: close, wait for that
// popstate, then go.
async function navigate(path) {
  const ours = !!window.history.state?.tOverlay
  close()
  if (ours) {
    await new Promise((resolve) => {
      const done = () => {
        window.removeEventListener('popstate', done)
        clearTimeout(timer)
        resolve()
      }
      const timer = setTimeout(done, 400)
      window.addEventListener('popstate', done)
    })
  }
  router.push(path)
}

// The workspace windows the project's settings lead to. They are modals of
// their own, so this one steps aside.
function openOutside(name) {
  close()
  dialogs.open(name)
}

const paneRef = ref(null)
function onEsc() {
  if (paneRef.value?.escape?.()) return
  close()
}

// ⌘, toggles; 1–4 switch tabs; Esc steps out one layer at a time.
// Only while this window is the top layer — it may sit over the task modal
// («Управлять тегами…» in its tag picker).
const topmost = () => {
  if (!show.value || typeof document === 'undefined') return false
  if (document.querySelector('.n-dropdown-menu')) return false
  const mods = document.querySelectorAll('.n-modal-container .n-modal')
  const top = mods[mods.length - 1]
  return !!top && (top.classList.contains('pw-card') || !!top.querySelector('.pw-card'))
}
useShortcuts(
  {
    'project.close': onEsc,
    'project.tab': (d, e) => {
      const i = Number(d) - 1
      if (i >= 0 && i < TABS.length && !isTypingTarget(e?.target)) pickTab(TABS[i])
    },
  },
  { when: topmost },
)

watch(show, (on) => {
  if (on) focusPane()
})
</script>

<template>
  <n-modal
    v-model:show="show"
    :auto-focus="false"
    :close-on-esc="false"
    :mask-closable="true"
    transform-origin="center"
  >
    <div
      v-if="project"
      class="pw-card"
      :class="{ 'pw-mobile': isMobile }"
      role="dialog"
      aria-modal="true"
      :aria-label="t('project.window.title')"
      data-testid="project-window"
    >
      <header class="pw-head">
        <span class="pw-badge" :class="{ bare: iconMode }" :style="boxStyle">
          <ProjectIcon :icon="project.icon" :initials="initials" :size="14" :color="glyphColor" />
        </span>
        <span class="pw-name">{{ project.name }}</span>
        <span class="pw-sub">{{ t('project.window.title') }}</span>
        <span class="pw-grow" />
        <ShortcutKeys v-if="!isMobile" id="project.window" class="pw-kbd" />
        <button
          type="button"
          class="pw-close"
          :title="t('common.action.close')"
          data-testid="project-window-close"
          @click="close"
        >
          <n-icon :component="CloseOutline" :size="18" />
        </button>
        <Kbd v-if="!isMobile">{{ CAPS.escape }}</Kbd>
      </header>

      <div class="pw-body">
        <nav class="pw-tabs" :aria-label="t('project.window.title')">
          <button
            v-for="tb in tabs"
            :key="tb.key"
            type="button"
            class="pw-tab"
            :class="{ active: win.tab === tb.key }"
            :data-testid="`project-tab-${tb.key}`"
            @click="pickTab(tb.key)"
          >
            <n-icon :component="tb.icon" :size="16" class="pw-tab-ic" />
            <span class="pw-tab-l">{{ tb.label }}</span>
            <span v-if="tb.count != null" class="pw-tab-n">{{ tb.count }}</span>
          </button>
          <template v-if="!isMobile">
            <div class="pw-tabs-h">{{ t('project.window.outside') }}</div>
            <button type="button" class="pw-tab" @click="openOutside('members')">
              <n-icon :component="PeopleOutline" :size="16" class="pw-tab-ic" />
              <span class="pw-tab-l">{{ t('project.window.members') }}</span>
              <span v-if="counts.members != null" class="pw-tab-n">{{ counts.members }}</span>
              <n-icon :component="OpenOutline" :size="13" class="pw-tab-out" />
            </button>
            <button type="button" class="pw-tab" @click="openOutside('gitlab')">
              <n-icon :component="GitBranchOutline" :size="16" class="pw-tab-ic" />
              <span class="pw-tab-l">GitLab</span>
              <n-icon :component="OpenOutline" :size="13" class="pw-tab-out" />
            </button>
            <span class="pw-grow" />
            <div class="pw-tabs-foot">
              <Kbd>{{ cap(1) }}</Kbd
              ><span>–</span><Kbd>{{ cap(4) }}</Kbd>
              <span>{{ t('project.window.tabsHint') }}</span>
            </div>
          </template>
        </nav>

        <section class="pw-pane">
          <ProjectGeneralTab
            v-if="win.tab === 'general'"
            ref="paneRef"
            :project="project"
            @changed="markChanged"
            @deleted="close"
          />
          <ProjectTagsTab
            v-else-if="win.tab === 'tags'"
            ref="paneRef"
            :project="project"
            :initial-filter="win.filter"
            @count="setCount('tags', $event)"
            @changed="markChanged"
          />
          <ProjectStagesTab
            v-else-if="win.tab === 'stages'"
            ref="paneRef"
            :project="project"
            @count="setCount('stages', $event)"
            @changed="markChanged"
            @open-stage="navigate"
          />
          <ProjectEstimationTab v-else ref="paneRef" :project="project" @changed="markChanged" />
        </section>
      </div>
    </div>
  </n-modal>
</template>

<style scoped>
/* 1080×760, radius 18, the pop level, the drag shadow — as the task modal. */
.pw-card {
  display: flex;
  flex-direction: column;
  width: min(1080px, 96vw);
  height: min(760px, 92vh);
  border-radius: var(--t-r-col);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
}
.pw-card.pw-mobile {
  width: 100vw;
  height: 100dvh;
  max-height: none;
  border-radius: 0;
}
.pw-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 18px 10px 20px;
  min-width: 0;
}
.pw-badge {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
}
.pw-badge.bare {
  color: var(--t-text1);
}
.pw-name {
  font-size: 15px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.pw-sub {
  flex: none;
  font-size: 13px;
  color: var(--t-text2);
}
.pw-grow {
  flex: 1;
}
.pw-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.pw-close:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.pw-close:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.pw-body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: 28px;
  padding: 0 12px 12px;
}
.pw-mobile .pw-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 0 0;
}
/* The sunken tab column — --t-sub, radius 14 (mockup ProjectWin). */
.pw-tabs {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px;
  border-radius: 14px;
  background: var(--t-sub);
  min-height: 0;
}
.pw-mobile .pw-tabs {
  flex-direction: row;
  overflow-x: auto;
  border-radius: 0;
  padding: 6px 10px;
  scrollbar-width: none;
}
.pw-tab {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  padding: 0 10px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
  flex: none;
}
.pw-tab:hover {
  background: var(--t-hover);
}
.pw-tab:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.pw-tab.active {
  background: var(--t-active);
  font-weight: 650;
}
.pw-tab-ic {
  color: var(--t-text2);
  flex: none;
}
.pw-tab.active .pw-tab-ic :deep([stroke='currentColor']) {
  stroke: url(#t-accent-grad-svg);
}
.pw-tab.active .pw-tab-ic {
  color: var(--t-primary);
}
.pw-tab-l {
  flex: 1;
  white-space: nowrap;
}
.pw-tab-n {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.pw-tab-out {
  color: var(--t-text3);
}
.pw-tabs-h {
  margin: 14px 10px 6px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.pw-tabs-foot {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.pw-pane {
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.pw-mobile .pw-pane {
  flex: 1;
  padding: 0 14px 14px;
}
</style>
