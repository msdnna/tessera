<script setup>
// The template gallery (#2734; v2 #3106, mockup TemplatesWin). Presentational:
// it renders the cards and reports what was picked, the view owns the API
// calls. Saved templates and the built-in starters arrive as one list; a card
// only needs to know whether it can be deleted (built-ins cannot).
//
// v2: a tier-2 window — search in the head (/), a sunken column of sources on
// the left, cards with a tinted preview, «Пустой документ» first. A click
// selects, a double click or ↵ creates; arrows move the selection. Deleting a
// saved template is a two-step button on its card. Template categories from the
// mockup are not in the data yet — a stub (#3045).
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NModal, NSpin, NTooltip } from 'naive-ui'
import {
  AddOutline,
  ArrowForwardOutline,
  CloudUploadOutline,
  DocumentOutline,
  DocumentTextOutline,
  SearchOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import Kbd from '@/components/Kbd.vue'
import { CAPS } from '@/utils/keyCaps'
import { IMPORT_EXTENSIONS } from '@/utils/docImport'
import { stubTask } from '@/utils/featureStubs'

const props = defineProps({
  show: { type: Boolean, default: false },
  templates: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  // Set while a document is being created from a template, so the gallery can
  // disable itself instead of letting an impatient second click create a second
  // document.
  busy: { type: String, default: '' },
  error: { type: String, default: '' },
  // Where the new document lands, for the head: «Новый документ в «…»».
  target: { type: String, default: '' },
})
const emit = defineEmits(['update:show', 'use', 'blank', 'upload', 'remove'])
const { t } = useI18n()
const categoriesTask = stubTask('documents.templateCategories')

const fileInput = ref(null)
const query = ref('')
const source = ref('all')
const searchEl = ref(null)
const gridEl = ref(null)

const counts = computed(() => ({
  all: props.templates.length,
  builtin: props.templates.filter((x) => x.builtin).length,
  mine: props.templates.filter((x) => !x.builtin).length,
}))
const SOURCES = ['all', 'builtin', 'mine']

const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return props.templates.filter((tpl) => {
    if (source.value === 'builtin' && !tpl.builtin) return false
    if (source.value === 'mine' && tpl.builtin) return false
    if (!q) return true
    return `${tpl.title} ${tpl.description || ''}`.toLowerCase().includes(q)
  })
})
// The blank document is offered while nothing narrows the list.
const withBlank = computed(() => !query.value.trim() && source.value === 'all')
// Selection: an index into [blank?, ...shown].
const cards = computed(() => [...(withBlank.value ? [{ id: '__blank' }] : []), ...shown.value])
const sel = ref(0)
watch(cards, () => {
  if (sel.value >= cards.value.length) sel.value = Math.max(0, cards.value.length - 1)
})
watch(
  () => props.show,
  async (on) => {
    if (!on) return
    query.value = ''
    source.value = 'all'
    sel.value = 0
    armed.value = ''
    await nextTick()
    gridEl.value?.focus({ preventScroll: true })
  },
)

// Same-hue preview tint per card, so the grid reads as a set of different
// kinds of document; saved templates take the accent.
const TONES = { meeting: '#f0a020', spec: '#7c5cff', retro: '#3aa675' }
function toneOf(tpl) {
  if (tpl.id === '__blank') return ''
  return (tpl.builtin && TONES[tpl.key]) || 'var(--t-primary)'
}
function tileStyle(tpl) {
  const c = toneOf(tpl)
  return c ? { '--tpl-tone': c } : {}
}

function pick(i) {
  sel.value = i
}
function use(card) {
  if (!card || props.busy) return
  if (card.id === '__blank') emit('blank')
  else emit('use', card)
}

// Two-step delete on a saved template's card.
const armed = ref('')
let disarmTimer = null
function onTrash(tpl) {
  if (armed.value !== tpl.id) {
    armed.value = tpl.id
    clearTimeout(disarmTimer)
    disarmTimer = setTimeout(() => (armed.value = ''), 4000)
    return
  }
  armed.value = ''
  emit('remove', tpl)
}

