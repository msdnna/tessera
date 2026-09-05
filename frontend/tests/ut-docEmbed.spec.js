import { describe, expect, it, vi } from 'vitest'
import { embedStatus, parseEmbedParams, sendToHost } from '@/utils/docEmbed'

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
