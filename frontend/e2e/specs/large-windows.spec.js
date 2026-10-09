import { test, expect, openBoard } from '../fixtures.js'

// The large windows of package F (v2, #2975): members, commands, background
// jobs, GitLab and the conflicts window — all on kit/LargeWindow. The doors to
// them (the workspace menu, the footer «⋯») are covered elsewhere; here the
// windows are opened through their store, as every door does.
const MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

async function openWindow(page, name) {
  await page.evaluate((n) => {
    const pinia = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia
    pinia._s.get('wsDialogs').open(n)
  }, name)
}

test('участники: адреса чипами, приглашение, отзыв с отменой, разделы цифрами', async ({
  page,
  seed,
}) => {
  await openBoard(page, seed.boardId)
  await openWindow(page, 'members')
  const win = page.getByTestId('members-window')
  await expect(win).toBeVisible()

  const field = page.getByTestId('members-invite-input')
  await field.click()
  // A comma turns the address into a chip; a broken one is marked.
  await field.pressSequentially('broken@nowhere,')
  await expect(page.getByTestId('invite-chip')).toHaveCount(1)
  await expect(page.getByTestId('invite-chip')).toHaveClass(/bad/)
  // ⌫ in the empty field takes it back.
  await field.press('Backspace')
  await expect(page.getByTestId('invite-chip')).toHaveCount(0)

  const guest = `e2e+${seed.runId}-guest@test.local`
  await field.fill(guest)
  await field.press('Enter') // → a chip
  await expect(page.getByTestId('invite-chip')).toHaveCount(1)
  await field.press('Enter') // → sent: nobody has that address, so an invitation
  await expect(page.getByTestId('members-invite-link')).toBeVisible()
  const row = page.getByTestId('invite-row').filter({ hasText: guest })
  await expect(row).toBeVisible()

  // Revoking is at once, and the toast brings it back.
  await row.getByTestId('invite-revoke').click()
  await expect(row).toHaveCount(0)
  await page.getByTestId('undo-toast-undo').click()
  await expect(page.getByTestId('invite-row').filter({ hasText: guest })).toBeVisible()

  // Digits switch the sections; Esc closes the window.
  await page.getByTestId('members-pane').click({ position: { x: 5, y: 5 } })
  await page.keyboard.press('2')
  await expect(page.getByTestId('members-tab-invites')).toHaveAttribute('aria-current', 'page')
  await page.keyboard.press('Escape')
  await expect(win).toHaveCount(0)
})

test('команды: N — новая строка, Enter сохраняет, ⌫ удаляет с отменой', async ({
  page,
  seed,
  backend,
}) => {
  await openBoard(page, seed.boardId)
  await openWindow(page, 'commands')
  await expect(page.getByTestId('commands-window')).toBeVisible()

  await page.keyboard.press('n')
  const key = page.getByTestId('cmd-key')
  await expect(key).toBeFocused()
  await key.fill('e2eok')
  await key.press('Tab')
  await page.getByTestId('cmd-desc').fill('Проверено e2e')
  await page.getByTestId('cmd-desc').press('Enter')
  const row = page.getByTestId('cmd-row').filter({ hasText: '/e2eok' })
  await expect(row).toBeVisible()
  await expect
    .poll(async () =>
      ((await backend.get(`/workspaces/${seed.workspaceId}/commands`)).custom || []).map(
        (c) => c.key,
      ),
    )
    .toContain('e2eok')

  // ⌫ on the row deletes at once; the toast restores it.
  await row.click()
  await page.keyboard.press('Backspace')
  await expect(row).toHaveCount(0)
  await page.getByTestId('undo-toast-undo').click()
  await expect(page.getByTestId('cmd-row').filter({ hasText: '/e2eok' })).toBeVisible()
  await expect
    .poll(async () =>
      ((await backend.get(`/workspaces/${seed.workspaceId}/commands`)).custom || []).map(
        (c) => c.key,
      ),
    )
    .toContain('e2eok')
})

test('фоновые задания: G J, фильтр по типу, деталь', async ({ page, seed, backend }) => {
  const me = await backend.get('/auth/me')
  test.skip(!me.user?.is_admin, 'фоновые задания видны только глобальному админу')
  await openBoard(page, seed.boardId)
  await page.keyboard.press('g')
  await page.keyboard.press('j')
  await expect(page.getByTestId('jobs-window')).toBeVisible()
  await page.getByTestId('jobs-tab-workers').click()
  await expect(page.locator('.bj-row').first()).toBeVisible()
  await page.locator('.bj-row').first().click()
  await expect(page.locator('.bj-detail-name')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('jobs-window')).toHaveCount(0)
})

