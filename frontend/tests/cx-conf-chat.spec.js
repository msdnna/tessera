import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { NPopconfirm } from 'naive-ui'

// The chat rail of a call (#2864, subtask #2873).
//
// Three things here are worth a test rather than a reading, because each of
// them fails silently:
//
//   - the socket nudge is payload-free, so the rail's only correct response is
//     to refetch. A rail that trusted the frame would show nothing at all.
//   - an image is fetched with our bearer credential and rendered from a blob,
//     never through an <img src> at the API. If that ever regresses to a plain
//     src, the pictures still appear on the web (same-origin cookie) and break
//     only in the desktop client, which is the worst possible place to find out.
//   - the object URLs are revoked on unmount, or every screenshot of every call
//     stays pinned in memory for the life of the tab.

// useMessage() throws without an <n-message-provider>; stub only that, as the
// other component specs do.
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

// The auth store decides whose messages carry a delete button; a stub keeps this
// file about the rail rather than about pinia.
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { id: 'me' }, isAdmin: false }),
}))

const { default: ConferenceChat } = await import('@/components/conference/ConferenceChat.vue')

const revoked = []
const created = []

function msg(over = {}) {
  return {
    id: over.id || 'm1',
    user_id: 'me',
    user_name: 'Я',
    body: 'привет',
    created_at: '2026-09-03T10:00:00Z',
    attachments: [],
    ...over,
  }
}

function page(messages, hasMore = false) {
  return { data: { messages, has_more: hasMore } }
}

