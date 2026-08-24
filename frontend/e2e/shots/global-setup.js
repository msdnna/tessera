import { mkdirSync, writeFileSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { waitForBackend } from '../api.js'
import { seedDemo } from './demo.js'

const here = dirname(fileURLToPath(import.meta.url))
export const SHOTS_SEED_FILE = resolve(here, '../.auth/shots-seed.json')

// One screenshot run = one throwaway user with a complete demo workspace. Same
// contract as the e2e global setup: nothing is shared with a previous run, so
// `tessera_test` is never truncated (and must not be — `make test-backend`
// shares it).
//
// The clock is frozen here, once, and handed to the seeder: every "через 2 дня"
// in the pictures is measured from the same instant, and a run that starts at
// 23:59 cannot produce a board where half the cards moved a day forward.
export default async function globalSetup() {
  await waitForBackend()
  const base = Date.now()
  const runId = `${base.toString(36)}${Math.random().toString(36).slice(2, 6)}`
  const seed = await seedDemo(runId, base)

  mkdirSync(dirname(SHOTS_SEED_FILE), { recursive: true })
  writeFileSync(SHOTS_SEED_FILE, JSON.stringify({ ...seed, base }, null, 2))
}
