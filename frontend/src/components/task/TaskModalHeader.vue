<script setup>
// Head of the task modal (v2, #2958): where the task lives — project › board ›
// #number, the crumbs doubling as the «move to another board» menu — and, on the
// right, the neighbour navigation (J/K), the layout switch, «⋯» and close.
//
// The «⋯» menu's contents stay with the modal (they act on its state: archive,
// delete, GitLab); they come in through the `more` slot.
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover } from 'naive-ui'
import {
  ChevronForwardOutline,
  ChevronUpOutline,
  ChevronDownOutline,
  CloseOutline,
  CopyOutline,
  CheckmarkOutline,
  EllipsisHorizontalOutline,
} from '@vicons/ionicons5'
import { projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { hueGrad } from '@/utils/gradient'
import ProjectIcon from '../ProjectIcon.vue'
import TesseraIcon from '../TesseraIcon.vue'
import TaskLayoutSwitch from '../TaskLayoutSwitch.vue'
import HelpHint from '../help/HelpHint.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  task: { type: Object, default: null },
  project: { type: Object, default: null },
  boardName: { type: String, default: '' },
  // { index, total } of the task in the board's current order; null hides J/K.
  nav: { type: Object, default: null },
  layout: { type: String, default: 'modal' },
  // No layout choice on a narrow screen (see effectiveTaskLayout).
  narrow: { type: Boolean, default: false },
  shared: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
})
const emit = defineEmits(['navigate', 'update:layout', 'share', 'close', 'transfer'])

const { t } = useI18n()
const store = useWorkspacesStore()

const initials = computed(() => (props.project?.name || '?').trim().slice(0, 2).toUpperCase())
// Same badge/icon colouring as the sidebar's project row.
const iconMode = computed(() => props.project?.icon_mode === 'icon')
const colored = computed(() => props.project?.color && props.project.color !== 'transparent')
const boxStyle = computed(() =>
  iconMode.value || props.project?.color === 'transparent'
    ? { background: 'transparent' }
    : { background: hueGrad(props.project?.color || 'var(--t-primary)') },
)
const glyphColor = computed(() =>
  iconMode.value ? (colored.value ? props.project.color : 'var(--t-primary)') : '',
)

// ── transfer to another board (the crumbs are its trigger) ──
const moveBoards = ref({}) // projectId -> board[]
const expandedProj = ref(new Set())
async function toggleProj(pid) {
  const s = new Set(expandedProj.value)
  if (s.has(pid)) s.delete(pid)
  else {
    s.add(pid)
    if (!moveBoards.value[pid]) {
      const r = await projApi.boards(pid)
      moveBoards.value = { ...moveBoards.value, [pid]: r.data || [] }
    }
  }
  expandedProj.value = s
}

const showMore = ref(false)
const showMove = ref(false)
const hasNav = computed(() => props.nav && props.nav.total > 1)
// Esc in the modal closes an open header menu before the modal itself.
function closeMenus() {
  if (!showMore.value && !showMove.value) return false
  showMore.value = false
  showMove.value = false
  return true
}
defineExpose({ closeMenus })
</script>

