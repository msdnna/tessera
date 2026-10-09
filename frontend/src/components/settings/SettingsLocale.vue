<script setup>
// Settings → «Язык и формат» (v2, #3106). Each select saves on change through
// the theme store, as before.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NSelect } from 'naive-ui'
import { useThemeStore } from '@/stores/theme'
import { timezoneOptions, countryOptions } from '@/utils/localeOptions'
import { useFormat } from '@/composables/useFormat'
import './settings.css'

const { t } = useI18n()
const theme = useThemeStore()
const { datePresetOptions } = useFormat()

const tzOptions = timezoneOptions()
const countryOpts = computed(() => countryOptions(theme.language || 'ru'))
// Option tables are computeds so switching the UI language relabels them (#2799).
const langOptions = computed(() => [
  { label: t('common.language.ru'), value: 'ru' },
  { label: t('common.language.en'), value: 'en' },
])
const timeFmtOptions = computed(() => [
  { label: t('settings.localization.time.h24'), value: '24h' },
  { label: t('settings.localization.time.h12'), value: '12h' },
])
const weekStartOptions = computed(() => [
  { label: t('settings.localization.weekStart.monday'), value: 1 },
  { label: t('settings.localization.weekStart.sunday'), value: 0 },
])
</script>

<template>
  <div class="sl">
    <section class="st-card">
      <div class="st-grid2">
        <label class="st-field">
          <span class="st-label">{{ t('settings.localization.language') }}</span>
          <n-select
            :value="theme.language"
            :options="langOptions"
            @update:value="(v) => theme.setLocale({ language: v })"
          />
        </label>
        <label class="st-field">
          <span class="st-label">{{ t('settings.localization.weekStartLabel') }}</span>
          <n-select
            :value="theme.weekStart"
            :options="weekStartOptions"
            @update:value="(v) => theme.setLocale({ week_start: v })"
          />
        </label>
        <label class="st-field">
          <span class="st-label">{{ t('settings.localization.timeFormat') }}</span>
          <n-select
            :value="theme.timeFormat"
            :options="timeFmtOptions"
            @update:value="(v) => theme.setLocale({ time_format: v })"
          />
        </label>
        <label class="st-field">
          <span class="st-label">{{ t('settings.localization.dateFormat') }}</span>
          <n-select
            :value="theme.dateFormat"
            :options="datePresetOptions"
            @update:value="(v) => theme.setLocale({ date_format: v })"
          />
        </label>
      </div>
    </section>
    <section class="st-card">
      <div class="st-grid2">
        <label class="st-field">
          <span class="st-label">{{ t('settings.localization.timezone') }}</span>
          <n-select
            :value="theme.timezone || null"
            :options="tzOptions"
            filterable
            clearable
            :placeholder="t('settings.localization.timezonePlaceholder')"
            @update:value="(v) => theme.setLocale({ timezone: v || '' })"
          />
        </label>
        <label class="st-field">
          <span class="st-label">{{ t('settings.localization.country') }}</span>
          <n-select
            :value="theme.country || null"
            :options="countryOpts"
            filterable
            clearable
            :placeholder="t('settings.localization.countryPlaceholder')"
            @update:value="(v) => theme.setLocale({ country: v || '' })"
          />
        </label>
      </div>
      <p class="st-hint">{{ t('settings.localization.hint') }}</p>
    </section>
  </div>
</template>

<style scoped>
.sl {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
</style>
