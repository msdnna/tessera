<script setup>
// The phone's header (v2, #2976 — package G). 56px under the status bar.
//
// On a board it names where you are — the project's badge, the project over
// the board — and a tap opens the tree in the drawer (there is no hamburger:
// a top corner is out of the thumb's reach, and the tree is needed less often
// than the tabs). On the right: «Вид доски», the board's menu and the bell.
//
// Elsewhere it is the way back to where the screen was opened from — «‹ Ещё»
// for the screens the «Ещё» tab gathers, «‹ Этапы» on a stage — with the
// page's own title in the page itself. Screens that carry their own large title
// and no way back («Мои задачи», «Ещё», the feed) have no header at all; the
// layout decides that.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import { ChevronBackOutline, ChevronDownOutline, OptionsOutline } from '@vicons/ionicons5'
import ProjectIcon from '@/components/ProjectIcon.vue'
import BoardMobileMenu from '@/components/BoardMobileMenu.vue'
import NotificationsBell from '@/components/NotificationsBell.vue'
import { useBoardViewStore } from '@/stores/boardView'
import { useWorkspacesStore } from '@/stores/workspaces'
import { hueGrad } from '@/utils/gradient'

defineEmits(['drawer'])
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const board = useBoardViewStore()
const ws = useWorkspacesStore()

const onBoard = computed(() => board.active && /\/board\//.test(route.path))
const project = computed(() => ws.projects.find((p) => p.id === board.projectId) || null)
const initials = computed(() => (project.value?.name || '?').trim().slice(0, 2).toUpperCase())
// The project's badge as the tree draws it: a tinted box, or the glyph alone.
const bare = computed(
  () => project.value?.icon_mode === 'icon' || project.value?.color === 'transparent',
)
const boxStyle = computed(() =>
  bare.value ? {} : { background: hueGrad(project.value?.color || 'var(--t-primary)') },
)
const glyphColor = computed(() => {
  if (!bare.value) return ''
  const c = project.value?.color
  return c && c !== 'transparent' ? c : 'var(--t-primary)'
})

// Where «back» leads from this screen.
const parent = computed(() => {
  const p = route.path
  if (/^\/project\/[^/]+\/stage\//.test(p))
    return { label: 'shell.nav.milestones', to: '/milestones' }
  if (/^\/documents\/./.test(p)) return { label: 'shell.nav.documents', to: '/documents' }
  if (/^\/conferences\/./.test(p)) return { label: 'shell.nav.conferences', to: '/conferences' }
  return { label: 'shell.user.more', to: '/more' }
})
function back() {
  // Straight back when that is where the reader came from; otherwise to the
  // parent, so «back» never leaves the app or lands somewhere unrelated.
  if (window.history.state?.back === parent.value.to) router.back()
  else router.push(parent.value.to)
}
</script>

<template>
  <div class="mtop" data-testid="mobile-topbar">
    <button
      v-if="onBoard"
      type="button"
      class="mt-switch"
      :aria-label="t('shell.mobile.openTree')"
      data-testid="mobile-board-switch"
      @click="$emit('drawer')"
    >
      <span v-if="project" class="mt-badge" :class="{ bare }" :style="boxStyle">
        <ProjectIcon :icon="project.icon" :initials="initials" :size="16" :color="glyphColor" />
      </span>
      <span class="mt-names">
        <span class="mt-proj">{{ project?.name }}</span>
        <span class="mt-board">
          <span class="mt-board-t">{{ board.board?.name }}</span>
          <n-icon :component="ChevronDownOutline" :size="14" class="mt-chev" />
        </span>
      </span>
    </button>
    <button v-else type="button" class="mt-back" data-testid="mobile-back" @click="back">
      <n-icon :component="ChevronBackOutline" :size="22" />
      <span class="mt-back-l">{{ t(parent.label) }}</span>
    </button>

    <!-- Page-owned controls teleport here (see Topbar.vue). -->
    <div id="tb-slot-left" class="tb-slot" />
    <span class="mt-fill" />
    <div id="tb-slot-right" class="tb-slot" />

    <template v-if="onBoard">
      <button
        type="button"
        class="mt-icon"
        :aria-label="t('board.tools.customize')"
        data-testid="mobile-board-view"
        @click="board.openCustomize()"
      >
        <n-icon :component="OptionsOutline" :size="22" />
      </button>
      <BoardMobileMenu />
    </template>
    <NotificationsBell />
  </div>
</template>

<style scoped>
.mtop {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 56px;
  padding: 0 8px 0 10px;
  box-sizing: border-box;
}
.mt-switch {
  flex: 0 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 48px;
  padding: 0 8px 0 4px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.mt-switch:active,
.mt-back:active,
.mt-icon:active {
  background: var(--t-hover);
}
.mt-badge {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 9px;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
}
.mt-badge.bare {
  background: var(--t-pill);
  color: var(--t-text1);
}
.mt-names {
  min-width: 0;
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}
.mt-proj {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
  color: var(--t-text3);
}
.mt-board {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}
.mt-board-t {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 17px;
  font-weight: 700;
}
.mt-chev {
  flex: none;
  color: var(--t-text2);
}
.mt-back {
  flex: 0 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 2px;
  height: 44px;
  padding: 0 10px 0 2px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 15px;
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
.mt-back-l {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mt-fill {
  flex: 1;
}
.tb-slot {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}
.tb-slot:empty {
  display: none;
}
.mt-icon {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  padding: 0;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
}
</style>
