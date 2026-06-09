<script setup>
import { onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  NConfigProvider,
  NGlobalStyle,
  NMessageProvider,
  NDialogProvider,
  ruRU,
  dateRuRU,
} from 'naive-ui'
import { useThemeStore } from '@/stores/theme'

const theme = useThemeStore()
const router = useRouter()

// When a refresh ultimately fails, the api client clears auth and fires this.
function onExpired() {
  router.push('/login')
}
onMounted(() => window.addEventListener('auth:expired', onExpired))
onUnmounted(() => window.removeEventListener('auth:expired', onExpired))
</script>

<template>
  <!-- Global accent gradient for SVG icons (referenced via fill/stroke:
       url(#t-accent-grad-svg) by the .grad-icon helper). Stops follow the theme
       via --t-primary. -->
  <svg width="0" height="0" aria-hidden="true" style="position: absolute">
    <defs>
      <linearGradient id="t-accent-grad-svg" x1="0" y1="1" x2="1" y2="0">
        <stop offset="0" style="stop-color: color-mix(in srgb, var(--t-primary) 86%, #000)" />
        <stop offset="0.5" style="stop-color: var(--t-primary)" />
        <stop offset="1" style="stop-color: color-mix(in srgb, var(--t-primary) 86%, #fff)" />
      </linearGradient>
    </defs>
  </svg>
  <n-config-provider
    :theme="theme.naiveTheme"
    :theme-overrides="theme.themeOverrides"
    :locale="ruRU"
    :date-locale="dateRuRU"
  >
    <n-global-style />
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
