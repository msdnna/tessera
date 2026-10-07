<script setup>
// «Новое пространство» and «Оформление пространства» as one popover form at
// the rail (v2 interaction model, #2969 / #2983, mockup PopForm kind=space /
// spaceStyle) — they replaced the create modal and WorkspaceAppearanceModal.
// Name, colour and the tile's icon (the first one: initials of the name).
// The name goes through the rename endpoint and the tile through its own, so a
// failure in one leaves the other intact.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import ColorIconPicker from '@/components/kit/ColorIconPicker.vue'
import { workspaces as wsApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { workspaceName } from '@/utils/defaultNames'
import { humanizeError } from '@/utils/errors'
import { PICKER_COLORS } from '@/utils/pickerColors'
import { tileColor } from './workspaceTile'

const props = defineProps({
  show: { type: Boolean, default: false },
  // 'create' | 'appearance'
  kind: { type: String, default: 'create' },
  workspace: { type: Object, default: null },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'right-start' },
  offset: { type: Object, default: () => ({ x: 8, y: -8 }) },
})
const emit = defineEmits(['update:show'])
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const store = useWorkspacesStore()

const name = ref('')
const color = ref(PICKER_COLORS[1].hex)
const icon = ref('')
const busy = ref(false)
const error = ref('')
const isCreate = computed(() => props.kind === 'create')

watch(
  () => [props.show, props.workspace, props.kind],
  () => {
    if (!props.show) return
    error.value = ''
    if (isCreate.value || !props.workspace) {
      name.value = ''
      color.value = PICKER_COLORS[1].hex
      icon.value = ''
    } else {
      name.value = workspaceName(props.workspace)
      color.value = tileColor(props.workspace)
      icon.value = props.workspace.icon || ''
    }
  },
  { immediate: true },
)

const title = computed(() => (isCreate.value ? t('shell.workspaceForm.createTitle') : t('shell.workspaceForm.styleTitle')))
const subtitle = computed(() =>
  isCreate.value
    ? t('shell.workspaceForm.createSub')
    : t('shell.workspaceForm.styleSub', { name: workspaceName(props.workspace) }),
)

async function submit() {
  const n = name.value.trim()
  if (!n || busy.value) return
  busy.value = true
  error.value = ''
  try {
    if (isCreate.value) {
      const w = await store.createWorkspace(n)
      const res = await wsApi.setAppearance(w.id, { color: color.value, icon: icon.value, icon_mode: 'badge' })
      store.patchWorkspace(res.data)
      // A board belongs to the workspace we just left.
      if (route.params.projectSlug || route.params.id) router.push('/')
    } else {
      const w = props.workspace
      if (n !== workspaceName(w)) store.patchWorkspace((await wsApi.update(w.id, { name: n })).data)
      const res = await wsApi.setAppearance(w.id, {
        color: color.value,
        icon: icon.value,
        icon_mode: w.icon_mode || 'badge',
      })
      store.patchWorkspace(res.data)
    }
    emit('update:show', false)
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
    :title="title"
    :subtitle="subtitle"
    :primary="isCreate ? t('common.action.create') : t('common.action.save')"
    :busy-label="isCreate ? t('shell.projectForm.creating') : t('common.form.saving')"
    :busy="busy"
    :disabled="!name.trim()"
    hint="tab"
    :testid="isCreate ? 'ws-create' : 'ws-appearance'"
    @update:show="emit('update:show', $event)"
    @submit="submit"
  >
    <FormField :label="t('shell.projectForm.name')" :error="error">
      <input
        v-model="name"
        class="kf-input"
        :placeholder="t('shell.workspace.namePlaceholder')"
        :disabled="busy"
        data-testid="ws-form-name"
      />
    </FormField>
    <FormField :label="isCreate ? t('shell.workspaceForm.look') : t('shell.workspaceForm.lookRail')">
      <ColorIconPicker v-model:icon="icon" v-model:color="color" :name="name" set="space" />
    </FormField>
  </PopoverForm>
</template>
