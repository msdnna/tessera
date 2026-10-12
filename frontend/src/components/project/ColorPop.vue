<script setup>
// A swatch row in a small popover at its anchor (#2990): the tag table's colour
// cell and «Перекрасить» on a selection. ←/→ walk the swatches, ↵ picks, Esc
// closes.
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAnchoredPanel } from '@/composables/useAnchoredPanel'
import { PICKER_COLORS } from '@/utils/pickerColors'
import { hueGrad } from '@/utils/gradient'
import { vLayer } from '@/utils/layerZ'

const props = defineProps({
  show: { type: Boolean, default: false },
  anchor: { type: Function, default: () => null },
  value: { type: String, default: '' },
  title: { type: String, default: '' },
  placement: { type: String, default: 'bottom-start' },
})
const emit = defineEmits(['update:show', 'pick'])
const { t } = useI18n()
const panel = ref(null)
const shown = computed(() => props.show)
const { style } = useAnchoredPanel({
  show: shown,
  anchor: () => props.anchor(),
  panel,
  placement: props.placement,
  offset: { x: 0, y: 6 },
  onOutside: () => emit('update:show', false),
})
const btns = ref([])
watch(
  () => props.show,
  async (on) => {
    if (!on) return
    await nextTick()
    const i = Math.max(
      0,
      PICKER_COLORS.findIndex((c) => c.hex === props.value),
    )
    btns.value[i]?.focus()
  },
)
function onKey(e) {
  const i = btns.value.indexOf(document.activeElement)
  if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
    e.preventDefault()
    const n = PICKER_COLORS.length
    const next = (i + (e.key === 'ArrowRight' ? 1 : n - 1)) % n
    btns.value[next]?.focus()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    emit('update:show', false)
  }
}
function pick(hex) {
  emit('pick', hex)
  emit('update:show', false)
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="show"
      ref="panel"
      v-layer
      class="cpop"
      :style="style"
      role="dialog"
      data-testid="color-pop"
      @keydown="onKey"
    >
      <div v-if="title" class="cpop-t">{{ title }}</div>
      <div class="cpop-row">
        <button
          v-for="(c, i) in PICKER_COLORS"
          :key="c.hex"
          :ref="(el) => (btns[i] = el)"
          type="button"
          class="cpop-sw"
          :class="{ on: c.hex === value }"
          :style="{ background: hueGrad(c.hex) }"
          :title="t(`common.colors.${c.key}`)"
          :data-color="c.hex"
          @click="pick(c.hex)"
        />
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.cpop {
  position: fixed;
  z-index: 2050;
  padding: 10px 12px;
  border-radius: var(--t-r-pop);
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
}
.cpop-t {
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--t-text3);
}
.cpop-row {
  display: flex;
  gap: 8px;
}
.cpop-sw {
  width: 22px;
  height: 22px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  outline: 0;
}
.cpop-sw.on {
  box-shadow:
    0 0 0 2px var(--t-pop),
    0 0 0 4px var(--t-primary);
}
.cpop-sw:focus-visible {
  box-shadow:
    0 0 0 2px var(--t-pop),
    0 0 0 4px var(--t-text2);
}
</style>
