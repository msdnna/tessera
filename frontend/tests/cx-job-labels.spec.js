import { describe, it, expect, beforeEach } from 'vitest'
import { i18n, setI18nLocale } from '@/i18n'
import { jobName, jobOpText } from '@/utils/jobLabels'

// Доработка 3 of #2800: the background-jobs panel used to show whatever Russian the
// server had already rendered. Now the server sends keys and the panel writes the
// phrase — these specs pin both the rendering and the fallback that keeps an unknown
// key (server ahead of this bundle) readable.

const ctx = () => ({ t: i18n.global.t, te: i18n.global.te })

const worker = (over = {}) => ({
  key: 'notify_delivery',
  name_key: 'notify_delivery',
  name: 'Доставка уведомлений',
  kind: 'worker',
  ...over,
})

const syncRun = (over = {}) => ({
  key: 'syncrun:1',
  name_key: 'gitlab_sync',
  name_arg: 'Pamir Scrum',
  name: 'Синхронизация GitLab · Pamir Scrum',
  kind: 'sync',
  ...over,
})

describe('jobName', () => {
  beforeEach(async () => {
    await setI18nLocale('ru')
  })

  it('renders a worker and a sync run in the active language', async () => {
    expect(jobName(worker(), ctx())).toBe('Доставка уведомлений')
    expect(jobName(syncRun(), ctx())).toBe('Синхронизация GitLab · Pamir Scrum')
    await setI18nLocale('en')
    expect(jobName(worker(), ctx())).toBe('Notification delivery')
    // The integration label is data, not interface — it stays as it is in both.
    expect(jobName(syncRun(), ctx())).toBe('GitLab sync · Pamir Scrum')
  })

  it('falls back to the server string for an unknown key or none at all', () => {
    expect(jobName(worker({ name_key: 'quantum_reindex' }), ctx())).toBe('Доставка уведомлений')
    expect(jobName(syncRun({ name_key: '' }), ctx())).toBe('Синхронизация GitLab · Pamir Scrum')
    expect(jobName(null, ctx())).toBe('')
  })
})

describe('jobOpText', () => {
  beforeEach(async () => {
    await setI18nLocale('ru')
  })

  it('renders a worker op and a sync mode in the active language', async () => {
    const tick = { current_op_key: 'delivery', current_op: 'рассылка уведомлений' }
    const full = { current_op_key: 'sync_full', current_op: 'полная синхронизация' }
    expect(jobOpText(tick, ctx())).toBe('рассылка уведомлений')
    expect(jobOpText(full, ctx())).toBe('полная синхронизация')
    await setI18nLocale('en')
    expect(jobOpText(tick, ctx())).toBe('delivering notifications')
    expect(jobOpText(full, ctx())).toBe('full sync')
  })

  it('falls back to the server string, and is empty when there is no op at all', () => {
    expect(jobOpText({ current_op_key: 'defrag', current_op: 'дефрагментация' }, ctx())).toBe(
      'дефрагментация',
    )
    expect(jobOpText({}, ctx())).toBe('')
  })
})
