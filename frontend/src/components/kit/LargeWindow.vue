<script setup>
// The large window of the v2 interaction model (tier 2, #2975 — mockups
// ProjectWin, GitlabWin, ConflictWin, JobsWin, MembersWin, CmdsWin): the frame
// every big window shares, so the five of package F and the «Проект» window
// read as one family.
//
//   1080×760, radius 18, the pop level, the drag shadow over the scrim.
//   A 56px header: a 24px gradient tile, crumbs (text3) → the title (15/650),
//   the owner's status on the right (#aside), then ✕ with its Esc cap.
//   A sunken 220px navigation column on --t-sub (radius 14, rows 34px, the
//   active one on --t-active), digits 1…N switch its sections.
//   The pane is the default slot.
//   On a phone the window takes the whole screen and the column becomes a row
//   of pills under the header.
//
// Esc steps out one layer at a time: whatever the pane holds open (its
// `escape()` returns true when it closed something) → the window. Focus goes
// back to the element that opened it; Back in the browser closes it.
//
// Navigation items (`items`):
//   { key, label, icon, count, tone: 'danger', testid }  a section
//   { key, label, icon, stub: 3075 }                     not built yet («Скоро · #N»)
//   { key, label, icon, count, out: true, onClick }      a door to another window
//   { heading: 'Тип' }                                    a group title
//   any of them with `desktop: true` is left out of the phone's pill row.
import { computed, nextTick, useId, useSlots, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NModal, NIcon, NTooltip } from 'naive-ui'
import { ChevronForwardOutline, CloseOutline, OpenOutline } from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS, cap } from '@/utils/keyCaps'
import { isTypingTarget } from '@/utils/keys'
import { useShortcuts } from '@/composables/useShortcuts'
import { useResponsive } from '@/composables/useResponsive'
import { useOverlayBack } from '@/composables/useOverlayBack'

const props = defineProps({
  show: { type: Boolean, default: false },
  // Crumbs before the title (text3), and the title itself.
  crumbs: { type: Array, default: () => [] },
  title: { type: String, default: '' },
  // Quiet words after the title (the «Проект» window: «Настройки проекта»).
  sub: { type: String, default: '' },
  // The header tile: an icon on a gradient of `tone` (the accent by default).
  icon: { type: [Object, Function], default: null },
  tone: { type: String, default: '' },
  items: { type: Array, default: () => [] },
  section: { type: String, default: '' },
  // Closes whatever the pane holds open; true when it did (Esc stops there).
  escape: { type: Function, default: null },
  testid: { type: String, default: 'large-window' },
  closeTestid: { type: String, default: '' },
  tabTestid: { type: String, default: 'lw-tab' },
  // The window pushes a history entry so Back closes it (off where the owner
  // already does that).
  historyBack: { type: Boolean, default: true },
  // Focus returns to the opener on close (off where the owner's store does it).
  restoreFocus: { type: Boolean, default: true },
  // Without the navigation column (the conflicts window has its own list there).
  noNav: { type: Boolean, default: false },
  // The word after «1 – N» at the foot of the column.
  footHint: { type: String, default: '' },
  // The fullscreen editor's frame (#3118, PackageJ «Полноэкранный»): 1240×788,
  // and its ✕ is «Свернуть» (the window folds back into the field it came from).
  wide: { type: Boolean, default: false },
  closeIcon: { type: [Object, Function], default: null },
  closeLabel: { type: String, default: '' },
})
const emit = defineEmits(['update:show', 'update:section', 'opened'])

const { t } = useI18n()
const { isMobile } = useResponsive()
const uid = useId()
// An owner's own badge (the project's colour and icon) brings its own paint.
const slots = useSlots()
const ownBadge = computed(() => !!slots.badge)

const visible = computed({
  get: () => props.show,
  set: (v) => {
    if (!v) close()
  },
})
if (props.historyBack) useOverlayBack(visible, () => close())

let returnTo = null
watch(
  () => props.show,
  (on) => {
    if (on) {
      returnTo = typeof document !== 'undefined' ? document.activeElement : null
      nextTick(() => emit('opened'))
      return
    }
    const el = returnTo
    returnTo = null
    if (props.restoreFocus && el && typeof el.focus === 'function' && el.isConnected) {
      setTimeout(() => el.focus(), 0)
    }
  },
)

function close() {
  emit('update:show', false)
}

// The sections digits reach: real ones only — a stub or a door to another
// window is not a section.
const sections = computed(() => props.items.filter((i) => i.key && !i.heading && !i.stub && !i.out))
const navItems = computed(() =>
  isMobile.value ? props.items.filter((i) => !i.desktop && !i.heading) : props.items,
)
const digitOf = (item) => {
  const i = sections.value.indexOf(item)
  return i >= 0 && i < 9 ? String(i + 1) : ''
}

