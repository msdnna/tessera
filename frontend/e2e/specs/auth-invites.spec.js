import { test, expect, signIn, openBoard } from '../fixtures.js'
import { api, register } from '../api.js'
import { mailLogAvailable, waitCode, waitToken } from '../mailLog.js'

// Вход и приглашения v2 (#3035): код подтверждения и смена почты, превью и
// отказ от приглашения, ссылка-приглашение, срок ссылки сброса, повтор
// приглашения. Коды и ссылки читаются из журнала e2e-бэкенда (no-op mailer
// пишет письма целиком) — без него спеки про почту пропускаются.
test.use({ signedIn: false })
// Экраны без сессии говорят на языке браузера, поэтому там — testid и числа,
// а не фразы.

function freshCreds(tag) {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  return {
    email: `e2e+${tag}${id}@test.local`,
    name: `E2E ${tag} ${id}`,
    password: 'e2e-password-123',
  }
}

async function typeCode(page, code) {
  await page.getByTestId('code-input').fill(code)
}

test('код подтверждения: ячейки, неверный код, верный — почта подтверждена', async ({ page }) => {
  test.skip(!mailLogAvailable(), 'нужен журнал e2e-бэкенда (E2E_BACKEND_LOG)')
  const creds = freshCreds('code')
  await register(creds)
  const code = await waitCode(creds.email)
  await signIn(page, creds)
  await page.goto('/verify-email')

  await expect(page.getByTestId('code-cell')).toHaveCount(6)
  await expect(page.getByText(`Код отправлен на ${creds.email}`)).toBeVisible()
  await expect(page.getByTestId('code-resend-wait')).toContainText(/0:\d\d/)

  const wrong = code === '000000' ? '111111' : '000000'
  await typeCode(page, wrong)
  await expect(page.getByTestId('code-error')).toContainText('осталось попыток: 4')

  await typeCode(page, code)
  await expect(page.getByTestId('verify-done')).toContainText(creds.email)
  await page.getByTestId('verify-continue').click()
  await expect(page).toHaveURL(/\/$/)
  const me = await api.post('/auth/login', { email: creds.email, password: creds.password })
  expect(me.user.email_verified).toBe(true)
})

test('«Изменить email»: код на новый адрес переносит аккаунт', async ({ page }) => {
  test.skip(!mailLogAvailable(), 'нужен журнал e2e-бэкенда (E2E_BACKEND_LOG)')
  const creds = freshCreds('chg')
  await register(creds)
  await signIn(page, creds)
  await page.goto('/verify-email')
  await page.getByTestId('code-change-email').click()
  // Неподтверждённый аккаунт (опечатка при регистрации) пароль не спрашивает.
  await expect(page.getByTestId('change-password')).toHaveCount(0)

  const moved = `e2e+moved${Date.now().toString(36)}@test.local`
  await page.getByTestId('change-email').locator('input').fill(moved)
  // Пауза между письмами — на адрес: исправленная опечатка уходит сразу.
  await page.getByTestId('change-submit').click()
  await expect(page.getByText(`Код отправлен на ${moved}`)).toBeVisible()
  await expect(page.getByTestId('code-cancel-change')).toBeVisible()

  await typeCode(page, await waitCode(moved))
  await expect(page.getByTestId('verify-done')).toContainText(moved)
  const res = await api.post('/auth/login', { email: moved, password: creds.password })
  expect(res.user.email_verified).toBe(true)
})

test('приглашение: превью без сессии и «Отклонить»', async ({ page, seed }) => {
  const email = `e2e+inv${Date.now().toString(36)}@test.local`
  const inv = await api.post(
    `/workspaces/${seed.workspaceId}/invitations`,
    { email, role: 'member' },
    seed.token,
  )
  const token = new URL(inv.link).searchParams.get('token')
  await page.goto(`/invite?token=${token}`)

  const card = page.getByTestId('invite-preview')
  await expect(card).toBeVisible()
  await expect(page.getByTestId('invite-space')).toHaveText(`E2E ${seed.runId}`)
  await expect(page.getByTestId('invite-role')).toHaveText(/участник|member/)
  await expect(page.getByTestId('invite-email')).toContainText(email)
  await expect(card).toContainText(/1 (проект|project)/)
  await expect(card).toContainText(seed.creds.name)

  await page.getByTestId('invite-decline').click()
  await expect(page.getByTestId('invite-state')).toBeVisible()
  await expect(page.getByTestId('invite-preview')).toHaveCount(0)
  const after = await api.post('/invitations/lookup', { token })
  expect(after.state).toBe('declined')
})

