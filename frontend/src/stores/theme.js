import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { darkTheme } from 'naive-ui'

// Theme store — light/dark baseline. Multi-color schemes land in Phase 3.
export const useThemeStore = defineStore('theme', () => {
  const isDark = ref(localStorage.getItem('tessera_dark') === '1')

  const naiveTheme = computed(() => (isDark.value ? darkTheme : null))

  function toggle() {
    isDark.value = !isDark.value
    localStorage.setItem('tessera_dark', isDark.value ? '1' : '0')
  }

  return { isDark, naiveTheme, toggle }
})
