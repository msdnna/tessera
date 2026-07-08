<script setup>
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
  // Per-field pill visibility (mutated via set-field so props stay read-only here).
  fieldVis: { type: Object, default: () => ({}) },
  // Reused composer facets (group/sort/filter) — rendered as summary chips + the
  // same add menu, so this panel doesn't reimplement the composer.
  facetChips: { type: Array, default: () => [] },
  addOptions: { type: Array, default: () => [] },
  currentViewName: { type: String, default: '' },
})
const emit = defineEmits(['set-field', 'add-facet', 'remove-chip', 'chip-click', 'rename-board'])

const FIELDS = [
  { key: 'priority', label: 'Приоритет' },
  { key: 'due', label: 'Срок' },
  { key: 'assignee', label: 'Исполнитель' },
  { key: 'tags', label: 'Теги' },
  { key: 'estimate', label: 'Оценка' },
  { key: 'milestone', label: 'Этап' },
  { key: 'description', label: 'Описание' },
  { key: 'number', label: 'Номер (#)' },
  { key: 'gitlab', label: 'GitLab' },
]
const fieldOn = (k) => props.fieldVis?.[k] !== false
</script>

<template>
  <n-drawer v-model:show="show" :width="340" placement="right">
    <n-drawer-content title="Настроить вид" closable :native-scrollbar="false">
      <!-- Board name (icon = future/Phase C) -->
      <div class="sec">
        <div class="sec-lbl">Доска</div>
        <n-input
          v-model:value="boardName"
          size="small"
          placeholder="Название доски"
          @blur="emit('rename-board', boardName)"
          @keyup.enter="emit('rename-board', boardName)"
        />
      </div>

      <n-divider />

      <!-- Card size -->
      <div class="sec">
        <div class="sec-lbl">Размер карточек</div>
        <n-radio-group v-model:value="cardSize" size="small">
          <n-radio-button value="compact">Компактно</n-radio-button>
          <n-radio-button value="medium">Средне</n-radio-button>
          <n-radio-button value="large">Крупно</n-radio-button>
        </n-radio-group>
      </div>

      <n-divider />

      <!-- Fields on the card -->
      <div class="sec">
        <div class="sec-lbl">Поля</div>
        <div class="row">
          <span>Стек (в столбик)</span>
          <n-switch v-model:value="stackFields" size="small" />
        </div>
        <div class="row">
          <span>Показывать пустые поля</span>
          <n-switch v-model:value="showEmpty" size="small" />
        </div>
        <div class="flds">
          <label v-for="f in FIELDS" :key="f.key" class="fld">
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
        <div class="sec-lbl">Колонки</div>
        <div class="row">
          <span>Сворачивать пустые колонки</span>
          <n-switch v-model:value="autoCollapseEmpty" size="small" />
        </div>
      </div>

      <n-divider />

      <!-- Group / sort / filter (reuse composer) + subtasks -->
      <div class="sec">
        <div class="sec-lbl">Группировка · сортировка · фильтр</div>
        <div class="chips">
          <span
            v-for="(c, ci) in facetChips"
            :key="ci"
            class="pchip"
            :class="{ group: c.kind === 'group' }"
            @click="emit('chip-click', c)"
          >
            {{ c.label }}
            <button v-if="c.kind !== 'group'" class="pchip-x" @click.stop="emit('remove-chip', c)">
              ×
            </button>
          </span>
          <n-dropdown
            trigger="click"
            placement="bottom-start"
            scrollable
            :options="addOptions"
            @select="(k) => emit('add-facet', k)"
          >
            <button class="pchip-add" title="Добавить группировку / сортировку / фильтр">
              <n-icon :component="AddOutline" :size="14" />
            </button>
          </n-dropdown>
        </div>
        <div class="row">
          <span><n-icon :component="GitBranchOutline" :size="14" /> Раскрыть подзадачи</span>
          <n-switch v-model:value="subtasksExpanded" size="small" />
        </div>
      </div>

      <n-divider />

      <!-- Views -->
      <div class="sec">
        <div class="sec-lbl">Представления</div>
        <div class="row">
          <span>Автосохранение{{ currentViewName ? `: ${currentViewName}` : '' }}</span>
          <n-switch v-model:value="autosaveView" size="small" :disabled="!currentViewName" />
        </div>
        <div class="hint">
          Сохранение и загрузка представлений — кнопки папки и дискеты на панели доски.
        </div>
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
.pchip.group {
  cursor: default;
}
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
