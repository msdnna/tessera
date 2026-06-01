<script setup>
import { NButton, NSpace, NText, NDropdown, NPopover, NSwitch, NTooltip } from 'naive-ui'
import { useRouter } from 'vue-router'
import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { useWorkspacesStore } from '@/stores/workspaces'

defineProps({ mobile: { type: Boolean, default: false } })
defineEmits(['menu'])

const theme = useThemeStore()
const authStore = useAuthStore()
const ws = useWorkspacesStore()
const router = useRouter()

const userOptions = [{ label: 'Выйти', key: 'logout' }]
function onUserSelect(key) {
  if (key === 'logout') {
    authStore.logout()
    router.push('/login')
  }
}
</script>

<template>
  <div class="topbar">
    <div class="left">
      <n-button v-if="mobile" quaternary circle @click="$emit('menu')">☰</n-button>
      <n-text strong>{{ ws.current?.name || 'Tessera' }}</n-text>
    </div>

    <n-space align="center" :size="6">
      <!-- Appearance: color schemes + dark toggle -->
      <n-popover trigger="click" placement="bottom-end">
        <template #trigger>
          <n-tooltip>
            <template #trigger>
              <n-button quaternary circle aria-label="Оформление">
                <span class="swatch" :style="{ background: theme.primaryColor }" />
              </n-button>
            </template>
            Оформление
          </n-tooltip>
        </template>
        <div class="appearance">
          <div class="row">
            <n-text depth="2">Тёмная тема</n-text>
            <n-switch :value="theme.isDark" @update:value="theme.toggle()" />
          </div>
          <div class="swatches">
            <button
              v-for="t in COLOR_THEMES"
              :key="t.key"
              class="swatch-btn"
              :class="{ active: t.key === theme.activeTheme.key }"
              :style="{ background: t.primary }"
              :title="t.name"
              @click="theme.selectColor(t)"
            />
          </div>
        </div>
      </n-popover>

      <n-dropdown trigger="click" :options="userOptions" @select="onUserSelect">
        <n-button quaternary>{{ authStore.user?.name || 'Профиль' }}</n-button>
      </n-dropdown>
    </n-space>
  </div>
</template>

<style scoped>
.topbar {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
}
.left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.swatch {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  display: inline-block;
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
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
  padding: 0;
}
.swatch-btn.active {
  border-color: var(--t-text1);
  box-shadow: 0 0 0 2px var(--t-surface);
}
</style>
