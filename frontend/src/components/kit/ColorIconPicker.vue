<script setup>
// «Иконка и цвет» of the v2 popover forms (#2969): seven colours and eight icons
// in one well, the first icon being the initials of the name typed above. «⋯»
// opens the full picker (search across every icon, upload) the old modals had.
//
// Keys while the well is focused: ←/→ colour, ↑/↓ icon.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover } from 'naive-ui'
import { EllipsisHorizontalOutline } from '@vicons/ionicons5'
import ProjectIcon from '@/components/ProjectIcon.vue'
import IconColorPicker from '@/components/IconColorPicker.vue'
import { hueGrad } from '@/utils/gradient'
import { initials as initialsOf } from '@/utils/initials'
import { PICKER_COLORS } from '@/utils/pickerColors'

const props = defineProps({
  icon: { type: String, default: '' },
  color: { type: String, default: '' },
  name: { type: String, default: '' },
  // 'project' or 'space' — which eight icons the row offers (mockup PopForm).
  set: { type: String, default: 'project' },
  allowUpload: { type: Boolean, default: false },
})
const emit = defineEmits(['update:icon', 'update:color'])
const { t } = useI18n()

const SETS = {
  project: ['', 'briefcase', 'code', 'rocket', 'flask', 'star', 'book', 'bulb'],
  space: ['', 'briefcase', 'home', 'code', 'rocket', 'school', 'heart', 'game'],
}
const icons = computed(() => SETS[props.set] || SETS.project)

const initials = computed(() => {
  const clean = String(props.name || '').replace(/[«»"]/g, '').trim()
  return clean ? initialsOf(clean) : t('common.picker.initialsEmpty')
})
const colorIndex = computed(() => PICKER_COLORS.findIndex((c) => c.hex === props.color))
const colorName = computed(() => {
  const c = PICKER_COLORS[colorIndex.value]
  return c ? t(`common.colors.${c.key}`).toLowerCase() : ''
})
const tileBg = computed(() => hueGrad(props.color || PICKER_COLORS[0].hex))

function onKeydown(e) {
  if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
    e.preventDefault()
    const n = PICKER_COLORS.length
    const i = colorIndex.value < 0 ? 0 : colorIndex.value
    const next = (i + (e.key === 'ArrowRight' ? 1 : -1) + n) % n
    emit('update:color', PICKER_COLORS[next].hex)
  } else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
    e.preventDefault()
    const list = icons.value
    const n = list.length
    const cur = list.indexOf(props.icon)
    const i = cur < 0 ? 0 : cur
    const next = (i + (e.key === 'ArrowDown' ? 1 : -1) + n) % n
    emit('update:icon', list[next])
  }
}

const fullOpen = ref(false)
defineExpose({ colorName })
</script>

<template>
  <div
    class="cip"
    tabindex="0"
    role="group"
    :aria-label="t('common.picker.label')"
    data-no-select
    data-testid="color-icon-picker"
    @keydown="onKeydown"
  >
    <div class="cip-colors">
      <button
        v-for="c in PICKER_COLORS"
        :key="c.hex"
        type="button"
        tabindex="-1"
        class="cip-swatch"
        :class="{ on: c.hex === color }"
        :style="{ background: hueGrad(c.hex), '--sw': c.hex }"
        :title="t(`common.colors.${c.key}`)"
        :aria-pressed="c.hex === color"
        @click="emit('update:color', c.hex)"
      />
    </div>
    <div class="cip-icons">
      <button
        v-for="key in icons"
        :key="key || '@'"
        type="button"
        tabindex="-1"
        class="cip-icon"
        :class="{ on: key === icon }"
        :style="key === icon ? { background: tileBg } : null"
        :aria-pressed="key === icon"
        @click="emit('update:icon', key)"
      >
        <span v-if="!key" class="cip-initials">{{ initials }}</span>
        <ProjectIcon v-else :icon="key" :size="15" />
      </button>
      <span class="cip-spacer" />
      <n-popover v-model:show="fullOpen" trigger="click" placement="bottom-end" :show-arrow="false">
        <template #trigger>
          <button type="button" tabindex="-1" class="cip-more" :title="t('common.picker.all')">
            <n-icon :component="EllipsisHorizontalOutline" :size="16" />
          </button>
        </template>
        <IconColorPicker
          :icon="icon"
          :color="color"
          :initials="initials"
          :allow-upload="allowUpload"
          :show-mode="false"
          @update:icon="emit('update:icon', $event)"
          @update:color="emit('update:color', $event)"
        />
      </n-popover>
    </div>
  </div>
</template>

<style scoped>
.cip {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 10px;
  border-radius: 10px;
  background: var(--t-input-bg);
  outline: 0;
}
.cip-colors {
  display: flex;
  gap: 10px;
  padding: 0 2px;
}
.cip-swatch {
  width: 20px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  transition: transform 0.12s ease;
}
.cip-swatch:not(.on):hover {
  transform: scale(1.12);
}
.cip-swatch.on {
  box-shadow:
    0 0 0 2px var(--t-input-bg),
    0 0 0 4px var(--sw);
}
.cip:focus-visible .cip-swatch.on {
  box-shadow: none;
  outline: 2px solid var(--t-primary);
  outline-offset: 3px;
}
.cip-icons {
  display: flex;
  align-items: center;
  gap: 4px;
}
.cip-icon,
.cip-more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
}
.cip-icon:not(.on):hover,
.cip-more:hover {
  background: var(--t-hover);
}
.cip-icon.on {
  color: #fff;
}
.cip-more {
  color: var(--t-text3);
}
.cip-initials {
  font-size: 11.5px;
  font-weight: 700;
}
.cip-spacer {
  flex: 1;
}
</style>
