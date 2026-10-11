<script setup>
// A floating layer at an anchor that can live INSIDE a Naive modal (#3113).
//
// The kit popovers teleport to <body>, which is right on a page and wrong inside
// a Naive dialog: vueuc's FocusTrap pulls any focus that lands outside the modal
// straight back to its first element (focus-trap `handleDocumentFocus`), so a
// search field or a keyboard-driven menu opened from the task modal could never
// hold the focus. This layer teleports into the enclosing modal's body when there
// is one (Naive provides it as 'n-modal-body') and into <body> otherwise, places
// itself with useAnchoredPanel (viewport clamp, click outside, shortcut blocker,
// focus return), and on a phone becomes a BottomSheet in the same place.
//
// The kit pieces drawn into it use their `inline` mode: <ActionMenu inline>,
// <PopoverForm inline>. Keys typed inside never reach the global dispatcher — the
// modal's own one-letter keys (E, S, J…) would otherwise fire under a menu.
import { computed, inject, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import BottomSheet from './BottomSheet.vue'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { useResponsive } from '@/composables/useResponsive'
import { vLayer } from '@/utils/layerZ'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  placement: { type: String, default: 'bottom-start' },
  offset: { type: Object, default: () => ({ x: 0, y: 6 }) },
  // The sheet's title on a phone.
  title: { type: String, default: '' },
  // Focus this selector inside the layer once it is open (a search field, the
  // inline menu); empty — focus stays where it was.
  focus: { type: String, default: '' },
  testid: { type: String, default: 'anchor-layer' },
})
const emit = defineEmits(['update:show'])

const modalBody = inject('n-modal-body', null)
const target = computed(() => modalBody?.value || 'body')

const { isMobile } = useResponsive()
const panel = ref(null)
const desktopShow = computed(() => props.show && !isMobile.value)
function close() {
  emit('update:show', false)
}
const { style, place } = useAnchoredPanel({
  show: desktopShow,
  anchor: () => props.anchor(),
  panel,
  placement: props.placement,
  offset: props.offset,
  onOutside: close,
})

// The content can grow after it opened (a list that loads, a section that
// appears) — re-place it then, or it hangs off the bottom of the screen.
let ro = null
watch(panel, (el) => {
  ro?.disconnect()
  ro = null
  if (!el || typeof ResizeObserver === 'undefined') return
  ro = new ResizeObserver(() => place())
  ro.observe(el)
})
onBeforeUnmount(() => ro?.disconnect())

const sheetShow = computed({
  get: () => props.show && isMobile.value,
  set: (v) => !v && close(),
})

// Focus the requested element once the layer is in the DOM — two ticks: the
// teleport target resolves on the first.
watch(
  () => props.show,
  async (on) => {
    if (!on || !props.focus) return
    await nextTick()
    await nextTick()
    const root = isMobile.value
      ? document.querySelector(`[data-testid="${props.testid}"]`)
      : panel.value
    root?.querySelector(props.focus)?.focus({ preventScroll: true })
  },
)
defineExpose({ place })
</script>

<template>
  <Teleport :to="target">
    <div
      v-if="desktopShow"
      ref="panel"
      v-layer
      class="al kit-layer"
      :style="style"
      :data-testid="testid"
      @keydown.stop
    >
      <slot />
    </div>
  </Teleport>
  <BottomSheet
    v-if="isMobile"
    v-model:show="sheetShow"
    :title="title"
    :to="target"
    :testid="testid"
  >
    <div class="al-sheet kit-layer" @keydown.stop><slot /></div>
  </BottomSheet>
</template>

<style scoped>
.al {
  position: fixed;
  z-index: 2050;
}
/* A kit piece drawn inline inside the sheet already sits on the sheet's own
   surface — its card (background, shadow, padding) would be a second box. */
.al-sheet :deep(.am.inline),
.al-sheet :deep(.pf.inline) {
  width: auto !important;
  background: transparent;
  box-shadow: none;
}
</style>
