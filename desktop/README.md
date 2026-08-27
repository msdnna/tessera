<p align="right"><b>Русский</b> · <a href="README.en.md">English</a></p>

# Tessera Desktop (Tauri v2)

Нативное десктоп-приложение (Windows + Linux) — обёртка вокруг общего Vue-фронтенда
(`../frontend`). Фронтенд определяет Tauri в рантайме и направляет API/WS на
сервер, заданный при входе (по умолчанию — прод `tessera.msdnna.website`) — см.
`frontend/src/utils/serverBase.js`. **Отдельного JS-пакета нет**: десктоп использует
dev-сервер и сборочный вывод фронтенда напрямую.

Версионируется независимо через `desktop/VERSION` (semver), как и остальные
компоненты. Бамп — `make bump-desktop BUMP=minor`, плюс запись в `CHANGELOG.md`
(см. конвенции в корневом `CLAUDE.md`).

## Тулчейн

- **Rust** (stable) через [rustup](https://rustup.rs). Cargo-bin в `PATH`.
- **Tauri CLI v2**: `cargo install tauri-cli --version '^2' --locked`
  (вызывается как `cargo tauri …`).

### Linux (собирается на dev-машине)

Системные `-dev`-пакеты (Debian/Ubuntu):

```
libwebkit2gtk-4.1-dev  libgtk-3-dev  libayatana-appindicator3-dev
librsvg2-dev  patchelf  build-essential  curl  wget  file
```

Сборка: `make desktop` (или `cargo tauri build`) → AppImage + `.deb` в
`src-tauri/target/release/bundle/`. `.deb` объявляет рантайм-зависимости
appindicator/webkit; пользователям AppImage нужен установленный
`libayatana-appindicator3` для системного трея (Фаза 2).

**AppImage под WSL:** шаг AppImage запускает `linuxdeploy` — сам по себе AppImage,
которому нужен FUSE, а он падает на многих WSL-конфигурациях (`failed to run
linuxdeploy`), даже с `APPIMAGE_EXTRACT_AND_RUN=1`. `.deb` там собирается нормально.
Собирайте AppImage (цель Linux-самообновления) на нативном Linux-хосте или CI-раннере.
`build-desktop-release.sh` терпит провал шага AppImage и всё равно публикует `.deb`.

### Windows (собирается нативно на Windows — не из WSL)

Кросс-компиляция из Linux непрактична, И нативный Windows-тулчейн (cargo/MSVC/NSIS)
**не умеет собирать через путь `\\wsl$` / UNC** — поэтому репозиторий должен лежать на
нативном Windows-диске. sh-скрипты и Makefile на Windows тоже не работают; используйте
`desktop\build-windows.ps1`.

Разовая настройка:
1. **Rust (MSVC)** через rustup (таргет `x86_64-pc-windows-msvc`).
2. **Visual Studio C++ Build Tools** (линкер MSVC).
3. **WebView2 Runtime** — предустановлен в актуальных Windows 10/11; иначе поставьте
   Evergreen-runtime.
4. `cargo install tauri-cli --version '^2' --locked`.
5. **Нативный checkout** (не путь `\\wsl.localhost\...`):
   ```
   git clone \\wsl.localhost\<distro>\home\msdnna\GolandProjects\tessera C:\src\tessera
   ```
   Позже синхронизировать `git -C C:\src\tessera pull`.

Каждая сборка (фронтенд собирается в WSL, на Windows — только Rust+NSIS):
1. В WSL: `corepack yarn --cwd frontend build`.
2. Зеркалировать собранный фронтенд в Windows-checkout (frontend\dist в gitignore,
   поэтому `git pull` его не привезёт):
   ```
   robocopy \\wsl.localhost\<distro>\home\msdnna\GolandProjects\tessera\frontend\dist C:\src\tessera\frontend\dist /MIR
   ```
3. В PowerShell:
   ```
   # подпись (самообновление) — сперва скопируйте ключ из WSL ~/.tessera/:
   $env:TAURI_SIGNING_PRIVATE_KEY = 'C:\path\to\tessera-desktop-updater.key'
   $env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD = ''
   pwsh C:\src\tessera\desktop\build-windows.ps1
   ```
   → NSIS `-setup.exe` (+ `.sig`) в `desktop\src-tauri\target\release\bundle\nsis\`.
   Передайте `-BuildFrontend`, чтобы собрать фронтенд на Windows (нужен Node 22 +
   предварительный `corepack yarn install` в `frontend\`).
4. Скопируйте `-setup.exe` + его `.sig` в `desktop-dist\` WSL-репозитория, затем
   выполните `make desktop-release` в WSL, чтобы вложить Windows-запись в `latest.json`.

## Разработка

```
make dev-desktop        # cargo tauri dev — поднимает Vite :5174 + приложение,
                        # ходит к локальному бэкенду через прокси
```

## Тестирование

Rust-часть (`src-tauri/src/lib.rs`, ~90 строк) — **чистый Tauri-клей**: связывает
плагины ОС-интеграции (single-instance, трей, автозапуск, уведомления, updater,
dialog/fs/clipboard) и поведение close-to-tray. Бизнес-логики в ней нет: обе функции
(`show_main`, `run`) требуют живой Tauri-рантайм и `AppHandle`, поэтому осмысленно
проверяются только end-to-end, а не юнит-тестами. Юнит-тесты здесь тестировали бы
фреймворк Tauri, а не наш код.

Вся прикладная логика живёт в общем Vue-фронтенде и покрыта его тест-сьютом
(`frontend/tests/`). Rust-оболочка держится под контролем качества через
`make lint-desktop` (`cargo fmt --check` + `cargo clippy -D warnings`), запускается в
CI. Интерактивное поведение (трей, close-to-tray, самообновление, deep-links)
проверяется вручную / через e2e.

## Релиз + самообновление

`tools/build-desktop-release.sh` собирает Linux-бандлы, подписывает артефакты updater
и пишет манифест updater в `desktop-dist/` для раздачи на
`https://tessera.msdnna.website/desktop/`. Windows-артефакты собираются на Windows и
кладутся в тот же каталог.

- **Приватный** minisign-ключ лежит вне репозитория —
  `~/.tessera/tessera-desktop-updater.key` (без пароля); **публичный** ключ вшит в
  `src-tauri/tauri.conf.json` (`plugins.updater.pubkey`). Подпись читает
  `TAURI_SIGNING_PRIVATE_KEY` / `TAURI_SIGNING_PRIVATE_KEY_PATH`. Берегите приватный
  ключ — его потеря ломает обновления (та же дисциплина, что с Android-keystore).
