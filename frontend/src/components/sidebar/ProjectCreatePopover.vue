<script setup>
// «Новый проект» as a popover form at its «+» (v2 interaction model, #2969,
// mockup PackageA frame 2) — it replaced ProjectCreateModal.
//
// Name, address and icon/colour. The address follows the name until the user
// edits it (aside «из названия»), and is only sent when edited: left alone, the
// server derives it the same way and keeps it unique. Picking the address is
// manager-only server-side, so for everyone else it is a preview, not a field.
// ⌘↵ creates and opens the project's first board.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import ColorIconPicker from '@/components/kit/ColorIconPicker.vue'
import { workspaces as wsApi, projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useTourStore } from '@/stores/tour'
import { makeSlug } from '@/utils/slug'
import { humanizeError } from '@/utils/errors'
import { PICKER_COLORS } from '@/utils/pickerColors'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  // Group to create the project in; null creates it at the tree root.
  groupId: { type: String, default: null },
  groupName: { type: String, default: '' },
  placement: { type: String, default: 'right-start' },
})
const emit = defineEmits(['update:show', 'created'])

const { t } = useI18n()
const router = useRouter()
const store = useWorkspacesStore()
const tour = useTourStore()

const name = ref('')
const slug = ref('')
const slugTouched = ref(false)
const color = ref(PICKER_COLORS[0].hex)
const icon = ref('code')
const busy = ref(false)
const error = ref('')
const slugError = ref('')

const canPickSlug = computed(() => store.canManage)
const preview = computed(() => makeSlug(slugTouched.value ? slug.value : name.value))

watch(
  () => props.show,
  (open) => {
    if (!open) return
    name.value = ''
    slug.value = ''
    slugTouched.value = false
    color.value = PICKER_COLORS[0].hex
    icon.value = 'code'
    error.value = ''
    slugError.value = ''
  },
)
watch(name, (v) => {
  if (!slugTouched.value) slug.value = makeSlug(v)
})
function onSlugInput(e) {
  slugTouched.value = true
  slug.value = e.target.value
  slugError.value = ''
}

const disabled = computed(
  () => !name.value.trim() || (slugTouched.value && !preview.value) || !!slugError.value,
)
const subtitle = computed(() =>
  props.groupName
    ? t('shell.projectForm.inGroup', { group: props.groupName })
    : t('shell.projectForm.atRoot'),
)

let aborted = false
async function submit({ open }) {
  if (disabled.value || busy.value) return
  busy.value = true
  aborted = false
  error.value = ''
  try {
    const body = { name: name.value.trim(), color: color.value, icon: icon.value }
    if (props.groupId) body.group_id = props.groupId
    if (canPickSlug.value && slugTouched.value && preview.value) body.slug = preview.value
    const res = await wsApi.createProject(store.currentId, body)
    if (aborted) return
    tour.noteCreated({ projectId: res.data?.id })
    emit('update:show', false)
    emit('created', res.data)
    await store.refresh()
    if (open && res.data) {
      // The project is born without a board; ⌘↵ lands on the first one if it
      // has any, else on the project's page with the board templates (#3047).
      const boards = (await projApi.boards(res.data.id)).data || []
      if (boards[0]) router.push(`/project/${res.data.slug}/board/${boards[0].slug}`)
      else router.push(`/project/${res.data.slug}`)
    }
  } catch (e) {
    if (e.status === 409) slugError.value = t('project.slug.taken')
    else error.value = humanizeError(e)
  } finally {
    busy.value = false
  }
}
// Esc while the request is out: the form closes and the late answer is ignored.
function abort() {
  aborted = true
  busy.value = false
  emit('update:show', false)
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :placement="placement"
    :width="348"
    :title="t('shell.projectForm.title')"
    :subtitle="subtitle"
    :primary="t('common.action.create')"
    :busy-label="t('shell.projectForm.creating')"
    :busy="busy"
    :disabled="disabled"
    hint="tab"
    testid="project-create"
    submit-tour="project-submit"
    @update:show="emit('update:show', $event)"
    @submit="submit"
    @abort="abort"
  >
    <FormField :label="t('shell.projectForm.name')" :error="error">
      <input
        v-model="name"
        class="kf-input"
        :placeholder="t('shell.projectForm.namePlaceholder')"
        :disabled="busy"
        data-tour="project-name"
        data-testid="project-create-name"
      />
    </FormField>
    <FormField
      :label="t('shell.projectForm.slug')"
      :aside="canPickSlug && !slugTouched ? t('shell.projectForm.fromName') : ''"
      :preview="preview"
      :error="slugError"
    >
      <input
        v-if="canPickSlug"
        :value="slug"
        class="kf-input"
        :class="{ error: !!slugError }"
        :placeholder="t('shell.projectForm.slugPlaceholder')"
        :disabled="busy"
        data-tour="project-slug"
        data-testid="project-create-slug"
        spellcheck="false"
        @input="onSlugInput"
      />
    </FormField>
    <FormField :label="t('common.picker.label')">
      <ColorIconPicker
        v-model:icon="icon"
        v-model:color="color"
        :name="name"
        set="project"
        allow-upload
      />
    </FormField>
  </PopoverForm>
</template>
