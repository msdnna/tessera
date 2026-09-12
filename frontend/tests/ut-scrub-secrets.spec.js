import { describe, it, expect } from 'vitest'
import { scrubUrl, scrubEvent, scrubBreadcrumb } from '@/utils/scrubSecrets'

describe('scrubUrl', () => {
  it('blanks secret query values and keeps the rest', () => {
    expect(scrubUrl('https://tessera.lan/recover?token=abc123')).toBe(
      'https://tessera.lan/recover?token=***',
    )
    expect(scrubUrl('/invite?token=abc&ref=email')).toBe('/invite?token=***&ref=email')
    expect(scrubUrl('/board?page=2&limit=50')).toBe('/board?page=2&limit=50')
  })

  it('blanks the fragment too — the OAuth handoff puts tokens there', () => {
    expect(scrubUrl('tessera://oauth/callback#access_token=a&refresh_token=b&state=c')).toBe(
      'tessera://oauth/callback#access_token=***&refresh_token=***&state=***',
    )
    expect(scrubUrl('/verify-email?token=t#section')).toBe('/verify-email?token=***#section')
  })

  it('leaves non-strings and plain paths alone', () => {
    expect(scrubUrl('/board')).toBe('/board')
    expect(scrubUrl('')).toBe('')
    expect(scrubUrl(undefined)).toBe(undefined)
    expect(scrubUrl(42)).toBe(42)
  })
})

describe('scrubEvent', () => {
  it('cleans the request, breadcrumbs, trace context and spans', () => {
    const event = {
      request: { url: 'https://tessera.lan/recover?token=abc', query_string: '?token=abc' },
      breadcrumbs: [
        { category: 'navigation', data: { from: '/login', to: '/recover?token=abc' } },
        { category: 'fetch', data: { url: '/api/uploads/1?sig=deadbeef' } },
      ],
      contexts: { trace: { data: { 'http.url': 'https://tessera.lan/recover?token=abc' } } },
      spans: [{ data: { 'url.full': '/invite?token=abc', 'url.query': '?token=abc' } }],
    }

    const out = scrubEvent(event)
    expect(JSON.stringify(out)).not.toContain('abc')
    expect(JSON.stringify(out)).not.toContain('deadbeef')
    expect(out.request.url).toBe('https://tessera.lan/recover?token=***')
    expect(out.breadcrumbs[0].data.to).toBe('/recover?token=***')
    expect(out.contexts.trace.data['http.url']).toBe('https://tessera.lan/recover?token=***')
    expect(out.spans[0].data['url.query']).toBe('?token=***')
  })

  it('survives the sparse events the SDK actually sends', () => {
    expect(scrubEvent(null)).toBe(null)
    const bare = { message: 'boom' }
    expect(scrubEvent(bare)).toBe(bare)
    expect(scrubBreadcrumb({ category: 'ui.click' })).toEqual({ category: 'ui.click' })
  })
})
