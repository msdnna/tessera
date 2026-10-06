<script setup>
// One tile of the rail (v2, #2955): a workspace (34px face in a 44px well) or,
// on desktop, the smaller head of a server group. The face is the same-hue
// accent gradient as project badges; `icon` mode tints the glyph instead.
import { computed } from 'vue'
import { NTooltip } from 'naive-ui'
import ProjectIcon from '@/components/ProjectIcon.vue'
import { hueGrad } from '@/utils/gradient'

const props = defineProps({
  label: { type: String, default: '?' }, // initials
  color: { type: String, default: '' },
  icon: { type: String, default: '' },
  iconMode: { type: String, default: 'badge' },
  active: { type: Boolean, default: false },
  muted: { type: Boolean, default: false },
  small: { type: Boolean, default: false },
  badge: { type: Number, default: 0 },
  dot: { type: Boolean, default: false },
})
defineEmits(['pick', 'menu'])
// Attributes (data-ws, …) belong on the button, not on the tooltip wrapper.
defineOptions({ inheritAttrs: false })

const bare = computed(() => props.iconMode === 'icon')
const faceStyle = computed(() =>
  bare.value ? { color: props.color } : { backgroundImage: hueGrad(props.color) },
)
</script>

<template>
  <n-tooltip placement="right" :delay="300">
    <template #trigger>
      <button
        v-bind="$attrs"
        type="button"
        class="rt"
        :class="{ active, muted, small }"
        :aria-current="active ? 'true' : undefined"
        @click="$emit('pick', $event)"
        @contextmenu.prevent="$emit('menu', $event)"
      >
        <span class="rt-face" :class="{ bare }" :style="faceStyle">
          <ProjectIcon
            v-if="icon"
            :icon="icon"
            :initials="label"
            :size="small ? 14 : 18"
            :color="bare ? color : ''"
          />
          <template v-else>{{ label }}</template>
        </span>
        <span v-if="badge > 0" class="rt-badge">{{ badge > 99 ? '99+' : badge }}</span>
        <span v-else-if="dot" class="rt-dot" />
        <slot />
      </button>
    </template>
    <slot name="tip" />
  </n-tooltip>
</template>

<style scoped>
.rt {
  position: relative;
  flex: none;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: 14px;
  background: transparent;
  cursor: pointer;
  transition: background 0.12s ease;
}
.rt:hover,
.rt.active {
  background: var(--t-hover);
}
.rt:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.rt-face {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.01em;
  overflow: hidden;
}
.rt-face.bare {
  background: var(--t-ctl);
}
.rt.small {
  width: 36px;
  height: 28px;
  border-radius: 9px;
}
.rt.small .rt-face {
  width: 24px;
  height: 24px;
  border-radius: 8px;
  font-size: 10px;
}
.rt.muted .rt-face {
  opacity: 0.45;
}
.rt-badge {
  position: absolute;
  top: 1px;
  right: 1px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  box-sizing: border-box;
  border-radius: 8px;
  background: #d03050;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
}
.rt.small .rt-badge {
  top: -4px;
  right: -2px;
}
.rt-dot {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  box-shadow: 0 0 0 2px var(--t-rail);
}
</style>
