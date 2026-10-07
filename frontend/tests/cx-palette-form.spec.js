// Palette forms (v2 interaction model, #2969 / #2982): «>Создать проект» walked
// through field by field without the mouse — Tab / ⇧Tab between fields, the
// input editing the current one, ↵ creating from any step.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

const createProject = vi.fn(() => Promise.resolve({ data: { id: 'p9', slug: 'tessera-web' } }))
const createBoard = vi.fn(() => Promise.resolve({ data: { id: 'b9', slug: 'retro' } }))
const push = vi.fn()

vi.mock('@/api', () => ({
  workspaces: {
    search: vi.fn(() => Promise.resolve({ data: { tasks: [], boards: [], notes: [], documents: [] } })),
    members: vi.fn(() => Promise.resolve({ data: [] })),
    milestones: vi.fn(() => Promise.resolve({ data: [] })),
    createProject: (...a) => createProject(...a),
    createGroup: vi.fn(() => Promise.resolve({ data: { id: 'g9' } })),
    tree: vi.fn(() => Promise.resolve({ data: { groups: [], projects: [] } })),
  },
  projects: {
    createBoard: (...a) => createBoard(...a),
    boards: vi.fn(() => Promise.resolve({ data: [] })),
    tags: vi.fn(() => Promise.resolve({ data: [] })),
  },
  tasks: { get: vi.fn(), update: vi.fn(), move: vi.fn() },
  boards: { get: vi.fn(), columns: vi.fn(() => Promise.resolve({ data: [] })), tasks: vi.fn() },
}))
vi.mock('vue-router', () => ({
  useRouter: () => ({ push, resolve: (p) => ({ href: String(p) }) }),
  useRoute: () => ({ params: { projectSlug: 'web' }, query: {}, path: '/' }),
}))
vi.mock('naive-ui', async (importOriginal) => ({
  ...(await importOriginal()),
  useMessage: () => ({ error: vi.fn(), success: vi.fn() }),
}))

describe('palette forms', () => {
  let wrapper
  let palette
  beforeEach(async () => {
    setActivePinia(createPinia())
    createProject.mockClear()
    createBoard.mockClear()
    push.mockClear()
    Element.prototype.scrollIntoView = vi.fn()
    const { useWorkspacesStore } = await import('@/stores/workspaces')
    const ws = useWorkspacesStore()
    ws.currentId = 'w1'
    ws.list = [{ id: 'w1', name: 'Tessera Dev', my_role: 'owner' }]
    ws.groups = [{ id: 'g1', name: 'Продукт', parent_id: null }]
    ws.projects = [{ id: 'p1', name: 'Веб', slug: 'web', group_id: 'g1' }]
    ws.refresh = vi.fn(() => Promise.resolve())
    ws.loadBoards = vi.fn(() => Promise.resolve())
  })
  afterEach(() => {
    wrapper?.unmount()
    document.body.innerHTML = ''
  })

  async function open(query) {
    const { default: CommandPalette } = await import('@/components/palette/CommandPalette.vue')
    const { usePaletteStore } = await import('@/stores/palette')
    palette = usePaletteStore()
    wrapper = mount(CommandPalette, { attachTo: document.body })
    palette.show(query)
    await flushPromises()
  }
  const input = () => document.querySelector('[data-testid=palette-input]')
  const key = async (k, opts = {}) => {
    input().dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...opts }))
    await flushPromises()
  }
  const type = async (text) => {
    input().value = text
    input().dispatchEvent(new Event('input'))
    await flushPromises()
  }

  it('lists the create commands under «>», with where they create', async () => {
    await open('>созд')
    const row = document.querySelector('[data-row="cmd:form.project"]')
    expect(row).toBeTruthy()
    expect(row.textContent).toContain('В группе «Продукт»')
  })

  it('↵ on «Создать проект» turns the palette into its form', async () => {
    await open('>создать проект')
    document.querySelector('[data-row="cmd:form.project"]').click()
    await flushPromises()
    expect(palette.scope).toEqual({ form: 'project', step: 0 })
    expect(document.querySelector('[data-testid="palette-form-chip"]').textContent).toContain('Создать проект')
    expect(palette.query).toBe('')
  })

  it('types the name, walks the fields, creates from any step', async () => {
    await open('>создать проект')
    document.querySelector('[data-row="cmd:form.project"]').click()
    await flushPromises()
    await type('Tessera Web')
    await key('Tab') // → address, derived from the name
    expect(palette.query).toBe('tessera-web')
    await key('Tab') // → group (defaults to the one the open project is in)
    expect(palette.scope.step).toBe(2)
    await key('Tab') // → icon and colour
    await key('ArrowRight') // next colour
    await key('Enter', { metaKey: false })
    expect(createProject).toHaveBeenCalledOnce()
    const [ws, body] = createProject.mock.calls[0]
    expect(ws).toBe('w1')
    expect(body).toMatchObject({ name: 'Tessera Web', group_id: 'g1', icon: 'code', color: '#2f80ed' })
    expect(body.slug).toBeUndefined() // left alone → the server derives it
    expect(palette.open).toBe(false)
  })

  it('refuses to create without a name and goes back to it', async () => {
    await open('>создать проект')
    document.querySelector('[data-row="cmd:form.project"]').click()
    await flushPromises()
    await key('Tab')
    await key('Enter')
    expect(createProject).not.toHaveBeenCalled()
    expect(palette.scope.step).toBe(0)
    expect(document.querySelector('.cp-error').textContent).toContain('Сначала название')
  })

  it('⇧Tab on the first field steps back to the commands', async () => {
    await open('>создать проект')
    document.querySelector('[data-row="cmd:form.project"]').click()
    await flushPromises()
    await key('Tab', { shiftKey: true })
    expect(palette.scope).toBeNull()
    expect(palette.query).toBe('>')
  })

  it('a board: the project pick creates, ⌘↵ opens the new board', async () => {
    await open('>создать доску')
    document.querySelector('[data-row="cmd:form.board"]').click()
    await flushPromises()
    await type('Ретро')
    await key('Enter', { metaKey: true, ctrlKey: true })
    expect(createBoard).toHaveBeenCalledWith('p1', { name: 'Ретро' })
    expect(push).toHaveBeenCalledWith('/project/web/board/retro')
  })
})
