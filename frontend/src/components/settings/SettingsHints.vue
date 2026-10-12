<script setup>
// Settings → «Подсказки» (#3000, #3101, mockup HintsPane): the tour and the
// hints, in one place.
//   - the tour: progress, the checklist on «Мои задачи» on/off, «Продолжить
//     главу» and «Начать с начала» (every mark of the tour is cleared, #3100);
//   - the kinds of hints: «Новое после обновлений» (the «Что нового» window and
//     the hints at what changed) and «Подсказки при первой встрече» — each a
//     switch kept as an ack `hints:off:<kind>`;
//   - every hint with its state («показана 6 окт.», «в очереди · 2-я», «ждёт
//     доску с этапом») and «Показать снова»; «Показать подсказки заново» for all.
// Everything saves at once — there is no «Сохранить».
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NSwitch } from 'naive-ui'
import {
  BulbOutline,
  CheckmarkOutline,
  GiftOutline,
  HourglassOutline,
  ListOutline,
  RefreshOutline,
  SparklesOutline,
  RemoveOutline,
} from '@vicons/ionicons5'
import { useOnboardingStore } from '@/stores/onboarding'
import { useHintsStore } from '@/stores/hints'
import { useTourStore } from '@/stores/tour'
import { useFormat } from '@/composables/useFormat'
import { useResponsive } from '@/composables/useResponsive'
import { useTourLauncher } from '@/composables/useTourLauncher'
import { HINTS } from '@/data/hints'
import './settings.css'

const { t } = useI18n()
const ob = useOnboardingStore()
const hints = useHintsStore()
const tour = useTourStore()
const { formatDate } = useFormat()
const { isMobile } = useResponsive()
const { launch } = useTourLauncher()

const tourLine = computed(() =>
  ob.basics.complete
    ? t('settings.hints.tour.passed', { n: ob.chaptersDone, of: ob.chaptersOpen }, ob.chaptersDone)
    : t('settings.hints.tour.basics', { n: ob.basics.done, of: ob.basics.total }),
)
const pct = computed(() => {
  const total = ob.basics.total + ob.chaptersOpen
  const done = ob.basics.done + ob.chaptersDone
  return total ? Math.round((done / total) * 100) : 0
})

function resume() {
  const left = tour.persisted()
  if (left) return launch(left.chapter, { fromId: left.step })
  if (!ob.basics.complete) return launch('start')
  const ch = ob.optional.find((c) => !c.complete && !c.locked)
  launch(ch ? ch.id : 'start')
}
function restart() {
  ob.restart({ mobile: isMobile.value })
}
function setChecklist(on) {
  if (on) ob.unhide()
  else ob.hide()
}

const rows = computed(() =>
  HINTS.map((h) => {
    const st = hints.status(h.key)
    let text
    let icon
    if (st.state === 'shown') {
      text = st.at
        ? t('settings.hints.state.shownAt', {
            date: formatDate(st.at, { day: 'numeric', month: 'short' }),
          })
        : t('settings.hints.state.shown')
      icon = CheckmarkOutline
    } else if (st.state === 'queue') {
      text = t('settings.hints.state.queue', { n: st.n })
      icon = ListOutline
    } else if (st.state === 'wait') {
      text = t(`hints.items.${h.key}.wait`)
      icon = HourglassOutline
    } else if (st.state === 'later') {
      text = t('settings.hints.state.later')
      icon = HourglassOutline
    } else {
      text = t('settings.hints.state.off')
      icon = RemoveOutline
    }
    return { ...h, state: st.state, text, icon }
  }),
)
</script>

