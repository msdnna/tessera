import { mkdirSync, writeFileSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { waitForBackend, newCredentials, register, seedBoard } from './api.js'

const here = dirname(fileURLToPath(import.meta.url))
export const SEED_FILE = resolve(here, '.auth/seed.json')

// One run = one user = one workspace. Nothing is shared with a previous run, so
// `tessera_test` never needs truncating (and must never be truncated — the DB is
// shared with `make test-backend`).
//
// Deliberately NOT captured here: a `storageState` snapshot. Tessera rotates the
// refresh token on every use, so a stored session cookie is single-use — the
// first test would spend it and every later test would load a revoked cookie and
// land on /login. Specs sign in per-test instead (see fixtures.js).
export default async function globalSetup() {
  await waitForBackend()

  // `runId` must be filesystem- and email-safe; the random tail keeps runs apart
  // even when two land in the same millisecond.
  const runId = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
  const creds = newCredentials(runId)
  // register() also opts the user out of the Get Started guide, which would
  // otherwise autostart over every spec — see the note there.
  const { token, user } = await register(creds)
  const seed = await seedBoard(token, runId)

  mkdirSync(dirname(SEED_FILE), { recursive: true })
  writeFileSync(
    SEED_FILE,
    JSON.stringify(
      {
        runId,
        creds,
        token,
        userId: user.id,
        workspaceId: seed.ws.id,
        projectId: seed.project.id,
        boardId: seed.board.id,
        columns: seed.columns.map((c) => ({ id: c.id, name: c.name })),
      },
      null,
      2,
    ),
  )
}
