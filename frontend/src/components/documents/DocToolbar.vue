<script setup>
import { computed } from 'vue'
import { NIcon, NSelect, NTabPane, NTabs } from 'naive-ui'
import { homeTools, insertTools, ALIGNMENTS } from '@/utils/docToolbar'
import { FONT_FAMILIES, FONT_SIZES } from '@/utils/docSchema'
import { LINE_HEIGHTS } from '@/utils/docExtensions/blockStyle'

const props = defineProps({
  editor: { type: Object, default: null },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['pick-image', 'set-link'])

// The editor is created asynchronously by the parent, so every accessor has to
// tolerate a null instance for the first paint.
const home = computed(() =>
  props.editor ? homeTools(props.editor, { onSetLink: () => emit('set-link') }) : [],
)
const insert = computed(() =>
  props.editor ? insertTools(props.editor, { onPickImage: () => emit('pick-image') }) : [],
)

const sizeOptions = FONT_SIZES.map((v) => ({ label: v.replace('px', ''), value: v }))
const lineOptions = LINE_HEIGHTS.map((v) => ({ label: v, value: v }))

const fontFamily = computed(() => props.editor?.getAttributes('textStyle')?.fontFamily || '')
const fontSize = computed(() => props.editor?.getAttributes('textStyle')?.fontSize || null)
const lineHeight = computed(() => {
  const e = props.editor
  if (!e) return null
  const attrs = e.getAttributes('paragraph')?.lineHeight || e.getAttributes('heading')?.lineHeight
  return attrs || null
})
const alignment = computed(() => {
  const e = props.editor
  if (!e) return null
  return ALIGNMENTS.map((a) => a.value).find((v) => e.isActive({ textAlign: v })) || null
})

function setFontFamily(v) {
  const chain = props.editor.chain().focus()
  if (v) chain.setFontFamily(v).run()
  else chain.unsetFontFamily().run()
}
function setFontSize(v) {
  const chain = props.editor.chain().focus()
  if (v) chain.setFontSize(v).run()
  else chain.unsetFontSize().run()
}
function setLineHeight(v) {
  props.editor
    .chain()
    .focus()
    .setLineHeight(v || null)
    .run()
}
function setAlign(v) {
  props.editor
    .chain()
    .focus()
    .setTextAlign(v || 'left')
    .run()
}
</script>

<template>
  <div class="doc-toolbar" :class="{ disabled }">
    <n-tabs type="line" size="small" animated>
      <n-tab-pane name="home" tab="Главная">
        <div class="row">
          <!-- mousedown.prevent keeps the selection inside ProseMirror: without it
               the button steals focus and the command applies to an empty range. -->
          <button
            v-for="t in home"
            :key="t.key"
            type="button"
            class="doc-tbtn"
            :class="[t.cls, { on: t.isActive() }]"
            :title="t.title"
            @mousedown.prevent="t.run()"
          >
            <n-icon v-if="t.icon" :component="t.icon" :size="15" />
            <template v-else>{{ t.text }}</template>
          </button>
          <span class="sep" />
          <n-select
            class="sel font"
            size="small"
            placeholder="Шрифт"
            :options="FONT_FAMILIES"
            :value="fontFamily"
            @update:value="setFontFamily"
          />
          <n-select
            class="sel num"
            size="small"
            placeholder="Кегль"
            clearable
            :options="sizeOptions"
            :value="fontSize"
            @update:value="setFontSize"
          />
          <n-select
            class="sel num"
            size="small"
            placeholder="Интервал"
            clearable
            :options="lineOptions"
            :value="lineHeight"
            @update:value="setLineHeight"
          />
          <n-select
            class="sel align"
            size="small"
            placeholder="Выравнивание"
            clearable
            :options="ALIGNMENTS"
            :value="alignment"
            @update:value="setAlign"
          />
        </div>
      </n-tab-pane>
      <n-tab-pane name="insert" tab="Вставка">
        <div class="row">
          <button
            v-for="t in insert"
            :key="t.key"
            type="button"
            class="doc-tbtn"
            :class="{ on: t.isActive() }"
            :title="t.title"
            @mousedown.prevent="t.run()"
          >
            <n-icon v-if="t.icon" :component="t.icon" :size="15" />
            <template v-else>{{ t.text }}</template>
          </button>
        </div>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
/* Deliberately the same skin as MarkdownEditor's .md2-tbtn: the two editors sit
   in the same product and a second, subtly different button style would read as
   a third-party panel dropped into Tessera. Colours come from the theme tokens
   only — no literal values, not even as var() fallbacks, because a hardcoded
   light fallback is what hid the broken token names in the first TipTap attempt
   (ed63159) until someone opened the dark theme. */
.doc-toolbar {
  border-bottom: 1px solid var(--t-border);
}
.doc-toolbar.disabled {
  opacity: 0.5;
  pointer-events: none;
}
.row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
  padding-bottom: 6px;
}
.doc-tbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 26px;
  padding: 0 5px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text2);
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
  transition:
    background 0.12s ease,
    color 0.12s ease;
}
.doc-tbtn:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
/* Active state uses the live accent token — the user changes it at runtime in
   the theme settings, so it cannot be baked in. */
.doc-tbtn.on {
  background: var(--t-primary);
  color: var(--t-on-primary);
}
.doc-tbtn.b {
  font-weight: 700;
}
.doc-tbtn.i {
  font-style: italic;
}
.doc-tbtn.u {
  text-decoration: underline;
}
.doc-tbtn.s {
  text-decoration: line-through;
}
.sep {
  width: 1px;
  align-self: stretch;
  margin: 2px 6px;
  background: var(--t-border);
}
.sel {
  margin-left: 4px;
}
.sel.font {
  width: 150px;
}
.sel.num {
  width: 104px;
}
.sel.align {
  width: 150px;
}
</style>
