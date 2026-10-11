<script setup>
// Tier 0 of the v2 interaction model (#2969): the text of a row or a header
// turns into a field in place — renaming, or typing the name of a new row.
//
// The field only reports what the user did; the owner decides what it means:
//   commit(value, { via: 'enter' | 'tab' | 'blur', shift })  — ↵ / ⇧↵ / Tab / click away
//   cancel()                                                  — Esc
// An `error` keeps the field open with a red ring and the reason under it; `busy`
// swaps the ↵ cap for a spinner while the save is in flight.
//
// The hint under the field stays visible after mouse use too: it is the only
// place Tab and ⇧↵ are explained (mockup «Отступления от handoff»).
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { AlertCircleOutline } from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import TesseraSpinner from '@/components/TesseraSpinner.vue'
import { vLayer } from '@/utils/layerZ'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  // 'row' — a sidebar tree row (28px field in a 32px row);
  // 'header' — a column header (26px in 30px, semibold).
  variant: { type: String, default: 'row' },
  error: { type: String, default: '' },
  busy: { type: Boolean, default: false },
  // Which keys the hint explains: 'save' (rename), 'create' (new row),
  // 'createView' (new board: + Tab switches the kind), 'column' (+ Tab → next).
  hint: { type: String, default: 'save' },
  // Horizontal offset of the hint tooltip, so it lines up with the text.
  hintLeft: { type: Number, default: 0 },
  showHint: { type: Boolean, default: true },
  // Focus and select on mount — what every real use wants; the design
  // system's previews turn it off so several fields don't fight for focus.
  autofocus: { type: Boolean, default: true },
  // Draw the hint in <body> (default — the sidebar clips overflow). The design
  // system's previews keep it in place, under the field, inside the card.
  teleportHint: { type: Boolean, default: true },
})
const emit = defineEmits(['update:modelValue', 'commit', 'cancel'])

const { t } = useI18n()
const input = ref(null)
const box = ref(null)

// The hint is drawn in <body> at the field's coordinates: the sidebar clips its
// overflow, and a tooltip inside the row would be cut at the sidebar's edge.
// It follows the field frame by frame while the field is open — a new row
// settles into a list that is still expanding, and the tree scrolls — and only
// one inline field exists at a time, so the loop costs next to nothing.
const tipPos = ref({ left: '0px', top: '0px' })
let raf = 0
function placeTip() {
  const r = box.value?.getBoundingClientRect()
  if (r) {
    const left = `${Math.round(r.left + props.hintLeft)}px`
    const top = `${Math.round(r.bottom + 6)}px`
    if (left !== tipPos.value.left || top !== tipPos.value.top) tipPos.value = { left, top }
  }
  raf = requestAnimationFrame(placeTip)
}
onMounted(() => {
  if (props.teleportHint) placeTip()
})
onBeforeUnmount(() => cancelAnimationFrame(raf))
const value = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

// A key that already settled the edit must not be followed by the blur that
// the owner's unmount (or the focus jump on Tab) triggers.
let settled = false

onMounted(async () => {
  if (!props.autofocus) return
  await nextTick()
  input.value?.focus()
  input.value?.select()
})

function onKeydown(e) {
  if (e.isComposing) return
  if (e.key === 'Enter') {
    e.preventDefault()
    e.stopPropagation()
    settled = true
    emit('commit', value.value, { via: 'enter', shift: e.shiftKey })
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    settled = true
    emit('cancel')
  } else if (e.key === 'Tab' && (props.hint === 'createView' || props.hint === 'column')) {
    e.preventDefault()
    e.stopPropagation()
    settled = true
    emit('commit', value.value, { via: 'tab', shift: e.shiftKey })
  }
}

function onBlur() {
  if (settled) return
  emit('commit', value.value, { via: 'blur', shift: false })
}

// Typing again after a ↵ the owner refused (an error) makes a later click-away
// a save attempt once more.
function onInput() {
  settled = false
}

