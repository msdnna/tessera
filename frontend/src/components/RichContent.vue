<script setup>
import { ref, watch, onMounted, nextTick } from 'vue'
import { renderRich } from '@/utils/markdown'
import { useThemeStore } from '@/stores/theme'

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
})
const emit = defineEmits(['toggle'])

const theme = useThemeStore()
const root = ref(null)
const html = ref('')
let mermaidMod = null
let seq = 0 // guards against out-of-order async renders

function build() {
  let out = renderRich(props.source, props.members)
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
// Delegate clicks: the checkbox itself (native toggle animates) OR the item's
// text (manually flip the box so it animates too), then toggle the markdown.
function onClick(e) {
  if (!props.interactive) return
  const t = e.target
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
      const { svg } = await mermaidMod.render(`mmd-${mine}-${i++}-${Math.random().toString(36).slice(2)}`, code)
      if (mine !== seq) return
      const wrap = document.createElement('div')
      wrap.className = 'mermaid-diagram'
      wrap.innerHTML = svg
      pre.replaceWith(wrap)
    } catch {
      pre.classList.add('mermaid-error')
    }
  }
}

watch(() => [props.source, theme.isDark], () => {
  // Skip the rebuild from a self-toggle (the DOM already reflects + animated it);
  // genuine source/theme changes still rebuild.
  if (skipBuild) {
    skipBuild = false
    return
  }
  build()
})
onMounted(build)
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div ref="root" class="md" :class="{ interactive }" @click="onClick" v-html="html" />
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
