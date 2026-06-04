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
})

const theme = useThemeStore()
const root = ref(null)
const html = ref('')
let mermaidMod = null
let seq = 0 // guards against out-of-order async renders

function build() {
  const out = renderRich(props.source, props.members)
  html.value = out || (props.empty ? `<em class="rc-empty">${props.empty}</em>` : '')
  nextTick(renderMermaid)
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

watch(() => [props.source, theme.isDark], build)
onMounted(build)
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -->
  <div ref="root" class="md" v-html="html" />
</template>

<style scoped>
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
