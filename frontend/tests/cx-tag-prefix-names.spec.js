// PUT /projects/:id/tag-prefixes — полная замена (DELETE-then-UPSERT), поэтому
// редактор подписей префиксов в TagManager обязан слать МЕРЖ поверх уже
// загруженного набора. Мобильное меню доски открывало тот же TagManager без
// prefix-names: подписи существующих префиксов показывались пустыми, а любой blur
// в поле (сохранение висит на @blur) отправлял частичный набор и стирал все
// подписи проекта — то есть заголовки групп killer-фичи «колонки = теги» (#2932).
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

const setTagPrefixes = vi.fn(() => Promise.resolve({ data: [] }))
vi.mock('@/api', () => ({
  projects: {
    setTagPrefixes: (...a) => setTagPrefixes(...a),
    // Tag rows themselves go through these; unused by the prefix-label editor.
    createTag: vi.fn(() => Promise.resolve({ data: {} })),
    updateTag: vi.fn(() => Promise.resolve({ data: {} })),
    deleteTag: vi.fn(() => Promise.resolve({ data: {} })),
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

const TAGS = [
  { id: 't1', name: 'effort::small', color: '#7c5cff' },
  { id: 't2', name: 'area::backend', color: '#2f80ed' },
]
const NAMES = { 'effort::': 'Сложность', 'area::': 'Область' }

// Rows are ordered by group label (collated), so address a field by its prefix key.
const pfxLabels = (w) =>
  Object.fromEntries(
    w.findAll('.pfx-row').map((r) => [r.find('.pfx-key').text(), r.find('input').element.value]),
  )
const pfxInput = (w, key) =>
  w
    .findAll('.pfx-row')
    .find((r) => r.find('.pfx-key').text() === key)
    .find('input')
const sentMap = (call) => Object.fromEntries(call[1].map((e) => [e.prefix, e.label]))

const settle = async (w) => {
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
}

describe('TagManager: редактор подписей префиксов', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
  })

  const open = async (props = {}) => {
    const TagManager = (await import('@/components/TagManager.vue')).default
    const w = mount(TagManager, {
      props: { projectId: 'p-1', tags: TAGS, prefixNames: NAMES, ...props },
      global: { stubs },
    })
    await settle(w)
    return w
  }

  it('показывает уже настроенные подписи в полях', async () => {
    const w = await open()
    expect(pfxLabels(w)).toEqual({ 'effort::': 'Сложность', 'area::': 'Область' })
  })

  it('правка одной подписи не сносит остальные', async () => {
    const w = await open()
    await pfxInput(w, 'effort::').setValue('Трудоёмкость')
    await pfxInput(w, 'effort::').trigger('blur')
    await settle(w)
    expect(setTagPrefixes).toHaveBeenCalledTimes(1)
    expect(sentMap(setTagPrefixes.mock.calls[0])).toEqual({
      'effort::': 'Трудоёмкость',
      'area::': 'Область',
    })
  })

  it('пустая подпись снимает только своё отображение', async () => {
    const w = await open()
    await pfxInput(w, 'area::').setValue('  ')
    await pfxInput(w, 'area::').trigger('blur')
    await settle(w)
    expect(sentMap(setTagPrefixes.mock.calls[0])).toEqual({ 'effort::': 'Сложность' })
  })

  it('уход из поля без правки не пишет на сервер', async () => {
    // Сохранение висит на @blur: простой тап по полю и уход не должны стоить
    // полной перезаписи набора и широковещалки по пространству.
    const w = await open()
    await pfxInput(w, 'effort::').trigger('blur')
    await settle(w)
    expect(setTagPrefixes).not.toHaveBeenCalled()
  })
})

describe('BoardMobileMenu: «Теги» с мобильной доски', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
  })

  const openTags = async () => {
    const { useBoardViewStore } = await import('@/stores/boardView')
    const store = useBoardViewStore()
    store.projectId = 'p-1'
    store.refill(store.tagsMap, Object.fromEntries(TAGS.map((t) => [t.id, t])))
    store.refill(store.prefixNames, NAMES)
    const BoardMobileMenu = (await import('@/components/BoardMobileMenu.vue')).default
    const w = mount(BoardMobileMenu, { global: { stubs } })
    w.findComponent(naive.NDropdown).vm.$emit('select', 'tags')
    await settle(w)
    return w
  }

  it('подписи префиксов проекта видны, а не пусты', async () => {
    const w = await openTags()
    expect(pfxLabels(w)).toEqual({ 'effort::': 'Сложность', 'area::': 'Область' })
  })

  it('правка подписи с мобильного не стирает остальные', async () => {
    const w = await openTags()
    await pfxInput(w, 'effort::').setValue('Трудоёмкость')
    await pfxInput(w, 'effort::').trigger('blur')
    await settle(w)
    expect(setTagPrefixes).toHaveBeenCalledTimes(1)
    expect(sentMap(setTagPrefixes.mock.calls[0])).toEqual({
      'effort::': 'Трудоёмкость',
      'area::': 'Область',
    })
  })
})
