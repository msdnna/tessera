<script setup>
import { NDropdown, NPopconfirm } from 'naive-ui'
import { PRIORITY_COLORS, PRIORITY_LABELS } from '@/styles/tokens'
import { hueGrad } from '@/utils/gradient'
import { useTaskMenu } from '@/composables/useTaskMenu'

const props = defineProps({
  // Grouped, filtered, sorted columns from KanbanBoard: [{ key, name, color }]
  columns: { type: Array, default: () => [] },
  // Real board status columns [{ id, name }] for the "move to column" menu.
  statusColumns: { type: Array, default: () => [] },
  // Map column/group key -> task[] (already filtered & sorted).
  lists: { type: Object, default: () => ({}) },
  tagsMap: { type: Object, default: () => ({}) },
  membersMap: { type: Object, default: () => ({}) },
})
const emit = defineEmits(['open', 'changed'])

const menu = useTaskMenu({
  onOpen: (id) => emit('open', id),
  onChanged: () => emit('changed'),
  columns: () => props.statusColumns,
})

function initials(name) {
  if (!name) return '?'
  return name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase())
    .join('')
}
function fmtDue(d) {
  return new Date(d).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' })
}
function isOverdue(t) {
  return t.due_date && !t.completed_at && new Date(t.due_date) < new Date(new Date().toDateString())
}
</script>

<template>
  <div class="list-view">
    <section v-for="col in columns" :key="col.key" class="lv-group">
      <header class="lv-ghead">
        <span class="lv-dot" :style="{ background: col.color ? hueGrad(col.color) : 'var(--t-border)' }" />
        <span class="lv-gname">{{ col.name }}</span>
        <span class="lv-gcount">{{ (lists[col.key] || []).length }}</span>
      </header>

      <div
        v-for="t in lists[col.key] || []"
        :key="t.id"
        class="lv-row"
        :class="{ done: t.completed_at }"
        @click="$emit('open', t.id)"
        @contextmenu.prevent.stop="menu.open($event, t)"
      >
        <span
          class="lv-pr"
          :style="{ background: hueGrad(PRIORITY_COLORS[t.priority || 0]) }"
          :title="PRIORITY_LABELS[t.priority || 0]"
        />
        <span v-if="t.number" class="lv-num">#{{ t.number }}</span>
        <span class="lv-title">{{ t.title }}</span>

        <span class="lv-tags">
          <span
            v-for="tid in t.tag_ids || []"
            :key="tid"
            class="lv-tag"
            :style="{
              borderColor: tagsMap[tid]?.color || 'var(--t-border)',
              color: tagsMap[tid]?.color || 'var(--t-text2)',
            }"
          >
            {{ tagsMap[tid]?.name }}
          </span>
        </span>

        <span v-if="t.due_date" class="lv-due" :class="{ overdue: isOverdue(t) }">
          {{ fmtDue(t.due_date) }}
        </span>

        <span class="lv-ava-row">
          <span
            v-for="uid in t.assignee_ids || []"
            :key="uid"
            class="lv-ava"
            :title="membersMap[uid]?.name"
          >
            {{ initials(membersMap[uid]?.name) }}
          </span>
        </span>
      </div>

      <div v-if="!(lists[col.key] || []).length" class="lv-empty">— пусто —</div>
    </section>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="menu.show.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :options="menu.options.value"
      @select="menu.select"
      @clickoutside="menu.show.value = false"
    />
    <n-popconfirm
      v-model:show="menu.deleteConfirmShow.value"
      :x="menu.x.value"
      :y="menu.y.value"
      :positive-button-props="{ type: 'error' }"
      positive-text="Удалить"
      @positive-click="menu.confirmDelete()"
      @clickoutside="menu.deleteConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      Удалить безвозвратно? Это действие необратимо.
    </n-popconfirm>
    <n-popconfirm
      v-model:show="menu.archiveConfirmShow.value"
      :x="menu.x.value"
      :y="menu.y.value"
      positive-text="В архив"
      @positive-click="menu.confirmArchive()"
      @clickoutside="menu.archiveConfirmShow.value = false"
    >
      <template #trigger><span /></template>
      Перенести задачу в архив?
    </n-popconfirm>
  </div>
</template>

<style scoped>
.list-view {
  padding: 4px 2px 40px;
  width: 100%;
}
.lv-group {
  margin-bottom: 18px;
}
.lv-ghead {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  margin-bottom: 4px;
  border-bottom: 1px solid var(--t-border);
}
.lv-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}
.lv-gname {
  font-weight: 600;
  color: var(--t-text1);
}
.lv-gcount {
  font-size: 12px;
  color: var(--t-text3);
}
.lv-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 8px;
  border-radius: 7px;
  cursor: pointer;
}
.lv-row:hover {
  background: var(--t-hover);
}
.lv-row.done .lv-title {
  text-decoration: line-through;
  color: var(--t-text3);
}
.lv-pr {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.lv-num {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
.lv-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--t-text1);
}
.lv-tags {
  display: flex;
  gap: 4px;
  flex: none;
}
.lv-tag {
  font-size: 11px;
  padding: 0 6px;
  border: 1px solid;
  border-radius: 10px;
  line-height: 17px;
}
.lv-due {
  flex: none;
  font-size: 12px;
  color: var(--t-text2);
}
.lv-due.overdue {
  font-weight: 600;
  background-image: linear-gradient(
    to top right,
    color-mix(in srgb, #e0533d 86%, #000),
    #e0533d 50%,
    color-mix(in srgb, #e0533d 86%, #fff)
  );
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}
.lv-ava-row {
  display: flex;
  flex: none;
  gap: -4px;
}
.lv-ava {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: -4px;
  border: 2px solid var(--t-surface);
}
.lv-empty {
  padding: 6px 8px;
  font-size: 12px;
  color: var(--t-text3);
}
</style>
