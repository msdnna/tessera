<script setup>
// The start screen's right column (v2, #2960): «Входящие» — today's reminders
// and the latest notes (#2998, mockup HomeB: one block in place of «Напоминания»
// and the notification feed, which stays under the bell) — and the week (my
// tasks due each day — done stacked under what is still open).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  AddOutline,
  ArrowForwardOutline,
  BarChartOutline,
  DocumentTextOutline,
  FileTrayOutline,
} from '@vicons/ionicons5'
import { useFormat } from '@/composables/useFormat'
import { useInboxFormat } from '@/composables/useInboxFormat'
import { useInboxStore } from '@/stores/inbox'
import { firstLine, recordState } from '@/utils/inbox'
import ShortcutKeys from '../ShortcutKeys.vue'

const props = defineProps({
  // Today's open reminders, soonest first.
  reminders: { type: Array, default: () => [] },
  // weekBars(): { days: [{ ms, done, open, today }], done, total }
  week: { type: Object, default: () => ({ days: [], done: 0, total: 0 }) },
})

const { t } = useI18n()
const { formatTime, formatDate } = useFormat()
const { ago, noteWhen } = useInboxFormat()
const inbox = useInboxStore()

const now = computed(() => inbox.now)
const nextId = computed(
  () => props.reminders.find((r) => Date.parse(r.remind_at) > now.value.getTime())?.id,
)
const notes = computed(() =>
  [...inbox.notes].sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at)).slice(0, 3),
)
const state = (r) => recordState(r, now.value)
function remHint(r) {
  const st = state(r)
  if (st === 'snoozed') return t('inbox.home.snoozed')
  if (st === 'fired' || r.id === nextId.value) return ago(r.remind_at, now.value)
  return ''
}
function noteSub(n) {
  const line = firstLine(n.body)
  return n.task ? `#${n.task.number} · ${line || n.task.title}` : line
}
const maxDay = computed(() => Math.max(1, ...props.week.days.map((d) => d.done + d.open)))
const H = 84 // px of a day's column
const barH = (n) => (n ? Math.max(8, Math.round((n / maxDay.value) * H)) : 0)

function openRecord(r) {
  inbox.reveal(r.kind, r.id)
}
</script>

