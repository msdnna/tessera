<p align="right"><b>Русский</b> · <a href="ARCHITECTURE.en.md">English</a></p>

# Tessera Android — Архитектура

Нативный Jetpack Compose-клиент бэкенда Tessera. Визуальный язык **повторяет
веб-фронтенд** (собственная дизайн-система, не стилизация Material 3), приложение —
**online-first**: ходит в API напрямую (как веб-клиент), без офлайн-БД и движка синка.

## Стек
- Kotlin + Jetpack Compose (Compose BOM 2026.04.01), AGP 9.2, Kotlin 2.3, Gradle 9.4.
- minSdk 24, targetSdk 36, compileSdk 37, тулчейн Java 17.
- Retrofit 3 + OkHttp 5 + Gson (REST), OkHttp WebSocket (realtime, поздняя фаза).
- Coroutines/Flow, AndroidX Lifecycle ViewModel, Navigation Compose, DataStore.
- Coil 2 (+SVG) для медиа. ktlint + detekt + JUnit4/Robolectric/MockWebServer/MockK.

## Слои
```
ui/            Compose: theme/, components/, screens/, viewmodels/, navigation/
data/
  model/       Gson-DTO, зеркалящие JSON бэкенда
  api/         ApiService (Retrofit) + RetrofitClient (OkHttp, auth/refresh)
  repository/  репозитории фич — единственные, кто зовёт ApiService
  preferences/ AppPreferences (DataStore: сессия + тема + override сервера)
  realtime/    WebSocket-клиент (поздняя фаза)
  AppContainer ручной DI: держит prefs + резолвит активный ApiService
util/          маппинг ошибок, хелперы
```

Поток: `Screen` → `ViewModel` (UI-состояние в StateFlow) → `Repository` → `ApiService`.
ViewModel никогда не трогает Retrofit напрямую.

## Дизайн-система (`ui/theme`)
- `TesseraColors` — токены нейтральной палитры, портированы 1:1 из
  `frontend/src/styles/tokens.js` (light + dark).
- `AccentTheme` — 7 акцентных схем из `stores/theme.js` (по умолчанию purple).
- `LocalTessera` CompositionLocal + аксессор `Tessera.colors`. Компоненты читают
  именно это, **никогда** `MaterialTheme.colorScheme`. Material 3 обёрнут лишь как
  тонкий хост (ripple, выделение текста, базовая типографика).
- Свои примитивы в `ui/components` (`TButton`, `TTextField`, `TCard`, …)
  воспроизводят веб-облик (радиус 8dp, бордерные инпуты, плоские нажатия без ripple).

## Аутентификация и сессия
- `/auth/login|register` → `{access_token, refresh_token, user}` сохраняются в
  DataStore и пушатся в `RetrofitClient`.
- `RetrofitClient` подставляет Bearer-токен и делает тихий refresh-on-401
  (единый коалесцированный in-flight refresh), зеркаля `frontend/src/api/index.js`.
  На невосстановимый 401 сигналит logout; при refresh сохраняет новую пару.
- `AppRoot` — гейт сессии: splash → auth → main.

## URL сервера
- `BuildConfig.DEFAULT_BASE_URL` = `https://tessera.msdnna.website` (прод).
- Переопределяется в рантайме (экран входа → «Настройки сервера»), так что тот же APK
  может смотреть на dev/test-бэкенд (`http://10.0.2.2:8090` из эмулятора).

## Локализация (#2803)
- Язык интерфейса берётся из `user_preferences.language`, а **не** из системной
  локали — иначе английский телефон и русская веб-сессия разъезжаются.
- `res/values` — русский (базовая локаль), `res/values-en` — английский;
  `util/Languages.kt` нормализует всё, что шлёт сервер (`en-US`, неизвестное,
  пустое → `ru`), а `Context.withLanguage()` строит локализованный контекст для кода
  вне Compose (уведомления, тосты).
- `ui/AppLocale.kt` оборачивает дерево в `AppRoot` и подменяет `LocalResources` +
  `LocalConfiguration`. `LocalContext` намеренно не трогаем: он должен оставаться
  Activity для мест `LocalContext.current as? Activity`. Смена языка — это
  рекомпозиция, а не пересоздание Activity.
- Строки извлекаются волнами. `HardcodedStringsTest` держит список файлов, которым
  ещё можно нести русские литералы
  (`app/src/test/resources/i18n/untranslated.txt`) и сверяет его точно — переведённый
  файл обязан покинуть список, а новый литерал не должен появиться в уже вышедшем.
  `StringResourcesTest` гарантирует паритет ключей ru/en, формы множественного числа
  (для ru нужны one/few/many/other) и аргументы форматирования.

## Раскладка
Повторяет **мобильную** раскладку веба: гамбургер открывает сайдбар в 280dp-drawer;
топбар сверху; контент доски ниже. (Вариант с bottom-nav может позже заменить drawer,
когда устаканится мобильный UX — см. заметки проекта.)

## Сборка и релиз
- `android/VERSION` — единственный источник истины; `versionCode` выводится из него.
- `make android` (debug) / `make android-release` (подписанный) — см. Makefile репо.
- APK копируются в `/apks` для будущей фичи самообновления внутри приложения.
