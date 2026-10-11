import { test, expect, openBoard } from '../fixtures.js'
import { api, newCredentials, register } from '../api.js'

// Presence on a board (#3116): a teammate who opens the board shows up first in
// its people cascade with a green dot, and in «Сейчас смотрят» on hover; closing
// their socket takes them off at once. The teammate is a raw socket from Node —
// the same frame the app sends.

const WS_URL =
  (process.env.E2E_API_URL || 'http://localhost:8092/api').replace(/^http/, 'ws') + '/ws'

function standOn(token, boardId) {
  return new Promise((resolve, reject) => {
    const sock = new WebSocket(WS_URL, ['bearer', token])
    sock.onopen = () => {
      sock.send(JSON.stringify({ type: 'presence', board_id: boardId }))
      resolve(sock)
    }
    sock.onerror = reject
  })
}

test('присутствие: коллега на доске — первым в каскаде с точкой', async ({
  page,
  seed,
  backend,
}) => {
  const fresh = await backend.freshBoard('presence')
  const creds = newCredentials(seed.runId, '-presence-mate')
  const mate = await register(creds)
  await api.post(
    `/workspaces/${fresh.ws.id}/members`,
    { email: creds.email, role: 'member' },
    seed.token,
  )

  await openBoard(page, fresh.board.id, fresh.ws.id)
  const sock = await standOn(mate.token, fresh.board.id)

  const here = page.locator('[data-testid="board-members"] [data-here]')
  await expect(here).toHaveCount(1)
  await page.getByTestId('board-members').hover()
  await expect(page.getByTestId('board-members-presence')).toContainText(creds.name)

  sock.close()
  await expect(here).toHaveCount(0)
})
