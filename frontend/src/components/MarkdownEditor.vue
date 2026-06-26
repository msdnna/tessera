<script setup>
import { ref, computed, nextTick, watch, onMounted, onBeforeUnmount } from 'vue'
import { NIcon, useMessage } from 'naive-ui'
import { LinkOutline, ImageOutline, GitNetworkOutline } from '@vicons/ionicons5'
import { uploads as uploadsApi } from '@/api'
import { toggleTaskMarker } from '@/utils/markdown'
import RichContent from './RichContent.vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: 'Напишите что-нибудь…' },
  // Workspace members for @-mentions: [{ id, label }]. Empty → mentions off.
  mentionItems: { type: Array, default: () => [] },
  minRows: { type: Number, default: 3 },
  // 'write' | 'preview' — sets the initial tab (re-applied when it changes,
  // e.g. when a different task loads). User tab clicks override locally.
  initialMode: { type: String, default: 'write' },
})
const emit = defineEmits(['update:modelValue', 'submit', 'blur', 'persist'])

const message = useMessage()
const mode = ref(props.initialMode) // 'write' | 'preview'
watch(
  () => props.initialMode,
  (m) => {
    mode.value = m
  },
)
const ta = ref(null)

// Members the user picked from the dropdown, so getMentions can resolve ids.
const picked = ref([])

function setValue(v) {
  emit('update:modelValue', v)
}

// Auto-grow: the textarea height follows its content so long text isn't trapped
// behind an inner scrollbar (the modal scrolls instead).
function autoGrow() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}
watch(() => props.modelValue, () => nextTick(autoGrow))
watch(mode, (m) => {
  if (m === 'write') nextTick(autoGrow)
})
onMounted(() => nextTick(autoGrow))

// Preview checkbox toggle: rewrite the matching task marker in the markdown and
// ask the parent to persist (description save / comment update).
function onToggleCheck(i) {
  setValue(toggleTaskMarker(props.modelValue, i))
  emit('persist')
}

// ── markdown formatting (applied to the textarea selection) ──
function applyAround(before, after = before) {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  const val = props.modelValue
  const next = val.slice(0, s) + before + val.slice(s, e) + after + val.slice(e)
  setValue(next)
  nextTick(() => {
    el.focus()
    el.selectionStart = s + before.length
    el.selectionEnd = e + before.length
    refreshBubble()
  })
}
function applyLinePrefix(prefix) {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  const val = props.modelValue
  const lineStart = val.lastIndexOf('\n', s - 1) + 1
  let lineEnd = val.indexOf('\n', e)
  if (lineEnd === -1) lineEnd = val.length
  const prefixed = val
    .slice(lineStart, lineEnd)
    .split('\n')
    .map((l) => prefix + l)
    .join('\n')
  setValue(val.slice(0, lineStart) + prefixed + val.slice(lineEnd))
  nextTick(() => {
    el.focus()
    el.selectionStart = lineStart
    el.selectionEnd = lineStart + prefixed.length
    refreshBubble()
  })
}
// Link: no prompt — insert a markdown link skeleton starting at https:// and
// drop the caret right after it so the user just keeps typing the address.
function insertLink() {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  const val = props.modelValue
  const text = val.slice(s, e) || 'текст'
  const href = 'https://'
  const md = `[${text}](${href})`
  setValue(val.slice(0, s) + md + val.slice(e))
  const caret = s + 1 + text.length + 2 + href.length // after "https://"
  nextTick(() => {
    el.focus()
    el.selectionStart = el.selectionEnd = caret
    hideBubble()
  })
}

// ── insert at caret (images, snippets) ──
function insertAtCaret(text) {
  const el = ta.value
  const val = props.modelValue
  const s = el ? el.selectionStart : val.length
  const e = el ? el.selectionEnd : val.length
  setValue(val.slice(0, s) + text + val.slice(e))
  const caret = s + text.length
  nextTick(() => {
    if (!el) return
    el.focus()
    el.selectionStart = el.selectionEnd = caret
  })
}

