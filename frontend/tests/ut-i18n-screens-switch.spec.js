import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NMessageProvider } from 'naive-ui'
import { setI18nLocale } from '@/i18n'
import { milestoneRange } from '@/utils/milestones'
import { prefixLabel, buildTagGroups } from '@/utils/tagGroups'
import { createFormatters } from '@/utils/format'
import NotFoundView from '@/views/NotFoundView.vue'
import HomeView from '@/views/HomeView.vue'
import MilestoneManager from '@/components/MilestoneManager.vue'

// Wave 9 of #2799 — the standalone screens (home, notes, reminders, milestones,
// the sign-up/recovery/invite flow) and the three helpers they share.
//
// Two shapes are pinned here:
//  1. Labels that used to be frozen at import or at prop-default time. A prop
//     default is the nastier of the two: it is never empty, so a `|| $t(…)`
//     fallback next to it can never fire, and the screen stays Russian in every
//     locale while looking perfectly translated in the source.
//  2. Helpers called from outside a setup context (milestones, tagGroups) — they
//     have to reach the catalogue on every call, not once on import.

vi.mock('@/api', () => {
  const ok = (data) => Promise.resolve({ data })
  return {
    workspaces: {
      summary: () => ok({ assigned: 1, active: 2, overdue: 0, due_today: 0, due_week: 0 }),
      tasks: () => ok([]),
      tags: () => ok([]),
      tagPrefixes: () => ok([]),
      members: () => ok([]),
      milestones: () => ok([]),
    },
    projects: { milestones: () => ok([]) },
    milestones: { pushToGitlab: vi.fn(), update: vi.fn(), remove: vi.fn() },
    gitlab: { listIntegrations: () => ok({ integrations: [] }) },
  }
})

// AuthLayout (and the home screen) reach for the theme/workspace stores.
beforeEach(() => {
  setActivePinia(createPinia())
})

// Every wrapper mounted here has to come down before the next test: a screen
// left mounted keeps re-rendering on later locale switches, and once its DOM is
// gone that render throws inside the scheduler — which silently drops the
// re-render of whatever was supposed to be under test.
const mounted = []
const track = (w) => {
  mounted.push(w)
  return w
}

afterEach(async () => {
  while (mounted.length) mounted.pop().unmount()
  document.body.innerHTML = ''
  await setI18nLocale('ru')
})

describe('404 screen', () => {
  it('falls back to the catalogue instead of a Russian prop default', async () => {
    const w = track(mount(NotFoundView, { global: { stubs: { RouterLink: true } } }))
    expect(w.text()).toContain('Страница не найдена')

    await setI18nLocale('en')
    // Before the fix the default was the Russian wording, so `title || $t(…)`
    // never reached the catalogue and this stayed Russian.
    expect(w.text()).toContain('Page not found')
    expect(w.text()).not.toContain('Страница не найдена')
  })

  it('still honours an explicit override', async () => {
    const w = track(
      mount(NotFoundView, {
        props: { title: 'Проект удалён', text: 'Вернитесь на главную' },
        global: { stubs: { RouterLink: true } },
      }),
    )
    expect(w.text()).toContain('Проект удалён')
  })
})

describe('home screen', () => {
  it('re-labels the summary cards after a switch', async () => {
    const w = track(mount(HomeView))
    const labels = () => w.vm.cards.map((c) => c.label)
    expect(labels()).toEqual([
      'Мои задачи',
      'Все активные',
      'Просрочено',
      'Сегодня',
      'На неделе',
      'Выполнено',
    ])

    await setI18nLocale('en')
    expect(labels()).toEqual([
      'My tasks',
      'All active',
      'Overdue',
      'Today',
      'This week',
      'Completed',
    ])
    // The keys drive the filter; they are wiring, not text.
    expect(w.vm.cards.map((c) => c.key)).toEqual([
      'me',
      'all',
      'overdue',
      'today',
      'week',
      'completed',
    ])
  })
})

describe('milestone manager', () => {
  it('re-renders its texts on a mounted modal', async () => {
    track(
      mount(
        {
          render: () =>
            h(NMessageProvider, null, {
              default: () =>
                h(MilestoneManager, { show: true, projectId: 'p1', projectName: 'Atlas' }),
            }),
        },
        { attachTo: document.body },
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    expect(document.body.textContent).toContain('Этапы — Atlas')

    await setI18nLocale('en')
    // The card is teleported, so let the whole render queue drain — one tick on
    // the wrapper is not enough to repaint the modal's own tree.
    await new Promise((r) => setTimeout(r, 0))
    expect(document.body.textContent).toContain('Milestones — Atlas')
    expect(document.body.textContent).not.toContain('Этапы — Atlas')
  })
})

describe('milestone range', () => {
  const fmt = (language) => createFormatters({ language, timezone: 'UTC' })

  it('speaks the current language on every call', async () => {
    const onlyDue = { due_date: '2026-06-15T00:00:00Z' }
    const onlyStart = { start_date: '2026-06-01T00:00:00Z' }
    expect(milestoneRange(onlyDue, fmt('ru'))).toBe('до 15 июн. 2026 г.')
    expect(milestoneRange(onlyStart, fmt('ru'))).toBe('с 1 июн. 2026 г.')

    await setI18nLocale('en')
    expect(milestoneRange(onlyDue, fmt('en'))).toBe('until 15 Jun 2026')
    expect(milestoneRange(onlyStart, fmt('en'))).toBe('from 1 Jun 2026')
  })

  it('needs no wording when both ends are set', () => {
    const both = { start_date: '2026-06-01T00:00:00Z', due_date: '2026-06-15T00:00:00Z' }
    expect(milestoneRange(both, fmt('ru'))).toBe('1 июн. 2026 г. – 15 июн. 2026 г.')
  })
})

describe('tag groups', () => {
  it('translates the prefix-less bucket', async () => {
    expect(prefixLabel('')).toBe('Вне группы')
    expect(prefixLabel('S: ')).toBe('S:')

    await setI18nLocale('en')
    expect(prefixLabel('')).toBe('Ungrouped')
    // A configured prefix name is user data — it stays as entered.
    expect(prefixLabel('S: ', { 's:': 'Статус' })).toBe('Статус')
  })

  it('keeps the bucket last and follows the language', async () => {
    const tags = [{ name: 'urgent' }, { name: 'S: open' }]
    expect(buildTagGroups(tags).map((g) => g.label)).toEqual(['S:', 'Вне группы'])

    await setI18nLocale('en')
    expect(buildTagGroups(tags).map((g) => g.label)).toEqual(['S:', 'Ungrouped'])
  })
})
