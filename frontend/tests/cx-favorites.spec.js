// Favourite boards (#3115): the pure helpers, the store (optimistic changes,
// the undo's restore, socket events) and the «Избранное» sidebar section.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

const api = {
  list: vi.fn(),
  add: vi.fn(),
  remove: vi.fn(() => Promise.resolve({ data: null })),
  move: vi.fn(),
}
vi.mock('@/api', () => ({
  favorites: {
    list: (...a) => api.list(...a),
    add: (...a) => api.add(...a),
    remove: (...a) => api.remove(...a),
    move: (...a) => api.move(...a),
  },
}))
const push = vi.fn()
let routeParams = {}
vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', params: routeParams, query: {} }),
  useRouter: () => ({ push, replace: vi.fn() }),
}))

const { favoriteNeighbors, favoritePatch, favoriteFirst } = await import('@/utils/favorites')
const { useFavoritesStore } = await import('@/stores/favorites')
const { useUndoStore } = await import('@/stores/undo')
const { default: FavoritesSection } = await import('@/components/sidebar/FavoritesSection.vue')

const fav = (id, extra = {}) => ({
  board_id: id,
  position: 0,
  name: `Доска ${id}`,
  slug: `board-${id}`,
  icon: '',
  color: '',
  icon_mode: 'badge',
  project_id: 'p1',
  project_name: 'Tessera Web',
  project_slug: 'web',
  project_color: '',
  workspace_id: 'w1',
  ...extra,
})
const ids = (list) => list.map((f) => f.board_id)

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  routeParams = {}
  document.body.innerHTML = ''
  api.list.mockResolvedValue({ data: [fav('a', { position: 1 }), fav('b', { position: 2 })] })
  api.add.mockImplementation((id) => Promise.resolve({ data: fav(id, { position: 3 }) }))
  api.move.mockImplementation((id) => Promise.resolve({ data: fav(id, { position: 9 }) }))
})

describe('utils/favorites', () => {
  it('names the neighbours the move endpoint takes', () => {
    const list = [fav('a'), fav('b'), fav('c')]
    expect(favoriteNeighbors(list, 0)).toEqual({ after_id: 'b' })
    expect(favoriteNeighbors(list, 1)).toEqual({ before_id: 'a', after_id: 'c' })
    expect(favoriteNeighbors(list, 2)).toEqual({ before_id: 'b' })
    expect(favoriteNeighbors([fav('a')], 0)).toEqual({})
  })

  it('patches the rows a board or a project event names', () => {
    const list = [fav('a'), fav('b')]
    const renamed = favoritePatch(list, 'board.updated', { id: 'b', name: 'Новое', color: '#f00' })
    expect(renamed[1]).toMatchObject({ name: 'Новое', color: '#f00', slug: 'board-b' })
    expect(renamed[0]).toBe(list[0])
    const proj = favoritePatch(list, 'project.updated', { id: 'p1', name: 'Web', slug: 'w' })
    expect(proj.map((f) => [f.project_name, f.project_slug])).toEqual([
      ['Web', 'w'],
      ['Web', 'w'],
    ])
    expect(favoritePatch(list, 'board.updated', { id: 'zzz', name: 'x' })).toBeNull()
  })

  it('puts favourites on top, keeping the order within each half', () => {
    const boards = [{ id: '1' }, { id: '2' }, { id: '3' }, { id: '4' }]
    const out = favoriteFirst(boards, (id) => id === '3' || id === '2')
    expect(out.map((b) => b.id)).toEqual(['2', '3', '1', '4'])
  })
})

