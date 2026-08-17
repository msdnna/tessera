<script setup>
import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { NPopover, useMessage } from 'naive-ui'
import { renderRich, sanitizeSvgFragment } from '@/utils/markdown'
import { resolveMention } from '@/utils/mentions'
import { useThemeStore } from '@/stores/theme'
import MentionCard from './MentionCard.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
// #N is resolved workspace-wide, so the endpoint lives on the workspaces api —
// `tasks` has no equivalent (asking it for one is what silently broke the chips).
import { workspaces as wsApi } from '@/api'
import { saveAttachment, attachmentIdFromHref } from '@/utils/download'

// Renders Markdown (via renderRich → sanitised HTML) and then asynchronously
// turns ```mermaid fenced blocks into SVG diagrams. Mermaid is loaded lazily
// (only when a diagram is present) so it never weighs down the main bundle.
const props = defineProps({
  source: { type: String, default: '' },
  members: { type: Array, default: () => [] },
  empty: { type: String, default: '' },
  // When true, GFM task-list checkboxes become clickable; clicking emits `toggle`
  // with the checkbox index so the parent can rewrite the stored markdown.
  interactive: { type: Boolean, default: false },
  // Opt-in: hovering an @-mention opens a card naming the person. Off by default
  // because RichContent also renders the description preview on board cards,
  // where a floating card would cover neighbours and fight drag-and-drop.
  mentionCards: { type: Boolean, default: false },
  // Opt-in: render "#123" as a link to that task. Off on board cards, where the
  // link would swallow clicks meant for the card and get in the way of dragging.
  taskRefs: { type: Boolean, default: false },
})
const emit = defineEmits(['toggle'])

// The popover is a second root node, so attrs are placed by hand on the content
// div — callers pass `class` (e.g. `.c-text`) and expect it there, not on a wrapper.
defineOptions({ inheritAttrs: false })

const theme = useThemeStore()
const ws = useWorkspacesStore()
const router = useRouter()
const message = useMessage()
const root = ref(null)
const html = ref('')
let mermaidMod = null
let seq = 0 // guards against out-of-order async renders

function build() {
  let out = renderRich(props.source, props.members, { taskRefs: props.taskRefs })
  // Interactive: drop the `disabled` marked stamps on task checkboxes so they
  // can receive clicks (the markdown rewrite is the source of truth, not the DOM).
  if (props.interactive && out) {
    out = out.replace(
      /<input\b([^>]*\btype="checkbox"[^>]*)>/gi,
      (_, attrs) => `<input${attrs.replace(/\s*disabled(="")?/gi, '')}>`,
    )
  }
  html.value = out || (props.empty ? `<em class="rc-empty">${props.empty}</em>` : '')
  nextTick(renderMermaid)
}

// A self-originated checkbox toggle already flipped (and animated) the box in the
// live DOM, so skip the source→html rebuild it triggers — rebuilding recreates the
// checkbox element and the CSS tick transition never plays.
let skipBuild = false
function queueToggle(box) {
  const boxes = [...root.value.querySelectorAll('input[type="checkbox"]')]
  const i = boxes.indexOf(box)
  if (i >= 0) {
    skipBuild = true
    emit('toggle', i)
  }
}
// A "#123" chip is resolved on click rather than at render time — a description
// can mention many tasks, and most are never clicked. Resolutions are cached for
// the session because the same chip gets clicked repeatedly.
const refCache = new Map()
async function openTaskRef(el) {
  const number = Number(el.dataset.taskRef)
  const wsId = ws.currentId
  if (!number || !wsId) return
  const key = `${wsId}:${number}`
  try {
    if (!refCache.has(key)) {
      const res = await wsApi.taskByNumber(wsId, number)
      refCache.set(key, res.data || null)
    }
    const task = refCache.get(key)
    // No board means the task exists but isn't on one — there is nowhere to go.
    if (!task?.board_id) {
      message.warning(`Задача #${number} не найдена`)
      return
    }
    router.push(`/board/${task.board_id}?task=${number}`)
  } catch {
    refCache.set(key, null)
    message.warning(`Задача #${number} не найдена`)
  }
}

// Attachment links can't be followed: the download route is behind auth and the
// token never leaves memory, so the click is turned into an api request.
async function downloadLink(id, filename) {
  try {
    if ((await saveAttachment(id, filename)) === 'saved') message.success('Файл сохранён')
  } catch (e) {
    message.error(e.message || 'Не удалось скачать файл')
  }
}

