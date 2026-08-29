<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NPopover } from 'naive-ui'
import { TabletLandscapeOutline, TabletPortraitOutline } from '@vicons/ionicons5'
import TesseraIcon from '@/components/TesseraIcon.vue'
import { toolbarGroups, lineHeightLabel } from '@/utils/docToolbar'
import { fontFamilies, FONT_SIZES } from '@/utils/docSchema'
import { LINE_HEIGHTS } from '@/utils/docExtensions/blockStyle'
import {
  MARGIN_PRESETS,
  PAGE_SIZES,
  isLandscape,
  marginKey,
  normalizePage,
  sizeKey,
  withMargins,
  withOrientation,
  withSize,
} from '@/utils/docPage'
import { sectionAt } from '@/utils/docExtensions/sectionBreak'

const { t } = useI18n()

const props = defineProps({
  editor: { type: Object, default: null },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['pick-image', 'set-link'])

// Vue knows nothing about ProseMirror transactions, so every accessor below —
// isActive, disabled, the current font — would freeze at whatever it read on the
// last render. Clicks happen to re-render, which is why this went unnoticed
// while the panel had no undo button; undo's enabled state changes from typing,
// not from clicking, and would have stayed wrong until the next click.
const tick = ref(0)
let detach = null
watch(
  () => props.editor,
  (editor) => {
    detach?.()
    detach = null
    if (!editor) return
    const bump = () => {
      tick.value += 1
    }
    editor.on('transaction', bump)
    editor.on('selectionUpdate', bump)
    detach = () => {
      editor.off('transaction', bump)
      editor.off('selectionUpdate', bump)
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => detach?.())

// The editor is created asynchronously by the parent, so every accessor has to
// tolerate a null instance for the first paint.
const groups = computed(() => {
  void tick.value
  if (!props.editor) return []
  return toolbarGroups(props.editor, {
    onSetLink: () => emit('set-link'),
    onPickImage: () => emit('pick-image'),
  })
})

const fontFamily = computed(() => {
  void tick.value
  return props.editor?.getAttributes('textStyle')?.fontFamily || ''
})
const fontSize = computed(() => {
  void tick.value
  return props.editor?.getAttributes('textStyle')?.fontSize || null
})
const lineHeight = computed(() => {
  void tick.value
  const e = props.editor
  if (!e) return null
  return e.getAttributes('paragraph')?.lineHeight || e.getAttributes('heading')?.lineHeight || null
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

// Font, size and spacing keep open-ended value sets, so they stay pickers rather
// than becoming rows of buttons like alignment did. Each trigger carries its
// own glyph: "14" and "1,5" sitting side by side as bare numbers read as one
// undifferentiated pair, which is exactly the complaint this rework answers —
// the letter and the line marks say which is which before the number is read.
const pickers = computed(() => {
  const fonts = fontFamilies()
  const fallback = t('documents.toolbar.picker.defaultValue')
  return [
    {
      key: 'font',
      title: t('documents.toolbar.picker.font', {
        value: fonts.find((f) => f.value === fontFamily.value)?.label || fallback,
      }),
      // The glyph is a sample of the alphabet the reader writes in, so it is a
      // translated string ("Аа" / "Aa"), not a fixed pair of letters.
      glyph: t('documents.toolbar.picker.fontGlyph'),
      glyphCls: 'font',
      style: fontFamily.value ? { fontFamily: fontFamily.value } : {},
      value: fontFamily.value,
      options: fonts.map((f) => ({
        label: f.label,
        value: f.value,
        style: f.value ? { fontFamily: f.value } : {},
      })),
      apply: setFontFamily,
    },
    {
      key: 'size',
      title: t('documents.toolbar.picker.size', {
        value: fontSize.value ? fontSize.value.replace('px', '') : fallback,
      }),
      glyph: 'A',
      glyphCls: 'size',
      style: {},
      valueText: fontSize.value ? fontSize.value.replace('px', '') : '',
      value: fontSize.value,
      options: [
        { label: t('documents.toolbar.picker.default'), value: null },
        ...FONT_SIZES.map((v) => ({ label: v.replace('px', ''), value: v })),
      ],
      apply: setFontSize,
    },
    {
      key: 'line',
      title: t('documents.toolbar.picker.lineHeight', {
        value: lineHeight.value ? lineHeightLabel(lineHeight.value) : fallback,
      }),
      vicon: 'line-height',
      style: {},
      valueText: lineHeight.value ? lineHeightLabel(lineHeight.value) : '',
      value: lineHeight.value,
      options: [
        { label: t('documents.toolbar.picker.default'), value: null },
        ...LINE_HEIGHTS.map((v) => ({ label: lineHeightLabel(v), value: v })),
      ],
      apply: setLineHeight,
    },
  ]
})

// Page setup (#2821): sheet size, orientation and margins, stored on the doc
// node rather than as an export option, because the sheet in the editor is laid
// out from it — an imported landscape document has to *look* landscape, or the
// table it was laid out for still hangs off the page.
//
// A popover rather than three more toolbar controls: these are per-document
// settings changed once and then left alone, and putting them in the row beside
// bold and italic would give the least-used controls the same weight as the
// most-used ones.
//
// Since #2827 the panel edits the geometry of the section the caret is in
// rather than the document's. For a document without a section break the two
// are the same thing (sectionAt returns the doc attribute as section 0), so
// nothing changes for it; for one with breaks this is what makes the panel
// usable at all — a second popover hanging off the break would be the same four
// controls twice, and the caret already says which section is meant.
const page = computed(() => {
  void tick.value
  const state = props.editor?.state
  return state ? sectionAt(state).page : normalizePage(null)
})
const landscape = computed(() => isLandscape(page.value))

const pageTitle = computed(() =>
  t('documents.toolbar.page.title', {
    size: t(`documents.toolbar.page.sizes.${sizeKey(page.value) || 'custom'}`),
    orientation: t(
      `documents.toolbar.page.${landscape.value ? 'landscape' : 'portrait'}`,
    ).toLowerCase(),
  }),
)

// Millimetres, trimmed of the trailing ".0" an imported page picks up from the
// twips it was converted out of — "209.9 × 297" is information, "210.0" is not.
const mm = (v) => String(Math.round(v * 10) / 10)
const pageDims = computed(() => {
  const p = page.value
  return t('documents.toolbar.page.dims', {
    w: mm(p.w),
    h: mm(p.h),
    margins: [p.mt, p.mr, p.mb, p.ml].map(mm).join(' / '),
  })
})

const sizeOptions = computed(() =>
  PAGE_SIZES.map((s) => ({
    key: s.key,
    label: t(`documents.toolbar.page.sizes.${s.key}`),
    on: sizeKey(page.value) === s.key,
    apply: () => applyPage(withSize(page.value, s)),
  })),
)
const marginOptions = computed(() =>
  MARGIN_PRESETS.map((m) => ({
    key: m.key,
    label: t(`documents.toolbar.page.margins.${m.key}`),
    on: marginKey(page.value) === m.key,
    apply: () => applyPage(withMargins(page.value, m)),
  })),
)

function applyPage(next) {
  props.editor?.chain().focus().setSectionPage(next).run()
}
function setOrientation(v) {
  applyPage(withOrientation(page.value, v))
}
// Closes the panel: the break lands below the caret, and a popover left open
// over it would hide the thing the click just produced.
function insertBreak() {
  openPicker.value = ''
  props.editor?.chain().focus().insertSectionBreak().run()
}

// One popover open at a time, and it closes on pick: naive's own click trigger
// would leave the list hanging open over the text after a value is chosen.
const openPicker = ref('')
function togglePicker(key) {
  openPicker.value = openPicker.value === key ? '' : key
}
function pick(picker, value) {
  openPicker.value = ''
  picker.apply(value)
}
</script>

<template>
  <div class="doc-toolbar" :class="{ disabled }">
    <!-- One row, no tabs: the second tab existed because four selects ate the
         width, and with alignment down to icons and the pickers down to glyphs
         everything fits. The separators are what group the tools now. -->
    <div class="row">
      <template v-for="(g, gi) in groups" :key="g.key">
        <span v-if="gi" class="sep" />
        <template v-if="g.kind === 'selects'">
          <n-popover
            v-for="p in pickers"
            :key="p.key"
            :show="openPicker === p.key"
            trigger="manual"
            placement="bottom-start"
            :show-arrow="false"
            @clickoutside="openPicker = ''"
          >
            <template #trigger>
              <button
                type="button"
                class="doc-tbtn value"
                :class="{ on: !!p.value }"
                :title="p.title"
                @click="togglePicker(p.key)"
              >
                <tessera-icon v-if="p.vicon" :name="p.vicon" :size="15" />
                <span v-if="p.glyph" class="glyph" :class="p.glyphCls" :style="p.style">{{
                  p.glyph
                }}</span>
                <span v-if="p.valueText" class="num">{{ p.valueText }}</span>
              </button>
            </template>
            <div class="opts">
              <button
                v-for="o in p.options"
                :key="String(o.value)"
                type="button"
                class="opt"
                :class="{ on: o.value === p.value }"
                :style="o.style"
                @click="pick(p, o.value)"
              >
                {{ o.label }}
              </button>
            </div>
          </n-popover>
        </template>
        <!-- mousedown.prevent keeps the selection inside ProseMirror: without it
             the button steals focus and the command applies to an empty range. -->
        <button
          v-for="cmd in g.items"
          :key="cmd.key"
          type="button"
          class="doc-tbtn"
          :class="[cmd.cls, { on: cmd.isActive() }]"
          :title="cmd.title"
          :data-tbtn="cmd.key"
          :disabled="cmd.disabled ? cmd.disabled() : false"
          @mousedown.prevent="cmd.run()"
        >
          <n-icon v-if="cmd.icon" :component="cmd.icon" :size="15" />
          <tessera-icon v-else-if="cmd.vicon" :name="cmd.vicon" :size="15" />
          <template v-else>{{ cmd.text }}</template>
        </button>
      </template>
      <!-- Page setup sits last and behind its own separator: it changes the
           sheet rather than the selection, so it does not belong in any of the
           command groups above. -->
      <span class="sep" />
      <n-popover
        :show="openPicker === 'page'"
        trigger="manual"
        placement="bottom-end"
        :show-arrow="false"
        @clickoutside="openPicker = ''"
      >
        <template #trigger>
          <button
            type="button"
            class="doc-tbtn"
            :title="pageTitle"
            data-tbtn="page"
            data-testid="doc-page-setup"
            @click="togglePicker('page')"
          >
            <n-icon
              :component="landscape ? TabletLandscapeOutline : TabletPortraitOutline"
              :size="15"
            />
          </button>
        </template>
        <div class="page-setup" data-testid="doc-page-panel">
          <div class="ps-row">
            <span class="ps-label">{{ $t('documents.toolbar.page.size') }}</span>
            <div class="ps-opts">
              <button
                v-for="s in sizeOptions"
                :key="s.key"
                type="button"
                class="ps-chip"
                :class="{ on: s.on }"
                :data-page-size="s.key"
                @click="s.apply()"
              >
                {{ s.label }}
              </button>
            </div>
          </div>
          <div class="ps-row">
            <span class="ps-label">{{ $t('documents.toolbar.page.orientation') }}</span>
            <div class="ps-opts">
              <button
                type="button"
                class="ps-chip"
                :class="{ on: !landscape }"
                data-page-orientation="portrait"
                @click="setOrientation(false)"
              >
                {{ $t('documents.toolbar.page.portrait') }}
              </button>
              <button
                type="button"
                class="ps-chip"
                :class="{ on: landscape }"
                data-page-orientation="landscape"
                @click="setOrientation(true)"
              >
                {{ $t('documents.toolbar.page.landscape') }}
              </button>
            </div>
          </div>
          <div class="ps-row">
            <span class="ps-label">{{ $t('documents.toolbar.page.marginsLabel') }}</span>
            <div class="ps-opts">
              <button
                v-for="m in marginOptions"
                :key="m.key"
                type="button"
                class="ps-chip"
                :class="{ on: m.on }"
                :data-page-margins="m.key"
                @click="m.apply()"
              >
                {{ m.label }}
              </button>
            </div>
          </div>
          <!-- The numbers, read-only. The pickers above cover what a document
               needs; showing the millimetres is what makes an imported geometry
               that matches no preset legible instead of leaving every chip
               unlit with no explanation. -->
          <p class="ps-current" data-testid="doc-page-dims">{{ pageDims }}</p>
          <!-- The break belongs here rather than in the toolbar row: what the
               three pickers above change is *this* section, and the only way to
               get a second one is from the same panel (#2827). It is drawn as a
               full-width action instead of a chip because it inserts something
               into the document rather than setting a value. -->
          <button
            type="button"
            class="ps-action"
            data-testid="doc-section-break"
            @click="insertBreak()"
          >
            {{ $t('documents.toolbar.page.sectionBreak') }}
          </button>
          <p class="ps-hint">{{ $t('documents.toolbar.page.sectionHint') }}</p>
        </div>
      </n-popover>
    </div>
  </div>
</template>

<style scoped>
/* Deliberately the same skin as MarkdownEditor's .md2-tbtn: the two editors sit
   in the same product and a second, subtly different button style would read as
   a third-party panel dropped into Tessera. Colours come from the theme tokens
   only — no literal values, not even as var() fallbacks, because a hardcoded
   light fallback is what hid the broken token names in the first TipTap attempt
   (ed63159) until someone opened the dark theme. */
/* No rule under the toolbar: the work area below already carries its own
   background and a bordered sheet, so a line here only doubles the seam
   (задача 2727). */
.doc-toolbar.disabled {
  opacity: 0.5;
  pointer-events: none;
}
.row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
  padding: 4px 0 6px;
}
.doc-tbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
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
/* A command with nothing to do (undo in a fresh document) says so instead of
   looking live and swallowing the click. */
.doc-tbtn:disabled {
  opacity: 0.4;
  cursor: default;
  background: transparent;
  color: var(--t-text3);
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
.doc-tbtn.value {
  padding: 0 6px;
}
.glyph {
  line-height: 1;
}
.glyph.font {
  font-size: 13px;
}
/* Smaller than the number beside it: the letter is the label, not the value. */
.glyph.size {
  font-size: 11px;
}
.num {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.sep {
  width: 1px;
  align-self: stretch;
  margin: 2px 6px;
  background: var(--t-border);
}
.opts {
  display: flex;
  flex-direction: column;
  min-width: 132px;
  margin: -4px;
}
.opt {
  padding: 6px 10px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text1);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.opt:hover {
  background: var(--t-hover);
}
.opt.on {
  color: var(--t-primary);
}
/* Page setup panel. Wider than the value pickers because its options are words
   rather than numbers, and laid out in labelled rows: three unlabelled chip
   strips would leave "Узкие" and "Альбомная" looking like members of one list. */
.page-setup {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 260px;
  margin: -4px;
}
.ps-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ps-label {
  color: var(--t-text3);
  font-size: 11px;
}
.ps-opts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.ps-chip {
  padding: 4px 9px;
  border: 1px solid var(--t-border);
  border-radius: 6px;
  background: transparent;
  color: var(--t-text1);
  font-size: 12px;
  cursor: pointer;
}
.ps-chip:hover {
  background: var(--t-hover);
}
/* Same accent treatment the toolbar buttons get when active — the panel is part
   of the same control, not a separate widget with its own idea of "selected". */
.ps-chip.on {
  border-color: transparent;
  background: var(--t-primary);
  color: var(--t-on-primary);
}
.ps-current {
  margin: 0;
  color: var(--t-text3);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}
/* The section break, behind its own divider: everything above sets a value on
   the current section, this one adds another section (задача 2827). */
.ps-action {
  width: 100%;
  margin-top: 2px;
  padding: 6px 9px;
  border: 1px solid var(--t-border);
  border-top-width: 1px;
  border-radius: 6px;
  background: transparent;
  color: var(--t-text1);
  font-size: 12px;
  cursor: pointer;
}
.ps-action:hover {
  background: var(--t-hover);
}
.ps-hint {
  margin: 0;
  color: var(--t-text3);
  font-size: 11px;
}
</style>
