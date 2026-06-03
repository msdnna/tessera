import { ref, computed, watch } from 'vue'

// Shared (module-singleton) desktop sidebar sizing, persisted to localStorage.
// The sidebar is drag-resizable: above the collapse threshold it shows the full
// width (clamped to a max); dragged narrower than the threshold it snaps to a
// compact icon rail.
const W_KEY = 'tessera_sidebar_width'
const C_KEY = 'tessera_sidebar_collapsed'

export const COLLAPSED_W = 60
// Below this dragged width the rail collapses; at/above it the sidebar expands.
export const COLLAPSE_AT = 170
// Expanded but narrower than this → "narrow" mode: tools move to the header and
// the add-workspace button is hidden so nothing overflows.
export const NARROW_AT = 216
export const MAX_EXPANDED = 264

function clampExpanded(w) {
  return Math.min(MAX_EXPANDED, Math.max(COLLAPSE_AT, w))
}

const storedW = parseInt(localStorage.getItem(W_KEY) || '', 10)
const expandedWidth = ref(Number.isFinite(storedW) ? clampExpanded(storedW) : 264)
const collapsed = ref(localStorage.getItem(C_KEY) === '1')

watch(expandedWidth, (v) => localStorage.setItem(W_KEY, String(v)))
watch(collapsed, (v) => localStorage.setItem(C_KEY, v ? '1' : '0'))

// Effective width the layout should render.
const layoutWidth = computed(() => (collapsed.value ? COLLAPSED_W : expandedWidth.value))
// Expanded yet too narrow to hold the brand tools comfortably.
const narrow = computed(() => !collapsed.value && expandedWidth.value < NARROW_AT)

export function useSidebarSize() {
  // Apply a live pointer x-position (≈ desired width) while dragging the divider.
  function applyDragWidth(px) {
    if (px < COLLAPSE_AT) {
      collapsed.value = true
    } else {
      collapsed.value = false
      expandedWidth.value = clampExpanded(px)
    }
  }
  function toggle() {
    collapsed.value = !collapsed.value
  }
  return { collapsed, narrow, layoutWidth, expandedWidth, applyDragWidth, toggle }
}
