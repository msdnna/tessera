import { describe, it, expect, vi, beforeEach } from 'vitest'

// The composable talks to the API and nothing else, so the client is the only
// thing worth faking here.
const api = {
  versions: vi.fn(),
  version: vi.fn(),
  snapshot: vi.fn(),
  restoreVersion: vi.fn(),
}
vi.mock('@/api', () => ({ documents: api }))

const { useDocVersions } = await import('@/composables/useDocVersions')

// Journal rows as the list endpoint returns them: newest first, no bodies.
function entry(revision, over = {}) {
  return {
    id: 'v' + revision,
    revision,
    title: 'Документ',
    preview: 'текст версии ' + revision,
    label: '',
    manual: false,
    author_name: 'Иван',
    created_at: '2026-08-15T10:00:00Z',
    updated_at: '2026-08-15T10:05:00Z',
    ...over,
  }
}

const doc = (id, text) => ({
  type: 'doc',
  content: [{ type: 'paragraph', attrs: { id }, content: [{ type: 'text', text }] }],
})

describe('useDocVersions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.versions.mockResolvedValue({ data: [entry(3), entry(2), entry(1)] })
    api.version.mockImplementation((id) =>
      Promise.resolve({ data: { id, content: doc('a', 'текст ' + id) } }),
    )
  })

  it('loads the journal for the document it is opened on', async () => {
    const v = useDocVersions()
    await v.open('doc-1')
    expect(api.versions).toHaveBeenCalledWith('doc-1')
    expect(v.versions.value).toHaveLength(3)
    // Nothing is compared until a version is picked — an empty diff on open
    // would read as "ничего не менялось".
    expect(v.ready.value).toBe(false)
    expect(v.rows.value).toEqual([])
  })

  // The default question a journal answers is "что изменилось с тех пор", so the
  // newest entry is the other side of the comparison unless one is chosen.
  it('compares the picked version against the newest one', async () => {
    const v = useDocVersions()
    await v.open('doc-1')
    await v.select('v1')
    expect(v.baseline.value.revision).toBe(3)
    expect(v.ready.value).toBe(true)
    expect(api.version).toHaveBeenCalledWith('v1')
    expect(api.version).toHaveBeenCalledWith('v3')
  })

  it('fetches a body once and reuses it', async () => {
    const v = useDocVersions()
    await v.open('doc-1')
    await v.select('v1')
    await v.select('v1') // deselect
    await v.select('v1')
    expect(api.version.mock.calls.filter(([id]) => id === 'v1')).toHaveLength(1)
  })

  // The newest entry is the live editing session: its body keeps changing as
  // people type, so a cached copy would show a comparison against the document
  // as it was minutes ago and present it as current.
  it('drops the cached body of the newest version on every reload', async () => {
    const v = useDocVersions()
    await v.open('doc-1')
    await v.select('v1')
    expect(api.version.mock.calls.filter(([id]) => id === 'v3')).toHaveLength(1)
    await v.load()
    await v.select('v2')
    expect(api.version.mock.calls.filter(([id]) => id === 'v3')).toHaveLength(2)
  })

  it('orders the comparison by revision, whichever side was picked', async () => {
    api.version.mockImplementation((id) =>
      Promise.resolve({
        data: { id, content: id === 'v1' ? doc('a', 'старое') : doc('a', 'новое') },
      }),
    )
    const v = useDocVersions()
    await v.open('doc-1')
    await v.select('v1')
    // Older on the left: the row shows what the text was, then what it became.
    expect(v.rows.value[0]).toMatchObject({ prevText: 'старое', text: 'новое' })
  })

  it('lets the reader compare two arbitrary versions', async () => {
    const v = useDocVersions()
    await v.open('doc-1')
    await v.compareWith('v2')
    await v.select('v1')
    expect(v.baseline.value.revision).toBe(2)
    expect(api.version).toHaveBeenCalledWith('v2')
    expect(api.version).not.toHaveBeenCalledWith('v3')
  })

  it('reloads the journal after a snapshot so the new entry is in it', async () => {
    api.snapshot.mockResolvedValue({ data: entry(4, { manual: true, label: 'согласовано' }) })
    const v = useDocVersions()
    await v.open('doc-1')
    api.versions.mockResolvedValue({ data: [entry(4, { manual: true }), entry(3)] })
    await v.snapshot('  согласовано  ')
    expect(api.snapshot).toHaveBeenCalledWith('doc-1', 'согласовано')
    expect(v.versions.value[0].manual).toBe(true)
  })

  it('returns the restored document so the caller can reload the editor', async () => {
    api.restoreVersion.mockResolvedValue({
      data: { id: 'doc-1', updated_at: 'v-new', content: doc('a', 'вернули') },
    })
    const v = useDocVersions()
    await v.open('doc-1')
    await v.select('v1')
    const restored = await v.restore('v1')
    expect(api.restoreVersion).toHaveBeenCalledWith('v1')
    expect(restored.updated_at).toBe('v-new')
    // The selection is dropped: it pointed at a comparison that is no longer the
    // one on screen.
    expect(v.selectedId.value).toBe('')
  })

  it('reports a failed load instead of showing an empty journal', async () => {
    api.versions.mockRejectedValue(new Error('сеть недоступна'))
    const v = useDocVersions()
    await v.open('doc-1')
    expect(v.error.value).toBe('сеть недоступна')
    expect(v.versions.value).toEqual([])
  })

  it('forgets everything when the document is closed', async () => {
    const v = useDocVersions()
    await v.open('doc-1')
    await v.select('v1')
    v.close()
    expect(v.versions.value).toEqual([])
    expect(v.selectedId.value).toBe('')
    await v.load()
    expect(api.versions).toHaveBeenCalledTimes(1)
  })
})
