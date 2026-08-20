import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// A stand-in for pdf.js that keeps its one rule about canvases: a page cannot
// be rendered into a canvas another render() still owns. The real library
// throws exactly this, and #2752 was the viewer walking into it — the document
// drew fine and an error banner sat above it.
const busy = new Set()
const pending = []

function makeRenderTask(canvasContext) {
  if (busy.has(canvasContext)) {
    throw new Error(
      'Cannot use the same canvas during multiple render() operations. ' +
        'Use different canvas or ensure previous operations were cancelled or completed.',
    )
  }
  busy.add(canvasContext)
  let settle
  const promise = new Promise((resolve, reject) => {
    settle = { resolve, reject }
  })
  const task = {
    promise,
    cancel() {
      busy.delete(canvasContext)
      const e = new Error('Rendering cancelled')
      e.name = 'RenderingCancelledException'
      settle.reject(e)
    },
    finish() {
      busy.delete(canvasContext)
      settle.resolve()
    },
    fail(e) {
      busy.delete(canvasContext)
      settle.reject(e)
    },
  }
  pending.push(task)
  return task
}

const pdfjs = {
  GlobalWorkerOptions: { workerSrc: '' },
  getDocument: () => ({
    promise: Promise.resolve({
      numPages: 3,
      getPage: () =>
        Promise.resolve({
          getViewport: ({ scale }) => ({ width: 600 * scale, height: 800 * scale }),
          render: ({ canvasContext }) => makeRenderTask(canvasContext),
        }),
      destroy: () => {},
    }),
  }),
}

vi.mock('pdfjs-dist', () => pdfjs)
vi.mock('pdfjs-dist/build/pdf.worker.min.mjs?url', () => ({ default: '/assets/worker.mjs' }))

const DocPdf = (await import('@/components/documents/DocPdf.vue')).default

const PROPS = { node: { attrs: { src: '/api/documents/asset?doc=1', name: 'a.pdf', size: 2048 } } }
// The viewer reaches its canvases through document.querySelector (the pages are
// node-view children, not refs), so a detached wrapper renders nothing at all.
const MOUNT = { props: PROPS, attachTo: document.body }

// Every canvas answers getContext with itself, so the fake can tell one page's
// canvas from another's. jsdom's own getContext returns null for all of them —
// the concurrency rule would be untestable, every page sharing one "context".
function stubCanvases() {
  return vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(function ctx() {
    return this
  })
}

async function drain(limit = 30) {
  for (let i = 0; i < limit && pending.length; i += 1) {
    pending.shift().finish()
    await flushPromises()
  }
}

describe('DocPdf viewer', () => {
  let wrapper
  let ctxSpy

  beforeEach(() => {
    busy.clear()
    pending.length = 0
    ctxSpy = stubCanvases()
  })

  afterEach(() => {
    wrapper?.unmount()
    ctxSpy.mockRestore()
  })

  it('queues a second render pass instead of drawing into a busy canvas', async () => {
    wrapper = mount(DocPdf, MOUNT)
    await flushPromises()
    // The first pass is parked on page 1 — the window a zoom, a scroll or the
    // ResizeObserver's first callback lands in.
    expect(pending).toHaveLength(1)

    await wrapper.find('[aria-label="Увеличить"]').trigger('click')
    await flushPromises()
    await drain()

    expect(wrapper.find('[data-testid="doc-pdf-error"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-pdf-page]')).toHaveLength(3)
  })

  // The banner in the report was the *symptom*; the invariant behind it is that
  // pdf.js is never handed a canvas twice over. Asserting it directly keeps the
  // test honest if the error text or the reporting path ever changes.
  it('never has two renders in flight at once', async () => {
    wrapper = mount(DocPdf, MOUNT)
    await flushPromises()

    let peak = busy.size
    for (let i = 0; i < 3; i += 1) {
      await wrapper.find('[aria-label="Увеличить"]').trigger('click')
      await flushPromises()
      peak = Math.max(peak, busy.size)
      pending.shift()?.finish()
      await flushPromises()
      peak = Math.max(peak, busy.size)
    }
    await drain()
    expect(peak).toBe(1)
  })

  it('still reports a render that genuinely fails', async () => {
    wrapper = mount(DocPdf, MOUNT)
    await flushPromises()

    pending.shift().fail(new Error('битый поток'))
    await flushPromises()

    expect(wrapper.find('[data-testid="doc-pdf-error"]').text()).toContain('битый поток')
  })

  it('cancels the page it is drawing when the block goes away', async () => {
    wrapper = mount(DocPdf, MOUNT)
    await flushPromises()
    const task = pending[0]

    wrapper.unmount()
    wrapper = null
    await flushPromises()

    expect(busy.size).toBe(0)
    await expect(task.promise).rejects.toThrow('Rendering cancelled')
  })
})