// ── image upload (button / paste / drop) ──
const imgInput = ref(null)
const uploading = ref(false)
async function uploadImage(file) {
  if (!file || !file.type.startsWith('image/')) return
  uploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    const res = await uploadsApi.upload(fd)
    if (res.data?.url) insertAtCaret(`\n![${file.name || 'image'}](${res.data.url})\n`)
  } catch (err) {
    message.error(err.message || 'Не удалось загрузить изображение')
  } finally {
    uploading.value = false
  }
}
function pickImage() {
  imgInput.value?.click?.()
}
function onImgFile(e) {
  const f = e.target.files && e.target.files[0]
  e.target.value = ''
  if (f) uploadImage(f)
}
function onPaste(e) {
  const items = e.clipboardData && e.clipboardData.items
  for (const it of items || []) {
    if (it.type && it.type.startsWith('image/')) {
      const f = it.getAsFile()
      if (f) {
        e.preventDefault()
        uploadImage(f)
        return
      }
    }
  }
}
function onDrop(e) {
  const files = e.dataTransfer && e.dataTransfer.files
  const img = files && [...files].find((f) => f.type.startsWith('image/'))
  if (img) {
    e.preventDefault()
    uploadImage(img)
  }
}
function insertMermaid() {
  insertAtCaret('\n```mermaid\nflowchart TD\n  A[Старт] --> B[Готово]\n```\n')
}

const tools = [
  { t: 'B', cls: 'b', title: 'Жирный', fn: () => applyAround('**') },
  { t: 'I', cls: 'i', title: 'Курсив', fn: () => applyAround('*') },
  { t: 'S', cls: 's', title: 'Зачёркнутый', fn: () => applyAround('~~') },
  { t: '</>', title: 'Код', fn: () => applyAround('`') },
  { t: 'H', title: 'Заголовок', fn: () => applyLinePrefix('## ') },
  { t: '•', title: 'Список', fn: () => applyLinePrefix('- ') },
  { t: '❝', title: 'Цитата', fn: () => applyLinePrefix('> ') },
  { icon: LinkOutline, title: 'Ссылка', fn: insertLink },
]

// ── selection bubble toolbar ──
const bubble = ref(null) // { top, left } in viewport coords, or null

// Mirror-div technique: measure the pixel position of a caret index in the
// textarea, so the bubble can float right above the selection.
function caretCoords(el, index) {
  const div = document.createElement('div')
  const cs = getComputedStyle(el)
  const copy = [
    'boxSizing',
    'width',
    'paddingTop',
    'paddingRight',
    'paddingBottom',
    'paddingLeft',
    'borderTopWidth',
    'borderRightWidth',
    'borderBottomWidth',
    'borderLeftWidth',
    'fontStyle',
    'fontVariant',
    'fontWeight',
    'fontSize',
    'fontFamily',
    'lineHeight',
    'letterSpacing',
    'textTransform',
    'wordSpacing',
  ]
  copy.forEach((p) => (div.style[p] = cs[p]))
  div.style.position = 'absolute'
  div.style.visibility = 'hidden'
  div.style.whiteSpace = 'pre-wrap'
  div.style.wordWrap = 'break-word'
  div.style.overflow = 'hidden'
  div.style.height = 'auto'
  div.textContent = el.value.slice(0, index)
  const span = document.createElement('span')
  span.textContent = el.value.slice(index) || '.'
  div.appendChild(span)
  document.body.appendChild(div)
  const top = span.offsetTop
  const left = span.offsetLeft
  document.body.removeChild(div)
  return { top, left, line: parseFloat(cs.lineHeight) || 18 }
}
function refreshBubble() {
  const el = ta.value
  if (!el || mode.value !== 'write') return hideBubble()
  if (el.selectionStart === el.selectionEnd) return hideBubble()
  const c = caretCoords(el, el.selectionStart)
  const rect = el.getBoundingClientRect()
  const x = rect.left + c.left - el.scrollLeft
  const y = rect.top + c.top - el.scrollTop
  const left = Math.min(Math.max(x, 8), window.innerWidth - 8)
  bubble.value = { left: `${left}px`, top: `${y - 10}px` }
}
function hideBubble() {
  bubble.value = null
}
function onBlur() {
  // Defer so a bubble-button mousedown can run before we tear it down.
  setTimeout(() => {
    hideBubble()
    emit('blur')
  }, 120)
}
onBeforeUnmount(hideBubble)

