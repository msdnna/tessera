<script setup>
import { ref, shallowRef, watch, onMounted, onBeforeUnmount } from 'vue'
import { Editor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import Mention from '@tiptap/extension-mention'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: 'Напишите что-нибудь…' },
  // Workspace members for @-mentions: [{ id, label }]. Empty → mentions off.
  mentionItems: { type: Array, default: () => [] },
  minRows: { type: Number, default: 3 },
})
const emit = defineEmits(['update:modelValue', 'submit', 'blur'])

const editor = shallowRef(null)
const root = ref(null)

// ── @-mention suggestion popup (plain DOM, no extra deps) ──
function makeMentionSuggestion() {
  return {
    char: '@',
    items: ({ query }) => {
      const q = query.toLowerCase()
      return props.mentionItems.filter((m) => m.label.toLowerCase().includes(q)).slice(0, 8)
    },
    render: () => {
      let popup, items, selected, command
      const pick = (i) => {
        const item = items[i]
        if (item) command({ id: item.id, label: item.label })
      }
      const paint = () => {
        if (!popup) return
        popup.innerHTML = ''
        if (!items.length) {
          popup.style.display = 'none'
          return
        }
        popup.style.display = 'block'
        items.forEach((it, i) => {
          const el = document.createElement('button')
          el.type = 'button'
          el.className = 'mention-opt' + (i === selected ? ' is-active' : '')
          el.textContent = it.label
          el.addEventListener('mousedown', (e) => {
            e.preventDefault()
            pick(i)
          })
          popup.appendChild(el)
        })
      }
      const place = (rect) => {
        if (!popup || !rect) return
        popup.style.left = `${rect.left}px`
        popup.style.top = `${rect.bottom + 4}px`
      }
      return {
        onStart: (p) => {
          items = p.items
          selected = 0
          command = p.command
          popup = document.createElement('div')
          popup.className = 'mention-popup'
          document.body.appendChild(popup)
          paint()
          place(p.clientRect?.())
        },
        onUpdate: (p) => {
          items = p.items
          selected = 0
          command = p.command
          paint()
          place(p.clientRect?.())
        },
        onKeyDown: (p) => {
          if (!items.length) return false
          if (p.event.key === 'ArrowDown') {
            selected = (selected + 1) % items.length
            paint()
            return true
          }
          if (p.event.key === 'ArrowUp') {
            selected = (selected - 1 + items.length) % items.length
            paint()
            return true
          }
          if (p.event.key === 'Enter') {
            pick(selected)
            return true
          }
          if (p.event.key === 'Escape') {
            popup?.remove()
            popup = null
            return true
          }
          return false
        },
        onExit: () => {
          popup?.remove()
          popup = null
        },
      }
    },
  }
}

onMounted(() => {
  const extensions = [
    StarterKit.configure({ link: { openOnClick: false, autolink: true } }),
    Placeholder.configure({ placeholder: () => props.placeholder }),
  ]
  if (props.mentionItems.length) {
    extensions.push(
      Mention.configure({
        HTMLAttributes: { class: 'mention' },
        suggestion: makeMentionSuggestion(),
      }),
    )
  }
  editor.value = new Editor({
    content: props.modelValue || '',
    extensions,
    onUpdate: ({ editor: e }) => emit('update:modelValue', e.getHTML()),
    onBlur: () => emit('blur'),
  })
})

onBeforeUnmount(() => editor.value?.destroy())

// Keep the editor in sync when the bound value changes from outside (e.g. when
// a different task loads), without clobbering the caret during local typing.
watch(
  () => props.modelValue,
  (val) => {
    const e = editor.value
    if (e && val !== e.getHTML()) {
      e.commands.setContent(val || '', { emitUpdate: false })
    }
  },
)

// Collect unique user ids of mention chips currently in the document.
function getMentions() {
  const e = editor.value
  if (!e) return []
  const ids = new Set()
  e.getJSON().content?.forEach(function walk(node) {
    if (node.type === 'mention' && node.attrs?.id) ids.add(node.attrs.id)
    node.content?.forEach(walk)
  })
  return [...ids]
}

function clear() {
  editor.value?.commands.clearContent(true)
}
function focus() {
  editor.value?.commands.focus()
}
defineExpose({ getMentions, clear, focus })

function onKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    e.preventDefault()
    emit('submit')
  }
}

// Toolbar command helpers (no-op until the editor exists).
const cmd = (fn) => () => fn(editor.value?.chain().focus())?.run?.()
const isActive = (name, attrs) => editor.value?.isActive(name, attrs) ?? false
function setLink() {
  const e = editor.value
  if (!e) return
  const prev = e.getAttributes('link').href || ''
  const url = window.prompt('Ссылка (URL):', prev)
  if (url === null) return
  if (url === '') {
    e.chain().focus().unsetLink().run()
    return
  }
  e.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
}
</script>