// Delegate clicks: task-ref chips and attachment links first (they work whether
// or not the content is interactive), then the checkbox itself (native toggle
// animates) OR the item's text (manually flip the box so it animates too).
function onClick(e) {
  const target = e.target
  // Touch first: there is no hover there, so a tap on the chip is the way in.
  if (props.mentionCards && coarsePointer()) {
    const mchip = mentionAt(target)
    if (mchip) {
      if (cardShow.value) closeCard()
      else openCard(mchip)
      return
    }
  }
  const chip = target?.closest?.('[data-task-ref]')
  if (chip) {
    e.preventDefault()
    openTaskRef(chip)
    return
  }
  const link = target?.closest?.('a[href]')
  const attId = link && attachmentIdFromHref(link.getAttribute('href'))
  if (attId) {
    e.preventDefault()
    downloadLink(attId, link.textContent.trim() || 'file')
    return
  }
  if (!props.interactive) return
  const t = target
  if (t && t.tagName === 'INPUT' && t.type === 'checkbox') {
    queueToggle(t) // let the native toggle proceed (no preventDefault) so it animates
    return
  }
  if (!t || t.closest('a, code, pre, .mention')) return
  const li = t.closest('li')
  const box = li && li.querySelector(':scope > input[type="checkbox"]')
  if (box) {
    box.checked = !box.checked // text click doesn't toggle natively — do it for the animation
    queueToggle(box)
  }
}

// ── mention hover cards ──
// Events are delegated from the root: the chips live inside v-html, so there is
// no component per chip to hang listeners on.
const cardItem = ref(null)
const cardShow = ref(false)
const cardX = ref(0)
const cardY = ref(0)
let openTimer = null
let closeTimer = null

function coarsePointer() {
  return !!window.matchMedia?.('(pointer: coarse)').matches
}

// The chip under `el`, or null: legacy TipTap-era chips carry data-type, freshly
// rendered ones carry both that and .mention. Mentions inside code blocks are
// highlighted by the regex but are not people — no card there.
function mentionAt(el) {
  if (!el?.closest) return null
  const chip = el.closest('[data-type="mention"], .mention')
  if (!chip || !root.value?.contains(chip)) return null
  return chip.closest('code, pre') ? null : chip
}

function openCard(chip) {
  const item = resolveMention(props.members, {
    id: chip.dataset.id,
    label: chip.dataset.label || chip.textContent.replace(/^@/, ''),
  })
  if (!item) return // a handle nobody owns — the card would just repeat the text
  // Anchor on the chip, not the cursor, so the card holds still while the mouse
  // moves inside the chip.
  const r = chip.getBoundingClientRect()
  cardX.value = r.left
  cardY.value = r.bottom + 6
  cardItem.value = item
  cardShow.value = true
}

function closeCard() {
  cardShow.value = false
}

function clearTimers() {
  clearTimeout(openTimer)
  clearTimeout(closeTimer)
  openTimer = null
  closeTimer = null
}

// Opening is delayed so running the mouse across a paragraph of mentions doesn't
// flash cards; closing is delayed so the cursor can travel onto the card itself.
function onOver(e) {
  if (!props.mentionCards) return
  const chip = mentionAt(e.target)
  if (!chip) return
  clearTimers()
  openTimer = setTimeout(() => openCard(chip), 300)
}

function onOut(e) {
  if (!props.mentionCards || !mentionAt(e.target)) return
  clearTimers()
  closeTimer = setTimeout(closeCard, 200)
}

function holdCard() {
  clearTimers()
}

function releaseCard() {
  clearTimers()
  closeTimer = setTimeout(closeCard, 200)
}

// A card pinned to viewport coordinates goes stale the moment anything scrolls,
// and Esc is the expected way out of a floating layer.
function onScroll() {
  if (cardShow.value) closeCard()
}

function onKeydown(e) {
  if (e.key === 'Escape') closeCard()
}

onMounted(() => {
  if (!props.mentionCards) return
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  clearTimers()
  window.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('keydown', onKeydown)
})

