<script setup>
import { NIcon, NTooltip } from 'naive-ui'
import {
  ChevronForwardOutline,
  CloudDownloadOutline,
  FileTrayOutline,
  StarOutline,
} from '@vicons/ionicons5'
import SearchBar from './SearchBar.vue'
import NotificationsBell from './NotificationsBell.vue'
import { computed, ref } from 'vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useBoardViewStore } from '@/stores/boardView'
import { useProjectWindowStore } from '@/stores/projectWindow'
import { useQuickStore } from '@/stores/quick'
import { useInboxStore } from '@/stores/inbox'
import { useToastsStore } from '@/stores/toasts'
import ShortcutKeys from './ShortcutKeys.vue'
import ActionMenu from './kit/ActionMenu.vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { workspaceName } from '@/utils/defaultNames'
import { stubTask } from '@/utils/featureStubs'

// The desktop header. A phone has a header of its own (mobile/MobileTopbar,
// #2976) next to the bottom navigation.

const { t } = useI18n()
const board = useBoardViewStore()
const ws = useWorkspacesStore()
const quick = useQuickStore()
const inbox = useInboxStore()
const toasts = useToastsStore()
const route = useRoute()
const router = useRouter()
const projectWindow = useProjectWindowStore()
const onBoard = () => board.active
// v2 (#2954): on a board the header's left side names where you are — project ›
// board. The view switch moved down into the board's sub-toolbar.
const project = computed(() => ws.projects.find((p) => p.id === board.projectId) || null)
const crumbProject = computed(() => project.value?.name || '')
const crumbBoard = computed(() => board.board?.name || '')
// Both crumbs are doors (#3112): the project opens its «Проект» window, the
// board lists the project's boards to jump to. The star after them is the
// favourite board — a stub until #3115.
const favoriteStub = stubTask('board.favorite')
function openProject() {
  if (board.projectId) projectWindow.open(board.projectId, 'general')
}
const boardCrumb = ref(null)
const boardsOpen = ref(false)
function toggleBoards() {
  boardsOpen.value = !boardsOpen.value
  // The tree loads a project's boards when it is expanded; the crumb may be
  // the first to ask.
  if (boardsOpen.value && board.projectId && !ws.boardsByProject[board.projectId])
    ws.loadBoards(board.projectId)
}
const boardItems = computed(() =>
  (ws.boardsByProject[board.projectId] || []).map((b) => ({
    key: b.id,
    label: b.name,
    glyph: 'layout-kanban',
    active: b.id === board.boardId,
  })),
)
function goBoard(id) {
  const b = (ws.boardsByProject[board.projectId] || []).find((x) => x.id === id)
  if (!b || b.id === board.boardId || !project.value) return
  router.push(`/project/${project.value.slug}/board/${b.slug}`)
}
// The start screen names itself and the workspace (v2, #2960).
const onHome = computed(() => route.path === '/')
// Screens with a section column name themselves the same way (v2, #3106):
// «Документы · пространство», «Настройки · профиль», «Администрирование ·
// сервер host».
const pageCrumb = computed(() => {
  const p = route.path
  if (p === '/documents' || p.startsWith('/documents/'))
    return { cur: t('documents.view.crumbRoot'), sub: workspaceName(ws.current) }
  if (p === '/settings') {
    // The section rides in ?s= (#3106); the crumb names the open one.
    const s = route.query.s
    const known = ['security', 'appearance', 'locale', 'notifications', 'app']
    return { cur: t('settings.title'), sub: t(`settings.nav.${known.includes(s) ? s : 'profile'}`) }
  }
  if (p === '/conferences') return { cur: t('conferences.title'), sub: workspaceName(ws.current) }
  if (p === '/admin')
    return {
      cur: t('settings.admin.title'),
      sub: t('settings.admin.crumb', { host: location.host }),
    }
  return null
})
</script>

