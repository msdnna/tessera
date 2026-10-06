<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NDrawer,
  NDrawerContent,
  NTooltip,
  NIcon,
  useMessage,
} from 'naive-ui'
import { ChevronBackOutline, ChevronForwardOutline } from '@vicons/ionicons5'
import ShortcutKeys from './ShortcutKeys.vue'
import ShortcutPending from './ShortcutPending.vue'
import CommandPalette from './palette/CommandPalette.vue'
import QuickActionsFab from './QuickActionsFab.vue'
import QuickCreateModal from './QuickCreateModal.vue'
import ShortcutsModal from './ShortcutsModal.vue'
import WorkspaceDialogsHost from './WorkspaceDialogsHost.vue'
import Sidebar from './Sidebar.vue'
import AppRail from './rail/AppRail.vue'
import Topbar from './Topbar.vue'
import ConflictResolverModal from './ConflictResolverModal.vue'
import WhatsNewModal from './WhatsNewModal.vue'
import SidebarSpotlight from './SidebarSpotlight.vue'
import TourOverlay from './TourOverlay.vue'
import HelpDrawer from './help/HelpDrawer.vue'
import HelpCenterModal from './help/HelpCenterModal.vue'
import { notificationChannels } from '@/api'
import { getDeviceId, deviceLabel } from '@/utils/device'
import { isTauri } from '@/utils/serverBase'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useNotificationsStore } from '@/stores/notifications'
import { useConflictsStore } from '@/stores/conflicts'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useTourStore } from '@/stores/tour'
import { useResponsive } from '@/composables/useResponsive'
import { useRealtime } from '@/composables/useRealtime'
import { useSidebarSize } from '@/composables/useSidebarSize'
import { useOverlayBack } from '@/composables/useOverlayBack'
import { useDesktopDeepLink } from '@/composables/useDesktopDeepLink'
import { useShortcuts } from '@/composables/useShortcuts'
import { install as installShortcuts } from '@/shortcuts/dispatcher'
import { useThemeStore } from '@/stores/theme'
import { usePaletteStore } from '@/stores/palette'
import { sGet } from '@/utils/serverStorage'

const { t } = useI18n()
const ws = useWorkspacesStore()
const authStore = useAuthStore()
const notes = useNotificationsStore()
const conflicts = useConflictsStore()
const whatsNew = useWhatsNewStore()
const tour = useTourStore()
const { isMobile } = useResponsive()
const { collapsed, layoutWidth, toggle } = useSidebarSize()

// The rail (#2955). On desktop it is docked at the app root (App.vue), so it also
// covers the login/offline screens (#2930); on the web it has nothing to offer
// there and lives here, with the signed-in shell. Either way this layout
// reserves its gutter via the global `--app-rail-w` custom property. On a phone
// the rail moves into the sidebar drawer.
const webRail = computed(() => !isTauri() && !isMobile.value)
watchEffect(() => {
  if (!isTauri())
    document.documentElement.style.setProperty('--app-rail-w', webRail.value ? '56px' : '0px')
})
onBeforeUnmount(() => {
  if (!isTauri()) document.documentElement.style.setProperty('--app-rail-w', '0px')
})
const route = useRoute()
const router = useRouter()
// The start screen's header is translucent and lies over the page, so the
// aurora runs under it (v2, #2960).
const homeGlass = computed(() => route.path === '/')
const message = useMessage()

// Desktop: native-notification click → focus window + open the task.
useDesktopDeepLink(router)

// ── keyboard ──
// The app-wide shortcut layer (v2, #2956): one window listener for every
// catalog shortcut, installed for as long as the signed-in shell is up. The
// shell owns the shortcuts that don't belong to any one screen.
let uninstallShortcuts = null
onMounted(() => (uninstallShortcuts = installShortcuts()))
onBeforeUnmount(() => uninstallShortcuts?.())

const theme = useThemeStore()
const palette = usePaletteStore()
useShortcuts({
  'nav.palette': () => palette.toggle(),
  // v2 (#2953): no drag-resizing — one width, collapsed to the icon rail by the
  // edge button or the `[` key. The drawer on mobile has its own button.
  'nav.sidebar': () => !isMobile.value && toggle(),
  'nav.goMine': () => router.push('/'),
  // The board the user last had open (BoardView remembers it); Home if none.
  'nav.goBoard': () => router.push(sGet('tessera_last_board') || '/'),
  'nav.goInbox': () => notes.requestFeed(),
  'view.theme': () => theme.toggle(),
})

