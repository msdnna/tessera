import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import DocToc from '@/components/documents/DocToc.vue'
import { UNTITLED_HEADING, docOutline } from '@/utils/docToc'

function h(id, level, text) {
  return {
    type: 'heading',
    attrs: { id, level },
    content: text === undefined ? [] : [{ type: 'text', text }],
  }
}

const DOC = {
  type: 'doc',
  content: [h('a', 1, 'Введение'), h('b', 2, 'Область применения'), h('c', 1, 'Термины')],
}

describe('DocToc panel', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  it('lists the headings in reading order', () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    const rows = wrapper.findAll('[data-testid="doc-toc-entry"]')
    expect(rows.map((r) => r.text())).toEqual(['Введение', 'Область применения', 'Термины'])
  })

  it('indents a nested heading', () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    const rows = wrapper.findAll('[data-testid="doc-toc-entry"]')
    // The indent is a padding on a flat list: on a 260px panel a deeply nested
    // heading inside nested <ul>s would have almost no width left for its text.
    expect(rows[0].attributes('style')).toContain('padding-left: 6px')
    expect(rows[1].attributes('style')).toContain('padding-left: 18px')
  })

  it('marks the section the caret is in', () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC), activeId: 'b' } })
    const active = wrapper
      .findAll('[data-testid="doc-toc-entry"]')
      .filter((r) => r.classes('active'))
    expect(active).toHaveLength(1)
    expect(active[0].text()).toBe('Область применения')
  })

  it('asks to jump to the block it was given, not to a position', () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    wrapper.findAll('[data-testid="doc-toc-entry"]')[2].trigger('click')
    expect(wrapper.emitted('go')).toEqual([['c']])
  })

  it('keeps an untitled heading clickable', () => {
    wrapper = mount(DocToc, { props: { rows: docOutline({ type: 'doc', content: [h('a', 1)] }) } })
    expect(wrapper.find('[data-testid="doc-toc-entry"]').text()).toBe(UNTITLED_HEADING)
  })

  it('explains itself when the document has no headings', () => {
    wrapper = mount(DocToc, { props: { rows: [] } })
    expect(wrapper.findAll('[data-testid="doc-toc-entry"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('Добавьте заголовки')
  })

  // The colours of this panel are guarded with the rest of the documents UI in
  // cx-doc-editor.spec.js (DOC_FILES) — one list, so a new panel cannot be added
  // without its theming being checked.
})