<template>
  <div class="topbar">
    <!-- Page-owned header controls land in these two slots via <teleport>. The
         board fills its side of the header from a store, but a view whose
         controls are wired to a dozen pieces of its own state (the open
         document, #2727) teleports the markup instead of lifting the state out.
         The containers are always rendered so the teleport target exists. -->
    <div class="tb-left">
      <div v-if="onBoard() && crumbBoard" class="crumbs" data-testid="board-crumbs">
        <button
          v-if="crumbProject"
          type="button"
          class="crumb crumb-btn"
          :title="t('board.crumbs.projectSettings')"
          data-tour="crumb-project"
          data-testid="crumb-project"
          @click="openProject"
        >
          {{ crumbProject }}
        </button>
        <n-icon
          v-if="crumbProject"
          :component="ChevronForwardOutline"
          :size="12"
          class="crumb-sep"
        />
        <button
          ref="boardCrumb"
          type="button"
          class="crumb crumb-btn crumb-cur"
          :class="{ open: boardsOpen }"
          :title="t('board.crumbs.boards')"
          aria-haspopup="menu"
          :aria-expanded="boardsOpen"
          data-testid="crumb-board"
          @click="toggleBoards"
        >
          {{ crumbBoard }}
        </button>
        <n-tooltip :delay="300">
          <template #trigger>
            <span
              class="crumb-star"
              role="button"
              tabindex="0"
              aria-disabled="true"
              :aria-label="t('board.crumbs.favorite')"
              data-testid="crumb-favorite"
            >
              <n-icon :component="StarOutline" :size="14" />
            </span>
          </template>
          {{ t('board.crumbs.favorite') }} · {{ t('common.stub.soon', { n: favoriteStub }) }}
        </n-tooltip>
        <ActionMenu
          v-model:show="boardsOpen"
          :anchor="() => boardCrumb"
          placement="bottom-start"
          :offset="{ x: 0, y: 6 }"
          :width="260"
          :header="crumbProject"
          :items="boardItems"
          testid="crumb-boards"
          @select="goBoard"
        />
      </div>
      <div v-if="onHome" class="crumbs" data-testid="home-crumbs">
        <span class="crumb crumb-cur">{{ t('shell.home.title') }}</span>
        <span class="crumb-dot">·</span>
        <span class="crumb">{{ workspaceName(ws.current) }}</span>
      </div>
      <div v-if="pageCrumb" class="crumbs" data-testid="page-crumbs">
        <span class="crumb crumb-cur">{{ pageCrumb.cur }}</span>
        <span class="crumb-dot">·</span>
        <span class="crumb">{{ pageCrumb.sub }}</span>
      </div>
      <div id="tb-slot-left" class="tb-slot" />
    </div>

    <div class="tb-center">
      <div class="search-wrap">
        <SearchBar />
      </div>
    </div>

    <div class="tb-right">
      <div id="tb-slot-right" class="tb-slot" />
      <!-- «Позже» on the update toast leaves this chip until the next load
           (#3117); left of «Входящие» — help lives in the rail in v2. -->
      <n-tooltip v-if="toasts.updateChip" :delay="200">
        <template #trigger>
          <button
            type="button"
            class="tb-update"
            data-testid="update-chip"
            @click="toasts.reload()"
          >
            <n-icon :component="CloudDownloadOutline" :size="15" />
            {{ t('toasts.update.chip') }}
          </button>
        </template>
        {{ t('toasts.update.chipTip') }}
      </n-tooltip>
      <!-- The board's tags, stages and archive moved into its «⋯» (#3112); help
           is the rail's «?» and F1, on the article about the screen. -->
      <!-- The bell is the shell's, not the workspace's (v2): the feed spans
           every workspace, so it stays here whatever the sidebar does. -->
      <!-- «Входящие» (#2997): the panel, with the count of reminders that went
           off; next to the bell, which keeps mentions and GitLab events. -->
      <n-tooltip :delay="400">
        <template #trigger>
          <button
            type="button"
            class="tb-inbox"
            :class="{ on: inbox.panelOpen }"
            :aria-label="t('inbox.title')"
            :aria-pressed="inbox.panelOpen"
            data-testid="inbox-button"
            @click="inbox.toggle()"
          >
            <n-icon :component="FileTrayOutline" :size="18" />
            <span v-if="inbox.fired" class="tb-inbox-n" data-testid="inbox-fired">{{
              inbox.fired > 9 ? '9+' : inbox.fired
            }}</span>
          </button>
        </template>
        <span class="tb-tip">{{ t('inbox.title') }} <ShortcutKeys id="nav.goInbox" /></span>
      </n-tooltip>
      <NotificationsBell />
      <!-- The cheat sheet (#2956). -->
      <button
        type="button"
        class="tb-keys"
        data-testid="shortcuts-button"
        :title="t('shell.keys.title')"
        @click="quick.sheetOpen = true"
      >
        <span class="tb-keys-l">{{ t('shell.keys.button') }}</span>
        <ShortcutKeys id="nav.shortcuts" />
      </button>
    </div>
  </div>
</template>

<style scoped>
/* The update chip (#3117): the accent, softly — over the canvas, not a card. */
.tb-update {
  flex: none;
  height: 30px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px 0 9px;
  border: 0;
  border-radius: 8px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-bg)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-bg))
  );
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
}
.tb-update .n-icon {
  color: var(--t-primary);
}
.tb-update:hover {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 22%, var(--t-bg)),
    color-mix(in srgb, var(--t-primary) 34%, var(--t-bg))
  );
}
.tb-update:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
/* «Входящие» (#2997): a quiet button; the count is the fired reminders —
   attention, so the orange of a fired reminder, not the accent. */
