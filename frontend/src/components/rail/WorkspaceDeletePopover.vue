<script setup>
// Deleting a space — the most dangerous gesture in the app — as a popover form
// with the name typed in (v2 interaction model, #2969, mockup PopForm
// kind=deleteSpace). It replaced ConfirmByName: «Удалить навсегда» lights up
// only when the typed name matches.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { workspaceName } from '@/utils/defaultNames'
import { humanizeError } from '@/utils/errors'

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
watch(
  () => props.show,
  (on) => {
    if (!on) return
    typed.value = ''
    error.value = ''
  },
)
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
    :width="320"
    :title="t('shell.workspaceForm.deleteTitle', { name })"
    :subtitle="t('shell.workspaceForm.deleteSub')"
    :primary="t('shell.workspaceForm.deleteForever')"
    :busy-label="t('common.menu.deleting')"
    :busy="busy"
    :disabled="!matches"
    danger
    :hint="t('shell.workspaceForm.deleteHint')"
    testid="ws-delete"
    @update:show="emit('update:show', $event)"
    @submit="submit"
  >
    <FormField :label="t('shell.workspaceForm.typeName')" :error="error">
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
