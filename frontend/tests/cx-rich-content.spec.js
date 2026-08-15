// RichContent's click delegation (#2717): "#N" chips and attachment links are
// intercepted rather than followed — the first needs resolving to a board, the
// second can't be a plain href because the download route is behind auth.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import RichContent from '@/components/RichContent.vue'
import { useWorkspacesStore } from '@/stores/workspaces'

const taskByNumber = vi.fn()
const downloadAttachment = vi.fn(() => Promise.resolve({ data: new Blob(['x']) }))
const warning = vi.fn()

vi.mock('@/api', () => ({
  tasks: {
    taskByNumber: (...a) => taskByNumber(...a),
    downloadAttachment: (...a) => downloadAttachment(...a),
  },
}))

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning }),
  }
})

// build() runs on mounted and only assigns the html ref, so the markup lands one
// tick later.
async function render(source, props = {}) {
  const w = mount(RichContent, { props: { source, taskRefs: true, ...props } })
  await nextTick()
  return w
}

describe('RichContent task references', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useWorkspacesStore().currentId = 'ws1'
    taskByNumber.mockReset()
    push.mockReset()
    warning.mockReset()
  })

  it('resolves a #N chip and routes to its board', async () => {
    taskByNumber.mockResolvedValue({ data: { id: 't9', board_id: 'b7', number: 2550 } })
    const w = await render('см. #2550')
    const chip = w.find('[data-task-ref]')
    expect(chip.exists()).toBe(true)

    await chip.trigger('click')
    await Promise.resolve()
    expect(taskByNumber).toHaveBeenCalledWith('ws1', 2550)
    expect(push).toHaveBeenCalledWith('/board/b7?task=2550')
  })

  it('caches the resolution instead of asking again on every click', async () => {
    taskByNumber.mockResolvedValue({ data: { board_id: 'b7' } })
    const w = await render('см. #2551')
    const chip = w.find('[data-task-ref]')
    await chip.trigger('click')
    await Promise.resolve()
    await chip.trigger('click')
    await Promise.resolve()
    expect(taskByNumber).toHaveBeenCalledTimes(1)
  })

  it('warns instead of navigating when the number does not resolve', async () => {
    taskByNumber.mockRejectedValue(new Error('404'))
    const w = await render('см. #2552')
    await w.find('[data-task-ref]').trigger('click')
    await Promise.resolve()
    await Promise.resolve()
    expect(push).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalled()
  })

  it('renders no chip at all when task refs are off', async () => {
    const w = await render('см. #2550', { taskRefs: false })
    expect(w.find('[data-task-ref]').exists()).toBe(false)
  })
})

describe('RichContent attachment links', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    downloadAttachment.mockClear()
  })

  it('downloads through the api instead of following the href', async () => {
    const id = '11111111-2222-3333-4444-555555555555'
    const w = await render(`[отчёт.pdf](/api/attachments/${id}/download)`)
    await w.find('a').trigger('click')
    await Promise.resolve()
    expect(downloadAttachment).toHaveBeenCalledWith(id)
  })

  it('leaves ordinary links to the browser', async () => {
    const w = await render('[сайт](https://example.com)')
    await w.find('a').trigger('click')
    expect(downloadAttachment).not.toHaveBeenCalled()
  })
})
