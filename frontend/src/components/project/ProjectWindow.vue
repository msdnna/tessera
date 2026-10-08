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
import {
  InformationCircleOutline,
  PricetagsOutline,
  TrailSignOutline,
  TimeOutline,
  PeopleOutline,
  GitBranchOutline,
} from '@vicons/ionicons5'
import { useProjectWindowStore, TABS } from '@/stores/projectWindow'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useWsDialogsStore } from '@/stores/wsDialogs'
import { useBoardViewStore } from '@/stores/boardView'
import { useResponsive } from '@/composables/useResponsive'
import { workspaces as wsApi } from '@/api'
import { hueGrad } from '@/utils/gradient'
import LargeWindow from '../kit/LargeWindow.vue'
import ProjectIcon from '../ProjectIcon.vue'
import ShortcutKeys from '../ShortcutKeys.vue'
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
// The tabs, then the workspace windows that also concern the project — as
// doors (they are windows of their own; this one steps aside).
const navItems = computed(() => [
  ...TABS.map((key) => ({
    key,
    icon: TAB_ICONS[key],
    label: t(`project.window.tab.${key}`),
    count: key === 'tags' ? counts.value.tags : key === 'stages' ? counts.value.stages : null,
  })),
  { heading: t('project.window.outside'), desktop: true },
  {
    key: 'members',
    out: true,
    desktop: true,
    icon: PeopleOutline,
    label: t('project.window.members'),
    count: counts.value.members,
    onClick: () => openOutside('members'),
  },
  {
    key: 'gitlab',
    out: true,
    desktop: true,
    icon: GitBranchOutline,
    label: 'GitLab',
    onClick: () => openOutside('gitlab'),
  },
])
const tab = computed({
  get: () => win.tab,
  set: (key) => pickTab(key),
})

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

// Esc: whatever the tab holds open first (LargeWindow asks), then the window.
// The frame owns the keys (1–4, Esc), the Back button and the scrim.
const paneRef = ref(null)
const paneEscape = () => !!paneRef.value?.escape?.()

watch(show, (on) => {
  if (on) focusPane()
})
</script>

<template>
  <LargeWindow
    v-model:show="show"
    v-model:section="tab"
    :title="project ? project.name : ''"
    :sub="t('project.window.title')"
    :foot-hint="t('project.window.tabsHint')"
    :items="navItems"
    :escape="paneEscape"
    :restore-focus="false"
    testid="project-window"
    close-testid="project-window-close"
    tab-testid="project-tab"
  >
    <template #badge>
      <span v-if="project" class="pw-badge" :class="{ bare: iconMode }" :style="boxStyle">
        <ProjectIcon :icon="project.icon" :initials="initials" :size="14" :color="glyphColor" />
      </span>
    </template>
    <template #aside>
      <ShortcutKeys v-if="!isMobile" id="project.window" class="pw-kbd" />
    </template>

    <template v-if="project">
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
    </template>
  </LargeWindow>
</template>

<style scoped>
/* The project's own badge stands in for the frame's tile: its colour and icon
   (a glyph in its colour when the project shows the icon bare). */
.pw-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  color: #fff;
}
.pw-badge.bare {
  color: var(--t-text1);
}
</style>
