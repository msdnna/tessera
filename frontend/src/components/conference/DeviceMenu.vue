<script setup>
// Device picker for the room toolbar (#2864, subtask #2871).
//
// One dropdown for all three kinds rather than three buttons: a standup toolbar
// has room for the two things people press constantly (mic, camera) and not for
// the thing they touch once a month.
import { computed, h } from 'vue'
import { NButton, NDropdown, NIcon } from 'naive-ui'
import { SettingsOutline, CheckmarkOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'

const props = defineProps({
  // { audioinput: [{id,label}], videoinput: [...], audiooutput: [...] }
  devices: { type: Object, required: true },
  selected: { type: Object, required: true },
  disabled: { type: Boolean, default: false },
  // Advanced noise suppression (#2889). It lives here rather than on the toolbar
  // because it is set once for a room and then forgotten — the same reason the
  // device pickers are behind this button.
  denoise: { type: Boolean, default: false },
  denoiseAvailable: { type: Boolean, default: false },
})
const emit = defineEmits(['select', 'toggle-denoise'])

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
      label: label(d, i),
      checked: props.selected[kind] === d.id,
    }))
  }
  // Hidden outright where AudioWorklet is missing: a toggle that cannot do
  // anything is worse than no toggle.
  if (props.denoiseAvailable) {
    if (out.length) out.push({ key: 'sep-denoise', type: 'divider' })
    out.push({
      key: 'h-denoise',
      type: 'group',
      label: t('conferences.media.processing'),
      children: [
        {
          key: 'denoise|toggle',
          label: t('conferences.media.denoise'),
          checked: props.denoise,
        },
      ],
    })
  }
  return out
})

// The chosen row is marked with an accent checkmark at the end of its label —
// the app's shared language for "selected" (the board dropdowns, #2891), not a
// black '●' glued into the text. Done through render-label, NOT the option
// `icon` slot: naive forces the prefix-icon colour to the menu text colour, so a
// checkmark placed there comes out black — the exact bug this is fixing.
function renderLabel(option) {
  if (!option.checked) return option.label
  return h(
    'div',
    { style: 'display:flex;align-items:center;justify-content:space-between;gap:24px' },
    [
      h('span', option.label),
      h(
        NIcon,
        { size: 15, style: 'color:var(--t-primary)' },
        { default: () => h(CheckmarkOutline) },
      ),
    ],
  )
}

function pick(key) {
  if (key === 'denoise|toggle') {
    emit('toggle-denoise')
    return
  }
  const [kind, id] = String(key).split('|')
  if (kind && id) emit('select', kind, id)
}
</script>

<template>
  <n-dropdown
    trigger="click"
    :options="options"
    :render-label="renderLabel"
    :disabled="disabled || !options.length"
    @select="pick"
  >
    <n-button quaternary circle :disabled="disabled" data-testid="conference-devices">
      <n-icon :component="SettingsOutline" />
    </n-button>
  </n-dropdown>
</template>
