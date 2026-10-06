<script>
// Only the menu opened last reacts to keys. Menus do nest — the breadcrumb "move
// to board" picker opens a board list inside the project list — and every one of
// them listens on document, so without a stack both would answer the same arrow.
const openMenus = []
</script>

<script setup>
// The item list of a property picker (priority, estimate, milestone, assignees,
// column, parent, tags, move-to-board), driven from the keyboard (#2932).
//
// Why a component and not a `tabindex` on each row: the popover body is
// teleported to <body>, and inside a modal vueuc's FocusTrap pulls any focus
// landing outside the modal straight back to its first element
// (vueuc/focus-trap `handleDocumentFocus` → `resetFocusTo('first')`). The rows
// therefore cannot hold focus at all — not even as real buttons, which the tag
// chips already are. So focus stays on the trigger and we drive a roving
// highlight from a document listener, the way naive's own select does.
import { inject, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps({
  // Rows are plain divs in most pickers and `<button class="tagchip">` in the
  // tag one; a caller with its own markup can widen the selector.
  itemSelector: { type: String, default: '.menu-item, .tagchip' },
})
const emit = defineEmits(['close'])

const root = ref(null)
const active = ref(-1)
const popover = inject('NPopover', null)

// Collecting the rows is also where their roles are stamped: the list is a
// v-for that re-renders on every filter keystroke, so there is no single moment
// after which the markup would stay put.
function items() {
  const el = root.value
  if (!el) return []
  const list = Array.from(el.querySelectorAll(props.itemSelector)).filter(
    (n) => !n.hasAttribute('disabled') && !n.classList.contains('disabled'),
  )
  for (const n of list) if (!n.hasAttribute('role')) n.setAttribute('role', 'menuitem')
  return list
}

function paint(list) {
  list.forEach((el, i) => {
    const on = i === active.value
    el.classList.toggle('kb-on', on)
    if (!on) {
      el.removeAttribute('aria-current')
      return
    }
    el.setAttribute('aria-current', 'true')
    el.scrollIntoView?.({ block: 'nearest' })
  })
}

function move(step) {
  const list = items()
  if (!list.length) return
  const n = list.length
  active.value = active.value < 0 && step < 0 ? n - 1 : (((active.value + step) % n) + n) % n
  paint(list)
}

function jump(last) {
  const list = items()
  if (!list.length) return
  active.value = last ? list.length - 1 : 0
  paint(list)
}

function activate() {
  const el = items()[active.value]
  if (!el) return false
  // A real click, so the keyboard goes through exactly the handler — and the
  // closing side effects — the mouse goes through.
  el.click()
  return true
}

// The filter field of the assignee picker rebuilds the list under the highlight;
// drop it rather than leave it pointing at whoever slid into that index.
function onInput() {
  const list = items()
  active.value = -1
  paint(list)
}

function isTyping(el) {
  if (!el) return false
  return el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable
}

function onKey(e) {
  if (openMenus[openMenus.length - 1] !== onKey || !root.value) return
  const k = e.key
  const typing = isTyping(e.target)
  // A text field *outside* the list — the «new milestone» / «new tag» box that
  // sits next to it in the same popover — is not the menu being operated, and
  // its own Enter has to reach it.
  if (typing && !root.value.contains(e.target)) return
  const eat = () => {
    e.preventDefault()
    e.stopPropagation()
  }
  if (k === 'ArrowDown' || k === 'ArrowUp') {
    // Taken even inside the filter field: there the arrows would only park the
    // caret, and picking from a filtered list is the whole point of typing.
    eat()
    move(k === 'ArrowDown' ? 1 : -1)
    return
  }
  if ((k === 'Home' || k === 'End') && !typing) {
    eat()
    jump(k === 'End')
    return
  }
  if (k === 'Enter' || (k === ' ' && !typing)) {
    // A focused <button> row (the tag chips are real buttons, and outside a
    // modal Tab does reach them) fires its own click on Enter/Space already.
    const ae = document.activeElement
    if (ae && root.value.contains(ae) && (ae.tagName === 'BUTTON' || ae.tagName === 'A')) return
    if (activate()) eat()
    return
  }
  if (k === 'Escape') {
    // This Esc is spent on the menu and must not reach the task modal
    // underneath, which naive closes from a document-level keydown of its own.
    eat()
    const trigger = popover?.getTriggerElement?.()
    // `handleClickOutside` is how naive dismisses a click-triggered popover from
    // the outside; the popover's own Esc only fires when it traps focus, which
    // these property menus don't.
    popover?.handleClickOutside?.(e)
    emit('close')
    trigger?.focus?.()
  }
}

onMounted(() => {
  openMenus.push(onKey)
  // Capture on document: the modal hears Esc through evtd's window-level bubble
  // handler, so only a capture listener gets to spend the key first.
  document.addEventListener('keydown', onKey, true)
  root.value?.addEventListener('input', onInput)
  nextTick(items)
})

onBeforeUnmount(() => {
  const i = openMenus.indexOf(onKey)
  if (i >= 0) openMenus.splice(i, 1)
  document.removeEventListener('keydown', onKey, true)
  root.value?.removeEventListener('input', onInput)
})
</script>

<template>
  <div ref="root" class="menu" role="menu"><slot /></div>
</template>

<style scoped>
/* Focus stays on the trigger, so the highlight is painted by hand. It mirrors
   `.menu-item:hover` (task/tab-shared.css) — the same row under the pointer and
   under the arrow keys must look the same. */
:deep(.kb-on) {
  background: var(--t-hover);
}
/* Tag chips already carry a colour of their own, so tint would be lost on them. */
:deep(.tagchip.kb-on) {
  outline: 2px solid var(--t-primary);
  outline-offset: 1px;
}
</style>
