<script setup>
// «Представления» — the first section of the board's view panel (v2, #2987,
// mockups CustomPanel / CatalogD): the reader's own views and the board's shared
// ones in one list (1–9 by place), the «изменено» note with «Обновить /
// Сбросить», «Сохранить текущее как…» inline at the bottom.
//
// A row: the glyph of its view kind, the name, the author of a shared one, a
// lock when the reader may not change it, the star (opens by default — for me,
// or for everybody on a shared view), «⋯» with the rest. Keys on a row under
// the keyboard cursor: ↵ apply, F2 rename, ⌫ delete (undo toast), ⌥↑/⌥↓ move,
// «.» the menu, ↑/↓ the next row.
//
// `bv` is the board's useBoardViews(), reactive (refs unwrapped).
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  AlertCircleOutline,
  BookmarksOutline,
  CreateOutline,
  EllipsisHorizontalOutline,
  LockClosedOutline,
  PeopleOutline,
  PersonOutline,
  RefreshOutline,
  StarOutline,
  ArrowUpOutline,
  ArrowDownOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import TesseraIcon from '@/components/TesseraIcon.vue'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import Kbd from '@/components/Kbd.vue'
import ShortcutKeys from '@/components/ShortcutKeys.vue'
import InlineEdit from '@/components/kit/InlineEdit.vue'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import { useResponsive } from '@/composables/useResponsive'
import { LAYOUT_ICONS, viewLayout, viewNameTaken } from '@/utils/boardViews'
import { CAPS } from '@/utils/keyCaps'
import { isMac } from '@/utils/keys'

const props = defineProps({
  bv: { type: Object, required: true },
  // May the reader create / edit the board's shared views (workspace owner/admin)?
  canShare: { type: Boolean, default: false },
  // How many people see the shared list.
  members: { type: Number, default: 0 },
})
const { t } = useI18n()
const { isMobile } = useResponsive()

const list = computed(() => props.bv.views)
const indexOf = (v) => list.value.indexOf(v)
const glyph = (v) => LAYOUT_ICONS[viewLayout(v)] || LAYOUT_ICONS.board
const isLoaded = (v) => props.bv.loadedId === v.id

// ── the «изменено» note ──
const changedText = computed(() =>
  (props.bv.changes || [])
    .map((c) =>
      t(`board.views.area.${c.area}`, {
        value:
          c.area === 'layout'
            ? t(`board.layout.${c.value}`)
            : c.area === 'group'
              ? t(`board.facet.group.${c.value}`)
              : '',
      }),
    )
    .join(', '),
)
const canUpdate = computed(() => !!props.bv.loaded?.editable)

// ── rename ──
const renamingId = ref('')
const renameDraft = ref('')
const renameError = ref('')
const renameBusy = ref(false)
function startRename(v) {
  if (!v.editable) return
  renamingId.value = v.id
  renameDraft.value = v.name
  renameError.value = ''
}
async function commitRename(value, { via }) {
  const v = list.value.find((x) => x.id === renamingId.value)
  if (!v) return (renamingId.value = '')
  const n = (value || '').trim()
  if (!n || n === v.name) {
    renamingId.value = ''
    return
  }
  renameBusy.value = true
  const err = await props.bv.rename(v, n)
  renameBusy.value = false
  if (err) {
    renameError.value = err
    if (via === 'blur') renamingId.value = ''
    return
  }
  renamingId.value = ''
  focusRow(v.id)
}

