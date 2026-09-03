import { test, expect, signIn } from '../fixtures.js'
import { newCredentials, register } from '../api.js'

// Conferences, end to end (#2864, subtask #2876).
//
// The unit tests drive `useConfRoom`/`useConfTransport` against a stubbed SDK and
// the Go tests drive `internal/confroom` and the token endpoint; between them
// they cover every branch and prove nothing about the two halves meeting in a
// browser. What only a real run shows: scheduling writes a row that the list
// picks up, «Подключиться» seats you on the server *and* opens the room socket,
// the roster and the chat on the far side of that socket are the ones the other
// participant sees, and leaving actually gives the seat back.
//
// Two layers, deliberately split by what the environment can offer:
//
//   • the room layer (this file's first two tests) needs only our own backend and
//     always runs;
//   • the media layer needs a live LiveKit *and* a backend configured to mint
//     tokens for it. That is a service the suite has no business starting, so the
//     media test asks the API for a token first and skips with a printed reason
//     when the answer is the documented 503. The header of deploy/livekit.e2e.yaml
//     has the two commands that bring it up.
//
// Media itself comes from Chrome's fake devices, wired in playwright.config.js:
// a headless browser has no camera at all, and on this dev box a real one would
// not help either — `getUserMedia` needs a secure context, which `:8083` and any
// LAN IP are not. `http://localhost:4174` is, which is why the preview port is
// the only place these specs can run.

// openConferences puts the browser on the section with the run's workspace
// already selected. Without the seeded `tessera_ws` the view has no workspace to
// list against and the «Запланировать» button stays disabled — a failure that
// reads as "the button does nothing".
async function openConferences(page, workspaceId) {
  await page.addInitScript((id) => localStorage.setItem('tessera_ws', id), workspaceId)
  await page.goto('/conferences')
  await expect(page.getByTestId('conference-schedule')).toBeEnabled()
}

// schedule creates a conference through the dialog and returns its id. Creating
// one opens it straight away — the dialog is a "start a meeting" button as much
// as a scheduler — so there is no list row to click here.
async function schedule(page, title) {
  await page.getByTestId('conference-schedule').click()
  // The testid sits on the naive wrapper, not on the control it renders.
  await page.getByTestId('conference-title').locator('input').fill(title)
  await page.getByTestId('conference-submit').click()

  await page.waitForURL(/\/conferences\/[0-9a-f-]{36}$/)
  await expect(page.getByTestId('conference-room')).toBeVisible()
  return new URL(page.url()).pathname.split('/').pop()
}

test('конференция: планирование, вход, ростер и чат', async ({ page, backend, seed }) => {
  const title = `E2E конференция ${seed.runId}`
  await openConferences(page, seed.workspaceId)
  const id = await schedule(page, title)

  // The list is a separate read path from the one that just opened us here, and
  // it is the one everybody else arrives through — so check the row landed.
  await page.goto('/conferences')
  const row = page.getByTestId('conference-row').filter({ hasText: title })
  await expect(row).toHaveCount(1)
  await row.click()
  await expect(page.getByTestId('conference-room')).toBeVisible()

  // Before joining the room is a stated non-participation, not a broken call.
  await expect(page.getByTestId('conference-participants')).toHaveCount(0)

  // Joining is two things at once and both have to land: a seat on the server
  // (the roster the *others* read) and the socket that streams it back to us.
  await page.getByTestId('conference-join').click()
  await expect(page.getByTestId('conference-leave')).toBeVisible()

  const roster = page.getByTestId('conference-participant')
  await expect(roster).toHaveCount(1)
  await expect(roster.first()).toContainText(seed.creds.name)

  // The chat rides the same conference, not the workspace hub — a message posted
  // here has to come back through the room's own history.
  await page.getByTestId('conference-rail-chat').click()
  const text = `привет из e2e ${seed.runId}`
  await page.getByTestId('conference-chat-input').locator('textarea').fill(text)
  await page.getByTestId('conference-chat-send').click()
  await expect(page.getByTestId('conference-chat-message').filter({ hasText: text })).toHaveCount(1)

  // Reopening proves the message was persisted rather than only echoed locally.
  // A reload drops the seat on purpose — a call is not something to rejoin behind
  // the user's back, with their mic reopened, just because the tab reloaded — so
  // the room comes back at «Подключиться» and the rail only exists once we are in
  // it again. Rejoining here is part of the assertion, not a workaround.
  await page.reload()
  await page.getByTestId('conference-join').click()
  await expect(page.getByTestId('conference-leave')).toBeVisible()
  await page.getByTestId('conference-rail-chat').click()
  await expect(page.getByTestId('conference-chat-message').filter({ hasText: text })).toHaveCount(1)

  // Leaving gives the seat back on the server, not just in this tab.
  await page.getByTestId('conference-leave').click()
  await expect(page.getByTestId('conference-join')).toBeVisible()

  const participants = await backend.get(`/conferences/${id}/participants`)
  const seated = (participants || []).filter((p) => p.joined_at && !p.left_at)
  expect(seated).toHaveLength(0)
})