beforeEach(() => {
  // useFormat reads the theme store for the date/time preferences.
  setActivePinia(createPinia())
  revoked.length = 0
  created.length = 0
  for (const fn of Object.values(api)) fn.mockReset()
  api.messages.mockResolvedValue(page([]))
  api.postMessage.mockResolvedValue({ data: msg({ id: 'new' }) })
  api.postMessageWithFiles.mockResolvedValue({ data: msg({ id: 'new-f' }) })
  api.removeMessage.mockResolvedValue({})
  api.attachment.mockResolvedValue({ data: new Blob(['bytes']) })
  vi.stubGlobal('URL', {
    createObjectURL: (b) => {
      const url = `blob:fake-${created.length}`
      created.push({ url, blob: b })
      return url
    },
    revokeObjectURL: (u) => revoked.push(u),
  })
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function rail(props = {}) {
  const w = mount(ConferenceChat, {
    props: { conferenceId: 'c1', ...props },
  })
  await flushPromises()
  return w
}

describe('loading the conversation', () => {
  it('asks for the newest page when it opens', async () => {
    const w = await rail()
    expect(api.messages).toHaveBeenCalledWith('c1', { limit: 50 })
    w.unmount()
  })

  // The rail's own note says a 403/404 means the call went away under us and
  // the screen around it already says so. That test read `e.response.status`,
  // and the api client rejects with a plain Error carrying `status` — no
  // `response` at all (#2932) — so the check never matched and the second toast
  // was shown every time.
  function refusal(status) {
    const e = new Error('нет доступа')
    e.status = status
    e.offline = false
    return e
  }

  it('stays quiet when the call it belongs to is gone', async () => {
    toast.error.mockClear()
    api.messages.mockRejectedValue(refusal(404))
    const w = await rail()
    expect(toast.error).not.toHaveBeenCalled()
    w.unmount()
  })

  it('stays quiet when the call is no longer ours to read', async () => {
    toast.error.mockClear()
    api.messages.mockRejectedValue(refusal(403))
    const w = await rail()
    expect(toast.error).not.toHaveBeenCalled()
    w.unmount()
  })

  it('still says so when the refusal is not about the call going away', async () => {
    toast.error.mockClear()
    api.messages.mockRejectedValue(refusal(500))
    const w = await rail()
    expect(toast.error).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  it('opens on the newest message once the rail actually shows it', async () => {
    // The room keeps this component alive behind v-show so switching tabs does
    // not refetch — which means the first load lands on a display:none element,
    // where scrollTop cannot move. Scrolling only on load left the chat sitting
    // on the oldest message every time it was opened.
    const w = await rail({ visible: false })
    const log = w.get('[data-testid="conference-chat-log"]').element
    Object.defineProperty(log, 'scrollHeight', { value: 900, configurable: true })
    Object.defineProperty(log, 'clientHeight', { value: 300, configurable: true })
    log.scrollTop = 0

    await w.setProps({ visible: true })
    await flushPromises()
    expect(log.scrollTop).toBe(900)
    w.unmount()
  })

  it('refetches on a nudge instead of trusting the frame', async () => {
    api.messages.mockResolvedValue(page([msg({ body: 'первое' })]))
    const w = await rail()
    expect(w.text()).toContain('первое')

    // The room socket says only "the chat changed" — see the note at the top of
    // the composable. Anything the rail wants, it has to go and read.
    api.messages.mockResolvedValue(
      page([msg({ body: 'первое' }), msg({ id: 'm2', body: 'второе' })]),
    )
    await w.setProps({ nudge: 1 })
    await flushPromises()

    expect(api.messages).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain('второе')
    w.unmount()
  })

  it('pages back with a (created_at, id) cursor, not an offset', async () => {
    api.messages.mockResolvedValue(
      page([msg({ id: 'm5', created_at: '2026-09-03T10:05:00Z' })], true),
    )
    const w = await rail()

    expect(w.find('[data-testid="conference-chat-older"]').exists()).toBe(true)
    api.messages.mockResolvedValue(page([msg({ id: 'm4', body: 'раньше' })]))
    await w.find('[data-testid="conference-chat-older"]').trigger('click')
    await flushPromises()

    expect(api.messages).toHaveBeenLastCalledWith('c1', {
      limit: 50,
      before_at: '2026-09-03T10:05:00Z',
      before_id: 'm5',
    })
    // Older messages go above the ones already on screen, not below them.
    const rows = w.findAll('[data-testid="conference-chat-message"]')
    expect(rows[0].text()).toContain('раньше')
    w.unmount()
  })

  it('keeps the history it paged back through when the next nudge lands', async () => {
    // The nudge fires on every line anybody types. Answering it by *replacing*
    // the log with the newest page threw away the older pages the reader had
    // just loaded: in a busy call the message being re-read vanished mid-
    // sentence and the view landed somewhere arbitrary — the very thing the
    // "don't scroll unless at the bottom" rule exists to prevent (#2932).
    api.messages.mockResolvedValue(
      page([msg({ id: 'm5', body: 'последнее', created_at: '2026-09-03T10:05:00Z' })], true),
    )
    const w = await rail()

    api.messages.mockResolvedValue(
      page([msg({ id: 'm4', body: 'раньше', created_at: '2026-09-03T10:04:00Z' })], false),
    )
    await w.find('[data-testid="conference-chat-older"]').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('раньше')

    api.messages.mockResolvedValue(
      page(
        [
          msg({ id: 'm5', body: 'последнее', created_at: '2026-09-03T10:05:00Z' }),
          msg({ id: 'm6', body: 'новое', created_at: '2026-09-03T10:06:00Z' }),
        ],
        true,
      ),
    )
    await w.setProps({ nudge: 1 })
    await flushPromises()

    const rows = w.findAll('[data-testid="conference-chat-message"]')
    expect(rows.map((r) => r.text()).join(' | ')).toContain('раньше')
    expect(rows).toHaveLength(3)
    expect(w.text()).toContain('новое')
    // …and the page's own has_more must not resurrect the "load older" button:
    // everything older is already on screen, the last paging said so.
    expect(w.find('[data-testid="conference-chat-older"]').exists()).toBe(false)
    w.unmount()
  })

  it('takes a message deleted by someone else off the rail', async () => {
    // The fold must not become "append whatever is new": inside the fetched
    // window the page is the truth, deletions included.
    api.messages.mockResolvedValue(
      page([
        msg({ id: 'm1', body: 'первое', created_at: '2026-09-03T10:00:00Z' }),
        msg({ id: 'm2', body: 'ошибка', created_at: '2026-09-03T10:01:00Z' }),
      ]),
    )
    const w = await rail()
    expect(w.text()).toContain('ошибка')

    api.messages.mockResolvedValue(
      page([msg({ id: 'm1', body: 'первое', created_at: '2026-09-03T10:00:00Z' })]),
    )
    await w.setProps({ nudge: 1 })
    await flushPromises()

    expect(w.text()).not.toContain('ошибка')
    w.unmount()
  })

  it('starts the log over when the rail is pointed at another call', async () => {
    api.messages.mockResolvedValue(
      page([msg({ id: 'm1', body: 'в первом звонке', created_at: '2026-09-03T10:00:00Z' })]),
    )
    const w = await rail()

    api.messages.mockResolvedValue(
      page([msg({ id: 'm9', body: 'во втором', created_at: '2026-09-04T10:00:00Z' })]),
    )
    await w.setProps({ conferenceId: 'c2' })
    await flushPromises()

    expect(w.text()).not.toContain('в первом звонке')
    expect(w.text()).toContain('во втором')
    w.unmount()
  })

  it('offers no "load older" when the whole chat fits', async () => {
    api.messages.mockResolvedValue(page([msg()], false))
    const w = await rail()
    expect(w.find('[data-testid="conference-chat-older"]').exists()).toBe(false)
    w.unmount()
  })
})

describe('sending', () => {
  it('posts plain text without a multipart encoder', async () => {
    const w = await rail()
    await w.find('[data-testid="conference-chat-input"] textarea').setValue('  привет  ')
    await w.find('[data-testid="conference-chat-send"]').trigger('click')
    await flushPromises()

    expect(api.postMessage).toHaveBeenCalledWith('c1', 'привет')
    expect(api.postMessageWithFiles).not.toHaveBeenCalled()
    w.unmount()
  })

  it('refuses to send nothing', async () => {
    const w = await rail()
    // Blank drafts are the common case for a stray Enter, and a chat that
    // accepts them fills with empty rows.
    await w.find('[data-testid="conference-chat-input"] textarea').setValue('   ')
    expect(w.find('[data-testid="conference-chat-send"]').attributes('disabled')).toBeDefined()
    w.unmount()
  })

  it('appends its own message rather than refetching the page', async () => {
    const w = await rail()
    api.postMessage.mockResolvedValue({ data: msg({ id: 'mine', body: 'моё' }) })

    await w.find('[data-testid="conference-chat-input"] textarea').setValue('моё')
    await w.find('[data-testid="conference-chat-send"]').trigger('click')
    await flushPromises()

    expect(w.text()).toContain('моё')
    // One fetch, from mounting. The round trip we just made is the freshest
    // thing there is.
    expect(api.messages).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  it('hides the composer once the call is over', async () => {
    const w = await rail({ readonly: true })
    expect(w.find('[data-testid="conference-chat-input"]').exists()).toBe(false)
    // Reading stays: the chat is the closest thing the meeting has to a protocol.
    expect(w.find('[data-testid="conference-chat-log"]').exists()).toBe(true)
    w.unmount()
  })
})

describe('attachments', () => {
  const image = {
    id: 'a1',
    filename: 'снимок.png',
    content_type: 'image/png',
    size: 12,
    is_image: true,
  }
  const file = {
    id: 'a2',
    filename: 'notes.txt',
    content_type: 'text/plain',
    size: 9,
    is_image: false,
  }

  it('fetches a picture with our credential and renders it from a blob', async () => {
    api.messages.mockResolvedValue(page([msg({ attachments: [image] })]))
    const w = await rail()

    expect(api.attachment).toHaveBeenCalledWith('a1')
    const img = w.find('[data-testid="conference-chat-image"] img')
    // A blob: URL, never '/api/conference-attachments/…': an <img> can carry
    // neither our bearer header nor the desktop client's cookie.
    expect(img.attributes('src')).toMatch(/^blob:/)
    w.unmount()
  })

  it('downloads each picture once however often the rail re-renders', async () => {
    api.messages.mockResolvedValue(page([msg({ attachments: [image] })]))
    const w = await rail()
    api.messages.mockResolvedValue(page([msg({ attachments: [image] })]))
    await w.setProps({ nudge: 1 })
    await flushPromises()

    expect(api.attachment).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  it('shows a non-image as a file chip, not as a broken picture', async () => {
    api.messages.mockResolvedValue(page([msg({ attachments: [file] })]))
    const w = await rail()

    expect(w.find('[data-testid="conference-chat-image"]').exists()).toBe(false)
    const chip = w.find('[data-testid="conference-chat-file"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('notes.txt')
    w.unmount()
  })

  it('weighs a file chip in the units the reader reads', async () => {
    api.messages.mockResolvedValue(
      page([msg({ attachments: [{ ...file, size: 3 * 1024 * 1024 }] })]),
    )
    const w = await rail()

    // The rail kept a private byte formatter with the units spelled out in
    // English and the decimal point written by hand — on the same screen where
    // the Android client already says «3 МБ» (util/ConferenceChat.kt confFileSize)
    // and where every other web surface goes through formatFileSize (#2799).
    const chip = w.find('[data-testid="conference-chat-file"]')
    expect(chip.text()).toContain('3 МБ')
    expect(chip.text()).not.toContain('3 MB')
    w.unmount()
  })

  it('opens a picture over the call instead of in a new tab', async () => {
    api.messages.mockResolvedValue(page([msg({ attachments: [image] })]))
    const w = await rail()

    await w.find('[data-testid="conference-chat-image"]').trigger('click')
    await flushPromises()
    // The modal is teleported out of this component, so the assertion is on the
    // document, not on the wrapper.
    expect(document.body.textContent).toContain('снимок.png')
    w.unmount()
  })

  it('revokes its blob URLs on unmount', async () => {
    api.messages.mockResolvedValue(page([msg({ attachments: [image] })]))
    const w = await rail()
    const url = created[0].url

    w.unmount()

    // Without this every screenshot of every call stays in memory until the tab
    // is closed — and nothing on screen would ever show that it had.
    expect(revoked).toContain(url)
  })
})

describe('deleting', () => {
  it('offers the bin on my own message', async () => {
    api.messages.mockResolvedValue(page([msg({ user_id: 'me' })]))
    const w = await rail()
    expect(w.find('[data-testid="conference-chat-delete"]').exists()).toBe(true)
    w.unmount()
  })

  it('withholds it on someone else’s, unless I moderate the call', async () => {
    api.messages.mockResolvedValue(page([msg({ user_id: 'other', user_name: 'Ира' })]))
    const w = await rail()
    expect(w.find('[data-testid="conference-chat-delete"]').exists()).toBe(false)
    w.unmount()

    const host = await rail({ canModerate: true })
    // Mirrors the server's check rather than replacing it: a member who forces
    // the button visible is still refused.
    expect(host.find('[data-testid="conference-chat-delete"]').exists()).toBe(true)
    host.unmount()
  })

  it('drops the line from the rail once the server accepts', async () => {
    api.messages.mockResolvedValue(page([msg({ id: 'gone', body: 'ошибся' })]))
    const w = await rail()

    // The bin only opens the prompt now — the guard itself is covered by
    // cx-conf-chat-delete-confirm.spec.js.
    w.findComponent(NPopconfirm).vm.$emit('positive-click')
    await flushPromises()

    expect(api.removeMessage).toHaveBeenCalledWith('gone')
    expect(w.findAll('[data-testid="conference-chat-message"]')).toHaveLength(0)
    w.unmount()
  })
})

// ── the composer's file picker ─────────────────────────────────────────
//
// Every rule here is the server's, and the one that was missing is the one that
// bounds the *request* rather than a file: `maxChatFiles` says how many may
// travel, never how much. Five 20 MB videos passed the count check and the
// per-file check on every client, and the server then refused the batch — for
// most of this route's life not even with its own sentence, because the route
// carried no body budget of its own and the transport answered first with
// "request body too large" (#2932).
describe('picking files for a message', () => {
  /** Hands the hidden input a set of picked files, as the browser would. */
  async function pick(w, files) {
    const input = w.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: files, configurable: true })
    await input.trigger('change')
  }

  /** A stand-in for a picked File — the composer reads `size` and `name`. */
  function file(mb, name) {
    return { name, size: Math.round(mb * 1024 * 1024) }
  }

  it('queues an ordinary screenshot', async () => {
    const w = await rail()
    toast.warning.mockClear()
    await pick(w, [file(2, 'Экран.png')])
    expect(w.findAll('[data-testid="conference-chat-pending"]')).toHaveLength(1)
    expect(toast.warning).not.toHaveBeenCalled()
    w.unmount()
  })

  it('refuses the file that takes the batch over the ceiling, and keeps the rest', async () => {
    const w = await rail()
    toast.warning.mockClear()
    // Each one is legal on its own; the third is what makes the request illegal.
    await pick(w, [file(10, 'а.mp4'), file(10, 'б.mp4'), file(10, 'в.mp4')])

    expect(w.findAll('[data-testid="conference-chat-pending"]')).toHaveLength(2)
    expect(toast.warning).toHaveBeenCalledTimes(1)
    expect(toast.warning.mock.calls[0][0]).toContain('25 МБ')
    w.unmount()
  })

  it('sends only what it queued', async () => {
    const w = await rail()
    await pick(w, [file(10, 'а.mp4'), file(10, 'б.mp4'), file(10, 'в.mp4')])
    await w.find('[data-testid="conference-chat-send"]').trigger('click')
    await flushPromises()

    const sent = api.postMessageWithFiles.mock.calls[0][2]
    expect(sent.map((f) => f.name)).toEqual(['а.mp4', 'б.mp4'])
    w.unmount()
  })

  // Negative controls: the two rules that already worked keep their own wording,
  // so the batch line does not quietly take over every refusal.
  it('still names an over-sized single file', async () => {
    const w = await rail()
    toast.warning.mockClear()
    await pick(w, [file(30, 'Огромное.mov')])
    expect(w.findAll('[data-testid="conference-chat-pending"]')).toHaveLength(0)
    expect(toast.warning.mock.calls[0][0]).toContain('Огромное.mov')
    w.unmount()
  })

  it('still refuses a sixth file', async () => {
    const w = await rail()
    toast.warning.mockClear()
    const six = Array.from({ length: 6 }, (_, i) => file(0.1, `f${i}.png`))
    await pick(w, six)
    expect(w.findAll('[data-testid="conference-chat-pending"]')).toHaveLength(5)
    expect(toast.warning).toHaveBeenCalledTimes(1)
    w.unmount()
  })
})