// ── save as ──
const saving = ref(false)
const saveName = ref('')
const saveScope = ref('user')
const saveError = ref('')
const saveBusy = ref(false)
const saveEditor = ref(null)
function startSave(name = '') {
  saving.value = true
  saveName.value = name
  saveError.value = ''
  saveScope.value = 'user'
  nextTick(() => saveEditor.value?.focus())
}
// An ask from outside (⌘⇧S, the chip, «Сохранить как моё») — also the one that
// opened the panel and mounted this section, hence `immediate`. Consumed here.
watch(
  () => props.bv.saveAsDraft,
  (d) => {
    if (!d) return
    props.bv.takeSaveAs()
    startSave(d.name || '')
  },
  { immediate: true },
)
async function commitSave(value, { via }) {
  const n = (value || '').trim()
  if (!n) {
    if (via !== 'enter') saving.value = false
    return
  }
  if (viewNameTaken(list.value, n, saveScope.value)) {
    saveError.value = t('board.views.nameTaken')
    if (via === 'blur') saving.value = false
    return
  }
  saveBusy.value = true
  const v = await props.bv.saveAs(n, { scope: saveScope.value })
  saveBusy.value = false
  if (v) {
    saving.value = false
    saveName.value = ''
  }
}
function toggleScope() {
  if (props.canShare) saveScope.value = saveScope.value === 'board' ? 'user' : 'board'
}

// ── the row menu ──
const alt = isMac ? '⌥' : 'Alt'
const menuFor = ref(null)
const menuAnchor = ref(null)
function openMenu(v, el) {
  menuFor.value = v
  menuAnchor.value = el
}
const menuItems = computed(() => {
  const v = menuFor.value
  if (!v) return []
  const shared = v.scope === 'board'
  const items = [
    { key: 'rename', label: t('board.views.rename'), icon: CreateOutline, caps: ['F2'] },
    { key: 'overwrite', label: t('board.views.overwrite'), icon: RefreshOutline },
    {
      key: 'default',
      label: shared ? t('board.views.defaultShared') : t('board.views.default'),
      icon: StarOutline,
      toggle: !!v.is_default,
    },
  ]
  if (props.canShare)
    items.push({
      key: 'scope',
      label: shared ? t('board.views.makeMine') : t('board.views.makeShared'),
      icon: shared ? PersonOutline : PeopleOutline,
    })
  items.push(
    { key: 'up', label: t('board.views.moveUp'), icon: ArrowUpOutline, caps: [alt, '↑'] },
    { key: 'down', label: t('board.views.moveDown'), icon: ArrowDownOutline, caps: [alt, '↓'] },
    { gap: true },
    {
      key: 'delete',
      label: t('board.views.delete'),
      icon: TrashOutline,
      danger: true,
      caps: ['⌫'],
    },
  )
  return items
})
function onMenu(key, value) {
  const v = menuFor.value
  if (!v) return
  if (key === 'rename') startRename(v)
  else if (key === 'overwrite') props.bv.overwrite(v)
  else if (key === 'default') props.bv.setDefault(v, value)
  else if (key === 'scope') props.bv.setScope(v, v.scope === 'board' ? 'user' : 'board')
  else if (key === 'up') props.bv.move(v, -1)
  else if (key === 'down') props.bv.move(v, 1)
  else if (key === 'delete') props.bv.remove(v)
}

// ── keys on a row ──
const rowsEl = ref(null)
function focusRow(id) {
  nextTick(() => rowsEl.value?.querySelector(`[data-view-id="${id}"]`)?.focus())
}
function onRowKey(e, v) {
  if (renamingId.value === v.id || e.isComposing) return
  const rows = [...(rowsEl.value?.querySelectorAll('.vs-row[data-view-id]') || [])]
  const i = rows.indexOf(e.currentTarget)
  if (e.key === 'Enter') {
    e.preventDefault()
    props.bv.apply(v)
  } else if (e.key === 'F2') {
    e.preventDefault()
    startRename(v)
  } else if ((e.key === 'Backspace' || e.key === 'Delete') && v.editable) {
    e.preventDefault()
    e.stopPropagation()
    const next = rows[i + 1] || rows[i - 1]
    props.bv.remove(v)
    if (next) nextTick(() => next.focus())
  } else if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown') && v.editable) {
    e.preventDefault()
    props.bv.move(v, e.key === 'ArrowUp' ? -1 : 1).then(() => focusRow(v.id))
  } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    const r = rows[i + (e.key === 'ArrowDown' ? 1 : -1)]
    if (r) {
      e.preventDefault()
      e.stopPropagation()
      r.focus()
    }
  } else if ((e.key === '.' || (e.key === 'F10' && e.shiftKey)) && v.editable) {
    e.preventDefault()
    openMenu(v, e.currentTarget)
  }
}

