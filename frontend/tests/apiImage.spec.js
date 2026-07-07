import { describe, it, expect } from 'vitest'
import { ref, computed } from 'vue'
import { useApiImage } from '@/composables/useApiImage'

// On web (no Tauri) absolutizeApiUrl is a no-op, so useApiImage returns the URL
// unchanged. The point here is that a plain string, a getter, AND a ref/computed
// all resolve — an earlier regression passed a computed ref and crashed with
// "e is not a function" because the composable called it instead of unwrapping it.
describe('useApiImage', () => {
  it('accepts a plain string', () => {
    expect(useApiImage('/api/users/1/avatar').value).toBe('/api/users/1/avatar')
  })

  it('accepts a getter', () => {
    expect(useApiImage(() => '/api/users/2/avatar').value).toBe('/api/users/2/avatar')
  })

  it('accepts a ref or computed', () => {
    expect(useApiImage(ref('/api/users/3/avatar')).value).toBe('/api/users/3/avatar')
    expect(useApiImage(computed(() => '/api/users/4/avatar')).value).toBe('/api/users/4/avatar')
  })

  it('resolves empty to an empty string', () => {
    expect(useApiImage('').value).toBe('')
    expect(useApiImage(() => undefined).value).toBe('')
  })
})
