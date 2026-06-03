<script setup>
import { ref, computed, nextTick, onBeforeUnmount } from 'vue'
import { renderRich } from '@/utils/markdown'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: 'Напишите что-нибудь…' },
  // Workspace members for @-mentions: [{ id, label }]. Empty → mentions off.
  mentionItems: { type: Array, default: () => [] },
  minRows: { type: Number, default: 3 },
})
const emit = defineEmits(['update:modelValue', 'submit', 'blur'])

const mode = ref('write') // 'write' | 'preview'
const ta = ref(null)
const previewHtml = computed(() => renderRich(props.modelValue, props.mentionItems))

// Members the user picked from the dropdown, so getMentions can resolve ids.
const picked = ref([])

function setValue(v) {
  emit('update:modelValue', v)
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

const tools = [
  { t: 'B', cls: 'b', title: 'Жирный', fn: () => applyAround('**') },
  { t: 'I', cls: 'i', title: 'Курсив', fn: () => applyAround('*') },
  { t: 'S', cls: 's', title: 'Зачёркнутый', fn: () => applyAround('~~') },
  { t: '</>', title: 'Код', fn: () => applyAround('`') },
  { t: 'H', title: 'Заголовок', fn: () => applyLinePrefix('## ') },
  { t: '•', title: 'Список', fn: () => applyLinePrefix('- ') },
  { t: '❝', title: 'Цитата', fn: () => applyLinePrefix('> ') },
  { t: '🔗', title: 'Ссылка', fn: insertLink },
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
  const upto = props.modelValue.slice(0, el.selectionStart)
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
  const val = props.modelValue
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
    </div>

    <div v-if="mode === 'write'" class="md2-write">
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
      />

      <div v-if="bubble" class="md2-bubble" :style="bubble">
        <button
          v-for="b in tools"
          :key="b.t"
          type="button"
          :class="b.cls"
          :title="b.title"
          @mousedown.prevent="b.fn"
        >
          {{ b.t }}
        </button>
      </div>

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

    <!-- eslint-disable-next-line vue/no-v-html -->
    <div v-else class="md md2-preview" v-html="previewHtml || '<em>Нечего показать</em>'" />
  </div>
</template>

<style scoped>
/* Seamless — the editor shares the modal's background, no box of its own. */
.md2 {
  width: 100%;
}
.md2-tabs {
  display: flex;
  gap: 14px;
  margin-bottom: 4px;
}
.md2-tabs button {
  border: none;
  background: transparent;
  color: var(--t-text3);
  font-size: 12px;
  padding: 2px 0;
  cursor: pointer;
  border-bottom: 2px solid transparent;
}
.md2-tabs button:hover {
  color: var(--t-text2);
}
.md2-tabs button.active {
  color: var(--t-text1);
  border-bottom-color: var(--t-primary);
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
  resize: vertical;
  padding: 2px 0;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 14px;
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
  background: var(--t-primary);
  color: var(--t-on-primary);
}
.md2-preview {
  padding: 2px 0;
  min-height: calc(v-bind(minRows) * 1.55em);
}
</style>
