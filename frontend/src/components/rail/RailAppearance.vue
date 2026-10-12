<script setup>
// Theme and accent at the foot of the rail (v2), next to the help centre. It is
// a personal setting, not the workspace's, so it left the workspace tool row.
import { NIcon, NPopover, NSwitch, NText, NTooltip } from 'naive-ui'
import { ColorPaletteOutline, MoonOutline, SunnyOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { hueGrad } from '@/utils/gradient'

const { t } = useI18n()
const theme = useThemeStore()
</script>

<template>
  <n-popover trigger="click" placement="right-end">
    <template #trigger>
      <span class="ra-wrap">
        <n-tooltip placement="right" :delay="300">
          <template #trigger>
            <button
              type="button"
              class="ra-btn"
              data-testid="rail-appearance"
              :aria-label="t('shell.tools.appearance')"
            >
              <n-icon :component="ColorPaletteOutline" :size="18" />
            </button>
          </template>
          {{ t('shell.tools.appearance') }}
        </n-tooltip>
      </span>
    </template>
    <div class="appearance">
      <div class="row">
        <n-text depth="2">{{ t('shell.tools.darkTheme') }}</n-text>
        <n-switch :value="theme.isDark" @update:value="theme.toggle()">
          <template #checked-icon><n-icon :component="MoonOutline" /></template>
          <template #unchecked-icon><n-icon :component="SunnyOutline" /></template>
        </n-switch>
      </div>
      <div class="swatches">
        <button
          v-for="ct in COLOR_THEMES"
          :key="ct.key"
          class="swatch-btn"
          :class="{ active: ct.key === theme.activeTheme.key }"
          :style="{ backgroundImage: hueGrad(ct.primary) }"
          :title="t(`settings.appearance.color.${ct.key}`)"
          @click="theme.selectColor(ct)"
        />
      </div>
    </div>
  </n-popover>
</template>

<style scoped>
.ra-wrap {
  display: flex;
}
.ra-btn {
  flex: none;
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: 10px;
  background: transparent;
  color: var(--t-text3);
  cursor: pointer;
}
.ra-btn:hover {
  color: var(--t-text1);
  background: var(--t-hover);
}
.appearance {
  width: 200px;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.swatch-btn {
  appearance: none;
  -webkit-appearance: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid transparent;
  background-origin: border-box;
  cursor: pointer;
  padding: 0;
}
.swatch-btn.active {
  border-color: var(--t-text1);
  box-shadow: 0 0 0 2px var(--t-surface);
}
</style>
