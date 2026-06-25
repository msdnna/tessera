<script setup>
import { ref, computed } from 'vue'
import { NPopover, NIcon } from 'naive-ui'
import { FolderOutline } from '@vicons/ionicons5'
import SidebarFlyout from './SidebarFlyout.vue'
import ProjectIcon from './ProjectIcon.vue'
import { hueGrad } from '@/utils/gradient'

// One icon in the collapsed rail (a root group or project). Hovering opens a
// flyout with the navigable subtree (budget-style).
const props = defineProps({
  node: { type: Object, required: true },
  kind: { type: String, required: true }, // 'group' | 'project'
})

const show = ref(false)
const initials = computed(() => (props.node.name || '?').trim().slice(0, 2).toUpperCase())

// Icon colouring mode — mirror ProjectRow / SidebarNode (badge box vs glyph).
const iconMode = computed(() => props.node.icon_mode === 'icon')
const colored = computed(() => props.node.color && props.node.color !== 'transparent')
const glyphColor = computed(() =>
  iconMode.value ? (colored.value ? props.node.color : 'var(--t-primary)') : '',
)
// Project: no colour ⇒ primary badge. Group: no colour ⇒ neutral.
const projBox = computed(() =>
  iconMode.value || props.node.color === 'transparent'
    ? { background: 'transparent' }
    : { background: hueGrad(props.node.color || 'var(--t-primary)') },
)
const projBare = computed(() => iconMode.value || props.node.color === 'transparent')
const grpBox = computed(() =>
  !iconMode.value && colored.value
    ? { background: hueGrad(props.node.color) }
    : { background: 'transparent' },
)
const grpBare = computed(() => iconMode.value || !colored.value)
</script>

<template>
  <n-popover
    v-model:show="show"
    trigger="hover"
    placement="right-start"
    :show-arrow="false"
    :delay="120"
    raw
  >
    <template #trigger>
      <button class="rail-item" :title="node.name">
        <span
          v-if="kind === 'project'"
          class="picon"
          :class="{ 'picon-bare': projBare }"
          :style="projBox"
        >
          <ProjectIcon :icon="node.icon" :initials="initials" :size="15" :color="glyphColor" />
        </span>
        <span v-else class="gicon" :class="{ 'gicon-bare': grpBare }" :style="grpBox">
          <ProjectIcon
            v-if="node.icon"
            :icon="node.icon"
            :initials="initials"
            :size="15"
            :color="glyphColor"
          />
          <n-icon v-else :component="FolderOutline" :size="18" :color="glyphColor || undefined" />
        </span>
      </button>
    </template>
    <div class="rail-pop">
      <SidebarFlyout :node="node" :kind="kind" @navigate="show = false" />
    </div>
  </n-popover>
</template>

<style scoped>
.rail-item {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 36px;
  border: none;
  background: none;
  cursor: pointer;
  border-radius: 8px;
  padding: 0;
}
.rail-item:hover {
  background: var(--t-hover);
}
.picon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 6px;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  flex: none;
}
.picon-bare {
  color: var(--t-text1);
}
.gicon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 6px;
  color: #fff;
  flex: none;
}
/* No coloured square — plain folder/glyph sits on the rail. */
.gicon-bare {
  color: var(--t-text2);
}
.rail-pop {
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 10px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.18);
  padding: 6px;
  max-height: 70vh;
  overflow-y: auto;
}
</style>