test('приглашение: свой аккаунт принимает из превью', async ({ page, seed }) => {
  const creds = freshCreds('acc')
  await register(creds)
  const inv = await api.post(
    `/workspaces/${seed.workspaceId}/invitations`,
    { email: creds.email, role: 'member' },
    seed.token,
  )
  const token = new URL(inv.link).searchParams.get('token')
  await signIn(page, creds)
  await page.goto(`/invite?token=${token}`)
  await expect(page.getByTestId('invite-email')).toContainText(creds.email)
  await page.getByTestId('invite-accept').click()
  await expect(page.getByTestId('invite-state')).toContainText('присоединились')
  const members = await api.get(`/workspaces/${seed.workspaceId}/members`, seed.token)
  expect(members.some((m) => m.email === creds.email)).toBe(true)
})

test('ссылка-приглашение: создать в «Участниках», открыть, войти по ней', async ({
  page,
  seed,
  browser,
}) => {
  await signIn(page)
  await openBoard(page, seed.boardId)
  await page.evaluate(() => {
    const pinia = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia
    pinia._s.get('wsDialogs').open('members')
  })
  await expect(page.getByTestId('members-window')).toBeVisible()
  // A link left by an earlier run of this spec on the shared workspace is
  // hidden (shown once): reset gives a fresh one either way.
  const create = page.getByTestId('members-shared-create')
  if (await create.isVisible()) await create.click()
  else await page.getByTestId('members-shared-reset').click()
  const url = page.getByTestId('members-shared-url')
  await expect(url).toContainText('/join/')
  await expect(page.getByTestId('members-shared-meta')).toContainText('Участник')
  const path = (await url.textContent()).trim().replace(/^[^/]+/, '')

  // A newcomer opens the link in a clean browser.
  const ctx = await browser.newContext()
  const guest = await ctx.newPage()
  await guest.goto(path)
  await expect(guest).toHaveURL(/\/invite\?token=/)
  await expect(guest.getByTestId('invite-preview')).toBeVisible()
  await expect(guest.getByTestId('invite-email')).toHaveCount(0)
  await guest.getByTestId('invite-accept').click()
  await expect(guest).toHaveURL(/\/register\?invite=/)
  const creds = freshCreds('link')
  await guest.getByTestId('register-name').locator('input').fill(creds.name)
  await guest.getByTestId('register-email').locator('input').fill(creds.email)
  await guest.getByTestId('register-password').locator('input').fill(creds.password)
  await guest.getByTestId('register-submit').click()
  await expect(guest.getByTestId('sidebar')).toBeVisible()
  await ctx.close()

  const members = await api.get(`/workspaces/${seed.workspaceId}/members`, seed.token)
  expect(members.some((m) => m.email === creds.email)).toBe(true)

  // «Сбросить»: the link just used stops working.
  const oldToken = path.split('/').pop()
  await page.getByTestId('members-shared-reset').click()
  await expect(url).not.toContainText(oldToken)
  const dead = await api.post('/invitations/lookup', { token: oldToken })
  expect(dead.state).toBe('revoked')
})

test('«Ещё раз» сразу после отправки — спокойная подсказка о паузе', async ({ page, seed }) => {
  const email = `e2e+again${Date.now().toString(36)}@test.local`
  await api.post(`/workspaces/${seed.workspaceId}/invitations`, { email }, seed.token)
  await signIn(page)
  await openBoard(page, seed.boardId)
  await page.evaluate(() => {
    const pinia = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia
    pinia._s.get('wsDialogs').open('members')
  })
  const row = page.getByTestId('invite-row').filter({ hasText: email })
  await expect(row).toContainText('отправлено')
  await row.getByTestId('invite-resend').click()
  await expect(page.getByTestId('toast').getByText('Письмо только что ушло')).toBeVisible()
})

test('ссылка сброса: «действует ещё N минут», а мёртвая — сразу', async ({ page }) => {
  test.skip(!mailLogAvailable(), 'нужен журнал e2e-бэкенда (E2E_BACKEND_LOG)')
  const creds = freshCreds('reset')
  await register(creds)
  await api.post('/auth/forgot-password', { email: creds.email })
  const token = await waitToken(creds.email)
  await page.goto(`/recover?token=${token}`)
  await expect(page.locator('.auth-sub')).toHaveText(/\b(59|60)\b/)
  await expect(page.getByTestId('reset-password')).toBeVisible()

  await page.goto('/recover?token=0000')
  await expect(page.getByTestId('reset-expired')).toBeVisible()
  await expect(page.getByTestId('reset-password')).toHaveCount(0)
})