function pick(item) {
  if (item.stub) return
  if (item.out) {
    item.onClick?.()
    return
  }
  emit('update:section', item.key)
}

function onEsc() {
  if (props.escape?.()) return
  close()
}

// The window's keys answer only while it is the top layer: it may sit over
// the task modal, and another large window may sit over it (the conflicts
// window over GitLab's).
const topmost = () => {
  if (!props.show || typeof document === 'undefined') return false
  if (document.querySelector('.n-dropdown-menu, .n-base-select-menu')) return false
  const mods = document.querySelectorAll('.n-modal-container .n-modal')
  const top = mods[mods.length - 1]
  const mine = `[data-lw="${uid}"]`
  return !!top && (top.matches(mine) || !!top.querySelector(mine))
}
useShortcuts(
  {
    'win.close': onEsc,
    'win.section': (d, e) => {
      const item = sections.value[Number(d) - 1]
      if (item && !isTypingTarget(e?.target)) pick(item)
    },
  },
  { when: topmost },
)

defineExpose({ topmost, close })
</script>

<template>
  <n-modal
    v-model:show="visible"
    :auto-focus="false"
    :close-on-esc="false"
    :mask-closable="true"
    transform-origin="center"
  >
    <!-- v-if: the card leaves at once rather than through the scrim's fade —
         a key pressed right after closing (E on the stage page the window led
         to) must not still find a modal on screen and be swallowed. -->
    <div
      v-if="show"
      class="lw-card"
      :class="{ 'lw-mobile': isMobile, 'lw-nonav': noNav, 'lw-wide': wide }"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      :data-lw="uid"
      :data-testid="testid"
    >
      <header class="lw-head">
        <span
          class="lw-badge"
          :style="tone ? { '--lw-tone': tone } : null"
          :class="{ toned: !!tone, own: ownBadge }"
        >
          <slot name="badge">
            <n-icon v-if="icon" :component="icon" :size="14" />
          </slot>
        </span>
        <nav class="lw-crumbs" :aria-label="title">
          <template v-if="!isMobile">
            <template v-for="(c, i) in crumbs" :key="i">
              <span class="lw-crumb">{{ c }}</span>
              <n-icon :component="ChevronForwardOutline" :size="12" class="lw-crumb-sep" />
            </template>
          </template>
          <span class="lw-title">{{ title }}</span>
          <span v-if="sub && !isMobile" class="lw-sub">{{ sub }}</span>
        </nav>
        <span class="lw-grow" />
        <!-- The header's middle (the fullscreen editor's mode segment). -->
        <template v-if="$slots.center">
          <slot name="center" />
          <span class="lw-grow" />
        </template>
        <slot name="aside" />
        <button
          type="button"
          class="lw-close"
          :title="closeLabel || t('common.action.close')"
          :aria-label="closeLabel || t('common.action.close')"
          :data-testid="closeTestid || `${testid}-close`"
          @click="close"
        >
          <n-icon :component="closeIcon || CloseOutline" :size="closeIcon ? 15 : 18" />
        </button>
        <Kbd v-if="!isMobile">{{ CAPS.escape }}</Kbd>
      </header>

      <div class="lw-body">
        <nav v-if="!noNav" class="lw-nav" :aria-label="title">
          <slot name="nav-top" />
          <template v-for="(item, idx) in navItems" :key="item.key || `h-${idx}`">
            <div v-if="item.heading" class="lw-nav-h">{{ item.heading }}</div>
            <n-tooltip v-else-if="item.stub" :delay="300" placement="right">
              <template #trigger>
                <span
                  class="lw-tab stub"
                  aria-disabled="true"
                  :data-testid="item.testid || `${tabTestid}-${item.key}`"
                >
                  <n-icon v-if="item.icon" :component="item.icon" :size="16" class="lw-tab-ic" />
                  <span class="lw-tab-l">{{ item.label }}</span>
                  <span class="lw-soon">{{ t('common.stub.badge') }}</span>
                </span>
              </template>
              {{ t('common.stub.soon', { n: item.stub }) }}
            </n-tooltip>
            <button
              v-else
              type="button"
              class="lw-tab"
              :class="{ active: !item.out && section === item.key, out: item.out }"
              :aria-current="!item.out && section === item.key ? 'page' : undefined"
              :data-testid="item.testid || `${tabTestid}-${item.key}`"
              :title="digitOf(item) && !isMobile ? cap(digitOf(item)) : undefined"
              @click="pick(item)"
            >
              <n-icon v-if="item.icon" :component="item.icon" :size="16" class="lw-tab-ic" />
              <span class="lw-tab-l">{{ item.label }}</span>
              <span
                v-if="item.count != null && item.count !== ''"
                class="lw-tab-n"
                :class="item.tone"
                >{{ item.count }}</span
              >
              <n-icon v-if="item.out" :component="OpenOutline" :size="13" class="lw-tab-out" />
            </button>
          </template>
          <template v-if="!isMobile">
            <span class="lw-grow" />
            <div class="lw-nav-foot">
              <slot name="nav-foot">
                <div v-if="sections.length > 1" class="lw-foot-row">
                  <span class="lw-foot-keys">
                    <Kbd>{{ cap(1) }}</Kbd
                    ><span>–</span><Kbd>{{ cap(sections.length) }}</Kbd>
                  </span>
                  <span>{{ footHint || t('common.window.sections') }}</span>
                </div>
              </slot>
            </div>
          </template>
        </nav>

        <section class="lw-pane">
          <slot />
        </section>
      </div>
      <!-- A footer across the whole window (the fullscreen editor's «Сохранено ·
           Esc свернуть · Готово ⌘↵»). -->
      <footer v-if="$slots.foot" class="lw-foot">
        <slot name="foot" />
      </footer>
    </div>
  </n-modal>
