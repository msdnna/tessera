import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// «Статья помогла?» under a help article (#3071): one vote per reader, article
// and language, stored as an acknowledgement — no table of its own.
const ack = vi.hoisted(() => vi.fn(() => Promise.resolve({ data: {} })))
vi.mock('@/api', () => ({
  acknowledgements: { list: vi.fn(() => Promise.resolve({ data: [] })), ack },
}))

import HelpFeedback from '@/components/help/HelpFeedback.vue'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { helpVoteKey } from '@/utils/helpFeedback'

beforeEach(() => {
  setActivePinia(createPinia())
  ack.mockClear()
})

describe('HelpFeedback', () => {
  it('stores «Да» as the article’s acknowledgement and thanks the reader', async () => {
    const w = mount(HelpFeedback, { props: { slug: 'faq', lang: 'ru' } })
    await w.get('[data-testid="help-feedback-yes"]').trigger('click')
    await flushPromises()
    expect(ack).toHaveBeenCalledWith('helpful:faq:ru:yes')
    expect(w.find('[data-testid="help-feedback-thanks"]').exists()).toBe(true)
    expect(w.find('[data-testid="help-feedback-no"]').exists()).toBe(false)
  })

  it('shows the thanks right away for an article already voted on, in that language', () => {
    useWhatsNewStore().acked.add(helpVoteKey('faq', 'en', 'no'))
    const en = mount(HelpFeedback, { props: { slug: 'faq', lang: 'en' } })
    expect(en.find('[data-testid="help-feedback-thanks"]').exists()).toBe(true)
    // The Russian text of the same article is a separate question.
    const ru = mount(HelpFeedback, { props: { slug: 'faq', lang: 'ru' } })
    expect(ru.find('[data-testid="help-feedback-yes"]').exists()).toBe(true)
  })
})
