// The «Проект» window (#2971): its tabs, and the tag table's bulk actions —
// merge into the most used tag, delete at once with an undo that restores what
// the server handed back, GitLab-ruled tags that cannot be selected.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { h, nextTick } from 'vue'
import * as naive from 'naive-ui'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ params: {}, query: {}, path: '/' }),
}))

const TAGS = [
  { id: 'a', name: 'design', color: '#eb2f96', task_count: 17, source: 'user' },
  { id: 'b', name: 'дизайн', color: '#eb2f96', task_count: 3, source: 'user' },
  { id: 'c', name: 'ui', color: '#eb2f96', task_count: 5, source: 'gitlab' },
  { id: 'd', name: 'type::bug', color: '#e0533d', task_count: 23, source: 'gitlab' },
]
const deleted = [{ ...TAGS[1], links: [{ task_id: 't1', source: 'user' }] }]
const mergeTags = vi.fn(() => Promise.resolve({ data: {} }))
const deleteTags = vi.fn(() => Promise.resolve({ data: { tags: deleted } }))
const restoreTags = vi.fn(() => Promise.resolve({ data: { tags: [] } }))
vi.mock('@/api', () => ({
  projects: {
    tags: vi.fn(() => Promise.resolve({ data: TAGS })),
    tagPrefixes: vi.fn(() => Promise.resolve({ data: [] })),
    mergeTags: (...a) => mergeTags(...a),
    deleteTags: (...a) => deleteTags(...a),
    restoreTags: (...a) => restoreTags(...a),
    milestones: vi.fn(() => Promise.resolve({ data: [] })),
    update: vi.fn(),
  },
  milestones: {},
  workspaces: { members: vi.fn(() => Promise.resolve({ data: [{}, {}] })) },
  gitlab: {
    listIntegrations: vi.fn(() =>
      Promise.resolve({
        data: {
          integrations: [
            {
              project_id: 'p1',
              enabled: true,
              project_path: 'tessera/web',
              label_rules: { rules: [{ match: 'type::', match_type: 'prefix', action: 'tag' }] },
            },
          ],
        },
      }),
    ),
  },
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
}

let wrapper = null
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

async function mountTags() {
  const { useWorkspacesStore } = await import('@/stores/workspaces')
  useWorkspacesStore().list = [{ id: 'w1', name: 'W' }]
  useWorkspacesStore().currentId = 'w1'
  const ProjectTagsTab = (await import('@/components/project/ProjectTagsTab.vue')).default
  wrapper = mount(ProjectTagsTab, {
    props: { project: { id: 'p1', slug: 'atlas' } },
    global: { stubs },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}
const row = (w, name) => w.find(`[data-tag="${name}"]`)

describe('tag table', () => {
  it('locks the tags of a GitLab rule: no checkbox, no menu', async () => {
    const w = await mountTags()
    const ruled = row(w, 'type::bug')
    expect(ruled.classes()).toContain('ruled')
    expect(ruled.find('[data-testid="tag-check"]').attributes('disabled')).toBeDefined()
    expect(ruled.find('[data-testid="tag-row-menu"]').exists()).toBe(false)
    expect(row(w, 'ui').classes()).not.toContain('ruled')
  })

  it('merges the selection into its most used tag', async () => {
    const w = await mountTags()
    for (const n of ['дизайн', 'ui', 'design'])
      await row(w, n).find('[data-testid="tag-check"]').trigger('click')
    expect(w.find('[data-testid="tag-selection-bar"]').exists()).toBe(true)
    await w.find('[data-testid="tag-merge"]').trigger('click')
    await nextTick()
    await document.body.querySelector('[data-testid="tag-merge-form-submit"]').click()
    await flushPromises()
    expect(mergeTags).toHaveBeenCalledTimes(1)
    const [pid, target, sources] = mergeTags.mock.calls[0]
    expect(pid).toBe('p1')
    expect(target).toBe('a')
    expect(sources.sort()).toEqual(['b', 'c'])
  })

  it('deletes at once and the undo restores what came back', async () => {
    const w = await mountTags()
    await row(w, 'дизайн').find('[data-testid="tag-check"]').trigger('click')
    await w.find('[data-testid="tag-delete-selected"]').trigger('click')
    await flushPromises()
    expect(deleteTags).toHaveBeenCalledWith('p1', ['b'])
    const { useUndoStore } = await import('@/stores/undo')
    const undo = useUndoStore()
    expect(undo.toasts).toHaveLength(1)
    await undo.undo(undo.toasts[0].id)
    expect(restoreTags).toHaveBeenCalledWith('p1', deleted)
  })

  it('Space selects the row under the cursor, Esc clears the selection', async () => {
    const w = await mountTags()
    const r = row(w, 'design')
    await r.trigger('focus')
    await w.find('.tt-body').trigger('keydown', { key: ' ' })
    expect(w.find('[data-testid="tag-selection-bar"]').exists()).toBe(true)
    expect(w.vm.$.exposed.escape()).toBe(true)
    await nextTick()
    expect(w.find('[data-testid="tag-selection-bar"]').exists()).toBe(false)
  })
})

describe('project window', () => {
  it('opens on the asked tab and switches tabs', async () => {
    const { useWorkspacesStore } = await import('@/stores/workspaces')
    const { useProjectWindowStore } = await import('@/stores/projectWindow')
    const ws = useWorkspacesStore()
    ws.projects = [{ id: 'p1', name: 'Atlas', slug: 'atlas', group_id: null }]
    const win = useProjectWindowStore()
    const ProjectWindow = (await import('@/components/project/ProjectWindow.vue')).default
    wrapper = mount(
      { render: () => h(naive.NMessageProvider, null, { default: () => h(ProjectWindow) }) },
      { attachTo: document.body, global: { stubs } },
    )
    win.open('p1', 'stages')
    await flushPromises()
    expect(document.body.querySelector('[data-testid="project-window"]')).toBeTruthy()
    expect(document.body.querySelector('[data-testid="project-stages"]')).toBeTruthy()
    document.body.querySelector('[data-testid="project-tab-tags"]').click()
    await flushPromises()
    expect(win.tab).toBe('tags')
    expect(document.body.querySelector('[data-testid="project-tags"]')).toBeTruthy()
    document.body.querySelector('[data-testid="project-window-close"]').click()
    await flushPromises()
    expect(win.projectId).toBe(null)
  })
})
