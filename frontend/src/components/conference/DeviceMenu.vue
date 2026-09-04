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

// Accent checkmark on the chosen row — the app's shared language for "selected",
// identical to the board dropdowns' `primIcon` (ProjectRow.vue). The colour must
// be the NIcon `color` PROP, not a `style:color`: naive's icon paints its own
// colour and ignores an inherited one, which is why the earlier style-based
// attempts came out grey/black (#2891). Unselected rows get no icon; naive still
// reserves the icon column for the whole menu, so labels don't shift.
const checkIcon = () => () =>
  h(NIcon, { color: 'var(--t-primary)' }, { default: () => h(CheckmarkOutline) })

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
      icon: props.selected[kind] === d.id ? checkIcon() : undefined,
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
          icon: props.denoise ? checkIcon() : undefined,
        },
      ],
    })
  }
  return out
})

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
    :disabled="disabled || !options.length"
    @select="pick"
  >
    <n-button quaternary circle :disabled="disabled" data-testid="conference-devices">
      <n-icon :component="SettingsOutline" />
    </n-button>
  </n-dropdown>
</template>