const failureFor = (v) => (props.bv.failure?.id === v.id ? props.bv.failure : null)
</script>

<template>
  <div class="vs" data-testid="views-section">
    <!-- «изменено»: what moved since the view was loaded, and the two ways out -->
    <div v-if="bv.loaded && bv.changed" class="vs-changed" data-testid="views-changed">
      <div class="vs-changed-text">
        <span class="vs-dot" aria-hidden="true" />
        <span>
          <b>{{ t('board.views.changedTitle', { name: bv.loaded.name }) }}</b>
          <span class="vs-changed-what">{{ changedText }}</span>
        </span>
      </div>
      <div class="vs-changed-acts">
        <button
          type="button"
          class="vs-btn primary"
          data-testid="views-update"
          @click="bv.update()"
        >
          {{ canUpdate ? t('board.views.update') : t('board.views.saveAsMine') }}
          <ShortcutKeys v-if="!isMobile" :id="canUpdate ? 'views.update' : 'views.saveAs'" joined />
        </button>
        <button type="button" class="vs-btn" data-testid="views-reset" @click="bv.reset()">
          {{ t('board.views.reset') }}
          <ShortcutKeys v-if="!isMobile" id="views.reset" joined />
        </button>
      </div>
    </div>

    <div v-if="!list.length" class="vs-empty">
      <n-icon :component="BookmarksOutline" :size="20" />
      <span>{{ t('board.views.empty') }}</span>
      <ShortcutKeys v-if="!isMobile" id="views.saveAs" joined />
    </div>

    <div ref="rowsEl" class="vs-rows">
      <template v-for="group in ['mine', 'shared']" :key="group">
        <template v-if="(group === 'mine' ? bv.mine : bv.shared).length">
          <div class="vs-head">
            <span>{{ t(`board.views.${group}`) }}</span>
            <span class="vs-head-aside">{{
              group === 'mine'
                ? bv.mine.length
                : members
                  ? t('board.views.sharedAside', members, { named: { n: members } })
                  : ''
            }}</span>
          </div>
          <template v-for="v in group === 'mine' ? bv.mine : bv.shared" :key="v.id">
            <div v-if="failureFor(v)" class="vs-row vs-fail" role="alert">
              <n-icon :component="AlertCircleOutline" :size="15" class="vs-red" />
              <span class="vs-fail-text">
                <span class="vs-fail-title">{{ failureFor(v).title }}</span>
                <span class="vs-fail-sub">{{ failureFor(v).sub }}</span>
              </span>
              <button type="button" class="vs-link" @click="startSave(v.name)">
                {{ t('board.views.asNew') }}
              </button>
              <Kbd v-if="!isMobile">{{ CAPS.enter }}</Kbd>
            </div>
            <div
              v-else
              class="vs-row"
              :class="{
                active: isLoaded(v),
                locked: !v.editable,
                busy: bv.busyId === v.id,
              }"
              tabindex="0"
              role="button"
              :data-view-id="v.id"
              :data-testid="`view-row-${indexOf(v) + 1}`"
              :title="!v.editable ? t('board.views.locked') : undefined"
              @click="renamingId !== v.id && bv.apply(v)"
              @dblclick="startRename(v)"
              @keydown="(e) => onRowKey(e, v)"
            >
              <span class="vs-glyph"><TesseraIcon :name="glyph(v)" :size="15" /></span>
              <InlineEdit
                v-if="renamingId === v.id"
                v-model="renameDraft"
                :busy="renameBusy"
                :error="renameError"
                :placeholder="t('board.views.namePlaceholder')"
                @commit="commitRename"
                @cancel="((renamingId = ''), focusRow(v.id))"
              />
              <template v-else>
                <span class="vs-name">{{ v.name }}</span>
                <span v-if="v.scope === 'board' && v.creator_name" class="vs-who">{{
                  v.creator_name
                }}</span>
                <n-icon
                  v-if="!v.editable"
                  :component="LockClosedOutline"
                  :size="13"
                  class="vs-lock"
                />
                <span
                  v-if="isLoaded(v) && bv.changed"
                  class="vs-dot"
                  :title="t('board.views.changed')"
                />
                <TesseraSpinner v-if="bv.busyId === v.id" :size="13" />
                <span class="vs-kbd-hint" aria-hidden="true">
                  <Kbd>{{ CAPS.enter }}</Kbd>
                  <ShortcutKeys v-if="v.editable" id="views.rename" />
                </span>
                <button
                  type="button"
                  class="vs-ic vs-star"
                  :class="{ on: v.is_default }"
                  :disabled="!v.editable"
                  :title="
                    v.is_default
                      ? t('board.views.defaultOn')
                      : v.scope === 'board'
                        ? t('board.views.defaultShared')
                        : t('board.views.default')
                  "
                  :data-testid="`view-star-${indexOf(v) + 1}`"
                  tabindex="-1"
                  @click.stop="v.editable && bv.setDefault(v, !v.is_default)"
                >
                  <n-icon :component="StarOutline" :size="14" />
                </button>
                <button
                  v-if="v.editable"
                  type="button"
                  class="vs-ic vs-more"
                  :title="t('common.action.more')"
                  :data-testid="`view-menu-${indexOf(v) + 1}`"
                  tabindex="-1"
                  @click.stop="(e) => openMenu(v, e.currentTarget)"
                >
                  <n-icon :component="EllipsisHorizontalOutline" :size="15" />
                </button>
                <span v-else class="vs-ic" />
              </template>
            </div>
          </template>
        </template>
      </template>
    </div>

    <!-- «Сохранить текущее как…»: a field in place, not a dialog -->
    <div v-if="saving" class="vs-save editing">
      <n-icon :component="AddOutline" :size="15" class="vs-save-ic" />
      <InlineEdit
        ref="saveEditor"
        v-model="saveName"
        hint="create"
        :busy="saveBusy"
        :error="saveError"
        :placeholder="t('board.views.namePlaceholder')"
        data-testid="views-save-input"
        @commit="commitSave"
        @cancel="saving = false"
      >
        <template v-if="canShare" #trailing>
          <button
            type="button"
            class="vs-scope"
            :class="{ shared: saveScope === 'board' }"
            :title="t('board.views.scopeToggle')"
            data-testid="views-save-scope"
            @mousedown.prevent
            @click="toggleScope"
          >
            <n-icon :component="saveScope === 'board' ? PeopleOutline : PersonOutline" :size="12" />
            {{ saveScope === 'board' ? t('board.views.scopeShared') : t('board.views.scopeMine') }}
          </button>
        </template>
      </InlineEdit>
    </div>
    <button v-else type="button" class="vs-save" data-testid="views-save-as" @click="startSave('')">
      <n-icon :component="AddOutline" :size="15" class="vs-save-ic" />
      <span class="vs-save-lbl">{{ t('board.views.saveAs') }}</span>
      <ShortcutKeys v-if="!isMobile" id="views.saveAs" joined />
    </button>

    <ActionMenu
      :show="!!menuFor"
      :anchor="() => menuAnchor"
      placement="bottom-end"
      :offset="{ x: 0, y: 4 }"
      :width="260"
      :header="menuFor?.name || ''"
      :items="menuItems"
      testid="view-menu"
      @update:show="(v) => !v && (menuFor = null)"
      @select="onMenu"
    />
  </div>
