<script setup>
import { NButton, NIcon } from 'naive-ui'
import { MenuOutline } from '@vicons/ionicons5'
import SearchBar from './SearchBar.vue'
import WorkspaceTools from './WorkspaceTools.vue'
import BoardLayoutSwitch from './BoardLayoutSwitch.vue'
import BoardActions from './BoardActions.vue'
import BoardMobileMenu from './BoardMobileMenu.vue'
import HelpHint from './help/HelpHint.vue'
import { useBoardViewStore } from '@/stores/boardView'

const props = defineProps({
  mobile: { type: Boolean, default: false },
  showTools: { type: Boolean, default: false },
})
defineEmits(['menu'])

const board = useBoardViewStore()
// Board layout switcher / actions only make sense on a desktop board view.
const onBoard = () => board.active && !props.mobile
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
      <BoardLayoutSwitch v-if="onBoard()" />
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
      <HelpHint :size="18" />
      <!-- When the sidebar is collapsed/narrow, its tools slide over here. -->
      <WorkspaceTools v-if="showTools" placement="bottom-end" />
    </div>
  </div>
</template>

<style scoped>
.topbar {
  height: 52px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  position: relative;
}
.topbar.mobile {
  padding-left: 52px;
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
  flex: none;
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
  flex: 1;
  display: flex;
  justify-content: center;
  min-width: 0;
}
.search-wrap {
  width: 100%;
  max-width: 520px;
}
</style>