describe('stores/favorites', () => {
  it('loads the workspace list and answers isFavorite', async () => {
    const s = useFavoritesStore()
    await s.load('w1')
    expect(api.list).toHaveBeenCalledWith('w1')
    expect(ids(s.items)).toEqual(['a', 'b'])
    expect(s.isFavorite('a')).toBe(true)
    expect(s.isFavorite('zzz')).toBe(false)
  })

  it('stars at once and takes the server row', async () => {
    const s = useFavoritesStore()
    await s.load('w1')
    const p = s.toggle({ id: 'c', name: 'Новая', slug: 'new', project_id: 'p1' }, { name: 'Web' })
    expect(s.isFavorite('c')).toBe(true) // before the answer
    expect(s.items[2]).toMatchObject({ name: 'Новая', project_name: 'Web' })
    await p
    expect(api.add).toHaveBeenCalledWith('c')
    expect(s.items[2].position).toBe(3)
  })

  it('puts the list back when the server refuses', async () => {
    const s = useFavoritesStore()
    await s.load('w1')
    api.add.mockRejectedValueOnce(new Error('403'))
    await expect(s.add({ id: 'c', name: 'x' })).rejects.toThrow()
    expect(ids(s.items)).toEqual(['a', 'b'])
  })

  it('moves to an index with the neighbours of the new place', async () => {
    api.list.mockResolvedValueOnce({ data: [fav('a'), fav('b'), fav('c')] })
    const s = useFavoritesStore()
    await s.load('w1')
    await s.move('c', 0)
    expect(ids(s.items)).toEqual(['c', 'a', 'b'])
    expect(api.move).toHaveBeenCalledWith('c', { after_id: 'a' })
    await s.move('c', 1)
    expect(api.move).toHaveBeenLastCalledWith('c', { before_id: 'a', after_id: 'b' })
    // Same place → no request.
    api.move.mockClear()
    await s.move('a', 0)
    expect(api.move).not.toHaveBeenCalled()
  })

  it('removes and restores into the same place', async () => {
    api.list.mockResolvedValueOnce({ data: [fav('a'), fav('b'), fav('c')] })
    const s = useFavoritesStore()
    await s.load('w1')
    const removed = await s.remove('b')
    expect(removed.neighbors).toEqual({ before_id: 'a', after_id: 'c' })
    expect(ids(s.items)).toEqual(['a', 'c'])
    await s.restore(removed)
    expect(api.add).toHaveBeenCalledWith('b')
    expect(api.move).toHaveBeenCalledWith('b', { before_id: 'a', after_id: 'c' })
    expect(ids(s.items)).toEqual(['a', 'b', 'c'])
  })

  it('follows the socket: own changes re-read, deleted boards drop and come back', async () => {
    const s = useFavoritesStore()
    await s.load('w1')
    api.list.mockClear()
    s.onEvent({ type: 'favorites.changed', scope: 'w2', data: {} })
    expect(api.list).not.toHaveBeenCalled()
    s.onEvent({ type: 'favorites.changed', scope: 'w1', data: {} })
    expect(api.list).toHaveBeenCalledTimes(1)
    await flushPromises()

    s.onEvent({ type: 'board.updated', scope: 'w1', data: { id: 'a', name: 'Переименована' } })
    expect(s.items[0].name).toBe('Переименована')

    s.onEvent({ type: 'board.deleted', scope: 'w1', data: { id: 'b' } })
    expect(ids(s.items)).toEqual(['a'])
    api.list.mockClear()
    s.onEvent({ type: 'board.created', scope: 'w1', data: { id: 'other' } })
    expect(api.list).not.toHaveBeenCalled() // somebody's new board — not ours
    s.onEvent({ type: 'board.created', scope: 'w1', data: { id: 'b' } })
    expect(api.list).toHaveBeenCalledTimes(1) // the restore

    s.onEvent({ type: 'project.deleted', scope: 'w1', data: { id: 'p1' } })
    expect(s.items).toEqual([])
  })

  it('forgets everything on reset', async () => {
    const s = useFavoritesStore()
    await s.load('w1')
    s.reset()
    expect(s.items).toEqual([])
    expect(s.wsId).toBe('')
  })
})

describe('FavoritesSection', () => {
  async function mountSection() {
    const s = useFavoritesStore()
    await s.load('w1')
    const w = mount(FavoritesSection, {
      global: { stubs: { ProjectIcon: true, TesseraIcon: true } },
      attachTo: document.body,
    })
    return { s, w }
  }

  it('is hidden while there are no favourites', async () => {
    api.list.mockResolvedValueOnce({ data: [] })
    const { w } = await mountSection()
    expect(w.find('[data-testid="sidebar-favorites"]').exists()).toBe(false)
    w.unmount()
  })

  it('draws the rows with the project and opens a board', async () => {
    routeParams = { projectSlug: 'web', boardSlug: 'board-b' }
    const { w } = await mountSection()
    const rows = w.findAll('[data-testid="favorite-row"]')
    expect(rows.map((r) => r.find('.name').text())).toEqual(['Доска a', 'Доска b'])
    expect(rows[0].find('[data-testid="favorite-project"]').text()).toBe('Tessera Web')
    expect(rows[1].classes()).toContain('active')
    expect(rows[0].attributes('data-tree-row')).toBeDefined()
    await rows[0].trigger('click')
    expect(push).toHaveBeenCalledWith('/project/web/board/board-a')
    w.unmount()
  })

  it('takes the star off from the menu, with an undo that restores', async () => {
    const { s, w } = await mountSection()
    await w.findAll('[data-testid="favorite-menu"]')[0].trigger('click')
    await flushPromises()
    document.body.querySelector('[data-testid="favorite-remove"]').click()
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith('a')
    expect(ids(s.items)).toEqual(['b'])
    const undo = useUndoStore()
    expect(undo.toasts).toHaveLength(1)
    expect(undo.toasts[0].icon).toBe('star')
    await undo.undo(undo.toasts[0].id)
    expect(api.add).toHaveBeenCalledWith('a')
    expect(ids(s.items)).toEqual(['a', 'b'])
    w.unmount()
  })

  it('⌘⌫ on a row takes the star off', async () => {
    const { s, w } = await mountSection()
    const row = w.findAll('[data-testid="favorite-row"]')[1]
    await row.trigger('keydown', { key: 'Backspace', ctrlKey: true })
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith('b')
    expect(ids(s.items)).toEqual(['a'])
    w.unmount()
  })
})
