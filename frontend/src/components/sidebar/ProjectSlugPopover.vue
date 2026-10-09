<script setup>
// «Сменить адрес…» (U) of a project as a popover form at its row (v2, #2969,
// mockup PopForm kind=slug) — it replaced the address modal. Manager-only on
// the server; the menu hides it for everyone else.
//
// The old address keeps leading here for 30 days (#3012). A taken address
// names its holder and offers the first free variant: the chip or ⌥↵ puts it
// into the field (#3013).
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import FormField from '@/components/kit/FormField.vue'
import { projects as projApi } from '@/api'
import { useWorkspacesStore } from '@/stores/workspaces'
import { makeSlug } from '@/utils/slug'
import { withProjectSlug } from '@/utils/projectRoute'
import { slugTaken } from '@/utils/slugTaken'
import { humanizeError } from '@/utils/errors'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  project: { type: Object, required: true },
  escapeBack: { type: Boolean, default: false },
})
const emit = defineEmits(['update:show', 'escape'])
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const store = useWorkspacesStore()

const value = ref('')
const error = ref('')
const suggestion = ref('')
const busy = ref(false)
const input = ref(null)
const preview = computed(() => makeSlug(value.value))
watch(
  () => props.show,
  (on) => {
    if (!on) return
    value.value = props.project.slug || ''
    clearError()
  },
)

function clearError() {
  error.value = ''
  suggestion.value = ''
}

// The chip or ⌥↵: the free variant goes into the field, the error clears and
// the form waits for ↵ — the user still sees what they are saving.
function useSuggestion() {
  if (!suggestion.value) return
  value.value = suggestion.value
  clearError()
  input.value?.focus()
}

async function submit() {
  const next = preview.value
  if (!next || busy.value) return
  if (next === props.project.slug) {
    emit('update:show', false)
    return
  }
  busy.value = true
  clearError()
  try {
    const res = await projApi.setSlug(props.project.id, next)
    emit('update:show', false)
    // Keep an open board of this project reachable: same board, new address.
    if (route.params.projectSlug === props.project.slug) {
      router.replace(withProjectSlug(route, res.data.slug))
    }
    await store.refresh()
  } catch (e) {
    const taken = slugTaken(e, t)
    if (taken) {
      error.value = taken.error
      suggestion.value = taken.suggestion
    } else {
      error.value = humanizeError(e)
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :width="340"
    :title="t('project.slug.title')"
    :subtitle="project.name"
    :primary="t('common.action.save')"
    :busy-label="t('common.form.saving')"
    :busy="busy"
    :disabled="!preview || !!error"
    :escape-back="escapeBack"
    testid="project-slug"
    @update:show="emit('update:show', $event)"
    @submit="submit"
    @suggest="useSuggestion"
    @escape="emit('escape')"
  >
    <FormField
      :label="t('shell.projectForm.slug')"
      :preview="preview"
      :error="error"
      :suggestion="suggestion"
      :message="t('project.slug.oldLinks', { old: project.slug })"
      @suggest="useSuggestion"
    >
      <input
        ref="input"
        v-model="value"
        class="kf-input"
        :class="{ error: !!error }"
        :placeholder="t('project.slug.placeholder')"
        :disabled="busy"
        spellcheck="false"
        data-testid="project-slug-input"
        @input="clearError"
      />
    </FormField>
  </PopoverForm>
</template>