.tb-inbox {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  min-width: 34px;
  padding: 0 8px;
  box-sizing: border-box;
  justify-content: center;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  cursor: pointer;
}
.tb-inbox:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tb-inbox.on {
  background: color-mix(in srgb, var(--t-primary) 18%, transparent);
  color: var(--t-primary);
}
.tb-inbox:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.tb-inbox-n {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  box-sizing: border-box;
  border-radius: 999px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 18px;
  text-align: center;
}
.tb-tip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
/* v2 (#2954): three tracks so the search sits on the true centre whatever the
   crumbs and actions on either side weigh. */
.topbar {
  height: 52px;
  display: grid;
  /* The right track never goes narrower than its actions; the crumbs on the
     left are what truncates on a narrow window. */
  grid-template-columns: minmax(0, 1fr) minmax(200px, 420px) minmax(max-content, 1fr);
  align-items: center;
  gap: 16px;
  padding: 0 16px;
  position: relative;
}
.tb-left,
.tb-right {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.tb-right {
  justify-content: flex-end;
}
.tb-keys {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 6px 0 10px;
  border: none;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.tb-keys:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* A narrow header keeps just the key cap. */
@media (max-width: 1439px) {
  .tb-keys-l {
    display: none;
  }
  .tb-keys {
    padding: 0 6px;
  }
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 13.5px;
}
.crumb {
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 220px;
}
.crumb-cur {
  font-weight: 600;
  color: var(--t-text1);
}
/* The crumbs are doors (#3112): text3 → text1 on hover, a soft well, no
   underline. */
.crumb-btn {
  flex: 0 1 auto;
  min-width: 0;
  height: 28px;
  margin: 0 -6px;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  font: inherit;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.crumb-btn:hover,
.crumb-btn.open {
  background: var(--t-hover);
  color: var(--t-text1);
}
.crumb-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
/* Favourite boards are not built yet (#3115): drawn, inert. */
.crumb-star {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin-left: 2px;
  border-radius: 7px;
  color: var(--t-text3);
  cursor: not-allowed;
  opacity: 0.7;
}
.crumb-sep,
.crumb-dot {
  flex: none;
  color: var(--t-text3);
}
/* Empty when no page teleports into it — no gap of its own in that case. */
.tb-slot {
  display: flex;
  align-items: center;
  gap: 8px;
  /* A page's crumbs (the stage page, #2993) shrink with the track. */
  min-width: 0;
}
.tb-slot:empty {
  display: none;
}
.tb-center {
  display: flex;
  justify-content: center;
  min-width: 0;
}
.search-wrap {
  width: 100%;
}
</style>
