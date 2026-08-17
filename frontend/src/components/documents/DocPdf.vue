<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NSpin } from 'naive-ui'
import {
  ChevronBackOutline,
  ChevronForwardOutline,
  DocumentTextOutline,
  DownloadOutline,
  RemoveOutline,
  AddOutline,
} from '@vicons/ionicons5'
import { NodeViewWrapper } from '@tiptap/vue-3'
import { clampPage, fitScale, formatFileSize, pagesAround } from '@/utils/docPdf'

// The node view for a `pdfEmbed` block (#2733). pdf.js and its worker are
// imported lazily inside onMounted rather than at module scope: the library is
// close to a megabyte, and a document without a PDF in it must not pay for the
// viewer. That also keeps the schema unit tests free of a canvas — they import
// this file through the extension, but never mount it.

const props = defineProps({
  node: { type: Object, required: true },
  editor: { type: Object, default: null },
  selected: { type: Boolean, default: false },
})

const src = computed(() => props.node?.attrs?.src || '')
const fileName = computed(() => props.node?.attrs?.name || 'документ.pdf')
const fileSize = computed(() => formatFileSize(props.node?.attrs?.size))

const scroller = ref(null)
const loading = ref(true)
const error = ref('')
const total = ref(0)
const page = ref(1)
// Page geometry at scale 1, used to size the placeholders for pages that have
// not been rendered yet. Without it the scroll height grows as pages arrive and
// the reader is thrown around the document mid-scroll.
const baseSize = ref({ width: 0, height: 0 })
const zoom = ref(1)
const availWidth = ref(0)

// Rendered bitmaps, keyed by page number. A Map rather than an array because
// only a window around the current page is ever populated.
const rendered = ref(new Map())

let pdfDoc = null
let resizeObserver = null
let destroyed = false
// The render task per page, so a scroll that outruns rendering can cancel work
// that is no longer wanted instead of queueing it behind the current viewport.
const tasks = new Map()

const scale = computed(() => {
  const fit = fitScale(baseSize.value.width, availWidth.value)
  return fit * zoom.value
})

const pageBoxes = computed(() => {
  const out = []
  const { width, height } = baseSize.value
  for (let i = 1; i <= total.value; i += 1) {
    out.push({
      number: i,
      width: Math.round(width * scale.value),
      height: Math.round(height * scale.value),
    })
  }
  return out
})

function setError(e) {
  // A failed fetch of a signed asset is by far the likeliest cause (an expired
  // link, or a document opened from a stale tab), and "не удалось открыть файл"
  // with no download button is a dead end — the link stays available below.
  error.value = e?.message ? `Не удалось открыть PDF: ${e.message}` : 'Не удалось открыть PDF'
  loading.value = false
}

async function renderPage(pdfjsPage, number) {
  const canvas = document.querySelector(`[data-pdf-page="${number}"] canvas`)
  if (!canvas) return
  const viewport = pdfjsPage.getViewport({ scale: scale.value })
  // Backing store at device resolution, CSS box at layout resolution: without
  // the ratio a page is legible on a laptop and mush on a phone.
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.floor(viewport.width * dpr)
  canvas.height = Math.floor(viewport.height * dpr)
  canvas.style.width = `${Math.floor(viewport.width)}px`
  canvas.style.height = `${Math.floor(viewport.height)}px`
  const task = pdfjsPage.render({
    canvasContext: canvas.getContext('2d'),
    viewport,
    transform: dpr === 1 ? null : [dpr, 0, 0, dpr, 0, 0],
  })
  tasks.set(number, task)
  try {
    await task.promise
    rendered.value.set(number, true)
    rendered.value = new Map(rendered.value)
  } catch (e) {
    // RenderingCancelledException is the normal outcome of scrolling past a
    // page, not a failure worth showing.
    if (e?.name !== 'RenderingCancelledException') throw e
  } finally {
    tasks.delete(number)
  }
}

async function renderWindow() {
  if (!pdfDoc || destroyed) return
  const wanted = pagesAround(page.value, total.value)
  const keep = new Set(wanted)
  for (const [number, task] of tasks) {
    if (!keep.has(number)) task.cancel()
  }
  for (const [number] of rendered.value) {
    if (!keep.has(number)) rendered.value.delete(number)
  }
  await nextTick()
  for (const number of wanted) {
    if (destroyed) return
    try {
      const pdfjsPage = await pdfDoc.getPage(number)
      await renderPage(pdfjsPage, number)
    } catch (e) {
      setError(e)
      return
    }
  }
}

function onScroll() {
  const el = scroller.value
  if (!el || !total.value) return
  // Which page is under the middle of the viewport. Cheaper and steadier than
  // an IntersectionObserver per page when pages are re-sized on zoom.
  const boxes = el.querySelectorAll('[data-pdf-page]')
  const mid = el.scrollTop + el.clientHeight / 2
  let current = 1
  for (const box of boxes) {
    if (box.offsetTop <= mid) current = Number(box.dataset.pdfPage) || current
  }
  if (current !== page.value) {
    page.value = current
    renderWindow()
  }
}

function goTo(next) {
  const target = clampPage(next, total.value)
  page.value = target
  const box = scroller.value?.querySelector(`[data-pdf-page="${target}"]`)
  if (box && scroller.value) scroller.value.scrollTop = box.offsetTop
  renderWindow()
}

function measure() {
  const el = scroller.value
  if (!el) return
  // 24px of gutter so a page does not sit flush against the scrollbar.
  availWidth.value = Math.max(0, el.clientWidth - 24)
}

