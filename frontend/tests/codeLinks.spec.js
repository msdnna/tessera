import { describe, it, expect } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import {
  changeStatus,
  ciMeta,
  branchLine,
  sortCodeLinks,
  codeLinksPatch,
  codeChangeText,
} from '@/utils/codeLinks'
import { eventText } from '@/utils/taskFeed'
import { useCodeLinksStore } from '@/stores/codeLinks'
import { i18n } from '@/i18n'

// «Разработка» (#2964): the merge-request card's pill and pipeline line, the
// order of several MRs, the socket patch and the history wording.
describe('codeLinks', () => {
  it('maps the review state onto a pill; a draft is neutral', () => {
    expect(changeStatus({ state: 'open' })).toEqual({ key: 'open', color: '#18a058' })
    expect(changeStatus({ state: 'open', draft: true })).toEqual({ key: 'draft', color: null })
    expect(changeStatus({ state: 'merged', draft: true }).key).toBe('merged')
    expect(changeStatus({ state: 'closed' }).key).toBe('closed')
    // Unknown → open rather than a blank pill.
    expect(changeStatus({ state: 'weird' }).key).toBe('open')
  })

  it('maps the CI status; no pipeline draws nothing', () => {
    expect(ciMeta('success')).toMatchObject({ key: 'success', icon: 'success' })
    expect(ciMeta('running').key).toBe('running')
    expect(ciMeta('failed').key).toBe('failed')
    expect(ciMeta('canceled').key).toBe('canceled')
    expect(ciMeta('manual').key).toBe('manual')
    expect(ciMeta('')).toBeNull()
    expect(ciMeta(undefined)).toBeNull()
  })

  it('renders the branch line', () => {
    expect(branchLine({ source_branch: 'feat/board-v2', target_branch: 'main' })).toBe(
      'feat/board-v2 → main',
    )
    expect(branchLine({ source_branch: 'feat/x' })).toBe('feat/x')
    expect(branchLine({})).toBe('')
  })

  it('orders open first, then the most recently active', () => {
    const list = sortCodeLinks([
      { number: 1, state: 'merged', updated_at: '2026-10-05T00:00:00Z' },
      { number: 2, state: 'open', updated_at: '2026-10-01T00:00:00Z' },
      { number: 3, state: 'open', updated_at: '2026-10-03T00:00:00Z' },
      { number: 4, state: 'closed', updated_at: null },
    ])
    expect(list.map((l) => l.number)).toEqual([3, 2, 1, 4])
  })

  it('patches the store from the socket; a fresh fetch wins', () => {
    setActivePinia(createPinia())
    const store = useCodeLinksStore()
    expect(codeLinksPatch({ type: 'task.updated', data: {} })).toBeNull()
    expect(codeLinksPatch({ type: 'task.code_links', data: {} })).toBeNull()
    store.onEvent({
      type: 'task.code_links',
      data: { task_id: 't1', code_links: [{ number: 418, state: 'open', ci_status: 'running' }] },
    })
    expect(store.byTask.t1[0].ci_status).toBe('running')
    // An emptied list is a real answer (the MR stopped naming the task).
    store.onEvent({ type: 'task.code_links', data: { task_id: 't1', code_links: null } })
    expect(store.byTask.t1).toEqual([])
    store.forget('t1')
    expect(store.byTask.t1).toBeUndefined()
  })

  it('words the history line', () => {
    expect(codeChangeText({ state: 'open', number: 418, branch: 'feat/board-v2' })).toEqual({
      key: 'codeOpenedOn',
      params: { ref: '!418', branch: 'feat/board-v2' },
    })
    expect(codeChangeText({ state: 'merged', number: 418 }).key).toBe('codeMerged')
    expect(codeChangeText({ state: 'closed', number: 9 }).key).toBe('codeClosed')
    expect(codeChangeText({ number: 5 }).key).toBe('codeOpened')

    const prev = i18n.global.locale.value
    i18n.global.locale.value = 'ru'
    expect(
      eventText({
        kind: 'code_change',
        data: { state: 'open', number: 418, branch: 'feat/board-v2' },
      }),
    ).toBe('открыт !418 в feat/board-v2')
    expect(eventText({ kind: 'code_change', data: { state: 'merged', number: 418 } })).toBe(
      'смержен !418',
    )
    i18n.global.locale.value = prev
  })
})
