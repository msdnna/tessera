import { ref } from 'vue'

// The one draft row of the sidebar tree (v2 interaction model, #2969): a new
// group typed in place. Module-level so the tree has at most one draft at a
// time — starting another (⇧N elsewhere) replaces it — and so the root list and
// any group node agree on where it is.
//
//   draft.value = { kind: 'group', parentId: null | '<group id>' }
const draft = ref(null)

export function useSidebarDraft() {
  return {
    draft,
    startGroup(parentId = null) {
      draft.value = { kind: 'group', parentId }
    },
    clear() {
      draft.value = null
    },
    isGroupDraft(parentId = null) {
      return draft.value?.kind === 'group' && (draft.value.parentId ?? null) === (parentId ?? null)
    },
  }
}
