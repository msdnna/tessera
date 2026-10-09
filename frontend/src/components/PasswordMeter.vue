<script setup>
// The strength meter under a new-password field (v2, #3107 — PackageE
// «Регистрация»): four short segments, coloured up to the score, and a line
// «Надёжный · 12 символов». Advice, not a rule — see utils/password.js.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { passwordStrength, passwordChars } from '@/utils/password'

const props = defineProps({
  value: { type: String, default: '' },
})

const { t } = useI18n()
// Red → amber → green → green: the last two steps are both "fine".
const HUES = ['', '#e0533d', '#f0a020', '#18a058', '#18a058']
const score = computed(() => passwordStrength(props.value))
const hue = computed(() => HUES[score.value])
const caption = computed(() =>
  t(
    'common.auth.strength.caption',
    { level: t(`common.auth.strength.level${score.value || 1}`), n: passwordChars(props.value) },
    passwordChars(props.value),
  ),
)
</script>

<template>
  <div v-if="value" class="pm" data-testid="password-meter">
    <div class="pm-bar">
      <span
        v-for="i in 4"
        :key="i"
        class="pm-seg"
        :style="i <= score ? { background: hue } : null"
      />
    </div>
    <div class="pm-cap">{{ caption }}</div>
  </div>
</template>

<style scoped>
.pm {
  margin: -10px 0 16px;
}
.pm-bar {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 4px;
}
.pm-seg {
  height: 4px;
  border-radius: 2px;
  background: var(--t-ctl);
  transition: background 0.2s ease;
}
.pm-cap {
  margin-top: 6px;
  font-size: 12px;
  color: var(--t-text3);
}
</style>
