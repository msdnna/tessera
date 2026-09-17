<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  NDrawer,
  NDrawerContent,
  NRadioGroup,
  NRadioButton,
  NSwitch,
  NDivider,
  NInput,
  NDropdown,
  NIcon,
} from 'naive-ui'
import { AddOutline, GitBranchOutline } from '@vicons/ionicons5'
import IconColorPicker from './IconColorPicker.vue'
import { useResponsive } from '@/composables/useResponsive'
import { useFacetAddMenu } from '@/composables/useFacetAddMenu'

// Two-way settings (bound to the board view in KanbanBoard). All persist via the
// parent's saved-view + localStorage machinery.
const show = defineModel('show', { type: Boolean, default: false })
const boardName = defineModel('boardName', { type: String, default: '' })
const cardSize = defineModel('cardSize', { type: String, default: 'medium' })
const stackFields = defineModel('stackFields', { type: Boolean, default: false })
const showEmpty = defineModel('showEmpty', { type: Boolean, default: true })
const autoCollapseEmpty = defineModel('autoCollapseEmpty', { type: Boolean, default: false })
const subtasksExpanded = defineModel('subtasksExpanded', { type: Boolean, default: false })
const autosaveView = defineModel('autosaveView', { type: Boolean, default: false })

const props = defineProps({
  // Archive view (?archived=1): look, don't touch. Everything else this panel holds
  // is view state (card size, pills, auto-collapse, facets, autosave) and stays
  // live there — only the board's own branding is a write to shared data.
  readonly: { type: Boolean, default: false },
  // Per-field pill visibility (mutated via set-field so props stay read-only here).
  fieldVis: { type: Object, default: () => ({}) },
  // Reused composer facets (group/sort/filter) — rendered as summary chips + the
  // same add menu, so this panel doesn't reimplement the composer.
  facetChips: { type: Array, default: () => [] },
  addOptions: { type: Array, default: () => [] },
  // Label of the grouping the group chip steps to when clicked (the chip cycles —
  // see useBoardFacets). Shown as its tooltip, the same one the composer carries.
  groupNextLabel: { type: String, default: '' },
  currentViewName: { type: String, default: '' },
  // Board branding (icon/colour/mode), mirrors projects/groups via IconColorPicker.
  boardIcon: { type: String, default: '' },
  boardColor: { type: String, default: '' },
  boardIconMode: { type: String, default: 'badge' },
})
const emit = defineEmits(['set-field', 'add-facet', 'remove-chip', 'chip-click', 'update-board'])

const { t } = useI18n()

const boardInitials = computed(() => (boardName.value || '?').trim().slice(0, 2).toUpperCase())

// Only the field order is fixed here — the labels are looked up per render, or
// switching the language would leave this list on the language of the first one
// (pitfall 1 of the #2799 plan).
const FIELD_KEYS = [
  'priority',
  'due',
  'assignee',
  'tags',
  'estimate',
  'milestone',
  'description',
  'number',
  'gitlab',
]
const fields = computed(() =>
  FIELD_KEYS.map((key) => ({ key, label: t(`board.customize.field.${key}`) })),
)
const fieldOn = (k) => props.fieldVis?.[k] !== false

// The «+» below opens the composer's own two-level facet menu. On a phone naive's
// side submenus open past the edge of the screen, which is why the composer drills
// into one sub-list at a time — and this drawer is the settings surface a phone can
// always reach (the composer's gear slides away once the chips fill the bar, and
// BoardMobileMenu's «Настроить вид» lands here), so it was the wrong one to leave
// fanning out. Own instance, own open/level state: sharing the composer's would pop
// both dropdowns at once.
const { isMobile } = useResponsive()
const {
  show: addShow,
  menuOptions: addMenuOptions,
  renderLabel: renderAddLabel,
  onSelect: onAddSelect,
  onShow: onAddShow,
} = useFacetAddMenu({
  options: computed(() => props.addOptions),
  isMobile,
  onPick: (k) => emit('add-facet', k),
})
</script>

