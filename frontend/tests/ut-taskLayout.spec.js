// Task presentation modes (#2716). The rule that matters is the narrow-screen
// override: a 560px side panel on a phone is a fullscreen sheet with worse
// ergonomics, so the saved preference is only honoured when it can mean something.
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  TASK_LAYOUTS,
  effectiveTaskLayout,
  loadTaskLayout,
  saveTaskLayout,
} from '@/utils/taskLayout'

describe('effectiveTaskLayout', () => {
  it('passes through the three known layouts on a wide screen', () => {
    for (const l of TASK_LAYOUTS) expect(effectiveTaskLayout(l, false)).toBe(l)
  })

  it('falls back to the modal for unknown or missing values', () => {
    expect(effectiveTaskLayout('drawer', false)).toBe('modal')
    expect(effectiveTaskLayout(null, false)).toBe('modal')
    expect(effectiveTaskLayout(undefined, false)).toBe('modal')
  })

  it('overrides the saved layout on narrow screens', () => {
    expect(effectiveTaskLayout('sidebar', true)).toBe('modal')
    expect(effectiveTaskLayout('fullscreen', true)).toBe('modal')
  })
})

describe('taskLayout persistence', () => {
  beforeEach(() => localStorage.clear())
  afterEach(() => vi.restoreAllMocks())

  it('round-trips a saved layout', () => {
    saveTaskLayout('sidebar')
    expect(loadTaskLayout()).toBe('sidebar')
  })

  it('defaults to the modal when nothing is stored', () => {
    expect(loadTaskLayout()).toBe('modal')
  })

  it('ignores a junk value already in storage', () => {
    localStorage.setItem('tessera_task_layout', 'drawer')
    expect(loadTaskLayout()).toBe('modal')
  })

  it('refuses to persist an unknown layout', () => {
    saveTaskLayout('sidebar')
    saveTaskLayout('drawer')
    expect(loadTaskLayout()).toBe('sidebar')
  })

  it('survives disabled storage instead of throwing', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    expect(loadTaskLayout()).toBe('modal')
    expect(() => saveTaskLayout('sidebar')).not.toThrow()
  })
})
