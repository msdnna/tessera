import { defineConfig, devices } from '@playwright/test'
import { resolve } from 'path'

// Ports: the backend under test is a throwaway build on :8092 (:8090 often has a
// zombie with older code — see CLAUDE.md), and the preview server is :4174
// because :5174 belongs to the dev window someone may have open.
const PREVIEW_PORT = Number(process.env.TESSERA_PREVIEW_PORT) || 4174
const API_TARGET = process.env.TESSERA_API_TARGET || 'http://localhost:8092'
const baseURL = process.env.E2E_BASE_URL || `http://localhost:${PREVIEW_PORT}`

export default defineConfig({
  testDir: './e2e/specs',
  // e2e drive a shared backend + a shared board; running them concurrently would
  // make the DnD and realtime specs race each other over the same columns.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  // One retry is enough to ride out a genuine flake; two just tripled the cost of
  // a real, reproducible failure (each hung spec waited out the full timeout).
  retries: process.env.CI ? 1 : 0,
  // Bail once a run is clearly broken instead of grinding through all specs (× the
  // per-spec timeout × retries) — that is what turned one regression into an
  // hour-long red job. A handful of failures is plenty of signal.
  maxFailures: process.env.CI ? 10 : undefined,
  timeout: 45000,
  expect: { timeout: 10000 },
  globalSetup: resolve('./e2e/global-setup.js'),
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], baseURL },
    },
  ],
  // The suite owns the preview server; the backend is expected to be up already
  // (`make e2e-backend-up`) because it needs a migrated DB, which Playwright has
  // no business managing. reuseExistingServer keeps local re-runs instant.
  webServer: {
    command: `yarn vite preview --port ${PREVIEW_PORT} --strictPort`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
    env: { TESSERA_API_TARGET: API_TARGET, TESSERA_PREVIEW_PORT: String(PREVIEW_PORT) },
  },
})
