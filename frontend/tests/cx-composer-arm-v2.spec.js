// The comment composer and the two-step row button of the v2 task modal (#3113).
//
// Composer (MarkdownEditor variant="composer", TaskModal mockup L80–87): one row
// — the reader's avatar, the field, «Aa», «Прикрепить», «Отправить ⌘↵». «Aa»
// brings the v2 bar (the same as the description editor's) above the field.
// It must not hang under «Файлы»: it is teleported out of the comments tab, so
// the tab's v-show never reached it — the modal hides its slot instead.
//
// ArmButton (package A's two-step item as a row button): the first press only
// arms it, the second does it; it disarms by itself after 4 s and on Esc, which
// it keeps from the dialog underneath.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import MarkdownEditor from '@/components/MarkdownEditor.vue'
import ArmButton from '@/components/kit/ArmButton.vue'

vi.mock('@/api', () => ({
  uploads: { upload: vi.fn() },
  tasks: { attachments: vi.fn(() => Promise.resolve({ data: [] })), uploadAttachment: vi.fn() },
}))
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

beforeEach(() => {
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

describe('composer (MarkdownEditor variant="composer")', () => {
  const mk = (props = {}) =>
    mount(MarkdownEditor, {
      props: {
        modelValue: '',
        variant: 'composer',
        send: true,
        avatar: { id: 'u1', name: 'Аня Ковалёва' },
        ...props,
      },
      global: { stubs: { RichContent: true, TesseraSpinner: true, teleport: true } },
    })

  // #3118 (PackageJ «Композер»): «Aa» opens the formatting strip, and the
  // choice is remembered for the next composer.
  it('is one row: avatar, field, Aa, attach, send — no strip until Aa', async () => {
    localStorage.removeItem('tessera_composer_fmt')
    const w = mk()
    expect(w.find('.md2-cava').exists()).toBe(true)
    expect(w.find('textarea').exists()).toBe(true)
    expect(w.find('[data-testid="composer-attach"]').exists()).toBe(true)
    expect(w.find('[data-testid="composer-send"]').exists()).toBe(true)
    expect(w.find('[data-testid="md2-strip"]').exists()).toBe(false)
    await w.find('[data-testid="composer-format"]').trigger('click')
    expect(w.find('[data-testid="md2-strip"]').exists()).toBe(true)
    expect(localStorage.getItem('tessera_composer_fmt')).toBe('1')
    expect(mk().find('[data-testid="md2-strip"]').exists()).toBe(true)
    await w.find('[data-testid="composer-format"]').trigger('click')
    expect(w.find('[data-testid="md2-strip"]').exists()).toBe(false)
    expect(localStorage.getItem('tessera_composer_fmt')).toBe('0')
  })

  it('sends on ⌘↵ and on the button; not while a send is in flight', async () => {
    const w = mk({ modelValue: 'привет' })
    await w.find('textarea').trigger('keydown', { key: 'Enter', ctrlKey: true })
    await w.find('[data-testid="composer-send"]').trigger('click')
    expect(w.emitted('submit')).toHaveLength(2)
    await w.setProps({ sending: true })
    await w.find('textarea').trigger('keydown', { key: 'Enter', metaKey: true })
    expect(w.emitted('submit')).toHaveLength(2)
  })

  it('the task modal hides the composer slot under «Файлы»', () => {
    const src = readFileSync(resolve(process.cwd(), 'src/components/TaskModal.vue'), 'utf8')
    expect(src).toMatch(/v-show="!ro && activityMode !== 'files'"\s+ref="composerEl"/)
  })
})

describe('ArmButton — the two-step row button', () => {
  let w = null
  afterEach(() => {
    w?.unmount()
    vi.useRealTimers()
  })

  it('the first press arms, the second confirms', async () => {
    w = mount(ArmButton, { props: { confirm: 'Удалить?', title: 'Удалить' } })
    await w.trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
    expect(w.classes()).toContain('armed')
    expect(w.text()).toContain('Удалить?')
    await w.trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
    expect(w.classes()).not.toContain('armed')
  })

  it('disarms by itself after 4 s', async () => {
    vi.useFakeTimers()
    w = mount(ArmButton, { props: { confirm: 'Удалить?' } })
    await w.trigger('click')
    vi.advanceTimersByTime(4100)
    await w.vm.$nextTick()
    expect(w.classes()).not.toContain('armed')
    await w.trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
  })

  it('Esc disarms and stays with the button', async () => {
    w = mount(ArmButton, { props: { confirm: 'Удалить?' }, attachTo: document.body })
    await w.trigger('click')
    const outer = vi.fn()
    document.addEventListener('keydown', outer)
    w.element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    document.removeEventListener('keydown', outer)
    await w.vm.$nextTick()
    expect(outer).not.toHaveBeenCalled()
    expect(w.classes()).not.toContain('armed')
  })
})
