import { describe, expect, it, vi } from 'vitest'
import {
  annotatePayload,
  embedStatus,
  parseEmbedParams,
  parseImportPayload,
  sendToHost,
} from '@/utils/docEmbed'

// The contract with the Android app (#2894 §4). Every rule here fails silently
// when it breaks — a dropped parameter gives a blank editor, a wrong status word
// gives a native "сохранено" over text that was never written — so the contract
// is asserted rather than trusted.
describe('parseEmbedParams', () => {
  it('takes the session and the workspace, and says when either is missing', () => {
    const ok = parseEmbedParams({ token: 'tok', ws: 'ws-1', theme: 'dark', lang: 'en' })
    expect(ok).toMatchObject({ token: 'tok', workspaceId: 'ws-1', dark: true, locale: 'en' })
    expect(ok.ready).toBe(true)

    // Rendering an empty editor without a session invites typing into a void:
    // the text would have nowhere to go and no error would say so.
    expect(parseEmbedParams({ ws: 'ws-1' }).ready).toBe(false)
    expect(parseEmbedParams({ token: 'tok' }).ready).toBe(false)
    expect(parseEmbedParams({}).ready).toBe(false)
  })

  it('is light unless dark is asked for by name', () => {
    expect(parseEmbedParams({ token: 't', ws: 'w', theme: 'light' }).dark).toBe(false)
    expect(parseEmbedParams({ token: 't', ws: 'w' }).dark).toBe(false)
    // Not "any value means dark": a stale host sending theme=1 must not flip it.
    expect(parseEmbedParams({ token: 't', ws: 'w', theme: '1' }).dark).toBe(false)
  })

  it('ignores a language this build has no messages for', () => {
    expect(parseEmbedParams({ token: 't', ws: 'w', lang: 'ru' }).locale).toBe('ru')
    // Empty, not the fallback: the view only switches when told to, and an
    // unknown code means the app is newer than this page.
    expect(parseEmbedParams({ token: 't', ws: 'w', lang: 'de' }).locale).toBe('')
    expect(parseEmbedParams({ token: 't', ws: 'w' }).locale).toBe('')
  })

  it('survives a query the router hands over as arrays or nulls', () => {
    // Repeated params arrive as arrays from vue-router; neither is a token.
    const parsed = parseEmbedParams({ token: ['a', 'b'], ws: null })
    expect(parsed.ready).toBe(false)
    expect(parsed.token).toBe('')
  })
})

describe('embedStatus', () => {
  it('reports what the host shows above the editor', () => {
    expect(embedStatus({})).toBe('saved')
    expect(embedStatus({ dirty: true })).toBe('dirty')
    expect(embedStatus({ saving: true })).toBe('saving')
    expect(embedStatus({ error: 'boom' })).toBe('error')
    expect(embedStatus({ conflict: true })).toBe('conflict')
  })

  it('ranks a conflict above everything else', () => {
    // Autosave stops on 409 while dirty stays true — the true statement is that
    // saving has stopped, not that there are unsaved changes.
    expect(embedStatus({ conflict: true, dirty: true, saving: true, error: 'x' })).toBe('conflict')
    // A failed write keeps its content queued, so dirty is true there too.
    expect(embedStatus({ error: 'x', dirty: true })).toBe('error')
    // Mid-flight beats queued: the debounce already fired.
    expect(embedStatus({ saving: true, dirty: true })).toBe('saving')
  })
})

describe('sendToHost', () => {
  it('stringifies objects, because everything crosses the bridge as a string', () => {
    const host = { onStatus: vi.fn() }
    expect(sendToHost('onStatus', { status: 'saved' }, host)).toBe(true)
    expect(host.onStatus).toHaveBeenCalledWith('{"status":"saved"}')
  })

  it('passes a string through untouched and a call with no payload bare', () => {
    const host = { onBlocked: vi.fn(), requestToken: vi.fn() }
    sendToHost('onBlocked', 'Пётр', host)
    expect(host.onBlocked).toHaveBeenCalledWith('Пётр')
    sendToHost('requestToken', undefined, host)
    expect(host.requestToken).toHaveBeenCalledWith()
  })

  it('is a no-op without a host, so the page still runs in a browser', () => {
    expect(sendToHost('onStatus', { status: 'saved' }, undefined)).toBe(false)
    expect(sendToHost('onNewEvent', {}, {})).toBe(false)
  })

  it('swallows a dead bridge instead of taking the editor down with it', () => {
    // The WebView being torn down mid-save is the real case: there is no one
    // left to show an error to, and throwing inside a watcher unmounts the view.
    const host = {
      onStatus: () => {
        throw new Error('bridge is gone')
      },
    }
    expect(sendToHost('onStatus', { status: 'saving' }, host)).toBe(false)
  })
})

