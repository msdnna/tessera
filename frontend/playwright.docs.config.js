import { defineConfig, devices } from '@playwright/test'
import { resolve } from 'path'

// Config for the help-centre screenshot pipeline (#2793) — `make help-shots`.
//
// A separate file rather than an extra project in playwright.config.js: the two
// runs want different things. The e2e suite asserts behaviour and must stay out
// of docs/, the shots run writes into the repo and needs its own seed (a demo
// workspace with plausible Russian labels — a run id in the sidebar would end up
// in the manual). Keeping them apart also means `corepack yarn e2e` cannot
// accidentally repaint every screenshot in a normal test run.
const PREVIEW_PORT = Number(process.env.TESSERA_PREVIEW_PORT) || 4175
const API_TARGET = process.env.TESSERA_API_TARGET || 'http://localhost:8092'
const baseURL = process.env.E2E_BASE_URL || `http://localhost:${PREVIEW_PORT}`

export default defineConfig({
  testDir: './e2e/shots',
  // The demo workspace is shared by every shot and the board shots toggle its
  // grouping, so they run one at a time — same reason the e2e suite does.
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 60000,
  expect: { timeout: 10000 },
  globalSetup: resolve('./e2e/shots/global-setup.js'),
  reporter: [['list']],
  use: {
    baseURL,
    // Motion off at the browser level too, not just per-screenshot: the app's own
    // entrance transitions would otherwise be half-played when the shutter fires.
    reducedMotion: 'reduce',
    trace: 'off',
    video: 'off',
  },
  projects: [
    {
      name: 'docs-shots',
      // The viewport belongs here, after the device spread — Desktop Chrome
      // carries its own 1280×720 and a top-level `use` would lose to it.
      // A fixed size at scale 1: the pictures are read inline in an article
      // column ~74ch wide, so a retina-sized PNG would only cost repo weight.
      // Changing this re-renders every asset — a deliberate, reviewable diff.
      use: {
        ...devices['Desktop Chrome'],
        baseURL,
        viewport: { width: 1280, height: 800 },
        deviceScaleFactor: 1,
      },
    },
  ],
  // The preview server is ours (:4175, one port above the e2e suite's, so both
  // can be up at once); the backend is expected to be running already —
  // `make e2e-backend-up`.
  webServer: {
    command: `yarn vite preview --port ${PREVIEW_PORT} --strictPort`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
    env: { TESSERA_API_TARGET: API_TARGET, TESSERA_PREVIEW_PORT: String(PREVIEW_PORT) },
  },
})
