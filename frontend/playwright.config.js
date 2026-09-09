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
      // Stated explicitly so the top-level testDir stays the desktop tier's even
      // though `mobile` below points somewhere else: without it the desktop
      // project would also pick up e2e/mobile/** and run every mobile spec at
      // 1280px, where the whole point of them is gone.
      testDir: './e2e/specs',
      use: {
        ...devices['Desktop Chrome'],
        baseURL,
        // Fake camera/microphone for the conference specs (#2876). Without these
        // the headless browser has no capture devices at all, so `getUserMedia`
        // rejects with NotFoundError and the call never reaches LIVE — a failure
        // that looks exactly like a broken transport.
        //   --use-fake-device-for-media-stream  a synthetic 640×480 rolling
        //     pattern + a 440 Hz tone, so `audioLevel` is non-zero and the
        //     speaker/level assertions have something real to measure;
        //   --use-fake-ui-for-media-stream      auto-grants the permission
        //     prompt, which headless Chrome would otherwise leave hanging;
        //   --autoplay-policy                   lets the remote <audio> element
        //     start without a gesture, so `audioBlocked` stays false;
        //   --allow-loopback-in-peer-connection makes Chrome gather a 127.0.0.1
        //     ICE candidate. It refuses to on a box with other interfaces, and an
        //     SFU on this machine's loopback is then unpairable — it offers
        //     127.0.0.1 and the browser has no socket to answer from, so the room
        //     hangs at "could not establish pc connection" with both sides
        //     apparently healthy.
        // Harmless for every other spec: nothing else asks for a device.
        launchOptions: {
          args: [
            '--use-fake-device-for-media-stream',
            '--use-fake-ui-for-media-stream',
            '--autoplay-policy=no-user-gesture-required',
            '--allow-loopback-in-peer-connection',
          ],
        },
        permissions: ['camera', 'microphone'],
      },
    },
    // Mobile tier (#2893): the same app at 390×844 with touch, in its own
    // directory so `--project=chromium` stays exactly as fast as before. Both
    // projects share the one seeded board, and with `workers: 1` they run one
    // after the other, so there is no race — but the run does take roughly twice
    // as long, which is why `make test-e2e-frontend` still runs the desktop tier
    // alone and `make test-e2e-frontend-mobile` asks for this one by name.
    {
      name: 'mobile',
      testDir: './e2e/mobile',
      use: { ...devices['Pixel 5'], baseURL },
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