function columns() {
  const tpl = gridEl.value && getComputedStyle(gridEl.value).gridTemplateColumns
  return tpl ? tpl.split(' ').filter(Boolean).length : 3
}
function onGridKey(e) {
  if (e.target instanceof HTMLInputElement) return
  const n = cards.value.length
  if (!n) return
  const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: columns(), ArrowUp: -columns() }[e.key]
  if (step) {
    e.preventDefault()
    sel.value = Math.min(n - 1, Math.max(0, sel.value + step))
    gridEl.value
      ?.querySelectorAll('[data-testid="tpl-tile"], .tpl-blank')
      [sel.value]?.scrollIntoView?.({ block: 'nearest' })
  } else if (e.key === 'Enter') {
    e.preventDefault()
    use(cards.value[sel.value])
  } else if (e.key === '/') {
    e.preventDefault()
    searchEl.value?.focus()
  }
}
function onSearchKey(e) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    gridEl.value?.focus()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    use(cards.value[sel.value])
  }
}

function pickFile() {
  fileInput.value?.click()
}
function onFile(e) {
  const file = e.target.files?.[0]
  // Reset first: picking the same file twice in a row fires no change event
  // otherwise, and a failed import is exactly when someone retries the same file.
  e.target.value = ''
  if (file) emit('upload', file)
}
</script>

<template>
  <n-modal
    :show="show"
    transform-origin="center"
    :auto-focus="false"
    @update:show="(v) => emit('update:show', v)"
  >
    <div
      class="tpl-win"
      role="dialog"
      aria-modal="true"
      :aria-label="t('documents.templates.title')"
    >
      <header class="tpl-head">
        <span class="tpl-title">{{ t('documents.templates.newTitle') }}</span>
        <span v-if="target" class="tpl-target">{{
          t('documents.templates.into', { target })
        }}</span>
        <span class="tpl-grow" />
        <label class="tpl-search">
          <n-icon :component="SearchOutline" :size="14" class="tpl-search-ic" />
          <input
            ref="searchEl"
            v-model="query"
            class="tpl-search-in"
            :placeholder="t('documents.templates.search')"
            @keydown="onSearchKey"
          />
          <Kbd v-if="!query" class="tpl-kbd">/</Kbd>
        </label>
        <Kbd class="tpl-kbd">{{ CAPS.escape }}</Kbd>
      </header>

      <div class="tpl-body">
        <nav class="tpl-nav">
          <button
            v-for="s in SOURCES"
            :key="s"
            type="button"
            class="tpl-src"
            :class="{ on: source === s }"
            :data-testid="`tpl-source-${s}`"
            @click="source = s"
          >
            <span class="tpl-src-l">{{ t(`documents.templates.source.${s}`) }}</span>
            <span class="tpl-src-n">{{ counts[s] }}</span>
          </button>
          <n-tooltip :delay="300" placement="right">
            <template #trigger>
              <span class="tpl-src stub" aria-disabled="true">
                <span class="tpl-src-l">{{ t('documents.templates.categories') }}</span>
                <span class="tpl-soon">{{ t('common.stub.badge') }}</span>
              </span>
            </template>
            {{ t('common.stub.soon', { n: categoriesTask }) }}
          </n-tooltip>
        </nav>

        <div class="tpl-main">
          <div v-if="error" class="tpl-error" role="alert">{{ error }}</div>
          <n-spin v-if="loading" size="small" />
          <div
            v-else
            ref="gridEl"
            class="tpl-grid"
            tabindex="0"
            :aria-label="t('documents.templates.title')"
            @keydown="onGridKey"
          >
            <button
              v-if="withBlank"
              type="button"
              class="tpl-tile tpl-blank"
              :class="{ on: sel === 0 }"
              tabindex="-1"
              data-testid="tpl-blank"
              @click="pick(0)"
              @dblclick="use(cards[0])"
            >
              <span class="tpl-prev blank">
                <n-icon :component="DocumentOutline" :size="18" class="tpl-prev-ic" />
                <span class="tpl-line" />
                <span class="tpl-line short" />
              </span>
              <span class="tpl-name">{{ t('documents.templates.blank') }}</span>
              <span class="tpl-desc">{{ t('documents.templates.blankHint') }}</span>
            </button>
            <!-- `tpl`, not `t`: the loop variable would shadow the translation
                 function and every t() inside the tile would call a template. -->
            <div
              v-for="(tpl, i) in shown"
              :key="tpl.id"
              class="tpl-tile"
              :class="{ on: sel === i + (withBlank ? 1 : 0), builtin: tpl.builtin }"
              :style="tileStyle(tpl)"
              data-testid="tpl-tile"
              :data-tpl="tpl.id"
              @click="pick(i + (withBlank ? 1 : 0))"
              @dblclick="use(tpl)"
            >
              <span class="tpl-prev">
                <span v-if="tpl.icon" class="tpl-emoji">{{ tpl.icon }}</span>
                <n-icon v-else :component="DocumentTextOutline" :size="18" class="tpl-prev-ic" />
                <span class="tpl-line" />
                <span class="tpl-line" />
                <span class="tpl-line short" />
              </span>
              <span class="tpl-name">{{ tpl.title }}</span>
              <span class="tpl-desc">{{
                tpl.description || tpl.preview || t('documents.templates.noDescription')
              }}</span>
              <span class="tpl-foot">
                <span class="tpl-origin">{{
                  tpl.builtin ? t('documents.templates.builtin') : tpl.author_name || ''
                }}</span>
                <button
                  v-if="!tpl.builtin"
                  type="button"
                  class="tpl-act trash"
                  :class="{ armed: armed === tpl.id }"
                  :title="t('documents.templates.removeTitle')"
                  :aria-label="
                    armed === tpl.id
                      ? t('documents.templates.removeConfirm')
                      : t('documents.templates.removeTitle')
                  "
                  @click.stop="onTrash(tpl)"
                >
                  <n-icon :component="TrashOutline" :size="14" />
                  <span v-if="armed === tpl.id">{{ t('documents.templates.removeArmed') }}</span>
                </button>
                <button
                  type="button"
                  class="tpl-act use"
                  :class="{ disabled: !!busy }"
                  :disabled="!!busy"
                  :title="t('documents.templates.use')"
                  data-testid="tpl-use"
                  @click.stop="use(tpl)"
                >
                  <n-icon :component="ArrowForwardOutline" :size="14" />
                </button>
              </span>
            </div>
            <p v-if="!shown.length" class="tpl-empty">
              {{ t('documents.templates.empty', { formats: IMPORT_EXTENSIONS.join(', ') }) }}
            </p>
          </div>
        </div>
      </div>

      <footer class="tpl-foot-bar">
        <span class="tpl-hint">
          <Kbd>{{ CAPS.arrows }}</Kbd>
          {{ t('documents.templates.hintPick') }}
        </span>
        <button type="button" class="tpl-btn" data-testid="tpl-upload" @click="pickFile">
          <n-icon :component="CloudUploadOutline" :size="15" />
          {{ t('documents.templates.upload') }}
        </button>
        <span class="tpl-grow" />
        <button
          type="button"
          class="tpl-btn primary"
          :disabled="!cards.length || !!busy"
          data-testid="tpl-create"
          @click="use(cards[sel])"
        >
          <n-icon v-if="cards[sel]?.id === '__blank'" :component="AddOutline" :size="15" />
          {{ t('documents.templates.create') }}
          <Kbd class="tpl-cap">{{ CAPS.enter }}</Kbd>
        </button>
      </footer>

      <input
        ref="fileInput"
        type="file"
        class="tpl-file"
        :accept="IMPORT_EXTENSIONS.join(',')"
        @change="onFile"
      />
    </div>
  </n-modal>
