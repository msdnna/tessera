# Tessera Android — Architecture

Native Jetpack Compose client for the Tessera backend. The visual language
**replicates the web frontend** (custom design system, not Material 3 styling),
and the app is **online-first** — it talks to the API directly (like the web
client), with no offline database or sync engine.

## Stack
- Kotlin + Jetpack Compose (Compose BOM 2026.04.01), AGP 9.2, Kotlin 2.3, Gradle 9.4.
- minSdk 24, targetSdk 36, compileSdk 37, Java 17 toolchain.
- Retrofit 3 + OkHttp 5 + Gson (REST), OkHttp WebSocket (realtime, later phase).
- Coroutines/Flow, AndroidX Lifecycle ViewModel, Navigation Compose, DataStore.
- Coil 2 (+SVG) for media. ktlint + detekt + JUnit4/Robolectric/MockWebServer/MockK.

## Layers
```
ui/            Compose: theme/, components/, screens/, viewmodels/, navigation/
data/
  model/       Gson DTOs mirroring backend JSON
  api/         ApiService (Retrofit) + RetrofitClient (OkHttp, auth/refresh)
  repository/  feature repositories — the only callers of ApiService
  preferences/ AppPreferences (DataStore: session + theme + server override)
  realtime/    WebSocket client (later phase)
  AppContainer manual DI: holds prefs + resolves the active ApiService
util/          error mapping, helpers
```

Flow: `Screen` → `ViewModel` (StateFlow UI state) → `Repository` → `ApiService`.
ViewModels never touch Retrofit directly.

## Design system (`ui/theme`)
- `TesseraColors` — neutral palette tokens ported 1:1 from
  `frontend/src/styles/tokens.js` (light + dark).
- `AccentTheme` — the 7 accent schemes from `stores/theme.js` (purple default).
- `LocalTessera` CompositionLocal + `Tessera.colors` accessor. Components read
  this, **never** `MaterialTheme.colorScheme`. Material 3 is wrapped only as a
  thin host (ripple, text selection, base typography).
- Custom primitives in `ui/components` (`TButton`, `TTextField`, `TCard`, …)
  reproduce the web look (8dp radius, bordered inputs, flat no-ripple presses).

## Auth & session
- `/auth/login|register` → `{access_token, refresh_token, user}` persisted in
  DataStore and pushed into `RetrofitClient`.
- `RetrofitClient` injects the Bearer token and performs silent refresh-on-401
  (single coalesced in-flight refresh), mirroring `frontend/src/api/index.js`.
  On unrecoverable 401 it signals a logout; on refresh it persists the new pair.
- `AppRoot` is the session gate: splash → auth → main.

## Server URL
- `BuildConfig.DEFAULT_BASE_URL` = `https://tessera.website.msdnna` (production).
- Overridable at runtime (login screen → "Настройки сервера") so the same APK
  can point at a dev/test backend (`http://10.0.2.2:8090` from the emulator).

## Layout
Mirrors the web's **mobile** layout: a hamburger opens the sidebar in a 280dp
drawer; topbar on top; board content below. (A bottom-nav variant may replace
the drawer later once mobile UX settles — see project notes.)

## Build & release
- `android/VERSION` is the single source of truth; `versionCode` derives from it.
- `make android` (debug) / `make android-release` (signed) — see repo Makefile.
- APKs are copied into `/apks` for the future in-app self-update feature.
