import { describe, it, expect } from 'vitest'
import { apiInRelease, productLabel, shortVersion } from '@/utils/release'

describe('product release (docs/versioning.md)', () => {
  const r = { product: '2.0.0', name: 'Tessera 2', components: { backend: '0.112.0' } }

  it('names the release the way people say it', () => {
    expect(shortVersion('2.0.0')).toBe('2.0')
    expect(shortVersion('2.1.3')).toBe('2.1.3')
    expect(productLabel(r)).toBe('Tessera 2.0')
    expect(productLabel(null)).toBe('')
  })

  it('tells a server outside the release', () => {
    expect(apiInRelease('0.112.0', r)).toBe(true)
    expect(apiInRelease('0.113.0', r)).toBe(false)
    expect(apiInRelease('', r)).toBe(true)
    expect(apiInRelease('0.1.0', null)).toBe(true)
  })
})