onMounted(async () => {
  if (!src.value) {
    setError(new Error('в блоке нет ссылки на файл'))
    return
  }
  measure()
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      const before = availWidth.value
      measure()
      if (before !== availWidth.value) renderWindow()
    })
    if (scroller.value) resizeObserver.observe(scroller.value)
  }
  try {
    const pdfjs = await import('pdfjs-dist')
    // ?url rather than new URL(...): the bare specifier form is not resolved by
    // the bundler, and a worker that fails to load makes pdf.js fall back to
    // parsing on the main thread — the page then freezes on a large file
    // instead of failing, which is much harder to attribute.
    const { default: workerUrl } = await import('pdfjs-dist/build/pdf.worker.min.mjs?url')
    pdfjs.GlobalWorkerOptions.workerSrc = workerUrl
    // The asset route is public-but-signed and sets no CORS headers, so the
    // request must be same-origin and must not carry credentials.
    pdfDoc = await pdfjs.getDocument({ url: src.value, withCredentials: false }).promise
    if (destroyed) return
    total.value = pdfDoc.numPages
    const first = await pdfDoc.getPage(1)
    const viewport = first.getViewport({ scale: 1 })
    baseSize.value = { width: viewport.width, height: viewport.height }
    loading.value = false
    await renderWindow()
  } catch (e) {
    setError(e)
  }
})

onBeforeUnmount(() => {
  destroyed = true
  for (const task of tasks.values()) task.cancel()
  tasks.clear()
  resizeObserver?.disconnect()
  // Frees the worker; without it every opened document leaves one behind and a
  // long session accumulates them.
  pdfDoc?.destroy?.()
  pdfDoc = null
})

watch(zoom, () => renderWindow())
</script>

<template>
  <NodeViewWrapper
    class="doc-pdf"
    :class="{ 'is-selected': selected }"
    data-pdf-embed
    data-testid="doc-pdf"
  >
    <header class="doc-pdf__bar">
      <NIcon :component="DocumentTextOutline" class="doc-pdf__icon" />
      <span class="doc-pdf__name" :title="fileName">{{ fileName }}</span>
      <span v-if="fileSize" class="doc-pdf__size">{{ fileSize }}</span>
      <span class="doc-pdf__spacer" />
      <template v-if="total">
        <NButton
          quaternary
          size="tiny"
          :disabled="page <= 1"
          aria-label="Предыдущая страница"
          @click="goTo(page - 1)"
        >
          <NIcon :component="ChevronBackOutline" />
        </NButton>
        <span class="doc-pdf__pages" data-testid="doc-pdf-pages"> {{ page }} / {{ total }} </span>
        <NButton
          quaternary
          size="tiny"
          :disabled="page >= total"
          aria-label="Следующая страница"
          @click="goTo(page + 1)"
        >
          <NIcon :component="ChevronForwardOutline" />
        </NButton>
        <NButton
          quaternary
          size="tiny"
          :disabled="zoom <= 0.5"
          aria-label="Уменьшить"
          @click="zoom = Math.max(0.5, zoom - 0.25)"
        >
          <NIcon :component="RemoveOutline" />
        </NButton>
        <NButton
          quaternary
          size="tiny"
          :disabled="zoom >= 3"
          aria-label="Увеличить"
          @click="zoom = Math.min(3, zoom + 0.25)"
        >
          <NIcon :component="AddOutline" />
        </NButton>
      </template>
      <NButton quaternary size="tiny" tag="a" :href="src" :download="fileName" aria-label="Скачать">
        <NIcon :component="DownloadOutline" />
      </NButton>
    </header>

    <div v-if="error" class="doc-pdf__error" data-testid="doc-pdf-error">{{ error }}</div>

    <div ref="scroller" class="doc-pdf__scroll" contenteditable="false" @scroll.passive="onScroll">
      <div v-if="loading" class="doc-pdf__loading"><NSpin size="small" /></div>
      <div
        v-for="box in pageBoxes"
        :key="box.number"
        class="doc-pdf__page"
        :data-pdf-page="box.number"
        :style="{ width: `${box.width}px`, height: `${box.height}px` }"
      >
        <canvas v-show="rendered.has(box.number)" />
        <span v-if="!rendered.has(box.number)" class="doc-pdf__placeholder">
          Стр. {{ box.number }}
        </span>
      </div>
    </div>
  </NodeViewWrapper>
</template>

<style scoped>
.doc-pdf {
  border: 1px solid var(--t-border);
  border-radius: 8px;
  overflow: hidden;
  margin: 12px 0;
  background: var(--t-surface);
}

.doc-pdf.is-selected {
  border-color: var(--t-primary);
}

.doc-pdf__bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--t-border);
  font-size: 13px;
  color: var(--t-text2);
}

.doc-pdf__icon {
  color: var(--t-text3);
}

.doc-pdf__name {
  color: var(--t-text1);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 40%;
}

.doc-pdf__size,
.doc-pdf__pages {
  color: var(--t-text3);
  font-variant-numeric: tabular-nums;
}

.doc-pdf__spacer {
  flex: 1;
}

.doc-pdf__error {
  padding: 12px;
  color: var(--t-error, #d03050);
  font-size: 13px;
}

/* The viewer scrolls inside the block rather than growing the page: a 300-page
   scan embedded halfway down a document would otherwise bury everything after
   it. */
.doc-pdf__scroll {
  max-height: 70vh;
  overflow: auto;
  padding: 12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  background: var(--t-bg, #f5f5f5);
}

.doc-pdf__loading {
  padding: 24px;
}

.doc-pdf__page {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #fff;
  box-shadow: 0 1px 4px rgb(0 0 0 / 18%);
  flex: none;
}

.doc-pdf__placeholder {
  color: #999;
  font-size: 12px;
}
</style>
