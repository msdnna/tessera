<script setup>
// The «Начало работы» checklist (#3000, mockup TourChecklist): the first card of
// the right column on «Мои задачи». Rows are RESULTS, not tour steps — «Создать
// задачу» ticks itself even when the task was made without the tour (#3102),
// and such a row says so. A row starts its chapter from the step that counts
// it; «Главы по выбору» start their chapter.
//
// Three looks: in progress (expanded), folded to one line, and all done (green,
// with the chapters as icons). Folding is this browser's (localStorage); hiding
// for good is the account's (an ack, also switchable in Settings → «Подсказки»).
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import {
  CheckmarkCircleOutline,
  CheckmarkOutline,
  ChevronDownOutline,
  ChevronUpOutline,
  EllipsisHorizontalOutline,
  EyeOffOutline,
  LockClosedOutline,
  RefreshOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import { useOnboardingStore } from '@/stores/onboarding'
import { useTourStore } from '@/stores/tour'
import { useTourLauncher } from '@/composables/useTourLauncher'
import { ITEMS, entryOfItem } from '@/data/getStarted'
import { tourIcon } from '@/utils/tourIcons'

const MIN_KEY = 'tessera_checklist_min'

const { t } = useI18n()
const ob = useOnboardingStore()
const tour = useTourStore()
const { launch } = useTourLauncher()

// Folded: the user's choice on this browser; until they make one, folded once
// «Основы» are behind them.
const stored = ref(localStorage.getItem(MIN_KEY))
const min = computed(() => (stored.value === null ? ob.basics.complete : stored.value === '1'))
function setMin(v) {
  stored.value = v ? '1' : '0'
  try {
    localStorage.setItem(MIN_KEY, stored.value)
  } catch {
    /* private mode — folds for this page only */
  }
}

const basicItems = computed(() => {
  let curSeen = false
  return (ITEMS.basics || []).map((id) => {
    const done = ob.isDone(id)
    const cur = !done && !curSeen
    if (cur) curSeen = true
    return { id, done, cur, self: done && ob.isSelf(id) }
  })
})
const chapters = computed(() => ob.optional)

// The header's «Основы · 4 из 6» / «Главы · 3 из 7».
const head = computed(() =>
  ob.basics.complete
    ? t('tour.checklist.chaptersOf', { n: ob.chaptersDone, of: ob.chaptersOpen })
    : t('tour.checklist.basicsOf', { n: ob.basics.done, of: ob.basics.total }),
)
const segs = computed(() =>
  ob.basics.complete
    ? { on: ob.chaptersDone, of: ob.chaptersOpen }
    : { on: ob.basics.done, of: ob.basics.total },
)
const pct = computed(() => (segs.value.of ? Math.round((segs.value.on / segs.value.of) * 100) : 0))

function startItem(id) {
  const at = entryOfItem(id)
  if (at) launch(at.chapter, { fromId: at.step })
}
// «Продолжить»: where the user stopped («Продолжить позже» kept the step), the
// first result of «Основы» still open, then the first open chapter.
function resume() {
  const left = tour.persisted()
  if (left && left.chapter !== 'mobile') return launch(left.chapter, { fromId: left.step })
  if (!ob.basics.complete && ob.nextBasic) return startItem(ob.nextBasic)
  const ch = chapters.value.find((c) => !c.complete && !c.locked)
  if (ch) launch(ch.id)
}
function startChapter(ch) {
  if (!ch.locked) launch(ch.id)
}

const menuOpen = ref(false)
const menuEl = ref(null)
const menuItems = computed(() => [
  {
    key: 'fold',
    label: t(min.value ? 'tour.checklist.expand' : 'tour.checklist.collapse'),
    icon: min.value ? ChevronDownOutline : ChevronUpOutline,
  },
  { key: 'restart', label: t('tour.checklist.restart'), icon: RefreshOutline },
  { key: 'hide', label: t('tour.checklist.hide'), icon: EyeOffOutline },
])
function onMenu(key) {
  menuOpen.value = false
  if (key === 'fold') setMin(!min.value)
  else if (key === 'restart') ob.restart()
  else if (key === 'hide') ob.hide()
}

// «Пройти главу заново» on the finished card: which one.
const againOpen = ref(false)
const againEl = ref(null)
const againItems = computed(() => [
  { key: 'basics', label: t('tour.chapters.basics.title'), icon: SparklesOutline },
  ...chapters.value.map((c) => ({
    key: c.id,
    label: t(`tour.chapters.${c.id}.title`),
    icon: tourIcon(c.icon),
    disabled: c.locked,
    disabledTitle: t('tour.checklist.locked'),
  })),
])
function onAgain(key) {
  againOpen.value = false
  launch(key)
}
</script>

<template>
  <section
    class="tc"
    data-testid="tour-checklist"
    :data-state="ob.allDone ? 'done' : min ? 'min' : 'run'"
  >
    <!-- All done -->
    <div v-if="ob.allDone" class="tc-card done">
      <div class="tc-dhead">
        <span class="tc-ok"><n-icon :component="CheckmarkOutline" :size="16" /></span>
        <span class="tc-dtitle">{{ t('tour.checklist.doneTitle') }}</span>
        <button
          ref="menuEl"
          type="button"
          class="tc-ib"
          :aria-label="t('tour.checklist.menu')"
          data-testid="tour-checklist-menu"
          @click="menuOpen = !menuOpen"
        >
          <n-icon :component="EllipsisHorizontalOutline" :size="15" />
        </button>
      </div>
      <p class="tc-dtext">
        {{
          t('tour.checklist.doneBody', { n: ob.chaptersDone, of: chapters.length }, ob.chaptersDone)
        }}
        <template v-if="chapters.some((c) => c.locked)">
          {{ t('tour.checklist.doneLocked') }}</template
        >
      </p>
      <div class="tc-icons">
        <span class="tc-ic" :title="t('tour.chapters.basics.title')">
          <n-icon :component="SparklesOutline" :size="14" />
        </span>
        <span
          v-for="c in chapters"
          :key="c.id"
          class="tc-ic"
          :class="{ lock: c.locked }"
          :title="t(`tour.chapters.${c.id}.title`)"
        >
          <n-icon :component="tourIcon(c.icon)" :size="14" />
        </span>
      </div>
      <div class="tc-dfoot">
        <button type="button" class="tc-ctl" data-testid="tour-checklist-hide" @click="ob.hide()">
          {{ t('tour.checklist.hideCard') }}
        </button>
        <span class="tc-fill" />
        <button
          ref="againEl"
          type="button"
          class="tc-link accent-grad-text"
          data-testid="tour-checklist-again"
          @click="againOpen = !againOpen"
        >
          {{ t('tour.checklist.again') }}
        </button>
      </div>
    </div>

    <!-- Folded to one line -->
    <div v-else-if="min" class="tc-card mini">
      <span class="tc-tile"><n-icon :component="SparklesOutline" :size="13" /></span>
      <span class="tc-mcol">
        <span class="tc-mtitle">{{ t('tour.checklist.title') }}</span>
        <span class="tc-mrow">
          <span class="tc-bar"><span class="tc-bar-on" :style="{ width: pct + '%' }" /></span>
          <span class="tc-mof">{{ t('tour.checklist.of', { n: segs.on, of: segs.of }) }}</span>
        </span>
      </span>
      <button type="button" class="tc-ctl" data-testid="tour-checklist-continue" @click="resume">
        {{ t('tour.checklist.continue') }}
      </button>
      <button
        type="button"
        class="tc-ib"
        :aria-label="t('tour.checklist.expand')"
        data-testid="tour-checklist-fold"
        @click="setMin(false)"
      >
        <n-icon :component="ChevronDownOutline" :size="14" />
      </button>
    </div>

    <!-- In progress -->
    <div v-else class="tc-card run">
      <div class="tc-head">
        <span class="tc-tile"><n-icon :component="SparklesOutline" :size="13" /></span>
        <span class="tc-title">{{ t('tour.checklist.title') }}</span>
        <span class="tc-meta">{{ head }}</span>
        <button
          type="button"
          class="tc-ib"
          :aria-label="t('tour.checklist.collapse')"
          data-testid="tour-checklist-fold"
          @click="setMin(true)"
        >
          <n-icon :component="ChevronUpOutline" :size="14" />
        </button>
        <button
          ref="menuEl"
          type="button"
          class="tc-ib"
          :aria-label="t('tour.checklist.menu')"
          data-testid="tour-checklist-menu"
          @click="menuOpen = !menuOpen"
        >
          <n-icon :component="EllipsisHorizontalOutline" :size="15" />
        </button>
      </div>
      <div class="tc-segs" aria-hidden="true">
        <span v-for="i in segs.of" :key="i" class="tc-seg" :class="{ on: i <= segs.on }" />
      </div>

      <div v-if="!ob.basics.complete" class="tc-rows">
        <template v-for="r in basicItems" :key="r.id">
          <button
            v-if="r.cur"
            type="button"
            class="tc-row cur"
            data-testid="tour-checklist-item"
            :data-item="r.id"
            @click="startItem(r.id)"
          >
            <span class="tc-ring"><span class="tc-ring-dot" /></span>
            <span class="tc-name">{{ t(`tour.items.${r.id}`) }}</span>
            <span class="tc-go">{{ t('tour.checklist.continue') }}</span>
          </button>
          <button
            v-else
            type="button"
            class="tc-row"
            :class="{ done: r.done }"
            data-testid="tour-checklist-item"
            :data-item="r.id"
            :data-done="r.done ? (r.self ? 'self' : 'tour') : null"
            @click="startItem(r.id)"
          >
            <n-icon v-if="r.done" :component="CheckmarkCircleOutline" :size="17" class="tc-check" />
            <span v-else class="tc-empty" />
            <span class="tc-name">{{ t(`tour.items.${r.id}`) }}</span>
            <span v-if="r.self" class="tc-note">{{ t('tour.checklist.self') }}</span>
            <span v-else-if="!r.done" class="tc-hint">{{ t('tour.checklist.fromHere') }}</span>
          </button>
        </template>
      </div>
      <div v-else class="tc-passed">
        <n-icon :component="CheckmarkCircleOutline" :size="17" class="tc-check" />
        {{ t('tour.checklist.basicsDone') }}
      </div>

      <div class="tc-sec">{{ t('tour.checklist.optional') }}</div>
      <div class="tc-chaps">
        <button
          v-for="c in chapters"
          :key="c.id"
          type="button"
          class="tc-chap"
          :class="{ lock: c.locked }"
          :aria-disabled="c.locked ? 'true' : null"
          data-testid="tour-checklist-chapter"
          :data-chapter="c.id"
          @click="startChapter(c)"
        >
          <span class="tc-ctile"><n-icon :component="tourIcon(c.icon)" :size="13" /></span>
          <span class="tc-name">{{ t(`tour.chapters.${c.id}.title`) }}</span>
          <span v-if="c.locked" class="tc-lock">
            <n-icon :component="LockClosedOutline" :size="12" />
            {{ t('tour.checklist.locked') }}
          </span>
          <span v-else-if="c.complete" class="tc-prog ok">{{ t('tour.checklist.passed') }}</span>
          <span v-else-if="c.done" class="tc-prog ok">
            {{ t('tour.checklist.of', { n: c.done, of: c.total }) }}
          </span>
          <span v-else class="tc-prog">{{
            t('tour.checklist.of', { n: c.done, of: c.total })
          }}</span>
        </button>
      </div>
    </div>

    <ActionMenu
      v-model:show="menuOpen"
      :anchor="() => menuEl"
      placement="bottom-end"
      :width="220"
      :items="menuItems"
      testid="tour-checklist-actions"
      @select="onMenu"
    />
    <ActionMenu
      v-model:show="againOpen"
      :anchor="() => againEl"
      placement="bottom-end"
      :width="240"
      :items="againItems"
      testid="tour-checklist-chapters"
      @select="onAgain"
    />
  </section>
</template>

<style scoped>
.tc {
  display: block;
}
.tc-card {
  border-radius: var(--t-r-pop, 16px);
  color: var(--t-text1);
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 5%, var(--t-col)),
    color-mix(in srgb, var(--t-primary) 13%, var(--t-col))
  );
}
.tc-card.run {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 12px 10px;
}
.tc-head {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 2px 0 4px;
}
.tc-tile {
  flex: 0 0 24px;
  width: 24px;
  height: 24px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-accent-grad);
  color: #fff;
}
.tc-title {
  flex: 1;
  min-width: 0;
  font-size: 13.5px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tc-meta {
  font-size: 11.5px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.tc-ib {
  flex: none;
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.tc-ib:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tc-segs {
  display: flex;
  gap: 3px;
  padding: 0 4px;
}
.tc-seg {
  flex: 1;
  height: 3px;
  border-radius: 2px;
  background: color-mix(in srgb, var(--t-text3) 28%, transparent);
}
.tc-seg.on {
  background: var(--t-accent-grad);
}
.tc-rows,
.tc-chaps {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.tc-rows {
  margin-top: 2px;
}
.tc-row,
.tc-chap {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  border: none;
  background: transparent;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.tc-row {
  height: 30px;
  padding: 0 6px;
  border-radius: 8px;
  font-size: 13px;
  color: var(--t-text2);
}
.tc-row.done {
  color: var(--t-text3);
}
.tc-row:not(.cur):hover {
  background: var(--t-hover);
}
.tc-row.cur {
  height: 38px;
  padding: 0 4px 0 6px;
  border-radius: 10px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
  font-weight: 600;
  color: var(--t-text1);
}
.tc-name {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tc-check {
  flex: none;
  color: #18a058;
}
.tc-empty,
.tc-ring {
  flex: 0 0 17px;
  width: 17px;
  height: 17px;
  box-sizing: border-box;
  border-radius: 50%;
  border: 2px solid var(--t-pill);
}
.tc-ring {
  border-color: var(--t-primary);
  display: flex;
  align-items: center;
  justify-content: center;
}
.tc-ring-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--t-accent-grad);
}
.tc-note,
.tc-hint {
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.tc-hint {
  opacity: 0;
}
.tc-row:hover .tc-hint {
  opacity: 1;
}
.tc-go {
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border-radius: 8px;
  background: var(--t-accent-grad);
  color: #fff;
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
}
.tc-passed {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 30px;
  padding: 0 6px;
  font-size: 13px;
  color: var(--t-text2);
}
.tc-sec {
  height: 24px;
  display: flex;
  align-items: center;
  padding: 6px 6px 0;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.tc-chap {
  height: 34px;
  padding: 0 6px;
  border-radius: 9px;
  font-size: 13px;
  color: var(--t-text1);
}
.tc-chap:hover {
  background: var(--t-hover);
}
.tc-chap.lock {
  cursor: default;
  color: var(--t-text3);
}
.tc-chap.lock:hover {
  background: transparent;
}
.tc-ctile {
  flex: 0 0 24px;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--t-pill);
  color: var(--t-text2);
}
.tc-prog {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.tc-prog.ok {
  height: 20px;
  display: flex;
  align-items: center;
  padding: 0 7px;
  border-radius: 999px;
  font-size: 11.5px;
  font-weight: 600;
  color: #18a058;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 12%, var(--t-col)),
    color-mix(in srgb, #18a058 22%, var(--t-col))
  );
}
.tc-lock {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}

/* Folded */
.tc-card.mini {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 52px;
  box-sizing: border-box;
  padding: 0 8px 0 12px;
}
.tc-mcol {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.tc-mtitle {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13.5px;
  font-weight: 650;
}
.tc-mrow {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tc-mof {
  white-space: nowrap;
  font-size: 12px;
  font-weight: 500;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.tc-bar {
  flex: 0 1 120px;
  min-width: 40px;
  height: 3px;
  border-radius: 2px;
  background: var(--t-pill);
  display: flex;
  overflow: hidden;
}
.tc-bar-on {
  background: var(--t-accent-grad);
  border-radius: 2px;
}
.tc-ctl {
  flex: none;
  height: 30px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border: none;
  border-radius: 8px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
}
.tc-ctl:hover {
  background: color-mix(in srgb, var(--t-text1) 12%, var(--t-ctl));
}

/* All done */
.tc-card.done {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 12px 10px 14px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 6%, var(--t-col)),
    color-mix(in srgb, #18a058 14%, var(--t-col))
  );
}
.tc-dhead {
  display: flex;
  align-items: center;
  gap: 10px;
}
.tc-ok {
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 86%, #000),
    #18a058 50%,
    color-mix(in srgb, #18a058 86%, #fff)
  );
}
.tc-dtitle {
  flex: 1;
  font-size: 14px;
  font-weight: 650;
}
.tc-dtext {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--t-text2);
  text-wrap: pretty;
}
.tc-icons {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.tc-ic {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #18a058;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, #18a058 12%, var(--t-col)),
    color-mix(in srgb, #18a058 22%, var(--t-col))
  );
}
.tc-ic.lock {
  opacity: 0.35;
}
.tc-dfoot {
  display: flex;
  align-items: center;
  gap: 6px;
  padding-top: 2px;
}
.tc-dfoot .tc-ctl {
  margin-left: -4px;
  font-weight: 400;
}
.tc-fill {
  flex: 1;
}
.tc-link {
  border: none;
  padding: 0;
  font: inherit;
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
.tc button:focus {
  outline: none;
}
.tc button:focus-visible {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--t-primary) 75%, transparent);
}
</style>