</template>

<style scoped>
/* 1080×760, radius 18, the pop level, the drag shadow — as the task modal. */
.lw-card {
  display: flex;
  flex-direction: column;
  width: min(1080px, 96vw);
  height: min(760px, 92vh);
  border-radius: var(--t-r-col);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
}
.lw-card.lw-wide {
  width: min(1240px, 96vw);
  height: min(788px, 92vh);
}
.lw-card.lw-mobile {
  width: 100vw;
  height: 100dvh;
  max-height: none;
  border-radius: 0;
}
.lw-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  box-sizing: border-box;
  padding: 0 16px 0 20px;
  min-width: 0;
}
.lw-mobile .lw-head {
  padding: 0 10px 0 14px;
}
.lw-badge {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  background: var(--t-accent-grad);
}
.lw-badge.toned {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--lw-tone) 82%, #000),
    var(--lw-tone) 50%,
    color-mix(in srgb, var(--lw-tone) 80%, #fff)
  );
}
.lw-badge.own {
  background: none;
}
.lw-sub {
  flex: none;
  font-size: 13px;
  color: var(--t-text2);
}
.lw-crumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
}
.lw-crumb {
  flex: none;
  font-size: 13.5px;
  color: var(--t-text3);
  white-space: nowrap;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.lw-crumb-sep {
  flex: none;
  color: var(--t-text3);
}
.lw-title {
  font-size: 15px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.lw-grow {
  flex: 1;
}
.lw-close {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.lw-close:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.lw-close:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
.lw-body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: 28px;
  padding: 0 12px 12px;
}
.lw-nonav .lw-body {
  grid-template-columns: minmax(0, 1fr);
}
.lw-mobile .lw-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0;
}
/* The sunken navigation column — --t-sub, radius 14. */
.lw-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px;
  border-radius: 14px;
  background: var(--t-sub);
  min-height: 0;
  overflow-y: auto;
  scrollbar-width: none;
}
.lw-mobile .lw-nav {
  flex: none;
  flex-direction: row;
  overflow-x: auto;
  overflow-y: hidden;
  border-radius: 0;
  padding: 6px 10px;
}
.lw-tab {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  padding: 0 10px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
  flex: none;
}
.lw-tab:hover {
  background: var(--t-hover);
}
.lw-tab:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.lw-tab.active {
  background: var(--t-active);
  font-weight: 650;
}
.lw-tab.stub {
  cursor: default;
  color: var(--t-text3);
}
.lw-tab.stub:hover {
  background: transparent;
}
.lw-tab-ic {
  color: var(--t-text2);
  flex: none;
}
.lw-tab.stub .lw-tab-ic {
  color: var(--t-text3);
}
.lw-tab.active .lw-tab-ic {
  color: var(--t-primary);
}
/* The active icon is the flat accent: a gradient stroke (objectBoundingBox)
   loses every straight segment of an outline icon — the list's bars, the
   reader's lines (reference-web-accent-gradient, gotcha 3). */
.lw-tab-l {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.lw-tab-n {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
/* A count that asks for attention (errors, conflicts): a red disc. */
.lw-tab-n.danger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  box-sizing: border-box;
  border-radius: 9px;
  background: var(--t-error-grad, #e0533d);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
}
.lw-tab-out {
  color: var(--t-text3);
}
.lw-soon {
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 11px;
}
.lw-nav-h {
  margin: 14px 10px 6px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.lw-nav-foot {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px 8px;
  font-size: 11.5px;
  color: var(--t-text3);
}
.lw-nav-foot :deep(.lw-foot-row) {
  display: flex;
  align-items: center;
  gap: 6px;
}
.lw-foot-keys {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.lw-foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  box-sizing: border-box;
  padding: 0 16px 0 20px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.lw-mobile .lw-foot {
  height: auto;
  min-height: 52px;
  padding: 6px 14px calc(6px + var(--safe-bottom, 0px));
}
.lw-pane {
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.lw-mobile .lw-pane {
  flex: 1;
  padding: 0 14px 14px;
}
</style>