// Live notifications for the bell + native device notifications, addressed to the
// current user. Not scoped to the current workspace — onEvent filters by user, and
// device notifications must fire wherever you are.
useRealtime(
  (ev) => {
    notes.onEvent(ev, authStore.user?.id)
    conflicts.onEvent(ev)
    onProjectGone(ev)
    onProjectSlugChanged(ev)
    // A renamed or recoloured workspace repaints its rail tile everywhere.
    if (ev.type === 'workspace.updated') ws.patchWorkspace(ev.data)
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
    message.info(t('shell.layout.projectGone'))
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
  // "What's new" after an update + sidebar spotlight hints (#2749). Best-effort:
  // decides on its own whether anything is worth showing (and stays silent on a
  // first run or when offline).
  //
  // The Get Started guide (#2753) rides on the same acks, so it decides right
  // after the load — no second request, and no chance of the two overlapping:
  // the guide's audience (brand-new accounts) is exactly the audience What's New
  // stays silent for. Only when the acks were actually read (`load()` → true):
  // an empty set from a failed request would re-run the guide for a user who has
  // already been through it.
  whatsNew
    .load()
    .then((ok) => ok && tour.autoStart({ acked: whatsNew.acked, mobile: isMobile.value }))
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
  <div
    class="app-shell"
    :style="{
      '--app-content-left': isMobile ? '0px' : `calc(var(--app-rail-w, 0px) + ${layoutWidth}px)`,
    }"
  >
    <!-- Web rail of workspaces (#2955), docked like the desktop one in App.vue. -->
    <div v-if="webRail" class="app-rail-dock"><AppRail /></div>

    <!-- Desktop: fixed sider + content -->
    <n-layout v-if="!isMobile" has-sider class="app-layout" style="height: 100vh">
      <n-layout-sider
        :width="layoutWidth"
        :show-trigger="false"
        content-style="padding: 0; height: 100%"
      >
        <Sidebar :mobile="false" :collapsed="collapsed" />
      </n-layout-sider>

      <!-- Collapse / expand: a round button on the sidebar's edge, shown while the
           pointer is over the sidebar (v2, #2953). -->
      <n-tooltip placement="right" :delay="400">
        <template #trigger>
          <button
            type="button"
            class="sider-toggle"
            data-testid="sidebar-toggle"
            :style="{ left: `calc(var(--app-rail-w, 0px) + ${layoutWidth}px - 12px)` }"
            :aria-label="t(collapsed ? 'shell.layout.expand' : 'shell.layout.collapse')"
            @click="toggle"
          >
            <n-icon
              :component="collapsed ? ChevronForwardOutline : ChevronBackOutline"
              :size="14"
            />
          </button>
        </template>
        <span class="sider-tip">
          {{ t(collapsed ? 'shell.layout.expand' : 'shell.layout.collapse') }}
          <ShortcutKeys id="nav.sidebar" />
        </span>
      </n-tooltip>

      <n-layout class="work-pane" :class="{ 'home-glass': homeGlass }">
        <n-layout-header :class="{ 'hdr-glass': homeGlass }">
          <Topbar />
        </n-layout-header>
        <n-layout-content
          class="page-slot page-slot--desktop"
          :content-style="homeGlass ? 'padding: 68px 16px 16px' : 'padding: 16px'"
          :style="{ height: homeGlass ? '100vh' : 'calc(100vh - 52px)' }"
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
        <QuickActionsFab />
      </n-layout>
    </n-layout>

    <!-- Mobile: hamburger opens the sidebar in a drawer -->
    <n-layout v-else class="work-pane" :class="{ 'home-glass': homeGlass }" style="height: 100vh">
      <n-layout-header :class="{ 'hdr-glass': homeGlass }">
        <Topbar :mobile="true" @menu="drawerOpen = true" />
      </n-layout-header>
      <n-layout-content
        class="page-slot page-slot--mobile"
        :content-style="homeGlass ? 'padding: 64px 12px 12px' : 'padding: 12px'"
        :style="{ height: homeGlass ? '100vh' : 'calc(100vh - 52px)' }"
      >
        <router-view v-slot="{ Component }">
          <!-- see the desktop branch: no out-in, leaving page goes absolute (#2708) -->
          <transition name="page">
            <component :is="Component" />
          </transition>
        </router-view>
      </n-layout-content>
      <QuickActionsFab />
      <n-drawer v-model:show="drawerOpen" :width="280" placement="left">
        <n-drawer-content body-content-style="padding: 0">
          <!-- The rail rides along in the drawer on a phone (#2955). -->
          <div class="drawer-split">
            <AppRail />
            <Sidebar :mobile="true" class="drawer-sidebar" />
          </div>
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

    <!-- "Что нового" after an update, then one-at-a-time sidebar spotlight hints
         with an animated arrow to the new section (#2749). -->
    <WhatsNewModal />
    <SidebarSpotlight />

    <!-- Get Started guide (#2753). Inert until the tour store is started — the
         autostart / «Обучение» entry point is wired up separately. -->
    <TourOverlay />

    <!-- Contextual help (#2794): one drawer for the whole app, opened by the «?»
         hints and by the «Справка» results in global search. -->
    <HelpDrawer />

    <!-- The help centre itself (#2792): a modal over the current screen, opened
         from the sidebar's «Помощь» menu. Not a route — reading the manual must
         not cost the reader the board they had open. -->
    <HelpCenterModal />

    <!-- ⌘K: search, commands and task actions (#2957). -->
    <CommandPalette />
    <!-- Quick create from the FAB / «N, then T/R/M», and the `?` cheat sheet (#2956). -->
    <QuickCreateModal />
    <ShortcutsModal />
    <WorkspaceDialogsHost />

    <!-- «G …» while a multi-key shortcut waits for its next key (#2956). -->
    <ShortcutPending />
  </div>
</template>

<style scoped>
/* v2 start screen (#2960): the header lies over the page, half see-through, and
   the aurora drifts under it. */
.home-glass {
  position: relative;
}
.hdr-glass {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 5;
  background: color-mix(in srgb, var(--t-bg) 52%, transparent) !important;
  backdrop-filter: saturate(1.4) blur(18px);
  -webkit-backdrop-filter: saturate(1.4) blur(18px);
}
:global([data-theme='light']) .hdr-glass {
  background: color-mix(in srgb, var(--t-bg) 58%, transparent) !important;
}

/* Single transition-trackable root; the inner n-layouts own the 100vh height.
   overflow:hidden pins the shell to the viewport — the scrolling belongs to the
   n-layout-content inside, never to the page itself (#2930). */
.app-shell {
  height: 100vh;
  overflow: hidden;
  /* Reserve the gutter for the app-root server rail (#2930); 0 on web/mobile
     where --app-rail-w is unset/0. border-box so the padding eats into the
     100vw width instead of overflowing it. */
  box-sizing: border-box;
  padding-left: var(--app-rail-w, 0px);
}
/* Same dock as App.vue's desktop rail: pinned left, under Naive's overlays. */
.app-rail-dock {
  position: fixed;
  left: 0;
  top: 0;
  bottom: 0;
  z-index: 5;
}
.drawer-split {
  display: flex;
  height: 100%;
}
.drawer-sidebar {
  flex: 1;
  min-width: 0;
}
/* Positioning context for the working-area version footer. */
.work-pane {
  position: relative;
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
/* Collapse / expand button on the sidebar's edge: a small round control on the
   popover level, revealed while the pointer is over the sidebar or the button. */
.sider-toggle {
  position: fixed;
  /* Mid-height on the edge — where the old resize grip sat, clear of the brand
     tools and the workspace switcher at the top. */
  top: calc(50% - 12px);
  z-index: 6;
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text2);
  cursor: pointer;
  opacity: 0;
  transition:
    left 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.15s ease;
}
.app-layout:has(.n-layout-sider:hover) .sider-toggle,
.sider-toggle:hover,
.sider-toggle:focus-visible {
  opacity: 1;
}
.sider-toggle:hover {
  color: var(--t-text1);
}
.sider-tip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
/* Animate the rail's width on toggle (Naive doesn't transition an externally
   bound :width). */
.app-layout :deep(.n-layout-sider) {
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1) !important;
}
</style>
