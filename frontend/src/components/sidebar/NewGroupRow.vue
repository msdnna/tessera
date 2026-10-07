<script setup>
// A new group typed in place in the sidebar tree (v2 interaction model, #2969,
// mockup SidebarA «Новая группа»): ↵ creates it, ⇧↵ creates it and offers
// another empty row, Esc or a click away with no name drops the row.
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline } from '@vicons/ionicons5'
import InlineEdit from '@/components/kit/InlineEdit.vue'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useTourStore } from '@/stores/tour'
import { useSidebarDraft } from '@/composables/useSidebarDraft'
import { humanizeError } from '@/utils/errors'

const props = defineProps({
  parentId: { type: String, default: null },
})
const { t } = useI18n()
const store = useWorkspacesStore()
const tour = useTourStore()
const draft = useSidebarDraft()

const name = ref('')
const busy = ref(false)
const error = ref('')
const editor = ref(null)

async function commit(value, { via, shift }) {
  const n = value.trim()
  if (!n) {
    if (via !== 'enter') draft.clear()
    return
  }
  busy.value = true
  error.value = ''
  try {
    const body = { name: n }
    if (props.parentId) body.parent_id = props.parentId
    const res = await wsApi.createGroup(store.currentId, body)
    // The Get Started guide's group steps point at the group this row made.
    tour.noteCreated({ groupId: res.data?.id })
    await store.refresh()
    if (via === 'enter' && shift) {
      name.value = ''
      editor.value?.focus()
    } else {
      draft.clear()
    }
  } catch (e) {
    error.value = humanizeError(e)
    editor.value?.focus()
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="row group-row new-group" data-testid="new-group-row">
    <n-icon class="chev" :component="ChevronForwardOutline" />
    <InlineEdit
      ref="editor"
      v-model="name"
      hint="create"
      :busy="busy"
      :error="error"
      :placeholder="t('shell.group.namePlaceholder')"
      :hint-left="-14"
      @commit="commit"
      @cancel="draft.clear()"
    />
  </div>
</template>

<style scoped>
.new-group {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 0 4px 0 8px;
  margin-top: 6px;
}
.chev {
  flex: none;
  font-size: 12px;
  color: var(--t-text3);
}
</style>
