<script setup>
// «Всплывающие в приложении» (#3117, PackageI «Настройки → Уведомления», mockup
// ToastSettings): the card before the channels. What the toasts show — the
// main switch, the «Про меня» kinds, the open board's activity and which of its
// events, folding bursts and the phone. Edits `prefs` in place (the screen's
// own copy of notification_prefs, saved as it changes by NotificationSettings);
// the toast object rides in `prefs.toasts`.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NCheckbox, NIcon, NSwitch } from 'naive-ui'
import { AddOutline, ChatbubblesOutline, RemoveOutline } from '@vicons/ionicons5'
import {
  ACTIVITY_KINDS,
  ACTIVITY_MODES,
  SOUND_MODES,
  BATCH_MAX,
  BATCH_MIN,
  ME_CATEGORIES,
  normalizePrefs,
} from '@/utils/toastModel'
import { playChime, resetChime } from '@/utils/chime'

const props = defineProps({ prefs: { type: Object, required: true } })
const { t } = useI18n()

// The stored object, read with its defaults; every edit writes the whole
// normalised object back, so the server never holds half of one.
const cfg = computed(() => normalizePrefs(props.prefs.toasts))
function set(patch) {
  // eslint-disable-next-line vue/no-mutating-props -- the screen's prefs object, saved by its owner
  props.prefs.toasts = { ...(props.prefs.toasts || {}), ...cfg.value, ...patch }
}
const enabled = computed({
  get: () => props.prefs.toasts_enabled !== false,
  // eslint-disable-next-line vue/no-mutating-props -- see set()
  set: (v) => (props.prefs.toasts_enabled = v),
})
function setMe(k, v) {
  set({ me: { ...cfg.value.me, [k]: v } })
}
function setKind(k, v) {
  set({ activityKinds: { ...cfg.value.activityKinds, [k]: v } })
}
function step(d) {
  const n = Math.min(BATCH_MAX, Math.max(BATCH_MIN, cfg.value.batchMin + d))
  set({ batchMin: n })
}
// «Звук» (#3121): picking a mode that rings plays the chime once, as a sample.
function setSound(m) {
  set({ sound: m })
  if (m !== 'off') {
    resetChime()
    playChime()
  }
}
</script>

