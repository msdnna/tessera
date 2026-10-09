<script setup>
// The editor's @ / # / / list at the caret (#3118, PackageJ «@, # и /»): the
// palette's container at 340px — sections in small caps, 34px rows, the
// keyboard row on --t-hover, and a footer «↑↓ · ↵ Tab вставить · Esc». An empty
// result says what happens to the text («останется текстом») instead of
// vanishing.
//
// Rendered in place (position: fixed at the caret), not teleported: inside the
// task modal a body-level layer would fall out of its focus trap. The field
// keeps the focus and owns the keys — this list only shows the cursor and takes
// clicks (mousedown, so the field doesn't blur).
import { computed, ref, watch, nextTick } from 'vue'
import { NIcon } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import UserAvatar from '../UserAvatar.vue'
import TesseraIcon from '../TesseraIcon.vue'
import TesseraSpinner from '../TesseraSpinner.vue'
import Kbd from '../Kbd.vue'
import { CAPS } from '@/utils/keyCaps'

const props = defineProps({
  // 'mention' | 'task' | 'command'
  kind: { type: String, required: true },
  list: { type: Array, default: () => [] },
  index: { type: Number, default: 0 },
  pos: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  // Shown in place of rows when there are none.
  empty: { type: String, default: '' },
  insertLabel: { type: String, default: '' },
})
const emit = defineEmits(['pick', 'hover'])

// The rows keep the class names of the popup they replaced, so the specs that
// drive the editor read them as before.
const ROW_CLASS = {
  mention: 'md2-mention-item',
  task: 'md2-mention-item md2-task-item',
  command: 'md2-mention-item md2-cmd-item',
}
const root = ref(null)
// Row numbering over the list's rows only — headings take no cursor.
const numbered = computed(() => {
  let n = -1
  return props.list.map((x) => (x.head ? { ...x, n: -1 } : { ...x, n: ++n }))
})
const hasRows = computed(() => numbered.value.some((x) => x.n >= 0))

// Keep the keyboard row in view inside a scrolled list.
watch(
  () => props.index,
  async () => {
    await nextTick()
    root.value?.querySelector('.es-row.active')?.scrollIntoView?.({ block: 'nearest' })
  },
)
defineExpose({ el: root })
</script>

<template>
  <div
    ref="root"
    class="es"
    :class="`es-${kind}`"
    :style="pos"
    role="listbox"
    data-testid="ed-suggest"
    @mousedown.prevent
  >
    <div class="es-scroll">
      <template v-if="hasRows">
        <template v-for="x in numbered" :key="x.head ? `h:${x.head}` : x.key">
          <div v-if="x.head" class="es-head">{{ x.head }}</div>
          <template v-else>
            <div
              class="es-row"
              :class="[ROW_CLASS[kind], { active: x.n === index, two: kind === 'task' }]"
              role="option"
              :aria-selected="x.n === index"
              data-testid="ed-suggest-row"
              @mousedown.prevent="emit('pick', x)"
              @mousemove="x.n !== index && emit('hover', x.n)"
            >
              <template v-if="kind === 'mention'">
                <UserAvatar
                  class="es-ava"
                  :user-id="x.avatarUserId"
                  :src="x.avatarSrc"
                  :name="x.name"
                />
                <span class="es-main">
                  <span class="es-name">{{ x.name }}</span>
                  <span v-if="x.login" class="es-sub">{{ x.login }}</span>
                </span>
                <span v-if="x.aside" class="es-aside">{{ x.aside }}</span>
              </template>
              <template v-else-if="kind === 'task'">
                <span class="es-line">
                  <span class="es-st" :style="x.color ? { color: x.color } : null">
                    <TesseraIcon :name="x.icon" :size="15" />
                  </span>
                  <span class="es-num">#{{ x.number }}</span>
                  <span class="es-name">{{ x.title }}</span>
                </span>
                <span v-if="x.sub" class="es-under">{{ x.sub }}</span>
              </template>
              <template v-else>
                <span class="es-cmd">{{ x.cmd }}</span>
                <span class="es-main">
                  <span class="es-name">{{ x.desc }}</span>
                </span>
                <span v-if="x.custom" class="es-pill">{{ x.customLabel }}</span>
                <span v-else-if="x.aside" class="es-aside">{{ x.aside }}</span>
              </template>
            </div>
          </template>
        </template>
      </template>
      <div v-else class="es-empty" data-testid="ed-suggest-empty">
        <TesseraSpinner v-if="loading" :size="15" />
        <n-icon v-else :component="SearchOutline" :size="15" class="es-empty-ic" />
        <span>{{ empty }}</span>
      </div>
    </div>
    <div class="es-foot">
      <span class="es-fk"
        ><Kbd>{{ CAPS.arrows }}</Kbd></span
      >
      <span class="es-fk"
        ><Kbd>{{ CAPS.enter }}</Kbd
        ><Kbd>{{ CAPS.tab }}</Kbd
        >{{ insertLabel }}</span
      >
      <span class="es-grow" />
      <Kbd>{{ CAPS.escape }}</Kbd>
    </div>
  </div>
</template>

<style scoped>
.es {
  position: fixed;
  left: 0;
  top: 0;
  z-index: 4100;
  width: 340px;
  max-width: calc(100vw - 16px);
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  padding: 6px;
  border-radius: 14px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
}
.es-scroll {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 330px;
  overflow-y: auto;
  scrollbar-width: thin;
}
.es-head {
  flex: none;
  display: flex;
  align-items: center;
  padding: 8px 10px 4px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--t-text3);
}
.es-row {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  height: 34px;
  padding: 0 8px 0 10px;
  border-radius: 7px;
  font-size: 13.5px;
  cursor: pointer;
}
.es-row.two {
  flex-direction: column;
  align-items: stretch;
  justify-content: center;
  gap: 2px;
  height: 52px;
}
.es-row.active {
  background: var(--t-hover);
}
.es-ava {
  flex: none;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
  font-size: 9px;
  font-weight: 600;
}
.es-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 8px;
  white-space: nowrap;
  overflow: hidden;
}
.es-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.es-sub,
.es-aside {
  font-size: 12px;
  color: var(--t-text3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.es-aside {
  flex: none;
}
.es-line {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.es-st {
  flex: none;
  display: flex;
  color: var(--t-text3);
}
.es-num {
  flex: none;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  color: var(--t-text3);
}
.es-under {
  padding-left: 25px;
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.es-cmd {
  flex: none;
  min-width: 72px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--t-primary);
}
.es-pill {
  flex: none;
  display: flex;
  align-items: center;
  height: 18px;
  padding: 0 7px;
  border-radius: 999px;
  background: var(--t-pill);
  font-size: 11px;
  color: var(--t-text2);
}
.es-empty {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 10px;
  font-size: 13px;
  line-height: 1.45;
  color: var(--t-text2);
}
.es-empty-ic {
  flex: none;
  color: var(--t-text3);
}
.es-foot {
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  height: 34px;
  margin: 4px -6px -6px;
  padding: 0 12px;
  border-radius: 0 0 14px 14px;
  background: color-mix(in srgb, var(--t-pop) 55%, var(--t-bg));
  font-size: 11.5px;
  color: var(--t-text3);
  white-space: nowrap;
}
.es-fk {
  display: flex;
  align-items: center;
  gap: 5px;
}
.es-grow {
  flex: 1;
}
@media (hover: none) {
  .es-foot {
    display: none;
  }
}
</style>
