# Changelog — Tessera Desktop

All notable changes to the Tessera desktop app (Tauri v2) are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/). The desktop app
is versioned independently (`desktop/VERSION`), like the other components.

## [0.2.1] — 2026-07-10

- **`downloads`-каталог в публикуемом `latest.json`** (`tools/build-desktop-release.sh`):
  рядом с подписанным `platforms`-блоком updater'а пишется несигнатурный `downloads`
  с артефактами (Linux: AppImage/`.deb`/будущий `.rpm`, Windows: `.exe`) — из него
  web-логин строит ссылки на скачивание. Формат апдейтера не тронут (лишний ключ он
  игнорирует). AppImage идёт первым как рекомендуемый (self-updating) Linux-вариант.
- **Ровно один файл на формат — текущей версии.** Раньше скрипт глобал `desktop-dist/`
  и вписывал КАЖДЫЙ installer (включая старые версии) → дубли в меню. Теперь: deb/AppImage
  берутся из свежего билд-вывода, Windows `.exe`/`.rpm` матчатся по `*_<VERSION>_*`
  (устаревшие файлы в `desktop-dist/` игнорируются). Это же исправляет и `platforms`-блок
  апдейтера (мог указывать на старый `.exe`).
- **Относительные ссылки в `downloads`.** Записи хранят имя файла (`{format, file}`),
  а не абсолютный URL — сайт резолвит их относительно своего origin
  (`<serverBase>/desktop/<file>`), без захардкоженного `tessera.msdnna.website`
  (как APK). Подписанный `platforms`-блок остаётся с абсолютным URL (требование Tauri).
- **AppImage-сборка в WSL чинится в самом скрипте.** WSL подмешивает Windows-пути в
  `PATH` (`…/WindowsApps`), и `linuxdeploy` (boost::filesystem) падает на них с
  «Permission denied» → обобщённое `failed to run linuxdeploy`. Скрипт теперь под WSL
  вычищает `/mnt/*` из `PATH`. Прочие пререквизиты AppImage в WSL (ставятся один раз):
  `sudo apt install -y libfuse2t64 patchelf`.

## [0.2.0] — 2026-07-07

Phase 2 — OS integration.

- **System tray**: menu «Открыть Tessera» / «Выход», left-click shows the window.
- **Close-to-tray**: the window's close button hides to the tray instead of
  quitting (keeps the WS alive so reminders keep arriving); real quit is via the
  tray «Выход».
- **Single-instance** (`tauri-plugin-single-instance`): a second launch focuses
  the running window instead of opening a duplicate.
- **Autostart** (`tauri-plugin-autostart`): optional launch-at-login (toggle in
  Settings → «Приложение»); autostarted with `--minimized` → straight to the tray.
- Window `show`/`unminimize`/`set-focus` capabilities for the tray + notification
  deep-link (see web 0.116.0).

## [0.1.1] — 2026-07-06

- **Devtools в релизе** (`tauri` feature `devtools`): webview-инспектор доступен по правому клику →
  Inspect в релизных сборках — удобно, пока десктоп стабилизируется. Убрать для «залоченного» релиза.
- Пара к фиксу web 0.115.1 (аватары/картинки грузятся через blob на десктопе).

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
