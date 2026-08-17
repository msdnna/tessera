import { describe, expect, it, vi } from 'vitest'
import {
  MAX_OFFICE_BYTES,
  OFFICE_EXTENSIONS,
  downloadBlob,
  exportFileName,
  importAccept,
  importOfficeFile,
  isOfficeFile,
} from '@/utils/docOffice'

function fakeFile(name, size = 10) {
  return { name, size }
}

function stubApi(overrides = {}) {
  return {
    importFile: vi.fn(async () => ({
      data: {
        document: { id: 'doc-1', updated_at: '2026-08-16T00:00:00Z', title: 'Договор' },
        html: '<p>Импортированный текст</p>',
        images_dropped: 0,
      },
    })),
    updateContent: vi.fn(async () => ({ data: { updated_at: '2026-08-16T00:00:01Z' } })),
    ...overrides,
  }
}

describe('isOfficeFile', () => {
  it('recognises the extensions the import route accepts', () => {
    for (const ext of OFFICE_EXTENSIONS) {
      expect(isOfficeFile('файл' + ext)).toBe(true)
      expect(isOfficeFile('ФАЙЛ' + ext.toUpperCase())).toBe(true)
    }
  })

  it('leaves the formats the browser handles on its own to the local path', () => {
    // .md and .json import without a sidecar (D9). Treating them as office
    // files would make an import that works everywhere start depending on
    // LibreOffice being deployed.
    expect(isOfficeFile('notes.md')).toBe(false)
    expect(isOfficeFile('template.json')).toBe(false)
    expect(isOfficeFile('noextension')).toBe(false)
    expect(isOfficeFile(null)).toBe(false)
  })
})

describe('importAccept', () => {
  it('covers both import paths in one picker', () => {
    const accept = importAccept()
    expect(accept).toContain('.docx')
    expect(accept).toContain('.md')
    expect(accept).toContain('.json')
  })
})

describe('importOfficeFile', () => {
  it('uploads the file and saves the parsed body onto the created document', async () => {
    const api = stubApi()
    const { document: doc, imagesDropped } = await importOfficeFile(
      api,
      'ws-1',
      fakeFile('Договор.docx'),
    )

    expect(api.importFile).toHaveBeenCalledTimes(1)
    const [wsId, form] = api.importFile.mock.calls[0]
    expect(wsId).toBe('ws-1')
    expect(form.get('file')).toBeTruthy()

    // The body is saved through the ordinary content endpoint, so an import is
    // validated by exactly the same server code as typing.
    expect(api.updateContent).toHaveBeenCalledTimes(1)
    const [id, content, updatedAt] = api.updateContent.mock.calls[0]
    expect(id).toBe('doc-1')
    expect(updatedAt).toBe('2026-08-16T00:00:00Z')
    expect(content.type).toBe('doc')
    expect(JSON.stringify(content)).toContain('Импортированный текст')

    // The version has to move to what the save returned: the editor opens on
    // this document straight away, and a stale updated_at makes its first
    // autosave collide with our own import.
    expect(doc.updated_at).toBe('2026-08-16T00:00:01Z')
    expect(imagesDropped).toBe(0)
  })

  it('passes the folder the user is looking at', async () => {
    const api = stubApi()
    await importOfficeFile(api, 'ws-1', fakeFile('a.docx'), { parentId: 'parent-9' })
    expect(api.importFile.mock.calls[0][1].get('parent_id')).toBe('parent-9')
  })

  it('reports dropped pictures rather than swallowing them', async () => {
    const api = stubApi({
      importFile: vi.fn(async () => ({
        data: {
          document: { id: 'doc-1', updated_at: 'v1' },
          html: '<p>t</p>',
          images_dropped: 3,
        },
      })),
    })
    const { imagesDropped } = await importOfficeFile(api, 'ws-1', fakeFile('a.docx'))
    expect(imagesDropped).toBe(3)
  })

  it('refuses a format the route does not take, without uploading', async () => {
    const api = stubApi()
    await expect(importOfficeFile(api, 'ws-1', fakeFile('notes.md'))).rejects.toThrow(
      /Поддерживаются/,
    )
    expect(api.importFile).not.toHaveBeenCalled()
  })

  it('refuses an oversized file before spending the upload', async () => {
    const api = stubApi()
    await expect(
      importOfficeFile(api, 'ws-1', fakeFile('big.docx', MAX_OFFICE_BYTES + 1)),
    ).rejects.toThrow(/20 МБ/)
    expect(api.importFile).not.toHaveBeenCalled()
  })

  it('fails loudly when the server answers without a document', async () => {
    const api = stubApi({ importFile: vi.fn(async () => ({ data: { html: '<p>x</p>' } })) })
    await expect(importOfficeFile(api, 'ws-1', fakeFile('a.docx'))).rejects.toThrow()
    expect(api.updateContent).not.toHaveBeenCalled()
  })

  it('requires a file', async () => {
    await expect(importOfficeFile(stubApi(), 'ws-1', null)).rejects.toThrow(/не выбран/)
  })
})

describe('exportFileName', () => {
  it('keeps a Cyrillic title intact', () => {
    expect(exportFileName('Протокол совещания', 'pdf')).toBe('Протокол совещания.pdf')
  })

  it('replaces only the characters a file name cannot hold', () => {
    expect(exportFileName('a/b:c*d?e"f<g>h|i', 'docx')).toBe('a_b_c_d_e_f_g_h_i.docx')
  })

  it('never produces a name that is only an extension', () => {
    // A download called ".pdf" is invisible in a downloads folder.
    expect(exportFileName('   ', 'pdf')).toBe('Документ.pdf')
    expect(exportFileName(null, 'html')).toBe('Документ.html')
  })
})

describe('downloadBlob', () => {
  it('revokes the object URL it created', () => {
    const createObjectURL = vi.fn(() => 'blob:x')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })
    const click = vi.fn()
    const anchor = { click, remove: vi.fn(), href: '', download: '' }
    vi.spyOn(document, 'createElement').mockReturnValue(anchor)
    vi.spyOn(document.body, 'appendChild').mockImplementation(() => anchor)

    downloadBlob(new Blob(['x']), 'Отчёт.pdf')

    expect(anchor.download).toBe('Отчёт.pdf')
    expect(click).toHaveBeenCalled()
    // Without this every export leaks the whole file for the lifetime of the
    // tab, and nobody attributes that to the download button.
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:x')

    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })
})
