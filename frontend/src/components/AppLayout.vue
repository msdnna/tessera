<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NDrawer,
  NDrawerContent,
  useMessage,
} from 'naive-ui'
import Sidebar from './Sidebar.vue'
import Topbar from './Topbar.vue'
import ConflictResolverModal from './ConflictResolverModal.vue'
import { notificationChannels } from '@/api'
import { getDeviceId, deviceLabel } from '@/utils/device'
import { isTauri } from '@/utils/serverBase'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useNotificationsStore } from '@/stores/notifications'
import { useConflictsStore } from '@/stores/conflicts'
import { useResponsive } from '@/composables/useResponsive'
import { useRealtime } from '@/composables/useRealtime'
import { useSidebarSize } from '@/composables/useSidebarSize'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { useDesktopDeepLink } from '@/composables/useDesktopDeepLink'

const ws = useWorkspacesStore()
const authStore = useAuthStore()
const notes = useNotificationsStore()
const conflicts = useConflictsStore()
const { isMobile } = useResponsive()
const { collapsed, narrow, layoutWidth, applyDragWidth, toggle } = useSidebarSize()
const route = useRoute()
const router = useRouter()
const message = useMessage()

// Desktop: native-notification click → focus window + open the task.
useDesktopDeepLink(router)

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

// Live notifications for the bell + native device notifications, addressed to the
// current user. Not scoped to the current workspace — onEvent filters by user, and
// device notifications must fire wherever you are.
useRealtime(
  (ev) => {
    notes.onEvent(ev, authStore.user?.id)
    conflicts.onEvent(ev)
    onProjectGone(ev)
    onProjectSlugChanged(ev)
  },
  // Reconnect / resync: rebuild the sidebar tree, which is the layout-level state
  // that can drift while the socket is down (projects added/removed elsewhere).
  () => ws.refresh(),
)

// An admin changing a project's address rewrites the URL of every board under
// it. Anyone with such a board open is now pointing at an address that no
// longer resolves, so move them to the same board at its new address. The
// person who made the change already did this locally; this no-ops for them.
function onProjectSlugChanged(ev) {
  if (ev.type !== 'project.updated' || ev.scope !== ws.currentId) return
  const known = ws.projects.find((p) => p.id === ev.data?.id)
  if (!known || !ev.data?.slug || known.slug === ev.data.slug) return
  const viewing = route.params.projectSlug === known.slug
  ws.refresh()
  if (viewing) {
    router.replace({ ...route, params: { ...route.params, projectSlug: ev.data.slug } })
  }
}

// A project leaving the current workspace (deleted, or transferred elsewhere) fires
// `project.deleted` in that workspace. Keep every connected client's tree in sync,
// and — for anyone viewing a board of that project — bail to Home before the board
// starts 404-ing (the "Не найдено" toast the user reported). The deleter/transferrer
// already navigates locally; this no-ops for them (they're no longer on that route).
function onProjectGone(ev) {
  if (ev.type !== 'project.deleted' || ev.scope !== ws.currentId) return
  const slug = route.params.projectSlug
  const viewing = slug && ws.projects.find((p) => p.slug === slug)?.id === ev.data?.id
  ws.refresh() // drop it from the sidebar tree for everyone in this workspace
  if (viewing) {
    message.info('Проект был удалён или перенесён в другое пространство — открыта «Главная»')
    router.push('/')
  }
}

const drawerOpen = ref(false)
// Browser Back closes the mobile sidebar drawer instead of navigating.
useOverlayBack(drawerOpen, () => (drawerOpen.value = false))

onMounted(async () => {
  await authStore.verify()
  await ws.loadWorkspaces()
  await notes.load()
  conflicts.load(ws.currentId)
  // On desktop, ask for notification permission up front so reminders can fire
  // without waiting for the first event (best-effort).
  if (isTauri()) {
    try {
      const { isPermissionGranted, requestPermission } =
        await import('@tauri-apps/plugin-notification')
      if (!(await isPermissionGranted())) await requestPermission()
    } catch {
      /* plugin unavailable — non-fatal */
    }
  }
  // Register this client as a routable "device" channel (best-effort).
  try {
    await notificationChannels.registerDevice({
      device_id: getDeviceId(),
      label: deviceLabel(),
      platform: isTauri() ? 'desktop' : 'web',
    })
  } catch {
    /* offline / unauthorized — non-fatal */
  }
})

// Close the mobile drawer on navigation.
watch(
  () => route.fullPath,
  () => (drawerOpen.value = false),
)
// Refresh the open-conflict count when the active workspace changes.
watch(
  () => ws.currentId,
  (id) => conflicts.load(id),
)
</script>

<template>
  <!-- Single root element: the App-level <transition mode="out-in"> can only track
       a component with one root. A second sibling root (e.g. the conflict modal)
       turns this into a fragment, and the leave callback never fires when AppLayout
       unmounts on logout — leaving a blank screen until a hard refresh. -->
  <div class="app-shell">
    <!-- Desktop: fixed sider + content -->
    <n-layout
      v-if="!isMobile"
      has-sider
      class="app-layout"
      :class="{ resizing: dragging }"
      style="height: 100vh"
    >
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
        <n-layout-content
          class="page-slot page-slot--desktop"
          content-style="padding: 16px"
          style="height: calc(100vh - 53px)"
        >
          <router-view v-slot="{ Component }">
            <!-- NOT mode="out-in": leaving a heavy view (BoardView / KanbanBoard,
                 a multi-root component) stalls the out-in enter and blanks the
                 content until reload (#2708). The leaving page is taken out of
                 flow (position:absolute, see .page-leave-active) so the incoming
                 page doesn't jump while the old one fades. -->
            <transition name="page">
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
      <n-layout-content
        class="page-slot page-slot--mobile"
        content-style="padding: 12px"
        style="height: calc(100vh - 53px)"
      >
        <router-view v-slot="{ Component }">
          <!-- see the desktop branch: no out-in, leaving page goes absolute (#2708) -->
          <transition name="page">
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

    <!-- App-level conflict resolver: opened from any surface via the conflicts store
       (e.g. the «Конфликт» pill on a task card). -->
    <ConflictResolverModal
      v-model:show="conflicts.resolverOpen"
      :ws-id="ws.currentId"
      :focus-task-id="conflicts.focusTaskId"
      @resolved="conflicts.load()"
    />
  </div>
</template>

<style scoped>
/* Single transition-trackable root; the inner n-layouts own the 100vh height. */
.app-shell {
  height: 100vh;
}
/* Positioning context for the page transition: the leaving route view is pulled
   out of flow (.page-leave-active in main.css) so the incoming view fills the
   space immediately instead of stacking below the fading one. Scoped to the
   scroll container Naive renders inside n-layout-content. --page-pad mirrors the
   content padding so the absolutely-positioned leaving view keeps its inset while
   it fades (inset:0 would let it snap to the edges — the padding «blink» in #2708). */
.page-slot :deep(.n-layout-scroll-container) {
  position: relative;
}
.page-slot--desktop :deep(.n-layout-scroll-container) {
  --page-pad: 16px;
}
.page-slot--mobile :deep(.n-layout-scroll-container) {
  --page-pad: 12px;
}
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
/* Animate the rail's width on toggle (Naive doesn't transition an externally
   bound :width). */
.app-layout :deep(.n-layout-sider) {
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1) !important;
}
/* While dragging, the sider must track the cursor with no width animation. */
.app-layout.resizing :deep(.n-layout-sider),
.app-layout.resizing :deep(.n-layout-sider-scroll-container) {
  transition: none !important;
}
</style>