</template>

<style scoped>
/* Tier 2: 960×620, radius 18, the pop level (mockup TemplatesWin). */
.tpl-win {
  display: flex;
  flex-direction: column;
  width: min(960px, 96vw);
  height: min(620px, 92vh);
  border-radius: var(--t-r-col);
  background: var(--t-pop);
  box-shadow: var(--t-sh-drag);
  color: var(--t-text1);
  overflow: hidden;
}
.tpl-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 16px 12px 20px;
  min-width: 0;
}
.tpl-title {
  font-size: 16px;
  font-weight: 700;
  white-space: nowrap;
}
.tpl-target {
  font-size: 13px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.tpl-grow {
  flex: 1;
}
.tpl-search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 280px;
  height: 34px;
  padding: 0 8px 0 12px;
  box-sizing: border-box;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
}
.tpl-search:focus-within {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.tpl-search-ic {
  color: var(--t-text3);
}
.tpl-search-in {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
}
.tpl-search-in::placeholder {
  color: var(--t-placeholder);
}
.tpl-body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 216px minmax(0, 1fr);
  gap: 20px;
  padding: 0 12px 12px;
}
.tpl-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px;
  border-radius: 14px;
  background: var(--t-sub);
}
.tpl-src {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 10px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}
.tpl-src:hover {
  background: var(--t-hover);
}
.tpl-src:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--t-primary);
}
.tpl-src.on {
  background: var(--t-active);
  font-weight: 650;
}
.tpl-src.stub {
  opacity: 0.55;
  cursor: default;
}
.tpl-src.stub:hover {
  background: transparent;
}
.tpl-src-l {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tpl-src-n {
  font-size: 12px;
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}
.tpl-soon {
  padding: 1px 7px;
  border-radius: var(--t-r-pill);
  background: var(--t-pill);
  color: var(--t-text3);
  font-size: 10.5px;
  font-weight: 650;
}
.tpl-main {
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.tpl-error {
  font-size: 12.5px;
  color: var(--t-error);
}
.tpl-grid {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  align-content: start;
  gap: 14px;
  padding: 4px;
  outline: none;
}
.tpl-tile {
  --tpl-tone: var(--t-text3);
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 10px 12px;
  border: 0;
  border-radius: var(--t-r-card);
  background: var(--t-card);
  box-shadow: var(--t-sh-card);
  color: var(--t-text1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.tpl-tile:hover {
  background: var(--t-card-hover);
}
/* Selected: a 2px accent ring — the only line here, and it is state. */
.tpl-tile.on {
  box-shadow: 0 0 0 2px var(--t-primary);
}
.tpl-grid:focus-visible .tpl-tile.on {
  box-shadow:
    0 0 0 2px var(--t-primary),
    0 0 0 5px color-mix(in srgb, var(--t-primary) 25%, transparent);
}
.tpl-prev {
  display: flex;
  flex-direction: column;
  gap: 6px;
  height: 92px;
  margin-bottom: 8px;
  padding: 12px;
  box-sizing: border-box;
  border-radius: 10px;
  background: linear-gradient(
    to top right,
    color-mix(in srgb, var(--tpl-tone) 18%, var(--t-card)),
    color-mix(in srgb, var(--tpl-tone) 30%, var(--t-card))
  );
}
.tpl-prev.blank {
  background: var(--t-ctl);
}
.tpl-prev-ic,
.tpl-emoji {
  color: var(--tpl-tone);
  font-size: 15px;
  line-height: 1;
  margin-bottom: 2px;
}
.tpl-line {
  height: 3px;
  width: 78%;
  border-radius: 2px;
  background: color-mix(in srgb, var(--t-text3) 55%, transparent);
}
.tpl-line.short {
  width: 48%;
}
.tpl-name {
  font-size: 13.5px;
  font-weight: 650;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tpl-desc {
  font-size: 12px;
  line-height: 1.4;
  color: var(--t-text3);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
}
.tpl-foot {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: auto;
  padding-top: 4px;
}
.tpl-origin {
  flex: 1;
  min-width: 0;
  font-size: 11px;
  color: var(--t-text3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tpl-act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 26px;
  height: 26px;
  padding: 0 6px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text2);
  font: inherit;
  font-size: 11.5px;
  cursor: pointer;
  opacity: 0;
}
.tpl-tile:hover .tpl-act,
.tpl-tile.on .tpl-act,
.tpl-act.armed,
.tpl-act:focus-visible {
  opacity: 1;
}
.tpl-act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.tpl-act.use {
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
}
.tpl-act.trash.armed {
  background: var(--t-error-grad);
  color: var(--t-on-primary);
  font-weight: 650;
}
@media (hover: none) {
  .tpl-act {
    opacity: 1;
  }
  .tpl-kbd,
  .tpl-hint,
  .tpl-cap {
    display: none;
  }
}
.tpl-empty {
  grid-column: 1 / -1;
  margin: 12px 0 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--t-text3);
}
.tpl-foot-bar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  background: var(--t-foot);
}
.tpl-hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--t-text3);
}
.tpl-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--t-r-ctl);
  background: var(--t-ctl);
  color: var(--t-text1);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.tpl-btn:hover {
  background: var(--t-hover);
}
.tpl-btn.primary {
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
}
.tpl-btn:disabled {
  opacity: 0.6;
  cursor: default;
}
.tpl-btn:focus-visible {
  outline: 2px solid var(--t-primary);
  outline-offset: 2px;
}
.tpl-cap {
  background: color-mix(in srgb, var(--t-on-primary) 22%, transparent);
  color: var(--t-on-primary);
}
.tpl-file {
  display: none;
}
@media (max-width: 760px) {
  .tpl-win {
    width: 100vw;
    height: 100dvh;
    border-radius: 0;
  }
  .tpl-body {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto minmax(0, 1fr);
    gap: 10px;
  }
  .tpl-nav {
    flex-direction: row;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .tpl-src {
    flex: none;
  }
  .tpl-head {
    flex-wrap: wrap;
  }
  .tpl-search {
    width: 100%;
  }
  .tpl-target {
    display: none;
  }
}
</style>
