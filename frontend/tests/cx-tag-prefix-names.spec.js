// Понятные имена префиксов в окне «Проект» (#2990).
//
// PUT /projects/:id/tag-prefixes — полная замена (DELETE-then-UPSERT): старый
// TagManager обязан был слать МЕРЖ поверх загруженного набора, а мобильное меню,
// открывшее его без prefix-names, однажды стёрло все подписи проекта — заголовки
// групп killer-фичи «колонки = теги» (#2932). Таблица тегов окна «Проект» правит
// имя прямо в заголовке группы и шлёт ТОЧЕЧНУЮ правку (PATCH {prefix, label}) —
// набор целиком больше никто не пересылает, соседние подписи тронуть нечем.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const TAGS = [
  { id: 't1', name: 'effort::small', color: '#7c5cff', task_count: 3 },
  { id: 't2', name: 'area::backend', color: '#2f80ed', task_count: 1 },
  { id: 't3', name: 'urgent', color: '#e0533d', task_count: 0 },
]
const PREFIXES = [
  { prefix: 'effort::', label: 'Сложность' },
  { prefix: 'area::', label: 'Область' },
]
const setTagPrefix = vi.fn(() => Promise.resolve({ data: [] }))
const setTagPrefixes = vi.fn(() => Promise.resolve({ data: [] }))
vi.mock('@/api', () => ({
  projects: {
    tags: vi.fn(() => Promise.resolve({ data: TAGS })),
    tagPrefixes: vi.fn(() => Promise.resolve({ data: PREFIXES })),
    setTagPrefix: (...a) => setTagPrefix(...a),
    setTagPrefixes: (...a) => setTagPrefixes(...a),
    createTag: vi.fn(() => Promise.resolve({ data: {} })),
    updateTag: vi.fn(() => Promise.resolve({ data: {} })),
  },
  gitlab: { listIntegrations: vi.fn(() => Promise.resolve({ data: { integrations: [] } })) },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

const settle = async (w) => {
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
}

const groupName = (w, label) =>
  w.findAll('[data-testid="tag-group-name"]').find((b) => b.text().includes(label))

describe('таблица тегов: имя префикса в заголовке группы', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
  })

  const open = async () => {
    const ProjectTagsTab = (await import('@/components/project/ProjectTagsTab.vue')).default
    const w = mount(ProjectTagsTab, {
      props: { project: { id: 'p-1', slug: 'p' } },
      global: { stubs },
      attachTo: document.body,
    })
    await settle(w)
    return w
  }

  it('показывает уже настроенные имена в заголовках', async () => {
    const w = await open()
    expect(groupName(w, 'Сложность')).toBeTruthy()
    expect(groupName(w, 'Область')).toBeTruthy()
    w.unmount()
  })

  it('правка одного имени шлёт только его — без пересылки набора', async () => {
    const w = await open()
    await groupName(w, 'Сложность').trigger('click')
    await settle(w)
    const input = w.find('.tt-gedit input')
    await input.setValue('Трудоёмкость')
    await input.trigger('keydown', { key: 'Enter' })
    await settle(w)
    expect(setTagPrefix).toHaveBeenCalledTimes(1)
    expect(setTagPrefix.mock.calls[0]).toEqual(['p-1', 'effort::', 'Трудоёмкость'])
    expect(setTagPrefixes).not.toHaveBeenCalled()
    w.unmount()
  })

  it('пустое имя снимает только своё отображение', async () => {
    const w = await open()
    await groupName(w, 'Область').trigger('click')
    await settle(w)
    const input = w.find('.tt-gedit input')
    await input.setValue('  ')
    await input.trigger('keydown', { key: 'Enter' })
    await settle(w)
    expect(setTagPrefix.mock.calls[0]).toEqual(['p-1', 'area::', ''])
    w.unmount()
  })

  it('уход из поля без правки не пишет на сервер', async () => {
    const w = await open()
    await groupName(w, 'Сложность').trigger('click')
    await settle(w)
    await w.find('.tt-gedit input').trigger('keydown', { key: 'Enter' })
    await settle(w)
    expect(setTagPrefix).not.toHaveBeenCalled()
    w.unmount()
  })
})

describe('BoardMobileMenu: «Теги» с мобильной доски', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('открывает окно «Проект» на вкладке «Теги»', async () => {
    const { useBoardViewStore } = await import('@/stores/boardView')
    const { useProjectWindowStore } = await import('@/stores/projectWindow')
    const store = useBoardViewStore()
    store.projectId = 'p-1'
    const BoardMobileMenu = (await import('@/components/BoardMobileMenu.vue')).default
    const w = mount(BoardMobileMenu, { global: { stubs } })
    w.findComponent({ name: 'ActionMenu' }).vm.$emit('select', 'tags')
    await settle(w)
    const win = useProjectWindowStore()
    expect(win.projectId).toBe('p-1')
    expect(win.tab).toBe('tags')
  })
})