<template>
  <div class="hs">
    <section class="hs-card" data-testid="home-inbox">
      <div class="hs-head">
        <n-icon :component="FileTrayOutline" :size="15" class="hs-ico" />
        <button type="button" class="hs-title hs-link" @click="inbox.open({ focus: true })">
          {{ t('inbox.home.title') }}
        </button>
        <span v-if="inbox.fired" class="hs-badge fired">{{ inbox.fired }}</span>
        <ShortcutKeys id="nav.goInbox" bare />
      </div>
      <div v-if="reminders.length" class="hs-sec">
        {{ t('inbox.home.today') }} <span class="hs-sec-n">{{ reminders.length }}</span>
      </div>
      <div
        v-for="r in reminders"
        :key="r.id"
        class="hs-row"
        :class="[state(r), { next: r.id === nextId }]"
        data-testid="home-inbox-reminder"
        @click="openRecord(r)"
      >
        <span class="hs-time" :class="[state(r), { 'accent-grad-text': r.id === nextId }]">{{
          formatTime(r.remind_at)
        }}</span>
        <span class="hs-text">{{ r.message }}</span>
        <span v-if="remHint(r)" class="hs-when" :class="state(r)">{{ remHint(r) }}</span>
      </div>
      <div v-if="notes.length" class="hs-sec">
        {{ t('inbox.home.notes') }} <span class="hs-sec-n">{{ notes.length }}</span>
      </div>
      <div
        v-for="n in notes"
        :key="n.id"
        class="hs-row hs-note"
        data-testid="home-inbox-note"
        @click="openRecord(n)"
      >
        <span class="hs-note-ic"><n-icon :component="DocumentTextOutline" :size="14" /></span>
        <span class="hs-note-t">
          <span class="hs-text">{{ n.title }}</span>
          <span v-if="noteSub(n)" class="hs-note-sub">{{ noteSub(n) }}</span>
        </span>
        <span class="hs-when">{{ noteWhen(n, now) }}</span>
      </div>
      <div v-if="!reminders.length && !notes.length" class="hs-empty">
        {{ t('inbox.home.empty') }}
      </div>
      <div class="hs-foot">
        <button type="button" class="hs-add" @click="inbox.startCapture()">
          <n-icon :component="AddOutline" :size="14" />
          {{ t('inbox.panel.add') }}
          <ShortcutKeys id="create.note" bare />
        </button>
        <span class="hs-fill" />
        <button
          type="button"
          class="hs-all"
          data-testid="home-inbox-all"
          @click="inbox.open({ focus: true })"
        >
          {{ t('inbox.home.all', { n: inbox.total }) }}
          <n-icon :component="ArrowForwardOutline" :size="13" />
        </button>
      </div>
    </section>

    <section class="hs-card">
      <div class="hs-head">
        <n-icon :component="BarChartOutline" :size="15" class="hs-ico" />
        <span class="hs-title">{{ t('shell.home.side.week') }}</span>
        <span class="hs-meta">{{
          t('shell.home.side.weekDone', { done: week.done, total: week.total })
        }}</span>
      </div>
      <div class="hs-week">
        <div v-for="d in week.days" :key="d.ms" class="hs-day" :class="{ today: d.today }">
          <div class="hs-col" :style="{ height: `${H}px` }">
            <div v-if="d.open" class="hs-bar open" :style="{ height: `${barH(d.open)}px` }" />
            <div v-if="d.done" class="hs-bar done" :style="{ height: `${barH(d.done)}px` }" />
          </div>
          <span class="hs-dow">{{ formatDate(d.ms, { weekday: 'short', timeZone: null }) }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.hs {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.hs-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 12px 10px;
  border-radius: 16px;
  background: var(--t-col);
}
.hs-head {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 24px;
  padding: 0 4px;
  font-size: 13px;
  font-weight: 650;
}
.hs-ico {
  color: var(--t-text2);
}
.hs-title {
  flex: 1;
}
.hs-meta {
  font-size: 12px;
  font-weight: 500;
  color: var(--t-text3);
}
.hs-badge {
  min-width: 20px;
  height: 18px;
  padding: 0 6px;
  box-sizing: border-box;
  border-radius: 9px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  background: var(--t-accent-grad);
}
.hs-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 38px;
  padding: 0 6px;
  border-radius: 9px;
  cursor: pointer;
}
.hs-row:hover {
  background: var(--t-hover);
}
.hs-time {
  flex: 0 0 44px;
  font-size: 13px;
  font-weight: 600;
  color: var(--t-text2);
  font-variant-numeric: tabular-nums;
}
.hs-text {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  line-height: 1.4;
  color: var(--t-text2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hs-row .hs-text {
  color: var(--t-text1);
}
.hs-link {
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.hs-badge.fired {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #f0a020 86%, #000),
    #f0a020 50%,
    color-mix(in srgb, #f0a020 86%, #fff)
  );
}
.hs-sec {
  padding: 8px 6px 2px;
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.hs-sec-n {
  margin-left: 4px;
  letter-spacing: 0;
  font-weight: 500;
}
.hs-time.fired,
.hs-when.fired {
  color: #f0a020;
}
.hs-time.snoozed {
  color: var(--t-text3);
  font-weight: 500;
}
.hs-note-ic {
  flex: none;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  background: var(--t-pill);
  color: var(--t-text2);
}
.hs-note-t {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 0;
}
.hs-note-sub {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--t-text3);
}
.hs-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 2px 0;
}
.hs-fill {
  flex: 1;
}
.hs-add,
.hs-all {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.hs-add:hover,
.hs-all:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.hs-note {
  min-height: 40px;
}

.hs-when {
  flex: none;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.hs-empty {
  padding: 6px;
  font-size: 12.5px;
  color: var(--t-text3);
}
.hs-week {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  padding: 6px 4px 2px;
}
.hs-day {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}
.hs-col {
  width: 22px;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  gap: 2px;
}
.hs-bar {
  border-radius: 5px;
}
.hs-bar.open {
  background: color-mix(in srgb, var(--t-text3) 26%, var(--t-col));
}
.hs-bar.done {
  background: color-mix(in srgb, #18a058 38%, var(--t-col));
}
.hs-day.today .hs-bar.open {
  background: var(--t-accent-grad);
}
.hs-day.today .hs-bar.done {
  background: color-mix(in srgb, #18a058 55%, var(--t-col));
}
.hs-dow {
  font-size: 11.5px;
  color: var(--t-text3);
}
.hs-day.today .hs-dow {
  font-weight: 700;
  color: var(--t-text1);
}
</style>
