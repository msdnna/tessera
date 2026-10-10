<script setup>
// Deleting a space — the most dangerous gesture in the app — as a popover form
// with the name typed in (v2 interaction model, #2969, mockup PopForm
// kind=deleteSpace). It replaced ConfirmByName: «Удалить навсегда» lights up
// only when the typed name matches.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useMessage } from '@/composables/useMessage'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { workspaces as wsApi } from '@/api'
import { workspaceName } from '@/utils/defaultNames'
import { humanizeError } from '@/utils/errors'
import { countsPhrase } from '@/utils/deleteCounts'

const props = defineProps({
  show: { type: Boolean, default: false },
  workspace: { type: Object, default: null },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'right-start' },
  offset: { type: Object, default: () => ({ x: 8, y: -8 }) },
})
const emit = defineEmits(['update:show'])
const { t } = useI18n()
const router = useRouter()
const message = useMessage()
const store = useWorkspacesStore()

const typed = ref('')
const busy = ref(false)
const error = ref('')
const name = computed(() => workspaceName(props.workspace))
// What goes with the space (#3014): «4 проекта, 23 доски и 612 задач будут
// удалены». Until the numbers arrive — or if they don't — the generic line.
const counts = ref(null)
watch(
  () => props.show,
  async (on) => {
    if (!on) return
    typed.value = ''
    error.value = ''
    counts.value = null
    const id = props.workspace?.id
    if (!id) return
    try {
      const res = await wsApi.counts(id)
      if (props.show && props.workspace?.id === id) counts.value = res.data
    } catch {
      // Not a gate: typing the name is.
    }
  },
)
const subtitle = computed(() => {
  const what = countsPhrase(t, counts.value)
  return what
    ? t('shell.workspaceForm.deleteSubCounts', { what })
    : t('shell.workspaceForm.deleteSub')
})
const matches = computed(() => typed.value.trim() === name.value)

async function submit() {
  if (!matches.value || busy.value || !props.workspace) return
  busy.value = true
  error.value = ''
  try {
    await store.removeWorkspace(props.workspace.id)
    emit('update:show', false)
    message.success(t('shell.workspace.deleted'))
    router.push('/') // the tree fully changed — land on Home
  } catch (e) {
    error.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :offset="offset"
    :width="340"
    :title="t('shell.workspaceForm.deleteTitle', { name })"
    :subtitle="subtitle"
    :primary="t('shell.workspaceForm.deleteForever')"
    :busy-label="t('common.menu.deleting')"
    :busy="busy"
    :disabled="!matches"
    danger
    testid="ws-delete"
    @update:show="emit('update:show', $event)"
    @submit="submit"
  >
    <FormField
      :label="t('shell.workspaceForm.typeName')"
      :error="error"
      :message="t('shell.workspaceForm.deleteHint')"
    >
      <input
        v-model="typed"
        class="kf-input"
        :placeholder="name"
        :disabled="busy"
        autocomplete="off"
        spellcheck="false"
        data-testid="ws-delete-name"
      />
    </FormField>
  </PopoverForm>
</template>
