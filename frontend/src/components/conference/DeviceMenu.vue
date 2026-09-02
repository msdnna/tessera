<script setup>
// Device picker for the room toolbar (#2864, subtask #2871).
//
// One dropdown for all three kinds rather than three buttons: a standup toolbar
// has room for the two things people press constantly (mic, camera) and not for
// the thing they touch once a month.
import { computed } from 'vue'
import { NButton, NDropdown, NIcon } from 'naive-ui'
import { SettingsOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'

const props = defineProps({
  // { audioinput: [{id,label}], videoinput: [...], audiooutput: [...] }
  devices: { type: Object, required: true },
  selected: { type: Object, required: true },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['select'])

const { t } = useI18n()

// Labels are empty until the browser has granted permission once — showing a row
// of blank lines reads as broken, so an unnamed device gets a numbered name.
function label(d, i) {
  return d.label || t('conferences.media.deviceFallback', { n: i + 1 })
}

const KINDS = [
  { kind: 'audioinput', title: 'conferences.media.mics' },
  { kind: 'videoinput', title: 'conferences.media.cams' },
  { kind: 'audiooutput', title: 'conferences.media.speakers' },
]

const options = computed(() => {
  const out = []
  for (const { kind, title } of KINDS) {
    const list = props.devices[kind] || []
    // A kind with nothing behind it is omitted entirely: Firefox exposes no
    // audiooutput at all, and an empty "Динамики" group would look like a bug.
    if (!list.length) continue
    if (out.length) out.push({ key: `sep-${kind}`, type: 'divider' })
    out.push({ key: `h-${kind}`, type: 'group', label: t(title), children: [] })
    const group = out[out.length - 1]
    group.children = list.map((d, i) => ({
      // The kind travels in the key because the handler only gets the key back,
      // and a device id alone would not say which slot to switch.
      key: `${kind}|${d.id}`,
      label: (props.selected[kind] === d.id ? '● ' : '') + label(d, i),
    }))
  }
  return out
})

function pick(key) {
  const [kind, id] = String(key).split('|')
  if (kind && id) emit('select', kind, id)
}
</script>

<template>
  <n-dropdown
    trigger="click"
    :options="options"
    :disabled="disabled || !options.length"
    @select="pick"
  >
    <n-button quaternary circle :disabled="disabled" data-testid="conference-devices">
      <n-icon :component="SettingsOutline" />
    </n-button>
  </n-dropdown>
</template>
