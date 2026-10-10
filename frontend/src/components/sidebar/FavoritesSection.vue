<script setup>
// «Избранное» on top of the sidebar tree (#3115): the reader's starred boards
// in their own order. The same section head and rows as «Проекты» below —
// the board's icon and colour, its project in quiet text after the name.
// Shown only when there is something in it.
//
// Rows are stops of the tree's keyboard walk (↑/↓ go on into «Проекты», G S
// lands on the open board's row), ↵ opens, «.» / ⇧F10 the menu, ⌘⌫ takes the
// star off with an undo toast. Drag to reorder (a long press on touch).
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import draggable from 'vuedraggable'
import { NIcon, NButton, NText } from 'naive-ui'
import { EllipsisHorizontalOutline, OpenOutline, StarOutline } from '@vicons/ionicons5'
import { useFavoritesStore } from '@/stores/favorites'
import { useUndoStore } from '@/stores/undo'
import { useMessage } from '@/composables/useMessage'
import { handleRowKeys, handleTreeNav } from '@/composables/useRowKeys'
import { onDragStart, onDragEnd } from '@/composables/useSidebarDnd'
import { hueGrad } from '@/utils/gradient'
import { humanizeError } from '@/utils/errors'
import { pressMoved } from '@/utils/dnd'
import ProjectIcon from '../ProjectIcon.vue'
import TesseraIcon from '../TesseraIcon.vue'
import ActionMenu from '../kit/ActionMenu.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const favorites = useFavoritesStore()
const undo = useUndoStore()
const message = useMessage()

// A mutable mirror for vuedraggable, re-taken whenever the store's list moves.
const model = ref([])
watch(
  () => favorites.items,
  (v) => (model.value = [...v]),
  { immediate: true },
)
async function onChange(evt) {
  const info = evt.moved
  if (!info) return
  try {
    await favorites.move(info.element.board_id, info.newIndex)
  } catch (e) {
    message.error(humanizeError(e))
  }
}

const path = (f) => `/project/${f.project_slug}/board/${f.slug}`
function open(f) {
  router.push(path(f))
}
const active = (f) =>
  route.params.projectSlug === f.project_slug &&
  route.params.boardSlug === f.slug &&
  !route.query.milestone

// The board's icon as the tree draws it (ProjectRow): its own glyph or the
// kanban one, in a tinted box — or bare, the glyph tinted instead.
const colored = (f) => f.color && f.color !== 'transparent'
const hasIcon = (f) => !!(f.icon || colored(f))
const bare = (f) => f.icon_mode === 'icon' || f.color === 'transparent'
const box = (f) =>
  bare(f) ? { background: 'transparent' } : { background: hueGrad(f.color || 'var(--t-primary)') }
const glyph = (f) => (f.icon_mode === 'icon' ? (colored(f) ? f.color : 'var(--t-primary)') : '')
const initials = (f) => (f.name || '?').trim().slice(0, 2).toUpperCase()

// ── taking the star off: at once, with the undo (#2969) ──
async function unstar(f) {
  const name = f.name
  let removed
  try {
    removed = await favorites.remove(f.board_id)
  } catch (e) {
    message.error(humanizeError(e))
    return
  }
  if (!removed) return
  undo.push({
    icon: 'star',
    text: () => t('shell.tree.favoriteRemoved', { name }),
    doneText: () => t('shell.tree.favoriteRestored', { name }),
    errorText: () => t('shell.tree.favoriteRestoreFailed'),
    undo: () => favorites.restore(removed),
  })
}

// ── the row menu ──
const rows = {}
const menuOpen = ref(false)
const menuAt = ref(null)
const menuTarget = ref(null)
function openMenu(f, at = null) {
  menuTarget.value = f
  menuAt.value = at
  menuOpen.value = true
}
function onCtx(e, f) {
  if (pressMoved()) return
  openMenu(f, { x: e.clientX, y: e.clientY })
}
const menuAnchor = () =>
  menuAt.value || (menuTarget.value && rows[menuTarget.value.board_id]) || null
const menuItems = computed(() => [
  { key: 'open', label: t('shell.tree.favoriteOpen'), icon: OpenOutline, keys: 'row.open' },
  { gap: true },
  {
    key: 'unstar',
    label: t('shell.tree.favoriteRemove'),
    icon: StarOutline,
    keys: 'row.delete',
    testid: 'favorite-remove',
  },
])
function onMenu(key) {
  const f = menuTarget.value
  if (!f) return
  if (key === 'open') open(f)
  else if (key === 'unstar') unstar(f)
}

// Touch long-press → the menu (the native contextmenu is unreliable on the
// draggable rows inside the phone's drawer — the same as ProjectRow).
let timer = null
function touchStart(e, f) {
  const tt = e.touches && e.touches[0]
  if (!tt) return
  const x = tt.clientX
  const y = tt.clientY
  clearTimeout(timer)
  timer = setTimeout(() => {
    if (!pressMoved()) openMenu(f, { x, y })
  }, 450)
}
function touchCancel() {
  clearTimeout(timer)
}

// ── keys ──
const sectionEl = ref(null)
function onNav(e) {
  handleTreeNav(e, sectionEl.value?.closest('.sidebar') || sectionEl.value)
}
function onRowKey(e, f) {
  handleRowKeys(e, {
    'row.open': () => open(f),
    'row.menu': () => openMenu(f),
    'row.menuF10': () => openMenu(f),
    'row.delete': () => unstar(f),
  })
}
</script>