<template>
  <section class="tst" :class="{ off: !enabled }" data-testid="notif-toasts">
    <div class="tst-head">
      <span class="tst-tile" aria-hidden="true">
        <n-icon :component="ChatbubblesOutline" :size="17" />
      </span>
      <div class="tst-titles">
        <span class="tst-title">{{ t('settings.notifications.toasts.title') }}</span>
        <span class="tst-sub">{{ t('settings.notifications.toasts.hint') }}</span>
      </div>
      <label class="tst-show">
        <span>{{ t('settings.notifications.toasts.show') }}</span>
        <n-switch v-model:value="enabled" size="small" data-testid="notif-toasts-enabled" />
      </label>
    </div>

    <div class="tst-grid">
      <div class="tst-card">
        <span class="tst-cap">{{ t('settings.notifications.toasts.me.title') }}</span>
        <n-checkbox
          v-for="k in ME_CATEGORIES"
          :key="k"
          :checked="cfg.me[k]"
          :disabled="!enabled"
          :data-testid="`notif-toasts-me-${k}`"
          @update:checked="(v) => setMe(k, v)"
        >
          {{ t(`settings.notifications.toasts.me.${k}`) }}
        </n-checkbox>
      </div>

      <div class="tst-card">
        <span class="tst-cap">{{ t('settings.notifications.toasts.activity.title') }}</span>
        <div class="tst-seg" role="radiogroup">
          <button
            v-for="m in ACTIVITY_MODES"
            :key="m"
            type="button"
            role="radio"
            class="tst-seg-b"
            :class="{ on: cfg.activity === m }"
            :aria-checked="cfg.activity === m"
            :disabled="!enabled"
            :data-testid="`notif-toasts-activity-${m}`"
            @click="set({ activity: m })"
          >
            {{ t(`settings.notifications.toasts.activity.${m}`) }}
          </button>
        </div>
        <span class="tst-note">{{ t('settings.notifications.toasts.activity.kinds') }}</span>
        <div class="tst-kinds">
          <n-checkbox
            v-for="k in ACTIVITY_KINDS"
            :key="k"
            :checked="cfg.activityKinds[k]"
            :disabled="!enabled || cfg.activity === 'off'"
            :data-testid="`notif-toasts-kind-${k}`"
            @update:checked="(v) => setKind(k, v)"
          >
            {{ t(`settings.notifications.toasts.activity.${k}`) }}
          </n-checkbox>
        </div>
        <span class="tst-fine">{{ t('settings.notifications.toasts.activity.note') }}</span>
      </div>

      <div class="tst-card">
        <span class="tst-cap">{{ t('settings.notifications.toasts.behavior.title') }}</span>
        <label class="tst-row">
          <span class="tst-row-t">
            <span>{{ t('settings.notifications.toasts.behavior.batch') }}</span>
            <span class="tst-fine">{{
              t('settings.notifications.toasts.behavior.batchHint')
            }}</span>
          </span>
          <n-switch
            :value="cfg.batch"
            size="small"
            :disabled="!enabled"
            data-testid="notif-toasts-batch"
            @update:value="(v) => set({ batch: v })"
          />
        </label>
        <div class="tst-step" :class="{ off: !cfg.batch }">
          <span>{{ t('settings.notifications.toasts.behavior.from') }}</span>
          <span class="tst-num">
            <button
              type="button"
              :aria-label="t('settings.notifications.toasts.behavior.less')"
              :disabled="!enabled || !cfg.batch || cfg.batchMin <= BATCH_MIN"
              data-testid="notif-toasts-batch-less"
              @click="step(-1)"
            >
              <n-icon :component="RemoveOutline" :size="13" />
            </button>
            <b data-testid="notif-toasts-batch-min">{{ cfg.batchMin }}</b>
            <button
              type="button"
              :aria-label="t('settings.notifications.toasts.behavior.moreN')"
              :disabled="!enabled || !cfg.batch || cfg.batchMin >= BATCH_MAX"
              data-testid="notif-toasts-batch-more"
              @click="step(1)"
            >
              <n-icon :component="AddOutline" :size="13" />
            </button>
          </span>
          <span>{{ t('settings.notifications.toasts.behavior.events') }}</span>
        </div>
        <label class="tst-row">
          <span class="tst-row-t">
            <span>{{ t('settings.notifications.toasts.behavior.phone') }}</span>
            <span class="tst-fine">{{
              t('settings.notifications.toasts.behavior.phoneHint')
            }}</span>
          </span>
          <n-switch
            :value="cfg.phone"
            size="small"
            :disabled="!enabled"
            data-testid="notif-toasts-phone"
            @update:value="(v) => set({ phone: v })"
          />
        </label>
        <div class="tst-row">
          <span class="tst-row-t">
            <span>{{ t('settings.notifications.toasts.behavior.sound') }}</span>
            <span class="tst-fine">{{
              t('settings.notifications.toasts.behavior.soundHint')
            }}</span>
          </span>
          <div class="tst-seg" role="radiogroup">
            <button
              v-for="m in SOUND_MODES"
              :key="m"
              type="button"
              role="radio"
              class="tst-seg-b"
              :class="{ on: cfg.sound === m }"
              :aria-checked="cfg.sound === m"
              :disabled="!enabled"
              :data-testid="`notif-toasts-sound-${m}`"
              @click="setSound(m)"
            >
              {{ t(`settings.notifications.toasts.behavior.soundMode.${m}`) }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.tst {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 14px 16px 16px;
  border-radius: 16px;
  background: var(--t-col);
  color: var(--t-text1);
  font-size: 13px;
}
.tst-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.tst-tile {
  flex: 0 0 34px;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--t-primary);
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-col)),
    color-mix(in srgb, var(--t-primary) 30%, var(--t-col))
  );
}
.tst-titles {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.tst-title {
  font-size: 14px;
  font-weight: 600;
}
.tst-sub {
  font-size: 12.5px;
  color: var(--t-text3);
}
.tst-show {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--t-text2);
  cursor: pointer;
}
.tst-grid {
  display: grid;
  /* The activity card is never narrower than its segmented control. */
  grid-template-columns: minmax(0, 1.1fr) minmax(min-content, 1.2fr) minmax(0, 1fr);
  gap: 12px;
}
.tst.off .tst-grid {
  opacity: 0.55;
}
.tst-card {
  display: flex;
  flex-direction: column;
  gap: 9px;
  padding: 12px 14px;
  border-radius: 12px;
  background: var(--t-card);
  min-width: 0;
}
.tst-cap {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.tst-seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--t-ctl);
}
.tst-seg-b {
  flex: 1 0 auto;
  height: 26px;
  padding: 0 8px;
  white-space: nowrap;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.tst-seg-b.on {
  font-weight: 600;
  color: var(--t-text1);
  background: var(--t-pop);
  box-shadow: var(--t-sh-card);
}
.tst-seg-b:disabled {
  cursor: default;
}
.tst-note {
  font-size: 12.5px;
  color: var(--t-text3);
}
.tst-kinds {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 9px;
}
.tst-fine {
  font-size: 12px;
  line-height: 1.45;
  color: var(--t-text3);
}
.tst-row {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}
.tst-row-t {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.tst-step {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: -4px;
  font-size: 12.5px;
  color: var(--t-text2);
}
.tst-step.off {
  opacity: 0.55;
}
.tst-num {
  height: 28px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 3px;
  border-radius: 8px;
  background: var(--t-input-bg);
}
.tst-num b {
  min-width: 18px;
  text-align: center;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--t-text1);
}
.tst-num button {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.tst-num button:hover:not(:disabled) {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tst-num button:disabled {
  opacity: 0.45;
  cursor: default;
}
@media (max-width: 1100px) {
  .tst-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 768px) {
  .tst-head {
    flex-wrap: wrap;
  }
}
</style>
