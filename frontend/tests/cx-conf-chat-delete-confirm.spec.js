import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { NPopconfirm } from 'naive-ui'

// Deleting a line of a call's chat used to happen on a single click of a 12px
// bin tucked between the author and the timestamp (#2932).
//
// That is the one destructive action in the whole conference feature that had
// no guard: the recording, the kick, ending the session and deleting the call
// itself all sit behind an n-popconfirm, and so does the very same bin on
// Android (conf_chat_delete_confirm). And it is not a local undo — the server
// drops the row and nudges every participant to refetch, so the message is gone
// from everyone's rail with nothing to restore it from.

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

function msg(over = {}) {
  return {
    id: 'm1',
    user_id: 'me',
    user_name: 'Я',
    body: 'ошибся',
    created_at: '2026-09-03T10:00:00Z',
    attachments: [],
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  for (const fn of Object.values(api)) fn.mockReset()
  api.messages.mockResolvedValue({ data: { messages: [msg()], has_more: false } })
  api.removeMessage.mockResolvedValue({})
  vi.stubGlobal('URL', { createObjectURL: () => 'blob:fake', revokeObjectURL: () => {} })
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function rail(props = {}) {
  const w = mount(ConferenceChat, { props: { conferenceId: 'c1', ...props } })
  await flushPromises()
  return w
}

describe('deleting a chat line asks first', () => {
  it('does not touch the server on the click that opens the prompt', async () => {
    const w = await rail()

    await w.find('[data-testid="conference-chat-delete"]').trigger('click')
    await flushPromises()

    expect(api.removeMessage).not.toHaveBeenCalled()
    // …and the line is still on screen while the prompt is up.
    expect(w.findAll('[data-testid="conference-chat-message"]')).toHaveLength(1)
    w.unmount()
  })

  it('deletes once the prompt is confirmed', async () => {
    const w = await rail()

    w.findComponent(NPopconfirm).vm.$emit('positive-click')
    await flushPromises()

    expect(api.removeMessage).toHaveBeenCalledWith('m1')
    expect(w.findAll('[data-testid="conference-chat-message"]')).toHaveLength(0)
    w.unmount()
  })

  it('carries a prompt that says the line goes for everyone', async () => {
    const w = await rail()

    // The prompt's body is teleported and only exists once it is open, so the
    // trigger has to be pressed before anything can be read off it.
    await w.find('[data-testid="conference-chat-delete"]').trigger('click')
    await flushPromises()

    // Wording, not just plumbing: a bare «Удалить?» would not tell the author
    // that the line leaves every other participant's rail too.
    expect(document.body.textContent).toContain('у всех участников')
    w.unmount()
  })

  it('offers no prompt where it offers no bin', async () => {
    api.messages.mockResolvedValue({
      data: { messages: [msg({ user_id: 'other', user_name: 'Ира' })], has_more: false },
    })
    const w = await rail()

    expect(w.findComponent(NPopconfirm).exists()).toBe(false)
    w.unmount()
  })
})
