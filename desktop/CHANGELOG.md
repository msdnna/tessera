# Changelog — Tessera Desktop

All notable changes to the Tessera desktop app (Tauri v2) are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/). The desktop app
is versioned independently (`desktop/VERSION`), like the other components.

## [0.1.0] — 2026-07-06

Phase 1 MVP — desktop shell around the shared Vue frontend.

- **Tauri v2 project** (`desktop/src-tauri`) hosting `frontend/dist`. Dev mode runs
  Vite (:5174) via `beforeDevCommand` and talks to the local backend through the
  proxy; production bundles point at the login-configured server (default prod).
- **Window:** 1280×800 (min 900×600), `dragDropEnabled: false` so the frontend's
  HTML5 image drag-drop / paste in the markdown editor keeps working.
- **Native notifications** via `tauri-plugin-notification` (routed from the shared
  device-notification path; web keeps the Web Notification API).
- **Self-update** via `tauri-plugin-updater` (+ `process`): manifest at
  `https://tessera.msdnna.website/desktop/latest.json`, minisign-signed.
- **File dialogs + fs** (`tauri-plugin-dialog`/`-fs`): native "Save as…" for
  downloaded attachments.
- **Clipboard** (`tauri-plugin-clipboard-manager`): image-paste fallback for Linux
  WebKitGTK, where in-webview clipboard image paste is unreliable.
- Bundles: Linux AppImage/.deb (built on the dev box), Windows NSIS `.exe` (built
  natively on Windows — see README).
