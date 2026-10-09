<script setup>
// The selection bubble over text that is only read (#3118, PackageJ «Пузырь в
// тексте только для чтения»): select a stretch of the description or of a
// comment and, 150 ms later, «Цитировать в ответ Q» and «Ссылка на фрагмент»
// float over it. Quoting hands the text to the task's comment composer
// (utils/quoteBus); a link to a fragment needs anchors the app doesn't keep yet
// — a visible stub until then.
//
// Rendered in place (position: fixed) rather than teleported, for the same
// reason as the editor's own popups: inside the task modal a body-level layer is
// outside its focus trap.
import { ref, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import { ChatbubbleOutline, LinkOutline } from '@vicons/ionicons5'
import ShortcutKeys from '../ShortcutKeys.vue'
import { useShortcuts } from '@/composables/useShortcuts'
import { requestQuote } from '@/utils/quoteBus'
import { stubTask } from '@/utils/featureStubs'
import { isTypingTarget } from '@/utils/keys'

const props = defineProps({
  // The element whose selection counts — a getter, so a re-rendered root is read
  // at the moment of the selection.
  target: { type: Function, required: true },
  taskId: { type: String, default: null },
  // Off where there is nothing to quote into (a read-only task).
  canQuote: { type: Boolean, default: true },
})

const { t } = useI18n()
const SHOW_DELAY = 150
const fragmentStub = stubTask('editor.fragmentLink')

const el = ref(null)
const shown = ref(null) // { left, top, text }
let timer = 0

function hide() {
  shown.value = null
}
function check() {
  const root = props.target?.()
  const sel = typeof window !== 'undefined' ? window.getSelection?.() : null
  if (!root || !sel || sel.isCollapsed || !sel.rangeCount) return hide()
  // A selection inside a field being edited here is the editor's own bubble's.
  const ae = document.activeElement
  if (isTypingTarget(ae) && root.contains(ae)) return hide()
  const range = sel.getRangeAt(0)
  if (!root.contains(range.commonAncestorContainer)) return hide()
  const text = String(sel).trim()
  if (!text) return hide()
  const r = range.getClientRects()[0] || range.getBoundingClientRect()
  shown.value = { left: r.left, top: r.top, text }
  nextTick(clamp)
}
// Keep the bubble on screen: it starts at the selection's first line and is
// pulled back from the right edge, and drops under the line when there is no
// room above it.
function clamp() {
  const box = el.value
  if (!box || !shown.value) return
  const w = box.offsetWidth
  const h = box.offsetHeight
  const vw = document.documentElement.clientWidth
  const left = Math.max(8, Math.min(shown.value.left - 6, vw - w - 8))
  const above = shown.value.top - h - 8
  box.style.left = `${left}px`
  box.style.top = `${above > 8 ? above : shown.value.top + 24}px`
}
function onSelectionChange() {
  clearTimeout(timer)
  timer = setTimeout(check, SHOW_DELAY)
}
function onScroll() {
  if (shown.value) check()
}

function quote() {
  if (!shown.value || !props.canQuote) return
  const text = shown.value.text
  if (requestQuote(props.taskId, text)) {
    window.getSelection?.()?.removeAllRanges?.()
    hide()
  }
}

onMounted(() => {
  document.addEventListener('selectionchange', onSelectionChange)
  window.addEventListener('scroll', onScroll, true)
})
onBeforeUnmount(() => {
  clearTimeout(timer)
  document.removeEventListener('selectionchange', onSelectionChange)
  window.removeEventListener('scroll', onScroll, true)
})
useShortcuts({ 'editor.quote': quote }, { when: () => !!shown.value && props.canQuote })
</script>

<template>
  <Transition name="erb">
    <div
      v-if="shown"
      ref="el"
      class="erb"
      role="toolbar"
      :aria-label="t('documents.editor.bubble.label')"
      data-testid="read-bubble"
      @mousedown.prevent
    >
      <button
        v-if="canQuote"
        type="button"
        class="erb-b"
        data-testid="read-bubble-quote"
        @click="quote"
      >
        <n-icon :component="ChatbubbleOutline" :size="14" class="erb-ic" />
        {{ t('documents.editor.bubble.quote') }}
        <ShortcutKeys id="editor.quote" />
      </button>
      <button
        type="button"
        class="erb-b stub"
        aria-disabled="true"
        :title="t('common.stub.soon', { n: fragmentStub })"
        data-testid="read-bubble-link"
      >
        <n-icon :component="LinkOutline" :size="14" class="erb-ic" />
        {{ t('documents.editor.bubble.fragment') }}
      </button>
    </div>
  </Transition>
</template>

<style scoped>
.erb {
  position: fixed;
  left: 0;
  top: 0;
  z-index: 4100;
  display: flex;
  gap: 2px;
  padding: 4px;
  border-radius: 10px;
  background: var(--t-pop);
  box-shadow: var(--t-sh-pop);
  color: var(--t-text1);
  --t-surface: var(--t-pop);
}
.erb-b {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 30px;
  padding: 0 6px 0 9px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--t-text1);
  font: inherit;
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
}
.erb-b:hover {
  background: var(--t-hover);
}
.erb-b.stub {
  padding-right: 9px;
  color: var(--t-text3);
  cursor: default;
}
.erb-b.stub:hover {
  background: transparent;
}
.erb-ic {
  color: var(--t-text2);
}
.stub .erb-ic {
  color: var(--t-text3);
}
.erb-enter-active,
.erb-leave-active {
  transition: opacity 0.12s ease;
}
.erb-enter-from,
.erb-leave-to {
  opacity: 0;
}
</style>
