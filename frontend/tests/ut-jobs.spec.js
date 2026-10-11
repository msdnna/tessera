import { describe, it, expect } from 'vitest'
import {
  jobUid,
  statusGroup,
  mergeLines,
  unionLines,
  progressPct,
  upsertJob,
  logText,
} from '@/utils/jobs'

const line = (seq, text = `l${seq}`, level = 'info') => ({
  seq,
  at: '2026-10-12T10:00:00Z',
  level,
  text,
})

describe('utils/jobs (#3067)', () => {
  it('names a run by its run id, a worker by its key', () => {
    expect(jobUid({ key: 'export:x', run_id: 'r1' })).toBe('r1')
    expect(jobUid({ key: 'notify_delivery' })).toBe('notify_delivery')
  })

  it('files a stopped run under «Завершены»', () => {
    expect(statusGroup({ status: 'cancelled' })).toBe('done')
    expect(statusGroup({ status: 'failed' })).toBe('failed')
  })

  it('appends only the lines past the last seq, and keeps the tail', () => {
    const have = [line(1), line(2)]
    expect(mergeLines(have, [line(2), line(3)]).map((l) => l.seq)).toEqual([1, 2, 3])
    expect(mergeLines(have, [line(1)])).toBe(have)
    expect(mergeLines([line(1), line(2), line(3)], [line(4)], 2).map((l) => l.seq)).toEqual([3, 4])
  })

  it('unions a backlog with what the socket brought, in order', () => {
    expect(unionLines([line(5), line(6)], [line(1), line(5)]).map((l) => l.seq)).toEqual([1, 5, 6])
  })

  it('computes progress only with a total', () => {
    expect(progressPct({ progress_done: 248, progress_total: 401 })).toBe(62)
    expect(progressPct({ progress_done: 3 })).toBeNull()
  })

  it('upserts a run by uid, keeping its place', () => {
    const list = [{ run_id: 'a', status: 'running' }, { key: 'w' }]
    const next = upsertJob(list, { run_id: 'a', status: 'done' })
    expect(next[0].status).toBe('done')
    expect(next).toHaveLength(2)
    expect(upsertJob(list, { run_id: 'b' })[0].run_id).toBe('b')
  })

  it('renders the log for the clipboard', () => {
    expect(logText([line(1, 'hello', 'warn')], () => '10:00')).toBe('10:00 WARN hello')
  })
})
