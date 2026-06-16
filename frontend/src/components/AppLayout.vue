<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NDrawer,
  NDrawerContent,
} from 'naive-ui'
import Sidebar from './Sidebar.vue'
import Topbar from './Topbar.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useNotificationsStore } from '@/stores/notifications'
import { useResponsive } from '@/composables/useResponsive'
import { useRealtime } from '@/composables/useRealtime'
import { useSidebarSize } from '@/composables/useSidebarSize'

const ws = useWorkspacesStore()
const authStore = useAuthStore()
const notes = useNotificationsStore()
const { isMobile } = useResponsive()
const { collapsed, narrow, layoutWidth, applyDragWidth, toggle } = useSidebarSize()
const route = useRoute()

// ── draggable sidebar divider ──
// Drag only engages once the pointer actually moves past a small threshold, so a
// double-click (to toggle the rail) never flips on the no-transition "resizing"
// state — the toggle then animates smoothly like the expand.
const dragging = ref(false)
let pressX = 0
function onDragMove(e) {
  if (!dragging.value && Math.abs(e.clientX - pressX) < 4) return
  dragging.value = true
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  applyDragWidth(e.clientX)
}
function stopDrag() {
  dragging.value = false
  window.removeEventListener('pointermove', onDragMove)
  window.removeEventListener('pointerup', stopDrag)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}
function startDrag(e) {
  pressX = e.clientX
  window.addEventListener('pointermove', onDragMove)
  window.addEventListener('pointerup', stopDrag)
}
onBeforeUnmount(stopDrag)

// Live notifications for the bell (scoped to the current workspace, addressed
// to the current user).
useRealtime((ev) => {
  if (ev.scope === ws.currentId) notes.onEvent(ev, authStore.user?.id)
})

const drawerOpen = ref(false)

onMounted(async () => {
  await authStore.verify()
  await ws.loadWorkspaces()
  await notes.load()
})

// Close the mobile drawer on navigation.
watch(
  () => route.fullPath,
  () => (drawerOpen.value = false),
)
</script>

<template>
  <!-- Desktop: fixed sider + content -->
  <n-layout v-if="!isMobile" has-sider class="app-layout" :class="{ resizing: dragging }" style="height: 100vh">
    <n-layout-sider
      bordered
      :width="layoutWidth"
      :show-trigger="false"
      content-style="padding: 0; height: 100%"
    >
      <Sidebar :mobile="false" :collapsed="collapsed" :narrow="narrow" />
    </n-layout-sider>

    <!-- Drag to resize; double-click to toggle the icon rail. -->
    <div
      class="sider-resizer"
      :class="{ active: dragging }"
      :style="{ left: layoutWidth + 'px' }"
      title="Потяните, чтобы изменить ширину (двойной клик — свернуть)"
      @pointerdown.prevent="startDrag"
      @dblclick="toggle"
    >
      <span class="rz-bar" />
    </div>

    <n-layout>
      <n-layout-header bordered>
        <Topbar :show-tools="collapsed || narrow" />
      </n-layout-header>
      <n-layout-content content-style="padding: 16px" style="height: calc(100vh - 53px)">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </n-layout-content>
    </n-layout>
  </n-layout>

  <!-- Mobile: hamburger opens the sidebar in a drawer -->
  <n-layout v-else style="height: 100vh">
    <n-layout-header bordered>
      <Topbar :mobile="true" @menu="drawerOpen = true" />
    </n-layout-header>
    <n-layout-content content-style="padding: 12px" style="height: calc(100vh - 53px)">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </n-layout-content>
    <n-drawer v-model:show="drawerOpen" :width="280" placement="left">
      <n-drawer-content body-content-style="padding: 0">
        <Sidebar :mobile="true" />
      </n-drawer-content>
    </n-drawer>
  </n-layout>
</template>

<style scoped>
/* Drag handle sitting in the gutter between the sidebar and the content. */
.sider-resizer {
  position: fixed;
  top: 0;
  height: 100vh;
  /* Width matches the content's left padding so the bar centres in the gutter
     between the sidebar border and the first card. */
  width: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: col-resize;
  z-index: 6;
  /* Follow the sider's width animation on toggle; no lag while dragging. */
  transition: left 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}
.sider-resizer.active {
  transition: none;
}
.rz-bar {
  width: 4px;
  height: 44px;
  border-radius: 4px;
  background: var(--t-border);
  transition:
    background 0.15s ease,
    height 0.15s ease;
}
.sider-resizer:hover .rz-bar,
.sider-resizer.active .rz-bar {
  background: var(--t-accent-grad-vert);
  height: 72px;
}
/* While dragging, the sider must track the cursor with no width animation. */
.app-layout.resizing :deep(.n-layout-sider),
.app-layout.resizing :deep(.n-layout-sider-scroll-container) {
  transition: none !important;
}
</style>
