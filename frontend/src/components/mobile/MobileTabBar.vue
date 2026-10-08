<script setup>
// The phone's bottom navigation (v2, #2976 — package G, model A «Нижняя
// навигация, 5 вкладок»): Мои задачи · Доска · Входящие · Поиск · Ещё. Every
// daily scenario is one tap under the thumb; the count of reminders that went
// off stays in sight. The project tree is a drawer (tap on the board's name, a
// swipe from the left edge, or «Доска» once more while a board is open).
//
// 64px of targets 78×64 on --t-sidebar; the active tab carries a 56×30 soft
// accent pill behind its icon. The gesture bar under it continues the same
// surface (env(safe-area-inset-bottom)).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import {
  CheckboxOutline,
  GridOutline,
  FileTrayOutline,
  SearchOutline,
  MenuOutline,
} from '@vicons/ionicons5'
import { useInboxStore } from '@/stores/inbox'
import { usePaletteStore } from '@/stores/palette'
import { sGet } from '@/utils/serverStorage'
import { unwindOverlay } from '@/utils/overlayNav'

const emit = defineEmits(['drawer'])
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const inbox = useInboxStore()
const palette = usePaletteStore()

const onBoard = computed(() => /^\/(project\/[^/]+\/(board|stage)|board)\//.test(route.path))
const active = computed(() => {
  if (palette.open) return 'search'
  if (inbox.panelOpen) return 'inbox'
  const p = route.path
  if (p === '/') return 'home'
  if (onBoard.value) return 'board'
  return 'more'
})

const TABS = [
  { key: 'home', icon: CheckboxOutline, label: 'shell.nav.home' },
  { key: 'board', icon: GridOutline, label: 'shell.mobile.board' },
  { key: 'inbox', icon: FileTrayOutline, label: 'shell.nav.inbox' },
  { key: 'search', icon: SearchOutline, label: 'shell.mobile.search' },
  { key: 'more', icon: MenuOutline, label: 'shell.user.more' },
]

// Close whatever overlay a tab sits under before going somewhere, and wait for
// its history entry to unwind — otherwise that back() undoes the push.
async function leaveOverlays() {
  if (palette.open) await unwindOverlay(() => palette.hide())
  if (inbox.panelOpen) await unwindOverlay(() => inbox.close())
}
async function go(path) {
  await leaveOverlays()
  if (route.fullPath !== path) router.push(path)
}

async function tap(key) {
  if (key === 'home') return go('/')
  if (key === 'more') return go('/more')
  if (key === 'search') {
    if (palette.open) return palette.hide()
    if (inbox.panelOpen) await unwindOverlay(() => inbox.close())
    return palette.show()
  }
  if (key === 'inbox') {
    if (inbox.panelOpen) return inbox.close()
    if (palette.open) await unwindOverlay(() => palette.hide())
    return inbox.open()
  }
  // «Доска»: the board you had open last; once more on it — the tree.
  if (onBoard.value && active.value === 'board') return emit('drawer')
  const last = sGet('tessera_last_board')
  if (last) return go(last)
  await leaveOverlays()
  emit('drawer')
}
</script>

<template>
  <nav class="mnav" :aria-label="t('shell.mobile.nav')" data-testid="mobile-tabbar">
    <button
      v-for="tab in TABS"
      :key="tab.key"
      type="button"
      class="mnav-tab"
      :class="{ on: active === tab.key }"
      :aria-current="active === tab.key ? 'page' : null"
      :data-testid="`mobile-tab-${tab.key}`"
      @click="tap(tab.key)"
    >
      <span class="mnav-pill">
        <n-icon :component="tab.icon" :size="22" />
        <span
          v-if="tab.key === 'inbox' && inbox.fired"
          class="mnav-badge"
          data-testid="mobile-tab-inbox-count"
          >{{ inbox.fired > 9 ? '9+' : inbox.fired }}</span
        >
      </span>
      <span class="mnav-l">{{ t(tab.label) }}</span>
    </button>
  </nav>
</template>

<style scoped>
.mnav {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  /* Over the page, the FAB (60) and the page-like «Входящие» sheet; under
     Naive's modals and drawers (2000+), which hide it as full sheets should. */
  z-index: 90;
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  height: calc(64px + var(--safe-bottom));
  padding-bottom: var(--safe-bottom);
  box-sizing: border-box;
  background: var(--t-sidebar);
}
.mnav-tab {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 0;
  height: 64px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.mnav-pill {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 30px;
  border-radius: 15px;
  transition: background 0.15s ease;
}
.mnav-tab.on .mnav-pill {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 18%, transparent),
    color-mix(in srgb, var(--t-primary) 30%, transparent)
  );
  color: var(--t-primary);
}
.mnav-l {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  letter-spacing: -0.01em;
  line-height: 1.1;
}
.mnav-tab.on .mnav-l {
  color: var(--t-text1);
  font-weight: 600;
}
.mnav-tab:focus-visible {
  outline: none;
}
.mnav-tab:focus-visible .mnav-pill {
  box-shadow: 0 0 0 2px var(--t-primary);
}
/* The fired reminders: attention, so the orange of a fired reminder. */
.mnav-badge {
  position: absolute;
  top: -5px;
  right: 6px;
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
</style>
