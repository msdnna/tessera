import { describe, it, expect, afterEach, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { i18n, setI18nLocale } from '@/i18n'
import { approvalStatusLabel, stepStatusLabel } from '@/utils/docApprovals'
import { formatFileSize } from '@/utils/docPdf'
import { authorLabel } from '@/utils/docComments'
import { blockText } from '@/utils/docDiff'
import { exportFileName } from '@/utils/docOffice'
import DocHistory from '@/components/documents/DocHistory.vue'
import DocLinks from '@/components/documents/DocLinks.vue'

// Wave 5 of #2799 — the sidebar panels (history, links, approvals) and the pure
// helpers behind them. Same guard as ut-i18n-doc-switch: the status tables here
// were module-level maps of Russian strings, so translating them «in place»
// would read correctly until the first switch and then stay frozen.

// useFormat() reaches for the theme store to read the date preferences.
beforeEach(() => setActivePinia(createPinia()))

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('approval labels follow a language switch', () => {
  it('re-reads the route and step statuses', async () => {
    expect(approvalStatusLabel('pending')).toBe('На согласовании')
    expect(stepStatusLabel('approved')).toBe('подписал')

    await setI18nLocale('en')
    expect(approvalStatusLabel('pending')).toBe('Under approval')
    expect(stepStatusLabel('approved')).toBe('signed')
  })

  // A status the server invented is shown as it came rather than as a missing
  // translation key — the panel must not turn an unknown state into noise.
  it('passes an unknown status through untranslated', () => {
    expect(approvalStatusLabel('escalated')).toBe('escalated')
    expect(stepStatusLabel('escalated')).toBe('escalated')
  })

  it('re-renders the route mode options of the links panel', async () => {
    const w = mount(DocLinks, { props: { wsId: 'w1' } })
    expect(w.vm.modeOptions.map((o) => o.label)).toEqual(['По очереди', 'Одновременно'])

    await setI18nLocale('en')
    expect(w.vm.modeOptions.map((o) => o.label)).toEqual(['One by one', 'All at once'])
    // The values are the wire format the server matches on; they never move.
    expect(w.vm.modeOptions.map((o) => o.value)).toEqual(['sequential', 'parallel'])
  })
})

describe('history panel', () => {
  const versions = [
    {
      id: 'v2',
      revision: 2,
      manual: false,
      created_at: '2026-08-20T10:00:00Z',
      updated_at: '2026-08-20T10:05:00Z',
      author_name: '',
      author_email: '',
      preview: '',
    },
  ]

  it('re-renders the entry, the unknown author and the empty preview', async () => {
    const w = mount(DocHistory, { props: { versions } })
    expect(w.find('.rev').text()).toBe('Версия 2')
    expect(w.find('.author').text()).toBe('Неизвестный автор')
    expect(w.find('.preview').text()).toBe('Пустой документ')

    await setI18nLocale('en')
    expect(w.find('.rev').text()).toBe('Version 2')
    expect(w.find('.author').text()).toBe('Unknown author')
    expect(w.find('.preview').text()).toBe('Empty document')
  })

  // The counters used to read «5 блоков» for every number. Russian has three
  // forms and English two, so the branch is the catalogue's job now.
  it.each([
    [1, '+1 блок'],
    [2, '+2 блока'],
    [5, '+5 блоков'],
    [21, '+21 блок'],
  ])('declines the added-block counter for %i', (n, expected) => {
    expect(i18n.global.t('documents.history.added', n, { count: n })).toBe(expected)
  })

  it('gives English its own two forms', async () => {
    await setI18nLocale('en')
    expect(i18n.global.t('documents.history.added', 1, { count: 1 })).toBe('+1 block')
    expect(i18n.global.t('documents.history.added', 5, { count: 5 })).toBe('+5 blocks')
  })

  it('re-reads the diff status badges', async () => {
    const w = mount(DocHistory, {
      props: {
        versions,
        selectedId: 'v2',
        ready: true,
        summary: { added: 1 },
        rows: [{ status: 'added', text: 'Абзац' }],
      },
    })
    expect(w.find('.badge').text()).toBe('добавлено')

    await setI18nLocale('en')
    expect(w.find('.badge').text()).toBe('added')
  })
})

describe('helpers that write text of their own', () => {
  it('formats a file size in the reader units and separator', async () => {
    expect(formatFileSize(512)).toBe('512 Б')
    expect(formatFileSize(1024 * 1024 * 12.5)).toBe('12,5 МБ')

    await setI18nLocale('en')
    expect(formatFileSize(512)).toBe('512 B')
    // The decimal separator comes from Intl now: the hand-written comma was
    // right for Russian and wrong for every locale that keeps the point.
    expect(formatFileSize(1024 * 1024 * 12.5)).toBe('12.5 MB')
  })

  it('re-reads the fallbacks of the comment author, the diff and the export name', async () => {
    expect(authorLabel({})).toBe('Участник')
    expect(blockText({ type: 'image' })).toBe('[изображение]')
    expect(exportFileName('', 'pdf')).toBe('Документ.pdf')

    await setI18nLocale('en')
    expect(authorLabel({})).toBe('Member')
    expect(blockText({ type: 'image' })).toBe('[image]')
    expect(exportFileName('', 'pdf')).toBe('Document.pdf')
  })
})
