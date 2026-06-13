<script setup>
import { onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  NConfigProvider,
  NGlobalStyle,
  NMessageProvider,
  ruRU,
  dateRuRU,
} from 'naive-ui'
import { useThemeStore } from '@/stores/theme'
import { PRIORITY_COLORS } from '@/styles/tokens'

const theme = useThemeStore()
const router = useRouter()

// Priority colours 1..4 (0 = "none", never gradient'd) → shared flag-icon gradients.
const PRIORITY_GRADS = PRIORITY_COLORS.slice(1)

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
      <!-- Error gradient for destructive popconfirm icons. -->
      <linearGradient id="t-error-grad-svg" x1="0" y1="1" x2="1" y2="0">
        <stop offset="0" style="stop-color: #b33030" />
        <stop offset="0.5" style="stop-color: #e5484d" />
        <stop offset="1" style="stop-color: #f58181" />
      </linearGradient>
      <!-- Priority flag gradients (one per level, indices 1..4 of PRIORITY_COLORS).
           Shared so a board with 100s of cards holds 4 defs, not one SVG per card. -->
      <linearGradient
        v-for="(c, i) in PRIORITY_GRADS"
        :id="`t-prio-grad-${i + 1}`"
        :key="i"
        x1="0"
        y1="1"
        x2="1"
        y2="0"
      >
        <stop offset="0" :style="{ stopColor: `color-mix(in srgb, ${c} 86%, #000)` }" />
        <stop offset="0.5" :style="{ stopColor: c }" />
        <stop offset="1" :style="{ stopColor: `color-mix(in srgb, ${c} 86%, #fff)` }" />
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
      <router-view />
    </n-message-provider>
  </n-config-provider>
</template>