// The tap that opens a native discussion (#2894 §5). Everything the phone needs
// leaves with it: there is no panel beside the text to look anything up in.
describe('annotatePayload', () => {
  const doc = {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        attrs: { id: 'p1' },
        content: [{ type: 'text', text: 'Первый  абзац ' }],
      },
      {
        type: 'bulletList',
        attrs: { id: 'l1' },
        content: [
          {
            type: 'listItem',
            content: [
              {
                type: 'paragraph',
                attrs: { id: 'p2' },
                content: [{ type: 'text', text: 'пункт' }],
              },
            ],
          },
        ],
      },
    ],
  }

  it('carries the anchor, the block text as it stands, and the reading order', () => {
    const payload = annotatePayload(doc, 'p1')
    expect(payload.block_id).toBe('p1')
    // Whitespace collapsed: the quote is a one-line label on a phone, not a
    // copy of the paragraph's formatting.
    expect(payload.quote).toBe('Первый абзац')
    // Reading order, including the paragraph nested inside the list — that is
    // what tells an anchored thread from a detached one on the host side.
    expect(payload.blocks).toEqual(['p1', 'l1', 'p2'])
  })

  it('quotes nothing when the block is already gone', () => {
    // A tap on the margin count of a block deleted meanwhile: the thread is
    // detached, and an empty quote is the honest answer rather than a stale one.
    const payload = annotatePayload(doc, 'vanished')
    expect(payload.block_id).toBe('vanished')
    expect(payload.quote).toBe('')
    expect(payload.blocks).toEqual(['p1', 'l1', 'p2'])
  })

  it('without a block, the sheet opens on the document', () => {
    const payload = annotatePayload(doc, '')
    expect(payload.block_id).toBe('')
    expect(payload.quote).toBe('')
  })
})

// The office import the phone hands over (#2894 §8). The payload crosses the
// bridge as a string, so nothing about it is guaranteed — and the failure it
// guards against is the loud one: applying an empty body to an open document
// replaces the text with nothing and then autosaves that.
describe('parseImportPayload', () => {
  it('takes the converted html and the page geometry beside it', () => {
    const payload = parseImportPayload(
      JSON.stringify({ html: '<p>Абзац</p>', page: { format: 'A4', orientation: 'portrait' } }),
    )
    expect(payload.html).toBe('<p>Абзац</p>')
    expect(payload.page).toEqual({ format: 'A4', orientation: 'portrait' })
    expect(payload.pdf).toBeNull()
  })

  it('accepts an object as readily as its serialized form', () => {
    // The bridge stringifies, but a WebView message handler need not — the two
    // call sites must not disagree about which one works.
    expect(parseImportPayload({ html: '<p>x</p>' }).html).toBe('<p>x</p>')
  })

  it('carries a stored pdf, which arrives with no html at all', () => {
    // A PDF is never converted, so an html-only rule would reject the one
    // import that works on an install with no sidecar deployed.
    const payload = parseImportPayload({ pdf: { src: '/api/documents/1/assets/a.pdf' } })
    expect(payload.pdf).toEqual({ src: '/api/documents/1/assets/a.pdf' })
    expect(payload.html).toBe('')
  })

  it('refuses a payload with nothing to apply', () => {
    // Each of these would reach the editor as "an import happened"; applying
    // any of them blanks the document the user has open.
    expect(parseImportPayload('not json')).toBeNull()
    expect(parseImportPayload('')).toBeNull()
    expect(parseImportPayload(null)).toBeNull()
    expect(parseImportPayload({})).toBeNull()
    expect(parseImportPayload({ html: '' })).toBeNull()
    expect(parseImportPayload({ html: '   ' })).toBeNull()
    // A pdf without a source is not a block anything can render.
    expect(parseImportPayload({ pdf: {} })).toBeNull()
    expect(parseImportPayload({ pdf: 'a.pdf' })).toBeNull()
  })

  it('drops page geometry that is not an object rather than passing it on', () => {
    // withImportedPage reads fields off it; a string here would throw inside
    // the editor, after the html had already been parsed.
    expect(parseImportPayload({ html: '<p>x</p>', page: 'A4' }).page).toBeNull()
  })
})