<template>
  <n-drawer v-model:show="show" :width="340" placement="right">
    <n-drawer-content :title="t('board.customize.title')" closable :native-scrollbar="false">
      <!-- Board: name, then icon + colour (like projects/groups). Dropped in the
           archive: the rest of this drawer is this reader's own view state, but the
           name/icon/colour are the LIVE board's — `update-board` PUTs them for the
           whole workspace and re-stamps the sidebar node. The archive is reached by
           a query flag on the very same board, and its gear (plus the phone's menu
           entry) opens this same drawer, so a read-only lens was offering a rename
           and an icon upload. IconColorPicker has no disabled state — and a greyed
           field nobody may use is noise anyway — so the section comes off whole,
           the way ColumnHeader drops its «…» menu. -->
      <template v-if="!readonly">
        <div class="sec">
          <div class="sec-lbl">{{ t('board.customize.board') }}</div>
          <n-input
            v-model:value="boardName"
            size="small"
            :placeholder="t('board.customize.boardName')"
            @blur="emit('update-board', { name: boardName })"
            @keyup.enter="emit('update-board', { name: boardName })"
          />
          <div class="board-icp">
            <IconColorPicker
              :icon="boardIcon"
              :color="boardColor"
              :mode="boardIconMode"
              :initials="boardInitials"
              fallback-kanban
              allow-upload
              @update:icon="(v) => emit('update-board', { icon: v })"
              @update:color="(v) => emit('update-board', { color: v })"
              @update:mode="(v) => emit('update-board', { icon_mode: v })"
            />
          </div>
        </div>

        <n-divider />
      </template>

      <!-- Card size -->
      <div class="sec">
        <div class="sec-lbl">{{ t('board.customize.cardSize') }}</div>
        <n-radio-group v-model:value="cardSize" size="small">
          <n-radio-button value="compact">{{ t('board.customize.sizeCompact') }}</n-radio-button>
          <n-radio-button value="medium">{{ t('board.customize.sizeMedium') }}</n-radio-button>
          <n-radio-button value="large">{{ t('board.customize.sizeLarge') }}</n-radio-button>
        </n-radio-group>
      </div>

      <n-divider />

      <!-- Fields on the card -->
      <div class="sec">
        <div class="sec-lbl">{{ t('board.customize.fields') }}</div>
        <div class="row">
          <span>{{ t('board.customize.stack') }}</span>
          <n-switch v-model:value="stackFields" size="small" />
        </div>
        <div class="row">
          <span>{{ t('board.customize.showEmpty') }}</span>
          <n-switch v-model:value="showEmpty" size="small" />
        </div>
        <div class="flds">
          <label v-for="f in fields" :key="f.key" class="fld">
            <span>{{ f.label }}</span>
            <n-switch
              :value="fieldOn(f.key)"
              size="small"
              @update:value="(v) => emit('set-field', f.key, v)"
            />
          </label>
        </div>
      </div>

      <n-divider />

      <!-- Columns -->
      <div class="sec">
        <div class="sec-lbl">{{ t('board.customize.columns') }}</div>
        <div class="row">
          <span>{{ t('board.customize.autoCollapse') }}</span>
          <n-switch v-model:value="autoCollapseEmpty" size="small" />
        </div>
      </div>

      <n-divider />

      <!-- Group / sort / filter (reuse composer) + subtasks -->
      <div class="sec">
        <div class="sec-lbl">{{ t('board.customize.facets') }}</div>
        <div class="chips">
          <!-- The group chip does the same thing here as in the composer — one click
               steps the grouping on — so it gets the same tooltip. It used to be
               styled `cursor: default`, which read as "summary only" while the click
               silently regrouped the board. -->
          <span
            v-for="(c, ci) in facetChips"
            :key="ci"
            class="pchip"
            :class="{ group: c.kind === 'group' }"
            :title="
              c.kind === 'group' && groupNextLabel
                ? t('board.composer.groupNext', { next: groupNextLabel })
                : undefined
            "
            @click="emit('chip-click', c)"
          >
            {{ c.label }}
            <button v-if="c.kind !== 'group'" class="pchip-x" @click.stop="emit('remove-chip', c)">
              ×
            </button>
          </span>
          <n-dropdown
            :show="addShow"
            trigger="click"
            placement="bottom-start"
            scrollable
            :options="addMenuOptions"
            :render-label="renderAddLabel"
            @update:show="onAddShow"
            @select="onAddSelect"
          >
            <button class="pchip-add" :title="t('board.composer.add')">
              <n-icon :component="AddOutline" :size="14" />
            </button>
          </n-dropdown>
        </div>
        <div class="row">
          <span
            ><n-icon :component="GitBranchOutline" :size="14" />
            {{ t('board.customize.subtasks') }}</span
          >
          <n-switch v-model:value="subtasksExpanded" size="small" />
        </div>
      </div>

      <n-divider />

      <!-- Views -->
      <div class="sec">
        <div class="sec-lbl">{{ t('board.customize.views') }}</div>
        <div class="row">
          <span>{{
            currentViewName
              ? t('board.customize.autosaveNamed', { name: currentViewName })
              : t('board.customize.autosave')
          }}</span>
          <n-switch v-model:value="autosaveView" size="small" :disabled="!currentViewName" />
        </div>
        <div class="hint">{{ t('board.customize.viewsHint') }}</div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.sec {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.sec-lbl {
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
/* Board icon/colour picker: left-align its rows (the shared component centres by
   default, which reads oddly in this left-aligned settings panel). */
.board-icp :deep(.icons),
.board-icp :deep(.swatches),
.board-icp :deep(.mode-toggle) {
  justify-content: flex-start;
  align-self: flex-start;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 26px;
}
.row span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--t-text1);
  font-size: 13px;
}
.flds {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 4px;
}
.fld {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 28px;
  cursor: pointer;
}
.fld span {
  font-size: 13px;
  color: var(--t-text2);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.pchip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 8px;
  background: var(--t-hover);
  color: var(--t-text2);
  font-size: 12px;
  cursor: pointer;
}
/* No «×» on the group chip (grouping is never "off"), but it IS clickable — it
   cycles the grouping exactly as the composer's does, so it keeps the pointer the
   other chips have rather than the `default` that used to deny it. */
.pchip-x {
  border: none;
  background: none;
  color: var(--t-text3);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 0;
}
.pchip-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 1px dashed var(--t-border);
  border-radius: 8px;
  background: none;
  color: var(--t-text2);
  cursor: pointer;
}
.pchip-add:hover {
  color: var(--t-primary);
  border-color: var(--t-primary);
}
.hint {
  font-size: 12px;
  color: var(--t-text3);
  line-height: 1.4;
}
</style>
