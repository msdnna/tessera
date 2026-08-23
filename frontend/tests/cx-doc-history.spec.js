import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

// useMessage() throws without an <n-message-provider>; keep the rest of naive-ui
// intact and stub only that, as the other component specs do.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const { default: DocHistory } = await import('@/components/documents/DocHistory.vue')

function entry(revision, over = {}) {
  return {
    id: 'v' + revision,
    revision,
    preview: 'текст версии ' + revision,
    label: '',
    manual: false,
    author_name: 'Иван Петров',
    created_at: '2026-08-15T10:00:00Z',
    updated_at: '2026-08-15T10:35:00Z',
    ...over,
  }
}

// These panels render dates through useFormat(), which reads the theme store —
// so a Pinia instance has to be active before they mount (#2798).
beforeEach(() => setActivePinia(createPinia()))

describe('DocHistory panel', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  it('lists versions with their author and span', () => {
    wrapper = mount(DocHistory, { props: { versions: [entry(2), entry(1)] } })
    const rows = wrapper.findAll('.entry')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('Версия 2')
    expect(rows[0].text()).toContain('Иван Петров')
    // A journal entry covers an editing session, so it carries a span rather
    // than a moment.
    expect(rows[0].text()).toMatch(/\d{2}:\d{2}–\d{2}:\d{2}/)
  })

  it('shows a manual snapshot with its label', () => {
    wrapper = mount(DocHistory, {
      props: { versions: [entry(3, { manual: true, label: 'согласованная редакция' })] },
    })
    expect(wrapper.find('.entry').classes()).toContain('milestone')
    expect(wrapper.text()).toContain('согласованная редакция')
  })

  it('says the history is empty rather than showing a blank panel', () => {
    wrapper = mount(DocHistory, { props: { versions: [] } })
    expect(wrapper.text()).toContain('История появится')
  })

  it('emits the picked version', async () => {
    wrapper = mount(DocHistory, { props: { versions: [entry(2), entry(1)] } })
    await wrapper.findAll('.entry')[1].trigger('click')
    expect(wrapper.emitted('select')[0]).toEqual(['v1'])
  })

  it('renders the comparison with a status on every changed block', () => {
    wrapper = mount(DocHistory, {
      props: {
        versions: [entry(2), entry(1)],
        selectedId: 'v1',
        baseline: entry(2),
        ready: true,
        summary: { added: 1, removed: 1, changed: 1, moved: 0, identical: false },
        rows: [
          { status: 'same', type: 'paragraph', text: 'без изменений' },
          { status: 'changed', type: 'paragraph', text: 'стало', prevText: 'было' },
          { status: 'removed', type: 'paragraph', text: 'удалённый абзац' },
          { status: 'added', type: 'paragraph', text: 'новый абзац' },
        ],
      },
    })
    expect(wrapper.findAll('.block')).toHaveLength(4)
    expect(wrapper.find('.block.changed').text()).toContain('было')
    expect(wrapper.find('.block.changed').text()).toContain('стало')
    // Status is spelled out, not carried by colour alone.
    expect(wrapper.find('.block.removed').text()).toContain('удалено')
    expect(wrapper.text()).toContain('Сравнение с версией 2')
  })

  it('says so when the two versions are identical', () => {
    wrapper = mount(DocHistory, {
      props: {
        versions: [entry(2), entry(1)],
        selectedId: 'v1',
        baseline: entry(2),
        ready: true,
        summary: { added: 0, removed: 0, changed: 0, moved: 0, identical: true },
        rows: [{ status: 'same', type: 'paragraph', text: 'одинаково' }],
      },
    })
    expect(wrapper.text()).toContain('Версии совпадают')
  })

  // A comparison drawn before both bodies arrived would show "no changes" and be
  // read as an answer.
  it('shows a loading state instead of an empty diff while a body is in flight', () => {
    wrapper = mount(DocHistory, {
      props: { versions: [entry(2), entry(1)], selectedId: 'v1', baseline: entry(2), ready: false },
    })
    expect(wrapper.text()).toContain('Загрузка версии')
    expect(wrapper.findAll('.block')).toHaveLength(0)
  })

  it('takes a named snapshot', async () => {
    wrapper = mount(DocHistory, { props: { versions: [entry(1)] } })
    await wrapper
      .findAll('button')
      .find((b) => b.text().includes('Сохранить версию'))
      .trigger('click')
    await nextTick()
    await wrapper.find('input').setValue('перед согласованием')
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Сохранить')
      .trigger('click')
    expect(wrapper.emitted('snapshot')[0]).toEqual(['перед согласованием'])
  })

  it('reports an error instead of pretending the journal is empty', () => {
    wrapper = mount(DocHistory, { props: { versions: [], error: 'сеть недоступна' } })
    expect(wrapper.text()).toContain('сеть недоступна')
  })
})
