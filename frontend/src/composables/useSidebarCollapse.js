import { ref, watch } from 'vue'

// Shared (module-singleton) collapsed state for the desktop sidebar, persisted
// to localStorage so it survives reloads — same idea as budget.
const KEY = 'tessera_sidebar_collapsed'
const collapsed = ref(localStorage.getItem(KEY) === '1')

watch(collapsed, (v) => localStorage.setItem(KEY, v ? '1' : '0'))

export function useSidebarCollapse() {
  function toggle() {
    collapsed.value = !collapsed.value
  }
  function setCollapsed(v) {
    collapsed.value = !!v
  }
  return { collapsed, toggle, setCollapsed }
}
