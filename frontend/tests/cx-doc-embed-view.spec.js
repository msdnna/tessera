import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, computed } from 'vue'
import { setActivePinia, createPinia } from 'pinia'

// #2894 rework. The embed page had unit tests for every pure rule it uses and
// still shipped dead: `onMounted` called `auth.setToken`, which the store never
// returned, so the very first statement after the params gate threw. The view
// never reached `load()`, `loading` stayed true, and — the part that made it
// hard to diagnose — the bridge was installed *after* the throwing line, so the
// host was not even told it had failed. The phone sat on "opening…" forever.
//
// Pure-function specs cannot catch that shape of bug: the contract they assert
// is fine, it is the wiring around it that is missing. So this file mounts the
// real view and asserts the two things the host actually depends on — the page
// reaches a terminal state, and it reports that state back.

const apiMock = vi.hoisted(() => ({
  documents: {
    bySlug: vi.fn(),
    get: vi.fn(),
    updateContent: vi.fn(),
    uploadAsset: vi.fn(),
    uploadPdf: vi.fn(),
  },
  setAccessToken: vi.fn(),
  setRefreshHook: vi.fn(),
  // Pulled in by the theme store, which the view drives from the host's theme.
  getAccessToken: vi.fn(() => ''),
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
}))
vi.mock('@/api', () => apiMock)

const routeMock = vi.hoisted(() => ({ value: { query: {}, params: {} } }))
vi.mock('vue-router', () => ({ useRoute: () => routeMock.value }))

// The collaboration layer is out of scope here: it opens a websocket and is
// covered by its own specs. Stub it so what remains under test is the view.
vi.mock('@/composables/useDocAutosave', () => ({
  useDocAutosave: () => ({
    saving: ref(false),
    dirty: ref(false),
    conflict: ref(false),
    error: ref(''),
    schedule: vi.fn(),
    flush: vi.fn(),
    cancel: vi.fn(),
    resolveConflict: vi.fn(),
  }),
}))
vi.mock('@/composables/useDocPresence', () => ({
  useDocPresence: () => ({
    foreignLocks: ref([]),
    commentsNudge: ref(0),
    contentNudge: ref(0),
    connId: ref('conn-1'),
    open: vi.fn(),
    close: vi.fn(),
    acquire: vi.fn(),
  }),
}))
vi.mock('@/composables/useDocComments', () => ({
  useDocComments: () => ({
    openCounts: computed(() => ({})),
    load: vi.fn(),
    open: vi.fn(() => Promise.resolve()),
    close: vi.fn(),
    setDoc: vi.fn(),
  }),
}))

const { default: DocEmbedView } = await import('@/views/DocEmbedView.vue')

const PARA = {
  type: 'doc',
  content: [{ type: 'paragraph', attrs: { id: 'b1' }, content: [{ type: 'text', text: 'текст' }] }],
}

function mountEmbed(query = { token: 'tok', ws: 'ws-1' }) {
  routeMock.value = { query, params: { slug: 'plan' } }
  return mount(DocEmbedView, {
    global: {
      stubs: { DocEditor: { name: 'DocEditor', template: '<div class="editor-stub" />' } },
    },
  })
}

describe('DocEmbedView', () => {
  let wrapper
  let calls

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    calls = []
    // The Android bridge, as the WebView injects it: a @JavascriptInterface
    // object whose methods take strings.
    window.AndroidDoc = new Proxy(
      {},
      {
        get:
          (_, name) =>
          (...args) =>
            calls.push({ name: String(name), args }),
        has: () => true,
      },
    )
    apiMock.documents.bySlug.mockResolvedValue({
      data: { id: 'd1', title: 'План', content: PARA, updated_at: '2026-09-07T00:00:00Z' },
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    delete window.AndroidDoc
  })

  it('installs the host session and gets as far as the editor', async () => {
    wrapper = mountEmbed()
    await flushPromises()

    // Neither the axios layer nor the store may be skipped: the requests carry
    // the token, and the components decide what to draw from the store.
    expect(apiMock.setAccessToken).toHaveBeenCalledWith('tok')
    const { useAuthStore } = await import('@/stores/auth')
    expect(useAuthStore().token).toBe('tok')

    expect(apiMock.documents.bySlug).toHaveBeenCalledWith('ws-1', 'plan')
    expect(wrapper.find('.editor-stub').exists()).toBe(true)
    expect(wrapper.find('[data-testid="doc-embed-error"]').exists()).toBe(false)
    expect(calls.map((c) => c.name)).toContain('onReady')
  })

  it('exposes the host→page bridge once the page is up', async () => {
    wrapper = mountEmbed()
    await flushPromises()

    // The host pushes refreshed tokens through this object. It is installed in
    // the same onMounted that sets the session, so anything that throws earlier
    // takes it down with it — and then a token that expires mid-edit can never
    // be replaced.
    expect(typeof window.tesseraEmbed?.setToken).toBe('function')
    window.tesseraEmbed.setToken('fresh-tok')
    expect(apiMock.setAccessToken).toHaveBeenCalledWith('fresh-tok')
    const { useAuthStore } = await import('@/stores/auth')
    expect(useAuthStore().token).toBe('fresh-tok')
  })

  it('tells the host when it was launched without a session', async () => {
    wrapper = mountEmbed({ ws: 'ws-1' })
    await flushPromises()

    // Silence is the failure this whole file is about: the host has no way to
    // distinguish "still loading" from "will never load", so the page must say
    // so rather than leave the spinner up.
    expect(wrapper.find('[data-testid="doc-embed-error"]').exists()).toBe(true)
    expect(apiMock.documents.bySlug).not.toHaveBeenCalled()
    expect(calls.find((c) => c.name === 'onError')).toBeTruthy()
  })

  it('reports a failed load instead of staying on the spinner', async () => {
    apiMock.documents.bySlug.mockRejectedValue(new Error('нет доступа'))
    wrapper = mountEmbed()
    await flushPromises()

    expect(wrapper.find('[data-testid="doc-embed-error"]').exists()).toBe(true)
    expect(calls.find((c) => c.name === 'onError')).toBeTruthy()
  })
})
