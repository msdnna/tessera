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

// The rail is what replaced the 260px column in #2728. The tests below are about
// the collapse, not the list: the list is covered above and does not change.
describe('DocToc rail', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  it('draws one tick per heading and accents the current section', () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC), activeId: 'b' } })
    const ticks = wrapper.findAll('[data-testid="doc-toc-tick"]')
    expect(ticks).toHaveLength(3)
    const active = ticks.filter((t) => t.classes('active'))
    expect(active).toHaveLength(1)
    // Second row is the nested heading, so the accent has to be on the second
    // tick — a rail that highlights the right count but the wrong tick reads as
    // "you are somewhere else in the document".
    expect(ticks.indexOf(active[0])).toBe(1)
  })

  it('makes a nested heading a shorter tick, so the rail has the shape of the document', () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    const ticks = wrapper.findAll('[data-testid="doc-toc-tick"]')
    expect(ticks[0].attributes('style')).toContain('width: 16px')
    expect(ticks[1].attributes('style')).toContain('width: 13px')
  })

  // These read the v-show style rather than calling isVisible(). Not a style
  // preference: isVisible() goes through getComputedStyle, and jsdom caches that
  // per element without invalidating it when an inline style changes on a
  // detached tree. One call while the popover is hidden makes every later call
  // in the same test answer "hidden" no matter what the DOM says — which is a
  // test that can only ever fail, or worse, only ever pass.
  const shown = (w) =>
    w.find('[data-testid="doc-toc-flyout"]').attributes('style') !== 'display: none;'

  it('keeps the titles collapsed until the rail is hovered', async () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    const toc = () => wrapper.find('[data-testid="doc-toc"]')
    // v-show, so the entries stay mounted — hidden, not absent.
    expect(shown(wrapper)).toBe(false)
    expect(wrapper.findAll('[data-testid="doc-toc-entry"]')).toHaveLength(3)
    await toc().trigger('mouseenter')
    expect(shown(wrapper)).toBe(true)
    await toc().trigger('mouseleave')
    expect(shown(wrapper)).toBe(false)
  })

  it('opens on keyboard focus, so the entries are not a Tab trap', async () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    await wrapper.find('[data-testid="doc-toc"]').trigger('focusin')
    expect(shown(wrapper)).toBe(true)
  })

  // The regression this component was rewritten around: a click on an entry
  // moves focus into the editor, and folding hover and focus into one flag made
  // that focusout close the outline the pointer was still resting on.
  it('stays open when focus leaves while the pointer is still on it', async () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    const toc = wrapper.find('[data-testid="doc-toc"]')
    await toc.trigger('mouseenter')
    await toc.trigger('focusin')
    await toc.trigger('focusout')
    expect(shown(wrapper)).toBe(true)
  })

  it('fades the rail out only while the popover is open', async () => {
    wrapper = mount(DocToc, { props: { rows: docOutline(DOC) } })
    const rail = () => wrapper.find('.rail')
    expect(rail().classes('faded')).toBe(false)
    await wrapper.find('[data-testid="doc-toc"]').trigger('mouseenter')
    // Faded, not hidden: the rail has to keep taking the pointer, or the
    // popover would close the instant it opened.
    expect(rail().classes('faded')).toBe(true)
    expect(rail().attributes('style')).toBeUndefined()
  })
})