// focus() re-arms the field after the owner kept it open (an error on ↵).
function focus() {
  settled = false
  input.value?.focus()
}
defineExpose({ focus })

const keysHint = computed(() => {
  switch (props.hint) {
    case 'create':
      return [
        [CAPS.enter, t('common.inline.create')],
        [CAPS.escape, ''],
      ]
    case 'createView':
      return [
        [CAPS.enter, t('common.inline.create')],
        [CAPS.tab, t('common.inline.kind')],
        [CAPS.escape, ''],
      ]
    case 'column':
      return [
        [CAPS.enter, t('common.inline.save')],
        [CAPS.escape, t('common.inline.cancel')],
        [CAPS.tab, t('common.inline.next')],
      ]
    default:
      return [
        [CAPS.enter, t('common.inline.save')],
        [CAPS.escape, t('common.inline.cancel')],
      ]
  }
})
</script>

<template>
  <span ref="box" class="ie" :class="[variant, { error: !!error }]">
    <input
      ref="input"
      v-model="value"
      class="ie-input"
      :placeholder="placeholder"
      :readonly="busy"
      :aria-invalid="!!error"
      data-testid="inline-edit"
      @keydown="onKeydown"
      @input="onInput"
      @blur="onBlur"
      @click.stop
      @dblclick.stop
      @mousedown.stop
    />
    <slot name="trailing" />
    <span class="ie-end">
      <TesseraSpinner v-if="busy" :size="14" />
      <n-icon v-else-if="error" :component="AlertCircleOutline" :size="15" class="ie-alert" />
      <Kbd v-else class="ie-kbd">{{ CAPS.enter }}</Kbd>
    </span>
    <Teleport to="body" :disabled="!teleportHint">
      <span
        v-if="showHint && (error || keysHint.length)"
        v-layer
        class="ie-tip"
        :class="{ err: !!error, local: !teleportHint }"
        :style="teleportHint ? tipPos : { left: `${hintLeft}px` }"
        role="status"
      >
        <template v-if="error">{{ error }}</template>
        <template v-else>
          <template v-for="([cap, text], i) in keysHint" :key="i">
            <Kbd :class="{ gap: i > 0 }">{{ cap }}</Kbd>
            <span v-if="text">{{ text }}</span>
          </template>
        </template>
      </span>
    </Teleport>
  </span>
</template>

<style scoped>
.ie {
  position: relative;
  z-index: 3;
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  height: 28px;
  padding: 0 4px 0 0;
  margin-left: -3px;
  box-sizing: border-box;
  border-radius: 7px;
  background: var(--t-input-bg);
  box-shadow: 0 0 0 2px var(--t-primary);
}
.ie.header {
  height: 26px;
}
.ie.error {
  box-shadow: 0 0 0 2px #e0533d;
}
.ie-input {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 0 0 0 7px;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
}
.ie.header .ie-input {
  padding-left: 6px;
  font-size: 13px;
  font-weight: 600;
}
.ie-input::placeholder {
  color: var(--t-placeholder);
  font-weight: 400;
}
.ie-input::selection {
  background: color-mix(in srgb, var(--t-primary) 38%, transparent);
}
.ie-end {
  flex: none;
  display: inline-flex;
  align-items: center;
}
.ie-kbd {
  min-width: 18px;
  height: 18px;
  padding: 0 4px;
  font-size: 10.5px;
  box-shadow: none;
}
.ie-alert {
  color: #e0533d;
}
.ie-tip {
  position: fixed;
  z-index: 2060;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 8px;
  border-radius: var(--t-r-tip);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text2);
  font-size: 12px;
  font-weight: 400;
  white-space: nowrap;
  pointer-events: none;
}
.ie-tip.local {
  position: absolute;
  top: calc(100% + 6px);
}
.ie-tip.err {
  padding: 0 10px;
  color: var(--t-text1);
}
.ie-tip .gap {
  margin-left: 4px;
}
</style>
