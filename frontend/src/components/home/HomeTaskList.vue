<script setup>
// «Мои задачи» on the start screen (v2, #2960): the sections by due date, each
// a tray of 42px rows — done toggle, priority stripe, title, project, status,
// due. The selected row (↑↓) carries the key hint above it.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  ChevronDownOutline,
  EllipseOutline,
  CheckmarkCircleOutline,
  CheckmarkOutline,
  AlarmOutline,
  TimeOutline,
} from '@vicons/ionicons5'
import SwipeRow from '../kit/SwipeRow.vue'
import { useResponsive } from '@/composables/useResponsive'
import { stubTask } from '@/utils/featureStubs'
import { useFormat } from '@/composables/useFormat'
import { PRIORITY_COLORS } from '@/styles/tokens'
import { hueGrad, hueGradVert } from '@/utils/gradient'
import { columnStatusName, isTodoColumn } from '@/utils/columnStatus'
import { taskColumnName } from '@/utils/defaultNames'
import { dueDay } from '@/utils/homeSections'
import TesseraIcon from '../TesseraIcon.vue'
import ShortcutKeys from '../ShortcutKeys.vue'

defineProps({
  // [{ key, tasks }] in display order (empty sections already dropped)
  sections: { type: Array, default: () => [] },
  folded: { type: Object, default: () => new Set() },
  selectedId: { type: String, default: null },
})
const emit = defineEmits(['open', 'complete', 'select', 'toggle'])

const { t } = useI18n()
const { formatDate, formatTime } = useFormat()

// A phone (#2976 — package G «Мои задачи»): each row is a 64px card, a tap
// opens the task, the status circle is a 44px target, and a swipe to the left
// offers «Готово» (and «Отложить», which a task does not have yet — #3108).
const { isMobile } = useResponsive()
const swipeActions = computed(() => [
  { key: 'done', label: t('shell.mobile.swipeDone'), icon: CheckmarkOutline, tone: '#18a058' },
  {
    key: 'snooze',
    label: t('shell.mobile.swipeSnooze'),
    icon: AlarmOutline,
    tone: '#f0a020',
    stub: stubTask('home.snoozeTask'),
  },
])
function onRow(task) {
  if (isMobile.value) emit('open', task)
  else emit('select', task.id)
}

const statusIcon = (task) =>
  columnStatusName({
    isDone: !!task.completed_at,
    first: isTodoColumn(task.column_name_key, task.column_name),
    name: task.column_name,
    nameKey: task.column_name_key,
  })
const initials = (name) => (name || '?').trim().slice(0, 2).toUpperCase()
const isTimed = (iso) => {
  const d = new Date(iso)
  return d.getUTCHours() !== 0 || d.getUTCMinutes() !== 0
}
const todayMs = () => {
  const n = new Date()
  return new Date(n.getFullYear(), n.getMonth(), n.getDate()).getTime()
}
// The due pill: the time for a timed task due today, «сегодня»-style relative
// words are left to the section header — the pill names the day otherwise.
function duePill(task) {
  if (!task.due_date) return null
  const day = dueDay(task.due_date)
  if (day === todayMs() && isTimed(task.due_date)) return { time: formatTime(task.due_date) }
  return {
    text: formatDate(day, { weekday: 'short', day: 'numeric', month: 'short', timeZone: null }),
  }
}
</script>

<template>
  <div class="hl">
    <section v-for="sec in sections" :key="sec.key" class="hl-sec" :data-section="sec.key">
      <button
        type="button"
        class="hl-head"
        :class="{ danger: sec.key === 'overdue' }"
        @click="emit('toggle', sec.key)"
      >
        <n-icon
          :component="ChevronDownOutline"
          :size="13"
          class="hl-chev"
          :class="{ folded: folded.has(sec.key) }"
        />
        <span class="hl-name">{{ t(`shell.home.section.${sec.key}`) }}</span>
        <span class="hl-count">{{ sec.tasks.length }}</span>
      </button>
      <div v-if="!folded.has(sec.key)" class="hl-tray">
        <component
          :is="isMobile ? SwipeRow : 'div'"
          v-for="task in sec.tasks"
          :key="task.id"
          class="hl-cell"
          v-bind="isMobile ? { right: swipeActions } : {}"
          @action="(key) => key === 'done' && emit('complete', task)"
        >
          <div
            class="hl-row"
            :class="{ sel: task.id === selectedId && !isMobile }"
            :data-task-id="task.id"
            @click="onRow(task)"
            @dblclick="emit('open', task)"
          >
            <div v-if="task.id === selectedId" class="hl-hint">
              <ShortcutKeys id="card.open" /> {{ t('shell.home.rowHint.open') }} ·
              <ShortcutKeys id="card.complete" /> {{ t('shell.home.rowHint.complete') }} ·
              <ShortcutKeys id="card.due" /> {{ t('shell.home.rowHint.due') }}
            </div>
            <button
              type="button"
              class="hl-check"
              :title="t('task.status.complete')"
              @click.stop="emit('complete', task)"
            >
              <n-icon class="hl-check-off" :component="EllipseOutline" :size="18" />
              <n-icon class="hl-check-on" :component="CheckmarkCircleOutline" :size="18" />
            </button>
            <span
              class="hl-prio"
              :style="
                task.priority ? { background: hueGradVert(PRIORITY_COLORS[task.priority]) } : {}
              "
            />
            <span class="hl-title" @click.stop="emit('open', task)">{{ task.title }}</span>
            <span class="hl-proj">
              <span
                class="hl-picon"
                :style="{ background: hueGrad(task.project_color || '#7c5cff') }"
                >{{ initials(task.project_name) }}</span
              >
              <span class="hl-pname">{{ task.project_name }}</span>
            </span>
            <span class="hl-status">
              <TesseraIcon
                :name="statusIcon(task)"
                :size="13"
                :style="{ color: task.column_color || 'var(--t-text3)' }"
              />
              <span>{{ taskColumnName(task) }}</span>
            </span>
            <span class="hl-due">
              <span v-if="duePill(task)" class="hl-pill" :class="{ over: sec.key === 'overdue' }">
                <template v-if="duePill(task).time">
                  <n-icon :component="TimeOutline" :size="12" />
                  {{ duePill(task).time }}
                </template>
                <template v-else>{{ duePill(task).text }}</template>
              </span>
            </span>
          </div>
        </component>
      </div>
    </section>
  </div>