<template>
  <div class="sh" data-testid="settings-hints">
    <div class="sh-grid">
      <section class="st-card sh-tour" data-testid="settings-hints-tour">
        <div class="sh-cap">
          <span class="sh-tile accent"><n-icon :component="SparklesOutline" :size="17" /></span>
          <span class="sh-cap-t">
            <span class="sh-cap-title">{{ t('settings.hints.tour.title') }}</span>
            <span class="sh-cap-sub">{{ tourLine }}</span>
          </span>
        </div>
        <span class="sh-bar"><span class="sh-bar-on" :style="{ width: pct + '%' }" /></span>
        <label class="st-toggle">
          <span class="st-toggle-l">
            {{ t('settings.hints.tour.checklist') }}
            <span class="st-toggle-sub">{{ t('settings.hints.tour.checklistSub') }}</span>
          </span>
          <n-switch
            :value="!ob.hidden"
            data-testid="settings-hints-checklist"
            @update:value="setChecklist"
          />
        </label>
        <div class="sh-btns">
          <button type="button" class="sh-ctl" data-testid="settings-hints-resume" @click="resume">
            {{ t('settings.hints.tour.resume') }}
          </button>
          <button
            type="button"
            class="sh-ctl"
            data-testid="settings-hints-restart"
            @click="restart"
          >
            {{ t('settings.hints.tour.restart') }}
          </button>
        </div>
      </section>

      <section class="st-card sh-kinds">
        <label class="sh-kind">
          <span class="sh-tile" style="--hue: #2f80ed"
            ><n-icon :component="GiftOutline" :size="17"
          /></span>
          <span class="sh-cap-t">
            <span class="sh-cap-title">{{ t('settings.hints.kinds.new') }}</span>
            <span class="sh-cap-sub">{{ t('settings.hints.kinds.newSub') }}</span>
          </span>
          <n-switch
            :value="hints.newOn"
            data-testid="settings-hints-new"
            @update:value="(v) => hints.setKind('new', v)"
          />
        </label>
        <label class="sh-kind">
          <span class="sh-tile" style="--hue: #0eb0a9"
            ><n-icon :component="BulbOutline" :size="17"
          /></span>
          <span class="sh-cap-t">
            <span class="sh-cap-title">{{ t('settings.hints.kinds.ctx') }}</span>
            <span class="sh-cap-sub">{{ t('settings.hints.kinds.ctxSub') }}</span>
          </span>
          <n-switch
            :value="hints.ctxOn"
            data-testid="settings-hints-ctx"
            @update:value="(v) => hints.setKind('ctx', v)"
          />
        </label>
        <p class="st-hint">{{ t('settings.hints.kinds.note') }}</p>
      </section>
    </div>

    <section class="sh-table" data-testid="settings-hints-table">
      <div class="sh-tr sh-th">
        <span>{{ t('settings.hints.col.hint') }}</span>
        <span>{{ t('settings.hints.col.kind') }}</span>
        <span>{{ t('settings.hints.col.state') }}</span>
        <span />
      </div>
      <div
        v-for="r in rows"
        :key="r.key"
        class="sh-tr"
        data-testid="settings-hints-row"
        :data-hint="r.key"
        :data-state="r.state"
      >
        <span class="sh-name">{{ t(`hints.items.${r.key}.name`) }}</span>
        <span>
          <span class="sh-pill" :class="r.kind">{{
            t(r.kind === 'ctx' ? 'settings.hints.kind.ctx' : 'settings.hints.kind.new')
          }}</span>
        </span>
        <span class="sh-state"><n-icon :component="r.icon" :size="13" />{{ r.text }}</span>
        <span class="sh-act">
          <button
            v-if="r.state === 'shown'"
            type="button"
            class="sh-again"
            data-testid="settings-hints-again"
            @click="hints.showAgain(r.key)"
          >
            {{ t('settings.hints.again') }}
          </button>
        </span>
      </div>
    </section>

    <div class="sh-reset">
      <button
        type="button"
        class="sh-ctl icon"
        data-testid="settings-hints-reset"
        @click="hints.resetAll()"
      >
        <n-icon :component="RefreshOutline" :size="15" />
        {{ t('settings.hints.reset') }}
      </button>
      <span class="sh-reset-note">{{ t('settings.hints.resetNote') }}</span>
    </div>
  </div>
</template>

<style scoped>
.sh {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.sh-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.sh-grid .st-card {
  padding: 14px;
  gap: 12px;
}
.sh-kinds {
  gap: 6px !important;
}
.sh-cap,
.sh-kind {
  display: flex;
  align-items: center;
  gap: 10px;
}
.sh-kind {
  min-height: 48px;
  cursor: pointer;
}
.sh-tile {
  flex: 0 0 34px;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--hue);
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--hue) 16%, var(--t-col)),
    color-mix(in srgb, var(--hue) 28%, var(--t-col))
  );
}
.sh-tile.accent {
  color: #fff;
  background: var(--t-accent-grad);
}
.sh-cap-t {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.sh-cap-title {
  font-size: 14px;
  font-weight: 600;
}
.sh-cap-sub {
  font-size: 12px;
  color: var(--t-text3);
}
.sh-bar {
  height: 3px;
  border-radius: 2px;
  background: var(--t-pill);
  display: flex;
  overflow: hidden;
}
.sh-bar-on {
  background: var(--t-accent-grad);
  border-radius: 2px;
}
.sh-btns {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.sh-ctl {
  height: 32px;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border: none;
  border-radius: 9px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.sh-ctl:hover {
  background: color-mix(in srgb, var(--t-text1) 12%, var(--t-ctl));
}
.sh-ctl.icon {
  height: 34px;
}
.sh-ctl.icon :deep(.n-icon) {
  color: var(--t-text2);
}
.sh-table {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px;
  border-radius: var(--t-r-pop, 16px);
  background: var(--t-col);
}
.sh-tr {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 150px 210px 130px;
  align-items: center;
  gap: 12px;
  min-height: 40px;
  padding: 0 8px 0 14px;
  border-radius: var(--t-r-ctl, 10px);
}
.sh-tr:not(.sh-th):hover {
  background: var(--t-hover);
}
.sh-th {
  min-height: 36px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.sh-name {
  font-size: 13.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sh-pill {
  height: 22px;
  display: inline-flex;
  align-items: center;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 12%, var(--t-col)),
    color-mix(in srgb, var(--t-primary) 22%, var(--t-col))
  );
}
.sh-pill.ctx {
  background: var(--t-pill);
  color: var(--t-text2);
  font-weight: 500;
}
.sh-state {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 12.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sh-act {
  display: flex;
  justify-content: flex-end;
}
.sh-again {
  height: 28px;
  padding: 0 10px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.sh-again:hover {
  background: var(--t-ctl);
  color: var(--t-text1);
}
.sh-reset {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.sh-reset-note {
  font-size: 12.5px;
  color: var(--t-text3);
}
.sh button:focus {
  outline: none;
}
.sh button:focus-visible {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--t-primary) 75%, transparent);
}
@media (max-width: 900px) {
  .sh-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .sh-tr {
    grid-template-columns: minmax(0, 1fr) auto;
    row-gap: 2px;
    padding: 8px 8px 8px 12px;
  }
  .sh-th {
    display: none;
  }
  .sh-state {
    grid-column: 1;
  }
  .sh-act {
    grid-column: 2;
    grid-row: 1 / span 2;
  }
}
</style>
