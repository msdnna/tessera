import { ref, computed, watch } from 'vue'

// Shared (module-singleton) desktop sidebar state, persisted to localStorage.
// v2 (#2953): the sidebar has one width; it is either expanded or collapsed to
// the icon rail — toggled by the edge button or the `[` key, no drag-resizing.
const C_KEY = 'tessera_sidebar_collapsed'

export const EXPANDED_W = 232
export const COLLAPSED_W = 60

const collapsed = ref(localStorage.getItem(C_KEY) === '1')
watch(collapsed, (v) => localStorage.setItem(C_KEY, v ? '1' : '0'))

// Effective width the layout should render.
const layoutWidth = computed(() => (collapsed.value ? COLLAPSED_W : EXPANDED_W))

export function useSidebarSize() {
  function toggle() {
    collapsed.value = !collapsed.value
  }
  return { collapsed, layoutWidth, toggle }
}