<template>
  <div ref="root" class="rich" :style="{ '--rich-min': minRows * 1.5 + 'em' }">
    <div v-if="editor" class="rich-toolbar">
      <button
        type="button"
        title="Жирный"
        :class="{ on: isActive('bold') }"
        @click="cmd((c) => c.toggleBold())"
      >
        <b>B</b>
      </button>
      <button
        type="button"
        title="Курсив"
        :class="{ on: isActive('italic') }"
        @click="cmd((c) => c.toggleItalic())"
      >
        <i>I</i>
      </button>
      <button
        type="button"
        title="Зачёркнутый"
        :class="{ on: isActive('strike') }"
        @click="cmd((c) => c.toggleStrike())"
      >
        <s>S</s>
      </button>
      <button
        type="button"
        title="Код"
        :class="{ on: isActive('code') }"
        @click="cmd((c) => c.toggleCode())"
      >
        &lt;/&gt;
      </button>
      <span class="rt-sep" />
      <button
        type="button"
        title="Заголовок"
        :class="{ on: isActive('heading', { level: 2 }) }"
        @click="cmd((c) => c.toggleHeading({ level: 2 }))"
      >
        H
      </button>
      <button
        type="button"
        title="Маркированный список"
        :class="{ on: isActive('bulletList') }"
        @click="cmd((c) => c.toggleBulletList())"
      >
        •
      </button>
      <button
        type="button"
        title="Нумерованный список"
        :class="{ on: isActive('orderedList') }"
        @click="cmd((c) => c.toggleOrderedList())"
      >
        1.
      </button>
      <button
        type="button"
        title="Цитата"
        :class="{ on: isActive('blockquote') }"
        @click="cmd((c) => c.toggleBlockquote())"
      >
        ❝
      </button>
      <button type="button" title="Ссылка" :class="{ on: isActive('link') }" @click="setLink">
        🔗
      </button>
    </div>
    <EditorContent class="rich-body" :editor="editor" @keydown="onKeydown" />
  </div>
</template>

<style scoped>
.rich {
  border: 1px solid var(--t-border, #e0e0e6);
  border-radius: 8px;
  background: var(--t-card, #fff);
  overflow: hidden;
}
.rich-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  align-items: center;
  padding: 4px 6px;
  border-bottom: 1px solid var(--t-border, #e0e0e6);
  background: var(--t-fill, #f6f7f9);
}
.rich-toolbar button {
  min-width: 26px;
  height: 26px;
  padding: 0 6px;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--t-text, #333);
  font-size: 13px;
  cursor: pointer;
  line-height: 1;
}
.rich-toolbar button:hover {
  background: var(--t-hover, rgba(0, 0, 0, 0.06));
}
.rich-toolbar button.on {
  background: var(--t-primary, #2f80ed);
  color: #fff;
}
.rt-sep {
  width: 1px;
  height: 16px;
  margin: 0 4px;
  background: var(--t-border, #e0e0e6);
}
.rich-body {
  padding: 8px 10px;
}
:deep(.ProseMirror) {
  min-height: var(--rich-min);
  outline: none;
  font-size: 14px;
  line-height: 1.55;
}
:deep(.ProseMirror p) {
  margin: 0 0 0.5em;
}
:deep(.ProseMirror p:last-child) {
  margin-bottom: 0;
}
:deep(.ProseMirror p.is-editor-empty:first-child::before) {
  content: attr(data-placeholder);
  float: left;
  height: 0;
  color: var(--t-text-3, #9aa0aa);
  pointer-events: none;
}
:deep(.ProseMirror .mention) {
  color: var(--t-primary, #2f80ed);
  background: color-mix(in srgb, var(--t-primary, #2f80ed) 12%, transparent);
  border-radius: 4px;
  padding: 0 3px;
  font-weight: 500;
}
</style>

<!-- Mention popup is teleported to <body>, so its styles must be global. -->
<style>
.mention-popup {
  position: fixed;
  z-index: 4000;
  min-width: 160px;
  max-height: 220px;
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--t-border, #e0e0e6);
  border-radius: 8px;
  background: var(--t-card, #fff);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
}
.mention-opt {
  display: block;
  width: 100%;
  padding: 6px 10px;
  border: none;
  border-radius: 5px;
  background: transparent;
  color: var(--t-text, #333);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.mention-opt:hover,
.mention-opt.is-active {
  background: var(--t-primary, #2f80ed);
  color: #fff;
}
</style>
