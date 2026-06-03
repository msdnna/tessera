<script setup>
import { ref, computed, nextTick } from 'vue'
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

// Members the user has actually picked from the dropdown, so getMentions can
// resolve ids without re-parsing ambiguous free text.
const picked = ref([])

function setValue(v) {
  emit('update:modelValue', v)
}

// ── markdown formatting toolbar ──
function applyAround(before, after = before) {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  const val = props.modelValue
  const sel = val.slice(s, e)
  const next = val.slice(0, s) + before + sel + after + val.slice(e)
  setValue(next)
  nextTick(() => {
    el.focus()
    el.selectionStart = s + before.length
    el.selectionEnd = e + before.length
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
  const block = val.slice(lineStart, lineEnd)
  const prefixed = block
    .split('\n')
    .map((l) => prefix + l)
    .join('\n')
  const next = val.slice(0, lineStart) + prefixed + val.slice(lineEnd)
  setValue(next)
  nextTick(() => {
    el.focus()
    el.selectionStart = lineStart
    el.selectionEnd = lineStart + prefixed.length
  })
}
function insertLink() {
  const el = ta.value
  if (!el) return
  const { selectionStart: s, selectionEnd: e } = el
  const sel = props.modelValue.slice(s, e) || 'текст'
  const url = window.prompt('Ссылка (URL):', 'https://')
  if (!url) return
  const md = `[${sel}](${url})`
  const next = props.modelValue.slice(0, s) + md + props.modelValue.slice(e)
  setValue(next)
  nextTick(() => el.focus())
}

const tools = [
  { t: 'B', title: 'Жирный (**)', fn: () => applyAround('**') },
  { t: 'I', title: 'Курсив (*)', fn: () => applyAround('*') },
  { t: 'S', title: 'Зачёркнутый (~~)', fn: () => applyAround('~~') },
  { t: '</>', title: 'Код (`)', fn: () => applyAround('`') },
  { t: 'H', title: 'Заголовок', fn: () => applyLinePrefix('## ') },
  { t: '•', title: 'Список', fn: () => applyLinePrefix('- ') },
  { t: '1.', title: 'Нумерованный список', fn: () => applyLinePrefix('1. ') },
  { t: '❝', title: 'Цитата', fn: () => applyLinePrefix('> ') },
  { t: '🔗', title: 'Ссылка', fn: insertLink },
]

// ── @-mention autocomplete ──
const mq = ref(null) // { start, query } when the dropdown is open, else null
const mqIndex = ref(0)
const mentionMatches = computed(() => {
  if (!mq.value) return []
  const q = mq.value.query.toLowerCase()
  return props.mentionItems.filter((m) => m.label.toLowerCase().includes(q)).slice(0, 8)
})

function detectMention() {
  const el = ta.value
  if (!el || !props.mentionItems.length) {
    mq.value = null
    return
  }
  const pos = el.selectionStart
  const upto = props.modelValue.slice(0, pos)
  // Match a trailing "@word" where @ starts at a word boundary.
  const m = upto.match(/(^|\s)@([^\s@]*)$/)
  if (m) {
    mq.value = { start: pos - m[2].length - 1, query: m[2] }
    mqIndex.value = 0
  } else {
    mq.value = null
  }
}
function pickMention(item) {
  if (!item || !mq.value) return
  const el = ta.value
  const pos = el.selectionStart
  const val = props.modelValue
  const insert = `@${item.label} `
  const next = val.slice(0, mq.value.start) + insert + val.slice(pos)
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

// Resolve mentioned user ids: picked members whose @label still appears in text.
function getMentions() {
  const text = props.modelValue
  return picked.value.filter((p) => text.includes(`@${p.label}`)).map((p) => p.id)
}
function clear() {
  setValue('')
  picked.value = []
}
function focus() {
  ta.value?.focus()
}
defineExpose({ getMentions, clear, focus })
</script>

<template>
  <div class="md-editor">
    <div class="md-tabs">
      <div class="md-tabbtns">
        <button
          type="button"
          :class="{ active: mode === 'write' }"
          @click="mode = 'write'"
        >
          Написать
        </button>
        <button
          type="button"
          :class="{ active: mode === 'preview' }"
          @click="mode = 'preview'"
        >
          Просмотр
        </button>
      </div>
      <div v-if="mode === 'write'" class="md-tools">
        <button
          v-for="b in tools"
          :key="b.t"
          type="button"
          :title="b.title"
          @mousedown.prevent
          @click="b.fn"
        >
          {{ b.t }}
        </button>
      </div>
    </div>

    <div v-if="mode === 'write'" class="md-write">
      <textarea
        ref="ta"
        :value="modelValue"
        :placeholder="placeholder"
        :rows="minRows"
        spellcheck="false"
        @input="onInput"
        @keydown="onKeydown"
        @blur="emit('blur')"
      />
      <ul v-if="mq && mentionMatches.length" class="md-mentions">
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
    <div v-else class="md md-preview" v-html="previewHtml || '<em>Нечего показать</em>'" />
  </div>
</template>

<style scoped>
.md-editor {
  border: 1px solid var(--t-border);
  border-radius: 8px;
  background: var(--t-surface);
  overflow: hidden;
}
.md-tabs {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 4px 6px;
  border-bottom: 1px solid var(--t-border);
  background: var(--t-surface-alt);
}
.md-tabbtns {
  display: flex;
  gap: 2px;
}
.md-tabbtns button {
  border: none;
  background: transparent;
  color: var(--t-text2);
  font-size: 13px;
  padding: 4px 10px;
  border-radius: 6px;
  cursor: pointer;
}
.md-tabbtns button:hover {
  background: var(--t-hover);
}
.md-tabbtns button.active {
  background: var(--t-surface);
  color: var(--t-text1);
  font-weight: 600;
}
.md-tools {
  display: flex;
  flex-wrap: wrap;
  gap: 1px;
}
.md-tools button {
  min-width: 26px;
  height: 26px;
  padding: 0 6px;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--t-text2);
  font-size: 13px;
  cursor: pointer;
  line-height: 1;
}
.md-tools button:hover {
  background: var(--t-hover);
  color: var(--t-text1);
}
.md-write {
  position: relative;
}
.md-write textarea {
  display: block;
  width: 100%;
  box-sizing: border-box;
  border: none;
  outline: none;
  resize: vertical;
  padding: 9px 11px;
  background: var(--t-surface);
  color: var(--t-text1);
  font: inherit;
  font-size: 14px;
  line-height: 1.55;
  min-height: calc(v-bind(minRows) * 1.55em);
}
.md-write textarea::placeholder {
  color: var(--t-text3);
}
.md-mentions {
  position: absolute;
  left: 10px;
  bottom: 8px;
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
.md-mentions li {
  padding: 6px 10px;
  border-radius: 5px;
  font-size: 13px;
  color: var(--t-text1);
  cursor: pointer;
}
.md-mentions li:hover,
.md-mentions li.active {
  background: var(--t-primary);
  color: var(--t-on-primary);
}
.md-preview {
  padding: 9px 11px;
  min-height: calc(v-bind(minRows) * 1.55em);
}
</style>