// GitLab needs a live instance and a global admin to bind a project, neither of
// which the e2e stand has — the window is driven against answers from here,
// and what it sends back is checked instead.
test('GitLab: разделы, правка правила и ⌘S, конфликт поверх и его применение', async ({
  page,
  seed,
}) => {
  const binding = {
    id: '00000000-0000-0000-0000-0000000000b1',
    name: '',
    project_path: 'group/e2e',
    board_id: seed.boardId,
    enabled: true,
    sync_interval_sec: 900,
    full_sync_interval_sec: 86400,
    scope: 'all',
    label_rules: { rules: [{ match: 'S: ', match_type: 'prefix', action: 'status' }] },
    writeback: { enabled: true, bindings: [] },
  }
  const sent = []
  await page.route('**/api/gitlab/connection', (r) =>
    r.fulfill({ json: { connected: true, base_url: 'https://gl.example', gl_username: 'e2e' } }),
  )
  await page.route('**/api/workspaces/*/gitlab/integrations', (r) =>
    r.fulfill({
      json: {
        integrations: [binding],
        default_rules: {},
        service_configured: false,
        is_admin: true,
      },
    }),
  )
  await page.route('**/api/workspaces/*/gitlab/integrations/*', async (r) => {
    if (r.request().method() !== 'PUT') return r.continue()
    const body = r.request().postDataJSON()
    sent.push(body)
    await r.fulfill({ json: { ...binding, ...body } })
  })
  const conflict = {
    id: '00000000-0000-0000-0000-0000000000c1',
    task_id: '00000000-0000-0000-0000-0000000000f1',
    task_number: 1,
    task_title: 'Экспорт',
    change_kind: 'title_desc',
    // Both sides moved: nothing is taken automatically.
    fields: [{ field: 'title', base: 'Экспорт', ours: 'Экспорт в PDF', theirs: 'Экспорт в CSV' }],
  }
  let resolved = null
  await page.route('**/api/workspaces/*/gitlab/conflicts', (r) =>
    r.fulfill({ json: resolved ? [] : [conflict] }),
  )
  await page.route('**/api/tasks/*/gitlab/conflicts/*/resolve', async (r) => {
    resolved = r.request().postDataJSON()
    await r.fulfill({ json: { status: 'ok' } })
  })

  await openBoard(page, seed.boardId)
  await openWindow(page, 'gitlab')
  const win = page.getByTestId('gitlab-window')
  await expect(win).toBeVisible()
  await expect(page.getByTestId('gitlab-binding-card')).toHaveCount(1)
  await expect(page.getByTestId('gitlab-dirty')).toHaveCount(0)

  // 2 — «Правила лейблов»; a new rule makes the form dirty, ⌘S sends it all.
  await page.getByTestId('gitlab-connections').click({ position: { x: 5, y: 5 } })
  await page.keyboard.press('2')
  await expect(page.getByTestId('gitlab-rules')).toBeVisible()
  await page.getByTestId('gitlab-rule-add').click()
  await page.getByTestId('gitlab-rule-match').last().fill('T: ')
  await expect(page.getByTestId('gitlab-dirty')).toBeVisible()
  await page.keyboard.press(`${MOD}+s`)
  await expect.poll(() => sent.length).toBe(1)
  expect(sent[0].label_rules.rules.map((r) => r.match)).toEqual(['S: ', 'T: '])
  expect(sent[0].project_path).toBe('group/e2e')
  await expect(page.getByTestId('gitlab-dirty')).toHaveCount(0)

  // 4 — the journal.
  await page.keyboard.press('Escape') // leaves the field
  await page.getByTestId('gitlab-rules').click({ position: { x: 5, y: 5 } })
  await page.keyboard.press('4')
  await expect(page.getByTestId('gitlab-journal')).toBeVisible()

  // The conflicts door opens the resolver over the window; G picks GitLab's
  // version, ⌘↵ applies.
  await page.getByTestId('gitlab-tab-conflicts').click()
  const cw = page.getByTestId('conflicts-window')
  await expect(cw).toBeVisible()
  await expect(page.getByTestId('conflict-apply')).toBeDisabled()
  await page.keyboard.press('g')
  await expect(page.getByTestId('conflict-apply')).toBeEnabled()
  await page.keyboard.press(`${MOD}+Enter`)
  await expect.poll(() => resolved).toEqual({ resolution: 'theirs' })
  await expect(cw).toHaveCount(0)
  // The GitLab window is still there under it; Esc closes it.
  await expect(win).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(win).toHaveCount(0)
})
