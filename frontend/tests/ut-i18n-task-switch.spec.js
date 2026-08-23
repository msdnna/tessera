import { describe, it, expect, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { i18n, setI18nLocale } from '@/i18n'

// useMessage() throws without an <n-message-provider>; keep the rest of naive-ui
// intact and stub only that, as the other component specs do.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const { default: TaskLayoutSwitch } = await import('@/components/TaskLayoutSwitch.vue')
const { default: TaskRelationsTab } = await import('@/components/task/TaskRelationsTab.vue')

// Same guard as ut-i18n-board-switch, for the task-card / task-modal surfaces of
// wave 2: their option tables (task-layout menu, relation kinds) were plain
// arrays built once at setup, so translating them «in place» would have frozen
// them at the language the component first mounted in. These mount, switch the
// locale and demand the rendered text follow.

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('task labels follow a language switch', () => {
  it('re-renders the task-layout menu', async () => {
    const w = mount(TaskLayoutSwitch, { props: { value: 'modal' } })
    expect(w.vm.opts.map((o) => o.label)).toEqual([
      'Модальное окно',
      'Полный экран',
      'Панель справа',
    ])

    await setI18nLocale('en')
    expect(w.vm.opts.map((o) => o.label)).toEqual(['Modal window', 'Full screen', 'Side panel'])
  })

  it('re-renders the relation-kind select and the kind shown on a row', async () => {
    const w = mount(TaskRelationsTab, {
      props: {
        taskId: 't1',
        wsId: 'w1',
        relations: [{ id: 'r1', kind: 'blocks', related_number: 7, related_title: 'Другая' }],
      },
    })
    expect(w.find('.rel-kind').text()).toBe('блокирует')

    await setI18nLocale('en')
    expect(w.find('.rel-kind').text()).toBe('blocks')
  })
})

describe('Russian plural forms on the reply counter', () => {
  // The tab used to decline «ответ/ответа/ответов» by hand; the CLDR rule
  // registered for `ru` now picks the form.
  it.each([
    [1, '1 ответ'],
    [2, '2 ответа'],
    [5, '5 ответов'],
    [21, '21 ответ'],
  ])('%i → %s', (n, expected) => {
    expect(i18n.global.t('task.comments.replies', n, { named: { n } })).toBe(expected)
  })
})