</template>

<style scoped>
.vs {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.vs-changed {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 2px 4px 6px;
  padding: 10px 12px;
  border-radius: 12px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 10%, var(--t-pop)),
    color-mix(in srgb, #f0a020 18%, var(--t-pop))
  );
}
.vs-changed-text {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12.5px;
  line-height: 1.45;
}
.vs-changed-text b {
  font-weight: 600;
}
.vs-changed-what {
  margin-left: 4px;
  color: var(--t-text2);
}
.vs-changed-text .vs-dot {
  margin-top: 5px;
}
.vs-dot {
  flex: 0 0 7px;
  width: 7px;
  height: 7px;
  border-radius: 4px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
}
.vs-changed-acts {
  display: flex;
  gap: 6px;
  padding-left: 15px;
}
.vs-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 28px;
  padding: 0 5px 0 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.vs-btn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.vs-btn.primary {
  background: var(--t-accent-grad);
  color: #fff;
  font-weight: 600;
}
.vs-btn.primary :deep(.kbd) {
  background: rgba(255, 255, 255, 0.22);
  color: #fff;
  box-shadow: none;
}
.vs-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.vs-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 14px 18px 10px;
  text-align: center;
  font-size: 12.5px;
  line-height: 1.45;
  color: var(--t-text3);
}
.vs-rows {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.vs-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.vs-head span:first-child {
  flex: 1;
}
.vs-head-aside {
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}
.vs-row {
  position: relative;
  isolation: isolate;
  display: flex;
  align-items: center;
  gap: 9px;
  height: 36px;
  padding: 0 4px 0 10px;
  border-radius: 9px;
  cursor: pointer;
  outline: 0;
}
.vs-row:hover {
  background: var(--t-hover);
}
.vs-row.active {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 14%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 24%, var(--t-pop))
  );
}
.vs-row:focus-visible {
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.vs-glyph {
  display: flex;
  color: var(--t-text3);
}
.vs-row.active .vs-glyph {
  color: var(--t-primary);
}
.vs-name {
  flex: 1;
  min-width: 0;
  font-size: 13.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.vs-row.active .vs-name {
  font-weight: 600;
}
.vs-who {
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.vs-lock {
  color: var(--t-text3);
}
.vs-ic {
  flex: 0 0 24px;
  width: 24px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.vs-ic:hover:not(:disabled) {
  background: var(--t-hover);
  color: var(--t-text1);
}
.vs-ic:disabled {
  cursor: default;
}
/* The star and «⋯» show on hover, on the active row and when starred; the
   star is drawn in the accent when set (outline only — mockup deviation). */
.vs-star,
.vs-more {
  opacity: 0;
}
.vs-row:hover .vs-star:not(:disabled),
.vs-row:hover .vs-more,
.vs-row.active .vs-star:not(:disabled),
.vs-row.active .vs-more,
.vs-star.on {
  opacity: 1;
}
.vs-star.on {
  color: var(--t-primary);
}
/* Keyboard-only: the ↵ / F2 caps on the row under the keyboard cursor. */
.vs-kbd-hint {
  display: none;
  gap: 3px;
}
.vs-row:focus-visible .vs-kbd-hint {
  display: inline-flex;
}
.vs-row:focus-visible .vs-star,
.vs-row:focus-visible .vs-more {
  display: none;
}
.vs-fail {
  height: auto;
  min-height: 46px;
  padding: 6px 6px 6px 10px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 10%, var(--t-pop)),
    color-mix(in srgb, #e0533d 18%, var(--t-pop))
  );
  cursor: default;
}
.vs-red {
  color: #e0533d;
}
.vs-fail-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.vs-fail-title {
  font-size: 13px;
  font-weight: 600;
}
.vs-fail-sub {
  font-size: 11.5px;
  color: var(--t-text2);
}
.vs-link {
  border: 0;
  background: none;
  color: var(--t-text2);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.vs-save {
  display: flex;
  align-items: center;
  gap: 9px;
  height: 36px;
  margin-top: 6px;
  padding: 0 6px 0 10px;
  border: 0;
  border-radius: 9px;
  background: var(--t-input-bg);
  color: var(--t-placeholder);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: text;
}
.vs-save.editing {
  background: transparent;
  padding-left: 13px;
}
.vs-save:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: -2px;
}
.vs-save-ic {
  color: var(--t-text3);
}
.vs-save-lbl {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.vs-scope {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 20px;
  padding: 0 6px;
  border: 0;
  border-radius: 5px;
  background: var(--t-pill);
  color: var(--t-text2);
  font: inherit;
  font-size: 11.5px;
  cursor: pointer;
}
.vs-scope.shared {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 26%, var(--t-pop))
  );
  color: var(--t-text1);
}
</style>
