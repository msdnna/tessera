import { describe, it, expect } from 'vitest'
import {
  MAX_PDF_BYTES,
  clampPage,
  fitScale,
  formatFileSize,
  isPdfFile,
  pagesAround,
  pdfBlockNode,
  pdfBlocksIn,
  pdfDocument,
} from '@/utils/docPdf'
import { isOfficeFile, importAccept, needsConverter } from '@/utils/docOffice'

describe('isPdfFile', () => {
  it('matches by extension, case-insensitively', () => {
    expect(isPdfFile('смета.pdf')).toBe(true)
    expect(isPdfFile('СМЕТА.PDF')).toBe(true)
    expect(isPdfFile('отчёт.docx')).toBe(false)
    expect(isPdfFile('')).toBe(false)
    expect(isPdfFile(null)).toBe(false)
  })

  // "pdf" appearing anywhere but at the end is a different file. Without the
  // anchor a "pdf-инструкция.docx" would take the store-don't-convert path and
  // be refused by the server's content sniff instead of being converted.
  it('does not match a name that merely contains pdf', () => {
    expect(isPdfFile('pdf-инструкция.docx')).toBe(false)
  })
})

describe('pdfBlockNode', () => {
  it('builds the block from an API descriptor', () => {
    const node = pdfBlockNode({ src: '/api/documents/asset?doc=1', name: 'смета.pdf', size: 2048 })
    expect(node).toEqual({
      type: 'pdfEmbed',
      attrs: { src: '/api/documents/asset?doc=1', name: 'смета.pdf', size: 2048 },
    })
  })

  it('falls back to a name rather than an empty header', () => {
    expect(pdfBlockNode({ src: '/x' }).attrs.name).toBe('документ.pdf')
    expect(pdfBlockNode({ src: '/x', size: 'nope' }).attrs.size).toBe(0)
  })

  // A block with no src renders as a viewer that can never load. Failing at the
  // point the response is malformed says which side is wrong.
  it('refuses a descriptor with no link', () => {
    expect(() => pdfBlockNode({ name: 'x.pdf' })).toThrow()
    expect(() => pdfBlockNode(null)).toThrow()
  })

  it('wraps a single block into a document body', () => {
    expect(pdfDocument({ src: '/x', name: 'a.pdf' })).toEqual({
      type: 'doc',
      content: [{ type: 'pdfEmbed', attrs: { src: '/x', name: 'a.pdf', size: 0 } }],
    })
  })
})

describe('formatFileSize', () => {
  it('scales and uses Russian units', () => {
    expect(formatFileSize(512)).toBe('512 Б')
    expect(formatFileSize(2048)).toBe('2 КБ')
    expect(formatFileSize(1024 * 1024 * 12.5)).toBe('12,5 МБ')
  })

  it('says nothing for an unknown size', () => {
    expect(formatFileSize(0)).toBe('')
    expect(formatFileSize(undefined)).toBe('')
    expect(formatFileSize(-1)).toBe('')
  })
})

describe('clampPage', () => {
  it('keeps a page inside the document', () => {
    expect(clampPage(0, 10)).toBe(1)
    expect(clampPage(5, 10)).toBe(5)
    expect(clampPage(99, 10)).toBe(10)
  })

  // "стр. 0 из 0" is the shipped version of this bug, so the floor is 1 even
  // for a document whose page count is not known yet.
  it('never returns zero', () => {
    expect(clampPage(1, 0)).toBe(1)
    expect(clampPage(NaN, NaN)).toBe(1)
  })
})

describe('fitScale', () => {
  it('fits the page to the available width', () => {
    expect(fitScale(600, 300)).toBe(0.5)
    expect(fitScale(600, 900)).toBe(1.5)
  })

  it('clamps rather than blowing a small page up across a wide monitor', () => {
    expect(fitScale(100, 4000)).toBe(2)
    expect(fitScale(4000, 100)).toBe(0.25)
  })

  // A container measured before layout reports 0. Scaling by zero renders an
  // empty canvas, which looks exactly like a file that failed to load.
  it('falls back to 1 when the container has no width yet', () => {
    expect(fitScale(600, 0)).toBe(1)
    expect(fitScale(0, 600)).toBe(1)
    expect(fitScale(600, undefined)).toBe(1)
  })
})

describe('pagesAround', () => {
  it('returns a window clipped to the document', () => {
    expect(pagesAround(1, 10, 2)).toEqual([1, 2, 3])
    expect(pagesAround(5, 10, 2)).toEqual([3, 4, 5, 6, 7])
    expect(pagesAround(10, 10, 2)).toEqual([8, 9, 10])
  })

  it('is empty for a document with no pages', () => {
    expect(pagesAround(1, 0)).toEqual([])
  })
})

describe('pdfBlocksIn', () => {
  it('finds embedded PDFs in reading order', () => {
    const doc = {
      type: 'doc',
      content: [
        { type: 'paragraph', content: [{ type: 'text', text: 'до' }] },
        { type: 'pdfEmbed', attrs: { src: '/a', name: 'a.pdf', size: 10, id: 'b1' } },
        {
          type: 'blockquote',
          content: [{ type: 'pdfEmbed', attrs: { src: '/b', name: 'b.pdf' } }],
        },
      ],
    }
    expect(pdfBlocksIn(doc).map((p) => p.src)).toEqual(['/a', '/b'])
    expect(pdfBlocksIn(doc)[0].id).toBe('b1')
    expect(pdfBlocksIn(doc)[1].size).toBe(0)
  })

  it('tolerates junk', () => {
    expect(pdfBlocksIn(null)).toEqual([])
    expect(pdfBlocksIn({ type: 'doc' })).toEqual([])
  })
})

// The picker and the converter gate are what decide whether the feature is
// reachable at all, and PDF is the one format that must stay reachable when the
// sidecar is not deployed.
describe('the import picker treats PDF as a server import that needs no sidecar', () => {
  it('offers .pdf', () => {
    expect(importAccept()).toContain('.pdf')
  })

  it('routes a PDF through the server import path', () => {
    expect(isOfficeFile('скан.pdf')).toBe(true)
  })

  it('does not gate a PDF behind the converter', () => {
    expect(needsConverter('скан.pdf')).toBe(false)
    expect(needsConverter('отчёт.docx')).toBe(true)
    expect(needsConverter('заметка.md')).toBe(false)
  })
})

it('caps a PDF at the same size the server does', () => {
  expect(MAX_PDF_BYTES).toBe(20 * 1024 * 1024)
})
