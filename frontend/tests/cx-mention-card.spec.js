// Hover cards behind @-mentions (RichContent `mention-cards`). The chips live
// inside v-html, so there is no child component to drive — the test works the
// delegated listeners on the root the way a mouse would.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import RichContent from '@/components/RichContent.vue'
import { buildMentionItems } from '@/utils/mentions'

// RichContent pulls in `useMessage`/`useRouter` for its #N and attachment click
// delegation (unrelated to hover cards); stub them so the component mounts
// without a message-provider/router, same as cx-rich-content.spec.js.
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const MEMBERS = [{ user_id: 'u1', name: 'Ann Lee', email: 'ann@t.io', role: 'owner' }]
const GL = [{ gl_username: 'v.sokolov', gl_name: 'Виктор Соколов', gl_avatar_url: '/a.png' }]
const ITEMS = buildMentionItems(MEMBERS, GL)

// The rendered HTML lands one tick after mount (onMounted sets it), so wait for
// it before reaching into the DOM.
async function render(source, props = {}) {
  const w = mount(RichContent, {
    attachTo: document.body,
    props: { source, members: ITEMS, mentionCards: true, ...props },
  })
  await w.vm.$nextTick()
  return w
}

// The card is teleported to body by n-popover, so it is looked up on the
// document — and scoped to the card itself, since the mounted content is
// attached to the same body and would otherwise match its own mention text.
const card = () => document.querySelector('.mc')
const cardText = () => card()?.textContent || ''

// RichContent has two root nodes (content + popover), so reach for the content
// div rather than the wrapper element.
const chipIn = (wrapper) => wrapper.find('.md').element.querySelector('[data-type="mention"]')

// The listeners are delegated from the root, so the event has to originate on
// the chip and bubble — test-utils' trigger() can't fake `target`.
async function fire(wrapper, chip, type) {
  chip.dispatchEvent(new MouseEvent(type, { bubbles: true }))
  vi.advanceTimersByTime(400) // clears the open/close delay
  await wrapper.vm.$nextTick()
  await wrapper.vm.$nextTick()
}

const hover = (wrapper, chip) => fire(wrapper, chip, 'mouseover')
const unhover = (wrapper, chip) => fire(wrapper, chip, 'mouseout')

describe('RichContent mention cards', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    document.body.innerHTML = ''
  })

  it('opens a card naming the member behind the chip', async () => {
    const w = await render('привет @Ann Lee')
    const chip = chipIn(w)
    expect(chip.dataset.id).toBe('u1')
    await hover(w, chip)
    expect(cardText()).toContain('Ann Lee')
    expect(cardText()).toContain('ann@t.io')
    expect(cardText()).toContain('Владелец')
    w.unmount()
  })

  it('shows name, handle and avatar for a GitLab-only user — no role, no email', async () => {
    const w = await render('привет @v.sokolov')
    await hover(w, chipIn(w))
    expect(cardText()).toContain('Виктор Соколов')
    expect(cardText()).toContain('@v.sokolov')
    expect(cardText()).not.toContain('Владелец')
    w.unmount()
  })

  it('shows no card for a handle nobody owns', async () => {
    const w = await render('привет @nobody')
    const chip = chipIn(w)
    expect(chip.dataset.id).toBeUndefined()
    await hover(w, chip)
    expect(card()).toBeNull()
    w.unmount()
  })

  it('leaves mentions inside code blocks alone — they name no one', async () => {
    const w = await render('`@Ann Lee`')
    // Mentions inside code aren't highlighted at all (renderRich skips code via
    // replaceOutsideCode), so there is no chip to hover and thus no card.
    expect(chipIn(w)).toBeNull()
    expect(card()).toBeNull()
    w.unmount()
  })

  it('closes when the pointer leaves the chip', async () => {
    const w = await render('привет @Ann Lee')
    const chip = chipIn(w)
    await hover(w, chip)
    expect(cardText()).toContain('ann@t.io')
    await unhover(w, chip)
    expect(cardText()).not.toContain('ann@t.io')
    w.unmount()
  })

  it('stays shut without the opt-in — board card previews use the same renderer', async () => {
    const w = await render('привет @Ann Lee', { mentionCards: false })
    await hover(w, chipIn(w))
    expect(card()).toBeNull()
    w.unmount()
  })
})