// ── @-mention autocomplete ──
const mq = ref(null) // { start, query } while open
const mqIndex = ref(0)
const mentionMatches = computed(() => {
  if (!mq.value) return []
  const q = mq.value.query.toLowerCase()
  return props.mentionItems.filter((m) => m.label.toLowerCase().includes(q)).slice(0, 8)
})
function detectMention() {
  const el = ta.value
  if (!el || !props.mentionItems.length) return (mq.value = null)
  // Read the live DOM value: the modelValue prop lags one input behind.
  const upto = el.value.slice(0, el.selectionStart)
  const m = upto.match(/(^|\s)@([^\s@]*)$/)
  if (m) {
    mq.value = { start: el.selectionStart - m[2].length - 1, query: m[2] }
    mqIndex.value = 0
  } else {
    mq.value = null
  }
}
function pickMention(item) {
  if (!item || !mq.value) return
  const el = ta.value
  const val = el.value
  const insert = `@${item.label} `
  const next = val.slice(0, mq.value.start) + insert + val.slice(el.selectionStart)
  if (!picked.value.some((p) => p.id === item.id)) picked.value.push({ ...item })
  setValue(next)
  const caret = mq.value.start + insert.length
  mq.value = null
  nextTick(() => {
    el.focus()
    el.selectionStart = el.selectionEnd = caret
  })
}

function onInput(e) {
  setValue(e.target.value)
  autoGrow()
  detectMention()
  hideBubble()
}
function onSelect() {
  if (!mq.value) refreshBubble()
}
function onKeydown(e) {
  if (mq.value && mentionMatches.value.length) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      mqIndex.value = (mqIndex.value + 1) % mentionMatches.value.length
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      mqIndex.value =
        (mqIndex.value - 1 + mentionMatches.value.length) % mentionMatches.value.length
      return
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      pickMention(mentionMatches.value[mqIndex.value])
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      mq.value = null
      return
    }
  }
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    e.preventDefault()
    emit('submit')
  }
}

function getMentions() {
  const text = props.modelValue
  return picked.value.filter((p) => text.includes(`@${p.label}`)).map((p) => p.id)
}
function clear() {
  setValue('')
  picked.value = []
  hideBubble()
}
function focus() {
  ta.value?.focus()
}
defineExpose({ getMentions, clear, focus })
</script>

<template>
  <div class="md2">
    <div class="md2-tabs">
      <button type="button" :class="{ active: mode === 'write' }" @click="mode = 'write'">
        Написать
      </button>
      <button type="button" :class="{ active: mode === 'preview' }" @click="mode = 'preview'">
        Просмотр
      </button>
      <span class="md2-spacer" />
      <template v-if="mode === 'write'">
        <button
          type="button"
          class="md2-act"
          :class="{ busy: uploading }"
          title="Вставить изображение"
          @click="pickImage"
        >
          <n-icon :component="ImageOutline" :size="16" />
        </button>
        <button
          type="button"
          class="md2-act"
          title="Вставить Mermaid-диаграмму"
          @click="insertMermaid"
        >
          <n-icon :component="GitNetworkOutline" :size="16" />
        </button>
      </template>
    </div>

    <input ref="imgInput" type="file" accept="image/*" hidden @change="onImgFile" />

    <Transition name="md2-fade" mode="out-in" @after-enter="autoGrow">
    <div v-if="mode === 'write'" key="write" class="md2-write">
      <textarea
        ref="ta"
        :value="modelValue"
        :placeholder="placeholder"
        :rows="minRows"
        spellcheck="false"
        @input="onInput"
        @keydown="onKeydown"
        @select="onSelect"
        @mouseup="onSelect"
        @scroll="hideBubble"
        @blur="onBlur"
        @paste="onPaste"
        @dragover.prevent
        @drop="onDrop"
      />

      <Transition name="bubble">
        <div v-if="bubble" class="md2-bubble" :style="bubble">
          <button
            v-for="b in tools"
            :key="b.title"
            type="button"
            :class="b.cls"
            :title="b.title"
            @mousedown.prevent="b.fn"
          >
            <n-icon v-if="b.icon" :component="b.icon" :size="15" />
            <template v-else>{{ b.t }}</template>
          </button>
        </div>
      </Transition>

      <ul v-if="mq && mentionMatches.length" class="md2-mentions">
        <li
          v-for="(m, i) in mentionMatches"
          :key="m.id"
          :class="{ active: i === mqIndex }"
          @mousedown.prevent="pickMention(m)"
        >
          {{ m.label }}
        </li>
      </ul>
    </div>

    <RichContent
      v-else
      key="preview"
      class="md2-preview"
      :source="modelValue"
      :members="mentionItems"
      interactive
      empty="Нечего показать"
      @toggle="onToggleCheck"
    />
    </Transition>
  </div>
