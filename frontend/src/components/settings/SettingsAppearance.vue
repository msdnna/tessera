<script setup>
// Settings → «Внешний вид» (v2, #3106). Everything here already saved itself
// through the theme store; the section only moves to v2 surfaces: the theme
// as three preview tiles, the accent as same-hue gradient swatches.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NInput } from 'naive-ui'
import { CheckmarkOutline } from '@vicons/ionicons5'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { swatchBg } from '@/utils/gradient'
import './settings.css'

const { t } = useI18n()
const theme = useThemeStore()

// Computed, not a module const: the labels follow the UI language (#2799).
const modes = computed(() => [
  { key: 'system', label: t('settings.appearance.mode.system') },
  { key: 'light', label: t('settings.appearance.mode.light') },
  { key: 'dark', label: t('settings.appearance.mode.dark') },
])
</script>

<template>
  <div class="sa">
    <section class="st-card">
      <div class="st-card-head">
        <span class="st-card-title">{{ t('settings.appearance.theme') }}</span>
      </div>
      <div class="sa-modes" role="radiogroup" :aria-label="t('settings.appearance.theme')">
        <button
          v-for="m in modes"
          :key="m.key"
          type="button"
          role="radio"
          class="sa-mode"
          :class="[`sa-mode-${m.key}`, { on: theme.themeMode === m.key }]"
          :aria-checked="theme.themeMode === m.key"
          :data-testid="`settings-theme-${m.key}`"
          @click="theme.setThemeMode(m.key)"
        >
          <span class="sa-prev" aria-hidden="true">
            <span class="sa-prev-side" />
            <span class="sa-prev-main">
              <span class="sa-prev-bar" />
              <span class="sa-prev-bar short" />
            </span>
          </span>
          <span class="sa-mode-l">{{ m.label }}</span>
        </button>
      </div>
    </section>

    <section class="st-card">
      <div class="st-card-head">
        <span class="st-card-title">{{ t('settings.appearance.accent') }}</span>
        <span class="st-card-aside">{{
          t(`settings.appearance.color.${theme.activeTheme?.key || 'purple'}`)
        }}</span>
      </div>
      <div class="sa-swatches" role="radiogroup" :aria-label="t('settings.appearance.accent')">
        <button
          v-for="ct in COLOR_THEMES"
          :key="ct.key"
          type="button"
          role="radio"
          class="sa-swatch"
          :class="{ on: theme.activeTheme?.key === ct.key }"
          :aria-checked="theme.activeTheme?.key === ct.key"
          :style="{ backgroundImage: swatchBg(ct.primary) }"
          :title="t(`settings.appearance.color.${ct.key}`)"
          :aria-label="t(`settings.appearance.color.${ct.key}`)"
          @click="theme.selectColor(ct)"
        >
          <n-icon
            v-if="theme.activeTheme?.key === ct.key"
            :component="CheckmarkOutline"
            :size="16"
          />
        </button>
      </div>
    </section>

    <section class="st-card">
      <label class="st-field">
        <span class="st-label">{{ t('settings.appearance.boardBackground') }}</span>
        <n-input
          :value="theme.boardBackground"
          :placeholder="t('settings.appearance.boardBackgroundPlaceholder')"
          clearable
          @update:value="theme.setBoardBackground"
        />
      </label>
    </section>
  </div>
</template>

<style scoped>
.sa {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.sa-modes {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.sa-mode {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 8px 10px;
  border: 0;
  border-radius: var(--t-r-card);
  background: var(--t-card);
  color: var(--t-text1);
  font: inherit;
  cursor: pointer;
  box-shadow: var(--t-sh-card);
}
.sa-mode:hover {
  background: var(--t-card-hover);
}
.sa-mode:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
/* The picked tile: a 2px accent ring — the only line here, and it is state. */
.sa-mode.on {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.sa-mode-l {
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  padding: 0 4px;
}
/* A miniature of the app in that theme: fixed colours on purpose — it is a
   picture of the other theme, not a surface of this one. */
.sa-prev {
  display: flex;
  gap: 6px;
  height: 64px;
  padding: 6px;
  border-radius: 10px;
  background: #f1f1f4;
}
.sa-prev-side {
  width: 22%;
  border-radius: 6px;
  background: #e2e2e8;
}
.sa-prev-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px;
  border-radius: 6px;
  background: #ffffff;
}
.sa-prev-bar {
  height: 7px;
  border-radius: 4px;
  background: var(--t-primary);
  opacity: 0.85;
}
.sa-prev-bar.short {
  width: 55%;
  background: #d9d9e0;
  opacity: 1;
}
.sa-mode-dark .sa-prev {
  background: #1d1d23;
}
.sa-mode-dark .sa-prev-side {
  background: #0b0b0e;
}
.sa-mode-dark .sa-prev-main {
  background: #27272e;
}
.sa-mode-dark .sa-prev-bar.short {
  background: #3a3a43;
}
.sa-mode-system .sa-prev {
  background: linear-gradient(to right, #f1f1f4 50%, #1d1d23 50%);
}
.sa-mode-system .sa-prev-main {
  background: linear-gradient(to right, #ffffff 42%, #27272e 42%);
}
.sa-swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.sa-swatch {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  padding: 0;
  border: 2px solid transparent;
  border-radius: 50%;
  background-origin: border-box;
  color: #fff;
  cursor: pointer;
}
.sa-swatch.on {
  box-shadow:
    0 0 0 2px var(--t-col),
    0 0 0 4px var(--t-text2);
}
.sa-swatch:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 4px;
}
@media (max-width: 560px) {
  .sa-modes {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 8px;
  }
  .sa-prev {
    height: 48px;
  }
}
</style>
