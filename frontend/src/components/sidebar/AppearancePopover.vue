<script setup>
// «Иконка и цвет…» of a tree row as a popover form at the row (v2 interaction
// model, #2969). The full picker of the old settings popovers — badge/glyph
// mode, a transparent colour, search across every icon, upload — so nothing
// those popovers offered is lost. Changes apply as they are picked, like
// before; «Готово» just closes.
import { useI18n } from 'vue-i18n'
import PopoverForm from '@/components/kit/PopoverForm.vue'
import IconColorPicker from '@/components/IconColorPicker.vue'

defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  icon: { type: String, default: '' },
  color: { type: String, default: '' },
  mode: { type: String, default: 'badge' },
  initials: { type: String, default: '?' },
  allowUpload: { type: Boolean, default: false },
  fallbackFolder: { type: Boolean, default: false },
  transparentDefault: { type: Boolean, default: false },
  escapeBack: { type: Boolean, default: false },
})
const emit = defineEmits(['update:show', 'update:icon', 'update:color', 'update:mode', 'escape'])
const { t } = useI18n()
</script>

<template>
  <PopoverForm
    :show="show"
    :anchor="anchor"
    :width="300"
    :title="title || t('common.picker.label')"
    :subtitle="subtitle"
    :primary="t('common.action.done')"
    :escape-back="escapeBack"
    testid="appearance-form"
    @update:show="emit('update:show', $event)"
    @submit="emit('update:show', false)"
    @escape="emit('escape')"
  >
    <IconColorPicker
      :icon="icon"
      :color="color"
      :mode="mode"
      :initials="initials"
      :allow-upload="allowUpload"
      :fallback-folder="fallbackFolder"
      :transparent-default="transparentDefault"
      @update:icon="emit('update:icon', $event)"
      @update:color="emit('update:color', $event)"
      @update:mode="emit('update:mode', $event)"
    />
  </PopoverForm>
</template>