<template>
  <div
    v-if="favorites.items.length"
    ref="sectionEl"
    class="favs"
    data-testid="sidebar-favorites"
    @keydown="onNav"
  >
    <div class="fav-head" data-tree-row tabindex="-1">
      <n-text depth="3" strong>{{ t('shell.tree.favorites') }}</n-text>
    </div>
    <div class="fav-list">
      <draggable
        :list="model"
        group="sidebar-fav"
        item-key="board_id"
        ghost-class="sb-ghost"
        :animation="150"
        :delay="160"
        :delay-on-touch-only="true"
        :touch-start-threshold="6"
        @start="onDragStart"
        @end="onDragEnd"
        @change="onChange"
      >
        <template #item="{ element: f }">
          <div
            :ref="(el) => (rows[f.board_id] = el)"
            class="row fav-row"
            :class="{
              active: active(f),
              ctx: menuOpen && menuTarget?.board_id === f.board_id,
            }"
            data-tree-row
            data-testid="favorite-row"
            :data-board="f.board_id"
            tabindex="-1"
            :title="`${f.name} · ${f.project_name}`"
            @click="open(f)"
            @keydown="onRowKey($event, f)"
            @contextmenu.prevent.stop="onCtx($event, f)"
            @touchstart.passive="touchStart($event, f)"
            @touchend="touchCancel"
            @touchcancel="touchCancel"
          >
            <span class="bicon">
              <span
                v-if="hasIcon(f)"
                class="picon"
                :class="{ 'picon-bare': bare(f) }"
                :style="box(f)"
              >
                <ProjectIcon
                  v-if="f.icon"
                  :icon="f.icon"
                  :initials="initials(f)"
                  :size="12"
                  :color="glyph(f)"
                />
                <TesseraIcon
                  v-else
                  name="layout-kanban"
                  :variant="bare(f) ? 'outline' : 'filled'"
                  :size="12"
                  :style="bare(f) && glyph(f) ? { color: glyph(f) } : {}"
                />
              </span>
              <TesseraIcon v-else name="layout-kanban" :size="15" />
            </span>
            <span class="name">{{ f.name }}</span>
            <span class="proj" data-testid="favorite-project">{{ f.project_name }}</span>
            <n-button
              class="hover-btn"
              :class="{ open: menuOpen && menuTarget?.board_id === f.board_id && !menuAt }"
              text
              size="tiny"
              data-testid="favorite-menu"
              @click.stop="openMenu(f)"
            >
              <n-icon :component="EllipsisHorizontalOutline" />
            </n-button>
          </div>
        </template>
      </draggable>
    </div>

    <ActionMenu
      v-model:show="menuOpen"
      :anchor="menuAnchor"
      :placement="menuAt ? 'point' : 'right-start'"
      :width="260"
      :header="menuTarget?.name || ''"
      :header-aside="menuTarget?.project_name || ''"
      :items="menuItems"
      testid="favorite-actions"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.favs {
  display: flex;
  flex-direction: column;
  /* Its own height (the sidebar column must not squeeze it under the tree);
     a long list scrolls inside and leaves the tree below its room. */
  flex: none;
  max-height: 38%;
}
/* The section head — the same as «Проекты» (Sidebar.vue .proj-head). */
.fav-head {
  display: flex;
  align-items: center;
  height: 28px;
  padding: 0 16px;
  margin-top: 14px;
  flex: none;
  outline: 0;
  border-radius: var(--t-r-ctl);
}
.fav-head:focus-visible {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.fav-head :deep(.n-text) {
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}
.fav-list {
  flex: 0 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 0 8px;
  scrollbar-width: thin;
}
/* Rows — the tree's board rows (ProjectRow.vue). */
.row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  box-sizing: border-box;
  padding: 0 8px;
  border-radius: var(--t-r-ctl);
  cursor: pointer;
  font-size: 13px;
  color: var(--t-text2);
  outline: 0;
  user-select: none;
}
.row:hover {
  background: var(--t-hover-side);
}
.row:focus-visible {
  background: var(--t-hover-side);
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.row.ctx {
  background: var(--t-hover);
  box-shadow: var(--t-sh-card);
}
.row.active {
  background: var(--t-active);
  color: var(--t-text1);
  font-weight: 600;
}
.row.active .bicon :deep([fill='currentColor']) {
  fill: url(#t-accent-grad-svg);
}
.row.active .bicon :deep([stroke='currentColor']) {
  stroke: url(#t-accent-grad-svg);
}
.bicon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  color: var(--t-text3);
  flex: none;
}
.picon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 6px;
  color: #fff;
  font-size: 10px;
  line-height: 1;
  font-weight: 700;
  overflow: hidden;
  flex: none;
}
.picon-bare {
  color: var(--t-text1);
}
.name {
  flex: 0 1 auto;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--t-text1);
}
/* The project, quiet, after the name: it gives way first. */
.proj {
  flex: 1 1 0;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 12px;
  font-weight: 400;
  color: var(--t-text3);
}
.hover-btn {
  flex: none;
  opacity: 0;
}
.row:not(:hover):not(.ctx):not(:focus-visible) .hover-btn:not(.open) {
  width: 0;
  overflow: hidden;
}
.row:hover .hover-btn,
.row.ctx .hover-btn,
.row:focus-visible .hover-btn {
  opacity: 1;
}
.hover-btn.open {
  opacity: 1 !important;
  background: var(--t-active);
  color: var(--t-primary);
  border-radius: 7px;
}
</style>