test('конференция: приглашённый виден в комнате обеим сторонам', async ({
  page,
  browser,
  backend,
  seed,
}) => {
  const baseURL = new URL(page.url() || 'http://localhost:4174').origin
  const creds = newCredentials(seed.runId, '-conf-mate')
  await register(creds)
  await backend.post(`/workspaces/${seed.workspaceId}/members`, { email: creds.email })

  const title = `E2E приглашение ${seed.runId}`
  await openConferences(page, seed.workspaceId)
  const id = await schedule(page, title)

  await page.getByTestId('conference-join').click()
  await expect(page.getByTestId('conference-leave')).toBeVisible()

  // Invite through the UI: the dialog is where the "who is left to invite" list
  // is computed, and it is that filtering (#2875) the roster then depends on.
  await page.getByTestId('conference-invite').click()
  const select = page.getByTestId('conference-invite-select')
  await select.click()
  // Naive's select options are plain divs in a portal — no `option` role to grab
  // them by, which is why the rest of the suite goes through the class too.
  await page.locator('.n-base-select-option', { hasText: creds.name }).first().click()
  // The menu of a multiple select stays open after a pick and covers the footer,
  // so «Пригласить» has to be uncovered before it can be clicked.
  await page.keyboard.press('Escape')
  await page.getByTestId('conference-invite-submit').click()

  // An invitee who has not arrived belongs under «Приглашены», apart from the
  // live roster — that separation is the whole point of the block.
  await expect(page.getByTestId('conference-invited')).toContainText(creds.name)
  await expect(page.getByTestId('conference-participant')).toHaveCount(1)

  const context = await browser.newContext({ baseURL })
  try {
    const matePage = await context.newPage()
    await signIn(matePage, creds)
    await openConferences(matePage, seed.workspaceId)
    await matePage.goto(`/conferences/${id}`)
    await matePage.getByTestId('conference-join').click()

    // Both sides now see two people, and the invitee has moved out of the
    // «Приглашены» list into the roster proper.
    await expect(matePage.getByTestId('conference-participant')).toHaveCount(2)
    await expect(page.getByTestId('conference-participant')).toHaveCount(2)
    await expect(page.getByTestId('conference-invited')).toHaveCount(0)

    await matePage.getByTestId('conference-leave').click()
    await expect(page.getByTestId('conference-participant')).toHaveCount(1)
  } finally {
    await context.close()
  }
})

test('конференция: медиа через фейковые устройства', async ({ page, backend, seed }) => {
  const title = `E2E медиа ${seed.runId}`
  await openConferences(page, seed.workspaceId)
  const id = await schedule(page, title)

  // The token endpoint is the honest probe for "can this box do media": it
  // answers 503 exactly when LIVEKIT_PUBLIC_URL is unset, which is the default
  // for `make e2e-backend-up`. Seat ourselves first — the endpoint refuses a
  // non-participant, and that 403 must not be read as "no LiveKit".
  await backend.post(`/conferences/${id}/join`)
  let configured = true
  try {
    await backend.post(`/conferences/${id}/token`, {})
  } catch (e) {
    configured = false
    test.skip(true, `LiveKit не настроен на бэкенде e2e — медиа-ярус пропущен: ${e.message}`)
  }
  await backend.post(`/conferences/${id}/leave`)
  if (!configured) return

  await page.getByTestId('conference-join').click()

  // LIVE, reached with a real SFU connection: our own tile on the stage is what
  // the transport only publishes once `room.connect` succeeded and the fake
  // camera/mic were accepted.
  await expect(page.getByTestId('conference-media-error')).toHaveCount(0)
  await expect(
    page.getByTestId('conference-tile').or(page.getByTestId('conference-screen-tile')),
  ).not.toHaveCount(0)

  // The mic starts on (audio is why people are here) and the toolbar button is
  // the one control that must survive a round trip through the SFU's publish
  // permissions rather than just flipping a local flag.
  const mic = page.getByTestId('conference-mic')
  await expect(mic).toBeEnabled()
  await mic.click()
  await expect(page.getByTestId('conference-mic-level')).toHaveCount(0)
  await mic.click()
  await expect(page.getByTestId('conference-mic-level')).toHaveCount(1)

  // The device menu is populated from `enumerateDevices`, which only returns
  // labelled entries once a capture permission has actually been granted — so a
  // non-empty menu is itself proof the fake devices were opened.
  await expect(page.getByTestId('conference-devices')).toBeEnabled()

  await page.getByTestId('conference-leave').click()
  await expect(page.getByTestId('conference-join')).toBeVisible()
})
