import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'

// useMessage() throws without an <n-message-provider>; keep the rest of naive-ui
// intact and stub only that, as the other component specs do.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// The panel lazily fetches the workspace's tasks and members for its pickers;
// neither is touched by the assertions below, and a real call would be a
// network error in jsdom.
vi.mock('@/api', () => ({
  workspaces: {
    tasks: vi.fn().mockResolvedValue({ data: [] }),
    members: vi.fn().mockResolvedValue({ data: [] }),
  },
}))

const { default: DocLinks } = await import('@/components/documents/DocLinks.vue')

const link = (over = {}) => ({
  id: 'l1',
  task_id: 't1',
  task_title: 'Согласовать смету',
  task_board_id: 'b1',
  block_id: '',
  quote: '',
  ...over,
})

const step = (over = {}) => ({
  id: 's1',
  approver_id: 'u1',
  approver_name: 'Аня',
  position: 0,
  status: 'pending',
  comment: '',
  ...over,
})

const approval = (steps, over = {}) => ({
  id: 'a1',
  status: 'pending',
  mode: 'sequential',
  title: 'Редакция 2',
  version_revision: 4,
  created_by_name: 'Иван Петров',
  created_at: '2026-08-15T10:00:00Z',
  steps,
  ...over,
})

describe('DocLinks panel', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  it('lists linked tasks', () => {
    wrapper = mount(DocLinks, { props: { links: [link()] } })
    expect(wrapper.findAll('[data-testid="doc-link"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Согласовать смету')
  })

  it('shows the quote of an anchored link, so it still says which clause', () => {
    wrapper = mount(DocLinks, {
      props: { links: [link({ block_id: 'b-7', quote: 'Стоимость работ не превышает' })] },
    })
    expect(wrapper.text()).toContain('Стоимость работ не превышает')
  })

  it('says both lists are empty rather than showing a blank panel', () => {
    wrapper = mount(DocLinks, { props: { links: [], approvals: [] } })
    expect(wrapper.text()).toContain('Связанных задач нет')
    expect(wrapper.text()).toContain('на согласование не отправлялся')
  })

  it('offers to link the selected block when one is armed', () => {
    wrapper = mount(DocLinks, { props: { anchorBlockId: 'b-7', anchorQuote: 'пункт 4.2' } })
    expect(wrapper.get('[data-testid="doc-link-add"]').text()).toContain('Связать блок')
  })

  it('renders a route with its revision and progress', () => {
    wrapper = mount(DocLinks, {
      props: {
        approvals: [
          approval([
            step({ id: 's1', status: 'approved', position: 0 }),
            step({ id: 's2', approver_id: 'u2', approver_name: 'Борис', position: 1 }),
          ]),
        ],
      },
    })
    const box = wrapper.get('[data-testid="doc-approval"]')
    expect(box.text()).toContain('На согласовании')
    // Which text is being agreed — a protocol that cannot name its revision is a
    // signature on a moving target.
    expect(box.text()).toContain('Версия 4')
    expect(box.text()).toContain('1 из 2')
  })

  it('offers the signature only to the approver whose turn it is', () => {
    const steps = [
      step({ id: 's1', approver_id: 'u1', position: 0 }),
      step({ id: 's2', approver_id: 'u2', position: 1 }),
    ]
    // Second in a sequential route: the server would answer 409, so the panel
    // must not offer the button at all.
    wrapper = mount(DocLinks, { props: { approvals: [approval(steps)], userId: 'u2' } })
    expect(wrapper.find('[data-testid="doc-approval-sign"]').exists()).toBe(false)
    wrapper.unmount()

    wrapper = mount(DocLinks, { props: { approvals: [approval(steps)], userId: 'u1' } })
    expect(wrapper.find('[data-testid="doc-approval-sign"]').exists()).toBe(true)
  })

  it('emits the decision with its comment', async () => {
    wrapper = mount(DocLinks, {
      props: { approvals: [approval([step({ approver_id: 'u1' })])], userId: 'u1' },
    })
    await wrapper.get('[data-testid="doc-approval-sign"]').trigger('click')
    await wrapper.get('textarea').setValue('со сметой согласен')
    const buttons = wrapper.findAll('button').filter((b) => b.text() === 'Согласовать')
    await buttons[0].trigger('click')
    expect(wrapper.emitted('decide')[0][0]).toEqual({
      id: 'a1',
      decision: 'approved',
      comment: 'со сметой согласен',
    })
  })

  it('marks each step by word as well as by colour', () => {
    wrapper = mount(DocLinks, {
      props: {
        approvals: [
          approval([
            step({ id: 's1', status: 'approved', position: 0 }),
            step({ id: 's2', approver_id: 'u2', approver_name: 'Борис', position: 1 }),
          ]),
        ],
      },
    })
    const steps = wrapper.findAll('.step')
    expect(steps[0].text()).toContain('подписал')
    expect(steps[0].classes()).toContain('signed')
    // Only the earliest pending step is the current one.
    expect(steps[1].text()).toContain('ждёт')
    expect(steps[1].classes()).toContain('current')
  })

  it('does not offer a second route while one is open', () => {
    wrapper = mount(DocLinks, { props: { canRaise: false } })
    expect(wrapper.get('[data-testid="doc-approval-raise"]').attributes('disabled')).toBeDefined()
  })
})