</template>

<style scoped>
.hl {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.hl-sec {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.hl-head {
  align-self: flex-start;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 30px;
  padding: 0 6px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-weight: 650;
  cursor: pointer;
}
.hl-head:hover {
  background: var(--t-hover);
}
.hl-head.danger .hl-name {
  color: #e0533d;
}
.hl-chev {
  color: var(--t-text3);
  transition: transform 0.15s ease;
}
.hl-chev.folded {
  transform: rotate(-90deg);
}
.hl-count {
  color: var(--t-text3);
  font-weight: 500;
}
.hl-tray {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px;
  border-radius: 14px;
  background: var(--t-col);
}
.hl-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 42px;
  padding: 0 10px 0 8px;
  border-radius: 10px;
  cursor: default;
}
.hl-row:hover {
  background: var(--t-hover);
}
.hl-row.sel {
  background: var(--t-active);
}
/* the key hint floats above the selected row */
.hl-hint {
  position: absolute;
  right: 10px;
  top: -30px;
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 26px;
  padding: 0 10px 0 6px;
  border-radius: 8px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  font-size: 12px;
  color: var(--t-text2);
  white-space: nowrap;
  pointer-events: none;
}
.hl-check {
  flex: none;
  position: relative;
  display: inline-flex;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  background: none;
  color: var(--t-text3);
  cursor: pointer;
}
.hl-check-on {
  position: absolute;
  inset: 0;
  opacity: 0;
  color: #18a058;
}
.hl-check:hover .hl-check-on {
  opacity: 1;
}
.hl-check:hover .hl-check-off {
  opacity: 0;
}
.hl-prio {
  flex: none;
  width: 3px;
  height: 18px;
  border-radius: 2px;
}
.hl-title {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  color: var(--t-text1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}
.hl-title:hover {
  text-decoration: underline;
  text-decoration-color: var(--t-text3);
}
.hl-proj {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 150px;
  font-size: 12px;
  color: var(--t-text3);
}
.hl-picon {
  flex: none;
  width: 16px;
  height: 16px;
  border-radius: 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 7.5px;
  font-weight: 700;
}
.hl-pname {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hl-status {
  flex: 0 0 118px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  white-space: nowrap;
}
.hl-due {
  flex: 0 0 96px;
  display: flex;
  justify-content: flex-end;
}
.hl-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  padding: 0 8px;
  border-radius: 11px;
  background: var(--t-pill);
  color: var(--t-text2);
  font-size: 12px;
  white-space: nowrap;
}
.hl-pill.over {
  background: color-mix(in srgb, #e0533d 18%, var(--t-col));
  color: #e0533d;
  font-weight: 600;
}
@media (max-width: 760px) {
  .hl-status {
    display: none;
  }
  .hl-hint {
    display: none;
  }
}

/* ── a phone: rows as 64px cards (#2976 — package G «Мои задачи») ── */
@media (max-width: 768px) {
  .hl-tray {
    gap: 8px;
    padding: 0;
    background: transparent;
  }
  .hl-row {
    display: grid;
    grid-template-columns: 44px minmax(0, 1fr) auto;
    grid-template-areas:
      'check title title'
      'check proj due';
    align-items: center;
    column-gap: 6px;
    row-gap: 4px;
    height: auto;
    min-height: 64px;
    padding: 10px 12px 10px 4px;
    box-sizing: border-box;
    border-radius: 14px;
    background: var(--t-card);
  }
  .hl-row:hover {
    background: var(--t-card);
  }
  .hl-check {
    grid-area: check;
    justify-self: center;
    width: 44px;
    height: 44px;
    align-items: center;
    justify-content: center;
  }
  .hl-check-on {
    inset: 13px;
  }
  .hl-prio {
    display: none;
  }
  .hl-title {
    grid-area: title;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    white-space: normal;
    font-size: 15px;
    font-weight: 600;
    line-height: 1.3;
  }
  .hl-proj {
    grid-area: proj;
    display: inline-flex;
    max-width: none;
    min-width: 0;
    font-size: 12.5px;
  }
  .hl-due {
    grid-area: due;
    flex: none;
  }
}
</style>
