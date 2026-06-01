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
