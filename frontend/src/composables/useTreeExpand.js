import { reactive, watch } from 'vue'

// Persists the sidebar tree's expand/collapse state per node id across reloads.
// Stores only explicit overrides; callers pass the default for nodes the user
// hasn't toggled (groups default open, projects default closed).
const KEY = 'tessera_tree_expanded'

function load() {
  try {
    return JSON.parse(localStorage.getItem(KEY) || '{}') || {}
  } catch {
    return {}
  }
}

const state = reactive(load())
watch(
  state,
  () => {
    try {
      localStorage.setItem(KEY, JSON.stringify(state))
    } catch {
      /* storage full / disabled — non-fatal */
    }
  },
  { deep: true },
)

export function useTreeExpand() {
  const isExpanded = (id, dflt) => (id in state ? state[id] : dflt)
  const setExpanded = (id, v) => {
    state[id] = v
  }
  return { isExpanded, setExpanded }
}