async function renderMermaid() {
  const el = root.value
  if (!el) return
  // marked emits ```mermaid as <pre><code class="language-mermaid">…</code></pre>.
  const blocks = el.querySelectorAll('code.language-mermaid')
  if (!blocks.length) return
  const mine = ++seq
  if (!mermaidMod) {
    try {
      mermaidMod = (await import('mermaid')).default
      mermaidMod.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        theme: theme.isDark ? 'dark' : 'default',
      })
    } catch {
      return
    }
  }
  if (mine !== seq) return
  let i = 0
  for (const codeEl of blocks) {
    const pre = codeEl.closest('pre') || codeEl
    const code = codeEl.textContent || ''
    try {
      const { svg } = await mermaidMod.render(
        `mmd-${mine}-${i++}-${Math.random().toString(36).slice(2)}`,
        code,
      )
      if (mine !== seq) return
      const wrap = document.createElement('div')
      wrap.className = 'mermaid-diagram'
      // Mermaid's output is markup, so it goes through the same sanitiser as the
      // surrounding Markdown before touching the DOM. It comes back as nodes, not
      // a string, so the vetted tree is what gets mounted (see sanitizeSvgFragment).
      const frag = sanitizeSvgFragment(svg)
      if (frag) wrap.append(frag)
      pre.replaceWith(wrap)
    } catch {
      pre.classList.add('mermaid-error')
    }
  }
}

watch(
  () => [props.source, theme.isDark],
  () => {
    // Skip the rebuild from a self-toggle (the DOM already reflects + animated it);
    // genuine source/theme changes still rebuild.
    if (skipBuild) {
      skipBuild = false
      return
    }
    build()
  },
)
onMounted(build)
</script>

<template>
  <!-- eslint-disable vue/no-v-html -->
  <div
    ref="root"
    class="md"
    :class="{ interactive }"
    v-bind="$attrs"
    @click="onClick"
    @mouseover="onOver"
    @mouseout="onOut"
    v-html="html"
  />
  <!-- eslint-enable vue/no-v-html -->
  <!-- z-index above the task modal's own layer: the card is teleported to body,
       so it would otherwise sit under the modal mask. -->
  <n-popover
    v-if="mentionCards"
    raw
    trigger="manual"
    placement="bottom-start"
    :show="cardShow"
    :x="cardX"
    :y="cardY"
    :show-arrow="false"
    :z-index="4100"
    @clickoutside="closeCard"
  >
    <MentionCard :item="cardItem" @mouseenter="holdCard" @mouseleave="releaseCard" />
  </n-popover>
</template>

<style scoped>
/* App-styled GFM task checkboxes: square, accent-tinted, aligned to the line.
   Interactive mode makes them clickable (pointer); read-only stays default. */
.md :deep(li.task-list-item),
.md :deep(li:has(> input[type='checkbox'])) {
  list-style: none;
}
.md :deep(input[type='checkbox']) {
  appearance: none;
  -webkit-appearance: none;
  width: 15px;
  height: 15px;
  margin: 0 7px 0 0;
  vertical-align: -2px;
  /* Muted (not the bright text3) so the box reads softly on dark + light. */
  border: 1.5px solid color-mix(in srgb, var(--t-text3) 45%, var(--t-border));
  border-radius: 4px;
  background: var(--t-surface);
  cursor: default;
  position: relative;
  flex: none;
  transition:
    border-color 0.15s ease,
    background-color 0.15s ease;
}
.md :deep(input[type='checkbox']:checked) {
  border-color: transparent;
  /* background-IMAGE (not the shorthand) + border-box origin so the gradient
     fills under the transparent border — the shorthand resets origin to
     padding-box, leaving a repeated-gradient "seam" ring (see gradient gotchas). */
  background-color: transparent;
  background-image: var(--t-accent-grad, var(--t-primary));
  background-origin: border-box;
}
/* Checkmark is always present; it pops in (scale + fade) on check and out on
   uncheck — the light tick animation. */
.md :deep(input[type='checkbox'])::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 50%;
  width: 4px;
  height: 8px;
  border: solid var(--t-on-primary, #fff);
  border-width: 0 2px 2px 0;
  transform: translate(-50%, -60%) rotate(45deg) scale(0.4);
  opacity: 0;
  transition:
    opacity 0.12s ease,
    transform 0.18s cubic-bezier(0.2, 0.7, 0.3, 1.55);
}
.md :deep(input[type='checkbox']:checked)::after {
  opacity: 1;
  transform: translate(-50%, -60%) rotate(45deg) scale(1);
}
.md.interactive :deep(input[type='checkbox']),
.md.interactive :deep(li:has(> input[type='checkbox'])) {
  cursor: pointer;
}
@media (prefers-reduced-motion: reduce) {
  .md :deep(input[type='checkbox']),
  .md :deep(input[type='checkbox'])::after {
    transition: none;
  }
}
.md :deep(.mermaid-diagram) {
  display: flex;
  justify-content: center;
  margin: 10px 0;
}
.md :deep(.mermaid-diagram svg) {
  max-width: 100%;
  height: auto;
}
.md :deep(.mermaid-error) {
  border-left: 3px solid #e0533d;
}
</style>
