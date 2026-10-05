<script setup>
import { NButton, NIcon } from 'naive-ui'
import { MenuOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import SearchBar from './SearchBar.vue'
import NotificationsBell from './NotificationsBell.vue'
import { computed } from 'vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import BoardActions from './BoardActions.vue'
import BoardMobileMenu from './BoardMobileMenu.vue'
import HelpHint from './help/HelpHint.vue'
import { useBoardViewStore } from '@/stores/boardView'
import { useQuickStore } from '@/stores/quick'
import ShortcutKeys from './ShortcutKeys.vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

const props = defineProps({
  mobile: { type: Boolean, default: false },
})
defineEmits(['menu'])

const { t } = useI18n()
const board = useBoardViewStore()
const ws = useWorkspacesStore()
const quick = useQuickStore()
// An open document fills the header with its own controls (#2727); the
// cheat-sheet button gives up its place there — `?` still opens the sheet.
const route = useRoute()
const pageOwnsHeader = computed(() => /^\/documents\/./.test(route.path))
// Board actions only make sense on a desktop board view.
const onBoard = () => board.active && !props.mobile
// v2 (#2954): on a board the header's left side names where you are — project ›
// board. The view switch moved down into the board's sub-toolbar.
const crumbProject = computed(() => ws.projects.find((p) => p.id === board.projectId)?.name || '')
const crumbBoard = computed(() => board.board?.name || '')
</script>

<template>
  <div class="topbar" :class="{ mobile }">
    <n-button v-if="mobile" class="menu-btn" quaternary circle @click="$emit('menu')">
      <n-icon :component="MenuOutline" />
    </n-button>

    <!-- Page-owned header controls land in these two slots via <teleport>. The
         board fills its side of the header from a store, but a view whose
         controls are wired to a dozen pieces of its own state (the open
         document, #2727) teleports the markup instead of lifting the state out.
         The containers are always rendered so the teleport target exists. -->
    <div class="tb-left">
      <div v-if="onBoard() && crumbBoard" class="crumbs" data-testid="board-crumbs">
        <span v-if="crumbProject" class="crumb">{{ crumbProject }}</span>
        <n-icon
          v-if="crumbProject"
          :component="ChevronForwardOutline"
          :size="12"
          class="crumb-sep"
        />
        <span class="crumb crumb-cur">{{ crumbBoard }}</span>
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
      <BoardActions v-if="onBoard()" />
      <!-- Mobile board controls: layout + tags + archive in one menu. -->
      <BoardMobileMenu v-if="mobile && board.active" />
      <!-- Contextual help for whatever screen is open; hides itself on /help
           and picks its article from the route (#2794). -->
      <!-- The bell is the shell's, not the workspace's (v2): the feed spans
           every workspace, so it stays here whatever the sidebar does. -->
      <NotificationsBell />
      <!-- The cheat sheet (#2956); on a phone there is no keyboard to press. -->
      <button
        v-if="!mobile && !pageOwnsHeader"
        type="button"
        class="tb-keys"
        data-testid="shortcuts-button"
        :title="t('shell.keys.title')"
        @click="quick.sheetOpen = true"
      >
        <span class="tb-keys-l">{{ t('shell.keys.button') }}</span>
        <ShortcutKeys id="nav.shortcuts" />
      </button>
      <HelpHint :size="18" />
    </div>
  </div>
</template>

<style scoped>
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
/* A phone has no room for three tracks: back to a row where the search takes
   whatever the hamburger and the board menu leave. */
.topbar.mobile {
  display: flex;
  gap: 8px;
  padding-left: 52px;
}
.topbar.mobile .tb-left:empty,
.topbar.mobile .tb-left:has(> .tb-slot:empty:only-child) {
  display: none;
}
.topbar.mobile .tb-center {
  flex: 1;
}
.topbar.mobile .tb-right {
  flex: none;
}
.menu-btn {
  position: absolute;
  left: 10px;
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
.crumb-sep {
  flex: none;
  color: var(--t-text3);
}
/* Empty when no page teleports into it — no gap of its own in that case. */
.tb-slot {
  display: flex;
  align-items: center;
  gap: 8px;
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
