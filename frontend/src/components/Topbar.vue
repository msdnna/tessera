<script setup>
import { NButton, NIcon } from 'naive-ui'
import { MenuOutline } from '@vicons/ionicons5'
import SearchBar from './SearchBar.vue'
import WorkspaceTools from './WorkspaceTools.vue'
import BoardLayoutSwitch from './BoardLayoutSwitch.vue'
import BoardActions from './BoardActions.vue'
import BoardMobileMenu from './BoardMobileMenu.vue'
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

    <div class="tb-left">
      <BoardLayoutSwitch v-if="onBoard()" />
    </div>

    <div class="tb-center">
      <div class="search-wrap">
        <SearchBar />
      </div>
    </div>

    <div class="tb-right">
      <BoardActions v-if="onBoard()" />
      <!-- Mobile board controls: layout + tags + archive in one menu. -->
      <BoardMobileMenu v-if="mobile && board.active" />
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