<template>
  <div class="tmh" :class="{ 'tmh-compact': layout === 'sidebar' }">
    <!-- An n-popover #trigger slot must resolve to exactly one child: the crumbs
         render only once the board is known. -->
    <n-popover
      v-if="project || boardName"
      v-model:show="showMove"
      trigger="click"
      placement="bottom-start"
      :disabled="readonly"
    >
      <template #trigger>
        <button type="button" class="tmh-crumbs" :title="readonly ? '' : t('task.head.moveTitle')">
          <span v-if="project" class="tmh-picon" :style="boxStyle">
            <ProjectIcon :icon="project.icon" :initials="initials" :size="12" :color="glyphColor" />
          </span>
          <span v-if="project" class="tmh-crumb">{{ project.name }}</span>
          <n-icon
            v-if="project && boardName"
            :component="ChevronForwardOutline"
            :size="12"
            class="tmh-sep"
          />
          <TesseraIcon v-if="boardName" name="layout-kanban" :size="14" class="tmh-bicon" />
          <span v-if="boardName" class="tmh-crumb">{{ boardName }}</span>
        </button>
      </template>
      <div class="menu move-menu">
        <div class="move-hint">{{ t('task.head.moveHint') }}</div>
        <div v-for="p in store.projects" :key="p.id">
          <div class="menu-item" @click="toggleProj(p.id)">
            <span class="grow">{{ p.name }}</span>
            <n-icon
              :component="ChevronForwardOutline"
              :size="12"
              class="move-chev"
              :class="{ open: expandedProj.has(p.id) }"
            />
          </div>
          <div v-if="expandedProj.has(p.id)" class="move-boards">
            <div
              v-for="bd in moveBoards[p.id] || []"
              :key="bd.id"
              class="menu-item board"
              @click="emit('transfer', bd.id)"
            >
              {{ bd.name }}
            </div>
            <span v-if="!(moveBoards[p.id] || []).length" class="move-hint">{{
              t('task.head.noBoards')
            }}</span>
          </div>
        </div>
      </div>
    </n-popover>
    <n-icon
      v-if="(project || boardName) && task?.number"
      :component="ChevronForwardOutline"
      :size="12"
      class="tmh-sep"
    />
    <span v-if="task?.number" class="tmh-num">#{{ task.number }}</span>
    <button
      v-if="task?.number"
      type="button"
      class="tmh-btn tmh-copy"
      :class="{ done: shared }"
      :title="shared ? t('task.head.shareDone') : t('task.head.copyLink')"
      @click="emit('share')"
    >
      <Transition name="tmh-pop" mode="out-in">
        <n-icon v-if="shared" key="ok" :component="CheckmarkOutline" :size="16" class="grad-icon" />
        <n-icon v-else key="copy" :component="CopyOutline" :size="16" />
      </Transition>
    </button>

    <span class="tmh-gap" />

    <template v-if="hasNav">
      <span class="tmh-nav">
        <ShortcutKeys id="task.next" />
        <ShortcutKeys id="task.prev" />
        <span class="tmh-count">{{
          t('task.head.navCount', { n: nav.index + 1, total: nav.total })
        }}</span>
      </span>
      <button
        type="button"
        class="tmh-btn"
        :title="t('task.head.prev')"
        data-testid="task-prev"
        :disabled="nav.index <= 0"
        @click="emit('navigate', -1)"
      >
        <n-icon :component="ChevronUpOutline" :size="17" />
      </button>
      <button
        type="button"
        class="tmh-btn"
        :title="t('task.head.next')"
        data-testid="task-next"
        :disabled="nav.index >= nav.total - 1"
        @click="emit('navigate', 1)"
      >
        <n-icon :component="ChevronDownOutline" :size="17" />
      </button>
      <span class="tmh-space" />
    </template>

    <TaskLayoutSwitch
      v-if="!narrow"
      :value="layout"
      @update:value="emit('update:layout', $event)"
    />
    <!-- The modal covers the topbar's «?», so it carries its own (#2794). -->
    <HelpHint slug="boards-and-tasks" :size="16" class="tmh-help" />
    <n-popover v-model:show="showMore" trigger="click" placement="bottom-end" :show-arrow="false">
      <template #trigger>
        <button type="button" class="tmh-btn" :title="t('task.head.more')" data-testid="task-more">
          <n-icon :component="EllipsisHorizontalOutline" :size="17" />
        </button>
      </template>
      <div class="menu tmh-more" @click="showMore = false">
        <slot name="more" />
      </div>
    </n-popover>
    <button
      type="button"
      class="tmh-btn tmh-close"
      :title="t('task.head.close')"
      data-testid="task-close"
      data-tour="tm-close"
      @click="emit('close')"
    >
      <n-icon :component="CloseOutline" :size="18" />
      <ShortcutKeys id="task.close" />
    </button>
  </div>
</template>

<style scoped>
@import './tab-shared.css';

.tmh {
  flex: 0 0 56px;
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 0 12px 0 20px;
  box-sizing: border-box;
}
.tmh-crumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  height: 30px;
  padding: 0 6px;
  margin-left: -6px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.tmh-crumbs:hover {
  background: var(--t-hover);
}
.tmh-crumbs:disabled,
.tmh-crumbs[disabled] {
  cursor: default;
}
.tmh-picon {
  flex: none;
  width: 20px;
  height: 20px;
  border-radius: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 9px;
  font-weight: 700;
}
.tmh-crumb {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 220px;
}
.tmh-sep {
  flex: none;
  color: var(--t-text3);
}
.tmh-bicon {
  flex: none;
  color: var(--t-text3);
}
.tmh-num {
  flex: none;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text1);
  font-variant-numeric: tabular-nums;
}
.tmh-gap {
  flex: 1;
}
.tmh-nav {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-right: 6px;
}
.tmh-count {
  margin-left: 4px;
  font-size: 12px;
  color: var(--t-text3);
  white-space: nowrap;
}
.tmh-space {
  width: 8px;
}
.tmh-btn,
.tmh :deep(.head-btn) {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 32px;
  height: 32px;
  padding: 0;
  border: none;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.tmh-btn:hover:not(:disabled),
.tmh :deep(.head-btn:hover) {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tmh-btn:disabled {
  opacity: 0.35;
  cursor: default;
}
.tmh-copy {
  width: 28px;
  height: 28px;
}
.tmh-close {
  width: auto;
  padding: 0 6px 0 8px;
}
.tmh-help {
  margin: 0 4px;
}
.tmh-more {
  min-width: 220px;
}
.move-menu {
  width: 240px;
  max-height: 320px;
  overflow-y: auto;
}
.move-hint {
  font-size: 12px;
  color: var(--t-text3);
  padding: 4px 6px;
}
.move-boards {
  padding-left: 12px;
}
.menu-item.board {
  color: var(--t-text2);
}
.move-chev {
  color: var(--t-text3);
  transition: transform 0.15s ease;
}
.move-chev.open {
  transform: rotate(90deg);
}
.grow {
  flex: 1;
}
.tmh-pop-enter-active,
.tmh-pop-leave-active {
  transition:
    opacity 0.14s ease,
    transform 0.14s ease;
}
.tmh-pop-enter-from,
.tmh-pop-leave-to {
  opacity: 0;
  transform: scale(0.7);
}
@media (prefers-reduced-motion: reduce) {
  .tmh-pop-enter-active,
  .tmh-pop-leave-active {
    transition: none;
  }
}
/* The 560px side panel keeps the arrows and drops their key hints. */
.tmh-compact .tmh-nav {
  display: none;
}
/* A phone has room for the crumbs or the navigation, not both. */
@media (max-width: 640px) {
  .tmh {
    padding: 0 8px 0 14px;
  }
  .tmh-nav,
  .tmh-crumb,
  .tmh-help {
    display: none;
  }
}
</style>