</template>

<style scoped>
/* Seamless — the editor shares the modal's background, no box of its own. */
.md2 {
  width: 100%;
}
/* Mirror the modal's bottom n-tabs (line, small) for a unified look. */
.md2-tabs {
  display: flex;
  align-items: center;
  gap: 18px;
  margin-bottom: 6px;
}
.md2-spacer {
  flex: 1;
}
.md2-act {
  border: none;
  background: transparent;
  color: var(--t-text2);
  cursor: pointer;
  padding: 2px 5px;
  border-radius: 5px;
  display: inline-flex;
  align-items: center;
}
.md2-act:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.md2-act.busy {
  opacity: 0.5;
  pointer-events: none;
}
.md2-tabs button {
  border: none;
  background: transparent;
  color: var(--t-text2);
  font-size: 14px;
  padding: 0 0 6px;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color 0.2s ease;
}
.md2-tabs button:hover {
  color: var(--t-text1);
}
.md2-tabs button.active {
  color: var(--t-primary);
  border-bottom-color: var(--t-primary);
}
/* Fade between Написать / Просмотр panes. */
.md2-fade-enter-active,
.md2-fade-leave-active {
  transition: opacity 0.15s ease;
}
.md2-fade-enter-from,
.md2-fade-leave-to {
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .md2-fade-enter-active,
  .md2-fade-leave-active {
    transition: none;
  }
}
.md2-write {
  position: relative;
}
.md2-write textarea {
  display: block;
  width: 100%;
  box-sizing: border-box;
  border: none;
  outline: none;
  /* Auto-grows to its content (height set in JS); no inner scrollbar. */
  resize: none;
  overflow: hidden;
  padding: 2px 0;
  background: transparent;
  color: var(--t-text1);
  /* Monospace so indentation (nested lists) lines up while editing. */
  font-family: ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.55;
  min-height: calc(v-bind(minRows) * 1.55em);
}
.md2-write textarea::placeholder {
  color: var(--t-text3);
}

/* Floating selection toolbar */
.md2-bubble {
  position: fixed;
  z-index: 4100;
  transform: translate(-4px, -100%);
  display: flex;
  gap: 1px;
  padding: 3px;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.28);
}
.md2-bubble button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 26px;
  padding: 0 6px;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--t-text1);
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
}
.md2-bubble button:hover {
  background: var(--t-hover);
}
.bubble-enter-active,
.bubble-leave-active {
  transition:
    opacity 0.12s ease,
    margin-top 0.12s ease;
}
.bubble-enter-from,
.bubble-leave-to {
  opacity: 0;
  margin-top: 4px;
}
.md2-bubble button.b {
  font-weight: 700;
}
.md2-bubble button.i {
  font-style: italic;
}
.md2-bubble button.s {
  text-decoration: line-through;
}

.md2-mentions {
  position: absolute;
  left: 0;
  bottom: 4px;
  z-index: 30;
  margin: 0;
  padding: 4px;
  list-style: none;
  min-width: 180px;
  max-height: 200px;
  overflow-y: auto;
  background: var(--t-surface);
  border: 1px solid var(--t-border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
}
.md2-mentions li {
  padding: 6px 10px;
  border-radius: 5px;
  font-size: 13px;
  color: var(--t-text1);
  cursor: pointer;
}
.md2-mentions li:hover,
.md2-mentions li.active {
  background: var(--t-accent-grad);
  color: var(--t-on-primary);
}
.md2-preview {
  padding: 2px 0;
  min-height: calc(v-bind(minRows) * 1.55em);
}
</style>
