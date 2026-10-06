import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

// Switching from one call to another while the rail is still fetching (#2932).
//
// `ConferenceRoom` carries no `:key`, so the rail is the same component across
// calls — only `conferenceId` changes. The watcher inside it clears the log, but
// that is only half of an invalidation: the page already in flight belongs to the
// call the reader has left, and when it lands `mergeChatTail` folds it into the
// new call's log. Everything in it is older than the new call's messages, so it
// settles *above* them and stays there for the rest of the call — another
// conversation, drawn as if it were this one.
//
// Android has had the other half from the start (`ConferenceChatViewModel`
// compares the id back after the await); these tests are that comparison, pinned
// for the web rail.

const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => toast }
})

const api = {
  messages: vi.fn(),
  postMessage: vi.fn(),
  postMessageWithFiles: vi.fn(),
  removeMessage: vi.fn(),
  attachment: vi.fn(),
}
vi.mock('@/api', () => ({
  conferences: api,
  getAccessToken: () => 'jwt',
  apiBaseURL: () => '/api',
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { id: 'me' }, isAdmin: false }),
}))

const { default: ConferenceChat } = await import('@/components/conference/ConferenceChat.vue')

function msg(id, at, body) {
  return { id, user_id: 'them', user_name: 'Кто-то', body, created_at: at, attachments: [] }
}

function page(messages, hasMore = false) {
  return { data: { messages, has_more: hasMore } }
}

/** A promise plus the handle that settles it, so a page can be held in flight. */
function deferred() {
  let resolve
  const promise = new Promise((r) => {
    resolve = r
  })
  return { promise, resolve }
}

// The call that was left, and the one that was opened. The old call's messages
// are the older ones — which is exactly why a stale page survives the fold.
const OLD = [msg('old-1', '2026-09-03T10:00:00Z', 'разговор про прошлый звонок')]
const NEW = [msg('new-1', '2026-09-03T12:00:00Z', 'здравствуйте')]

beforeEach(() => {
  setActivePinia(createPinia())
  for (const fn of Object.values(api)) fn.mockReset()
  api.messages.mockResolvedValue(page([]))
  api.postMessage.mockResolvedValue({ data: msg('sent', '2026-09-03T10:30:00Z', 'ушло') })
  api.attachment.mockResolvedValue({ data: new Blob(['bytes']) })
  vi.stubGlobal('URL', { createObjectURL: () => 'blob:fake', revokeObjectURL: () => {} })
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

/** The bodies on screen, top to bottom. */
function bodies(w) {
  return w.findAll('[data-testid="conference-chat-message"] .text').map((n) => n.text())
}

const older = (w) => w.find('[data-testid="conference-chat-older"]')

/** Mounts the rail on `id` with its first page held in flight. */
function railWithHeldPage(id) {
  const held = deferred()
  api.messages.mockReturnValueOnce(held.promise)
  const w = mount(ConferenceChat, { props: { conferenceId: id, visible: true } })
  return { w, held }
}

describe('leaving a call while its chat is still loading', () => {
  it('drops the page the call we left was fetching', async () => {
    const { w, held } = railWithHeldPage('c-old')
    await flushPromises()

    // The reader opens the other call before the first page arrives.
    api.messages.mockResolvedValue(page(NEW))
    await w.setProps({ conferenceId: 'c-new' })
    await flushPromises()

    // Only now does the call we left answer.
    held.resolve(page(OLD))
    await flushPromises()

    expect(bodies(w)).toEqual(['здравствуйте'])
    w.unmount()
  })

  it('does not let a stale page decide whether there is older text', async () => {
    const { w, held } = railWithHeldPage('c-old')
    await flushPromises()

    api.messages.mockResolvedValue(page(NEW, false))
    await w.setProps({ conferenceId: 'c-new' })
    await flushPromises()

    // The call we left had history behind its page; the one we opened does not.
    held.resolve(page(OLD, true))
    await flushPromises()

    expect(older(w).exists()).toBe(false)
    w.unmount()
  })

  it('drops an older page asked of the call we left', async () => {
    api.messages.mockResolvedValue(page(NEW, true))
    const w = mount(ConferenceChat, { props: { conferenceId: 'c-old', visible: true } })
    await flushPromises()

    const held = deferred()
    api.messages.mockReturnValueOnce(held.promise)
    await older(w).trigger('click')
    await flushPromises()

    api.messages.mockResolvedValue(page(NEW))
    await w.setProps({ conferenceId: 'c-new' })
    await flushPromises()

    held.resolve(page(OLD))
    await flushPromises()

    expect(bodies(w)).toEqual(['здравствуйте'])
    w.unmount()
  })

  // The empty-state line is a claim about the call on screen. A stale answer
  // that clears the spinner makes the rail say "чат пуст" about a call whose
  // page nobody has read yet.
  it('does not call the new call empty on the strength of a stale answer', async () => {
    const { w, held } = railWithHeldPage('c-old')
    await flushPromises()

    const second = deferred()
    api.messages.mockReturnValueOnce(second.promise)
    await w.setProps({ conferenceId: 'c-new' })
    await flushPromises()

    held.resolve(page([]))
    await flushPromises()
    expect(w.find('.none').exists()).toBe(false)

    second.resolve(page(NEW))
    await flushPromises()
    expect(bodies(w)).toEqual(['здравствуйте'])
    w.unmount()
  })

  it('does not drop a message sent in the call we left into this one', async () => {
    api.messages.mockResolvedValue(page(NEW))
    const w = mount(ConferenceChat, { props: { conferenceId: 'c-old', visible: true } })
    await flushPromises()

    const held = deferred()
    api.postMessage.mockReturnValueOnce(held.promise)
    await w.find('[data-testid="conference-chat-input"] textarea').setValue('пока')
    await w.find('[data-testid="conference-chat-send"]').trigger('click')
    await flushPromises()

    await w.setProps({ conferenceId: 'c-new' })
    await flushPromises()

    held.resolve({ data: msg('sent', '2026-09-03T10:30:00Z', 'пока') })
    await flushPromises()

    expect(bodies(w)).toEqual(['здравствуйте'])
    w.unmount()
  })
})
