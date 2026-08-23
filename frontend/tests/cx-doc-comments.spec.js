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

const { default: DocComments } = await import('@/components/documents/DocComments.vue')
const { default: DocEditor } = await import('@/components/documents/DocEditor.vue')

function thread(id, over = {}) {
  return {
    id,
    block_id: 'b1',
    body: `замечание ${id}`,
    quote: '',
    author_id: 'u1',
    author_name: 'Иван',
    created_at: '2026-08-15T10:00:00Z',
    resolved_at: null,
    replies: [],
    ...over,
  }
}

function groups(over = {}) {
  return { anchored: [], document: [], detached: [], ...over }
}

// These panels render dates through useFormat(), which reads the theme store —
// so a Pinia instance has to be active before they mount (#2798).
beforeEach(() => setActivePinia(createPinia()))

describe('DocComments panel', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  it('tags every card with its thread and block, so the link layer can find it', () => {
    // The parent draws a line to each card. It addresses them by data attribute
    // rather than by class: the markup belongs to this component, and a selector
    // reaching in from outside would break on any change to it.
    wrapper = mount(DocComments, {
      props: {
        groups: groups({
          anchored: [thread('t1'), thread('t2', { block_id: 'b2' })],
          document: [thread('t3', { block_id: '' })],
        }),
      },
    })
    const cards = wrapper.findAll('[data-thread-id]')
    expect(cards.map((c) => c.attributes('data-thread-id'))).toEqual(['t1', 't2', 't3'])
    expect(cards.map((c) => c.attributes('data-block-id'))).toEqual(['b1', 'b2', ''])
  })

  it('flags a settled card, so the link layer can stop drawing to it', () => {
    wrapper = mount(DocComments, {
      props: {
        groups: groups({
          anchored: [thread('t1'), thread('t2', { resolved_at: '2026-08-16T10:00:00Z' })],
        }),
      },
      attachTo: document.body,
    })
    expect(wrapper.vm.cardAnchors().map((a) => a.resolved)).toEqual([false, true])
  })

  it('reports one anchor per card', () => {
    wrapper = mount(DocComments, {
      props: { groups: groups({ anchored: [thread('t1'), thread('t2', { block_id: 'b2' })] }) },
      attachTo: document.body,
    })
    const anchors = wrapper.vm.cardAnchors()
    expect(anchors.map((a) => a.id)).toEqual(['t1', 't2'])
    expect(anchors.map((a) => a.blockId)).toEqual(['b1', 'b2'])
    // jsdom gives every box zero size, so the coordinates themselves say
    // nothing — the shape of the answer is what this can honestly check.
    for (const a of anchors) {
      expect(typeof a.x).toBe('number')
      expect(typeof a.y).toBe('number')
      expect(typeof a.visible).toBe('boolean')
    }
  })

  it('reports no anchors when there are no threads', () => {
    wrapper = mount(DocComments, { props: { groups: groups() } })
    expect(wrapper.vm.cardAnchors()).toEqual([])
  })

  it('shows a detached thread under its own heading, not silently dropped', () => {
    // A rewritten paragraph is the normal course of a review; the discussion that
    // asked for the rewrite has to survive it.
    wrapper = mount(DocComments, {
      props: { groups: groups({ detached: [thread('t1', { body: 'нужен срок' })] }) },
    })
    expect(wrapper.text()).toContain('Блок удалён')
    expect(wrapper.text()).toContain('нужен срок')
  })

  it('offers edit and delete only on one’s own comments', () => {
    wrapper = mount(DocComments, {
      props: {
        groups: groups({ anchored: [thread('t1', { author_id: 'someone-else' })] }),
        userId: 'u1',
      },
    })
    expect(wrapper.find('[title="Изменить"]').exists()).toBe(false)
    expect(wrapper.find('[title="Удалить тред"]').exists()).toBe(false)
  })

  it('offers resolve on someone else’s thread — closing a remark is any member’s job', () => {
    wrapper = mount(DocComments, {
      props: {
        groups: groups({ anchored: [thread('t1', { author_id: 'someone-else' })] }),
        userId: 'u1',
      },
    })
    expect(wrapper.find('[title="Пометить решённым"]').exists()).toBe(true)
  })

  it('emits resolve with the flag, and reopen for a settled thread', async () => {
    wrapper = mount(DocComments, {
      props: {
        groups: groups({ anchored: [thread('t1', { resolved_at: '2026-08-15T12:00:00Z' })] }),
      },
    })
    await wrapper.find('[title="Вернуть в работу"]').trigger('click')
    expect(wrapper.emitted('resolve')[0][0]).toEqual({ id: 't1', resolved: false })
  })

  it('does not offer a reply box on a resolved thread', () => {
    wrapper = mount(DocComments, {
      props: {
        groups: groups({
          anchored: [thread('t1', { resolved_at: '2026-08-15T12:00:00Z' })],
        }),
      },
    })
    expect(wrapper.html()).not.toContain('Ответить…')
  })

  it('says what the draft is armed against, and can unpin it', async () => {
    wrapper = mount(DocComments, {
      props: { groups: groups(), pendingBlock: true, pendingQuote: 'Исполнитель обязан' },
    })
    expect(wrapper.text()).toContain('Исполнитель обязан')
    await wrapper.find('[title="Снять привязку"]').trigger('click')
    expect(wrapper.emitted('clear-anchor')).toHaveLength(1)
  })

  it('selects a thread when it is clicked, so the editor can scroll to the block', async () => {
    wrapper = mount(DocComments, { props: { groups: groups({ anchored: [thread('t1')] }) } })
    await wrapper.find('.thread').trigger('click')
    expect(wrapper.emitted('select')[0]).toEqual(['b1'])
  })
})

describe('DocEditor annotations', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  function doc(...texts) {
    return {
      type: 'doc',
      content: texts.map((text, i) => ({
        type: 'paragraph',
        attrs: { id: `b${i + 1}` },
        content: [{ type: 'text', text }],
      })),
    }
  }

  it('marks a block that carries an open discussion, with its count', async () => {
    wrapper = mount(DocEditor, {
      props: {
        modelValue: doc('Исполнитель обязан', 'Второй абзац'),
        comments: [{ block_id: 'b1', count: 2 }],
      },
      attachTo: document.body,
    })
    await nextTick()

    const marked = wrapper.element.querySelectorAll('.doc-block-commented')
    expect(marked).toHaveLength(1)
    expect(marked[0].getAttribute('data-comment-count')).toBe('2')
  })

  it('writes nothing into the document when a block is marked', async () => {
    // The decoration is the whole mechanism: annotating must not modify the
    // content, must not start an autosave, and must not need the block to be
    // free of someone else's lock.
    wrapper = mount(DocEditor, {
      props: { modelValue: doc('Первый'), comments: [] },
      attachTo: document.body,
    })
    await nextTick()

    await wrapper.setProps({ comments: [{ block_id: 'b1', count: 1 }] })
    await nextTick()

    expect(wrapper.emitted('change')).toBeUndefined()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
