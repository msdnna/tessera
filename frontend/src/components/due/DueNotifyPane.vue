<script setup>
// «Напоминание о сроке» — the due picker's second door (v2, #3118, mockup
// DuePicker pane=not): on / off / by default, how early, how often again.
//
// The wire keeps three tri-states (`enabled` inherit|on|off, `lead` and `repeat`
// minutes with -1 = «как в настройках»). A chip shows the value in force: the
// task's own (filled) or the inherited default (outlined, «по умолчанию»).
// Pressing the task's own chip again hands the field back to the default.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { effectiveNotify, LEAD_CHIPS, leadLabel, REPEAT_CHIPS, repeatLabel } from '@/utils/dueModel'

const props = defineProps({
  notify: { type: Object, required: true }, // { enabled, lead, repeat }
  defaults: { type: Object, default: null },
  mobile: { type: Boolean, default: false },
})
const emit = defineEmits(['notify'])
const { t } = useI18n()

const eff = computed(() => effectiveNotify(props.notify, props.defaults))
const MODES = ['inherit', 'on', 'off']
const modeKey = { inherit: 'default', on: 'on', off: 'off' }
const own = (field) => props.notify[field] !== -1 && props.notify[field] != null

function chipState(field, v) {
  if (own(field)) return props.notify[field] === v ? 'on' : ''
  return eff.value[field] === v ? 'inh' : ''
}
function pick(field, v) {
  emit('notify', { [field]: own(field) && props.notify[field] === v ? -1 : v })
}
const defaultsLine = computed(() => {
  const d = effectiveNotify({ enabled: 'inherit', lead: -1, repeat: -1 }, props.defaults)
  if (!d.enabled) return t('task.due.notify.defaultsOff')
  return t('task.due.notify.defaultsLine', {
    lead: leadLabel(d.lead).toLowerCase(),
    repeat: repeatLabel(d.repeat).toLowerCase(),
  })
})
</script>

<template>
  <div class="dnp" :class="{ mobile, dim: !eff.enabled }" data-testid="due-notify">
    <div class="dnp-sec">{{ t('task.due.notify.title') }}</div>
    <div class="dnp-seg" role="radiogroup" :aria-label="t('task.due.notify.title')">
      <button
        v-for="md in MODES"
        :key="md"
        type="button"
        class="dnp-sego"
        :class="{ on: notify.enabled === md }"
        role="radio"
        :aria-checked="notify.enabled === md"
        :data-mode="md"
        @click="emit('notify', { enabled: md })"
      >
        {{ t(`task.due.notify.${modeKey[md]}`) }}
      </button>
    </div>
    <div class="dnp-sec">{{ t('task.due.notify.lead') }}</div>
    <div class="dnp-chips">
      <button
        v-for="v in LEAD_CHIPS"
        :key="v"
        type="button"
        class="dnp-chip"
        :class="chipState('lead', v)"
        :data-lead="v"
        :disabled="!eff.enabled"
        @click="pick('lead', v)"
      >
        {{ leadLabel(v) }}
      </button>
    </div>
    <div class="dnp-sec">{{ t('task.due.notify.repeatTitle') }}</div>
    <div class="dnp-chips">
      <button
        v-for="v in REPEAT_CHIPS"
        :key="v"
        type="button"
        class="dnp-chip"
        :class="chipState('repeat', v)"
        :data-repeat="v"
        :disabled="!eff.enabled"
        @click="pick('repeat', v)"
      >
        {{ repeatLabel(v) }}
      </button>
    </div>
    <div class="dnp-note">{{ t('task.due.notify.note', { defaults: defaultsLine }) }}</div>
  </div>
</template>

<style scoped>
.dnp {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0 6px 6px;
}
.dnp-sec {
  padding: 8px 4px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.dnp-seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--t-ctl);
}
.dnp-sego {
  flex: 1;
  height: 28px;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.dnp-sego:hover {
  color: var(--t-text1);
}
.dnp-sego.on {
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font-weight: 600;
}
.dnp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.dnp-chip {
  height: 30px;
  min-width: 30px;
  box-sizing: border-box;
  padding: 0 11px;
  border: 0;
  border-radius: 999px;
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.dnp-chip:hover:not(:disabled) {
  background: var(--t-hover);
}
.dnp-chip:disabled {
  opacity: 0.45;
  cursor: default;
}
.dnp-chip.on {
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--t-primary) 16%, var(--t-pop)),
    color-mix(in srgb, var(--t-primary) 28%, var(--t-pop))
  );
  font-weight: 600;
}
/* The default in force — the same tint, quieter, with a ring: «как в настройках». */
.dnp-chip.inh {
  background: color-mix(in srgb, var(--t-primary) 10%, var(--t-pop));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--t-primary) 45%, transparent);
  font-weight: 600;
}
.dnp-note {
  margin-top: 6px;
  padding: 9px 10px;
  border-radius: 9px;
  background: var(--t-sub);
  font-size: 12.5px;
  line-height: 1.45;
  color: var(--t-text2);
}
.dnp.mobile {
  padding: 0;
}
.dnp.mobile .dnp-sego {
  height: 36px;
  font-size: 13.5px;
}
.dnp.mobile .dnp-chip {
  height: 38px;
  font-size: 14px;
}
</style>
