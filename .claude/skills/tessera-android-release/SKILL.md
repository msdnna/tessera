---
name: tessera-android-release
description: Build and publish a signed Tessera Android release APK with self-update metadata. Use when shipping a new Android version — covers the R8/Gson keep-rule trap, the shared budget keystore, latest.json self-update publishing, and the install-signature gotcha. This dev host IS the deployed server.
---

# tessera-android-release

Сборка и публикация подписанного релиза Android-клиента Tessera. **RELEASE — дефолт**
(R8 + shrinkResources → APK ~2 МБ и плавный; debug ~15 МБ и лагал на анимациях).

> Этот dev-хост И ЕСТЬ боевой сервер: сборка/подпись здесь через
> `android/local.env` + общий keystore, apks раздаются фронтом на `:8083/apks/`.
> Значит, отсюда можно и собрать, и опубликовать релиз.

## Перед сборкой — лучше проверь
- **R8/Gson-ловушка (стоила release-only бага):** ЛЮБАЯ Gson-модель ДОЛЖНА жить в пакете
  `data.model` (единственный, что покрыт keep-правилом). Иначе release переименует поля →
  Gson десериализует в дефолты (так молча сломался self-update, когда `LatestRelease`
  лежал в `update/`). Debug этого не ловит (без R8). Проверь новые модели.
- ktlint + detekt зелёные: `make lint-android` (auto-fix — `make format-android`).
- Версия сбампана: `make bump-android BUMP=...` + запись в `android/CHANGELOG.md` (скилл tessera-ship).

## Сборка релиза
```bash
make android-release        # = tools/build-android-release.sh
```
Что делает:
- `assembleRelease` (читает SDK/JDK/SOCKS-proxy из gitignored `android/local.env`).
- **Подпись — ОБЩИМ budget-keystore** `/home/msdnna/budget.jks`, alias `budget` (тот же
  ключ, что у budget-go; creds в `local.env`; `signingConfigs.release` падает на debug-ключ,
  если env пуст).
- Кладёт `android/msdnna-tessera-v<version>.apk` И копию в репозиторный `apks/`.
- Пишет `apks/latest.json` `{version, versionCode, apk, notes}`, где
  **versionCode = major*10000 + minor*100 + patch**.

Debug-сборка (когда реально нужна): `make android` (`build.sh` → `assembleDebug`).

## Публикация self-update
- Фронт-nginx раздаёт bind-mount `./apks` на `/apks/`; приложение (`UpdateRepository` +
  `UpdateViewModel`/`UpdateDialog` + sidebar-строка «Обновить») опрашивает
  `<server>/apks/latest.json` на старте и каждый ON_RESUME, качает и ставит через package installer.
- Файлы `latest.json` + именованный apk уже на месте после `make android-release` → happy-path
  работает. **Caveat:** правило `location /apks/ { try_files $uri =404; }` запечено в фронт-образ,
  но контейнер мог быть пересоздан только под bind-mount → отсутствующий apk падает в SPA-index
  (200 HTML). Чтобы активировать строгий 404 — пересобрать фронт-образ (для happy-path не нужно).

## Установка на устройство (гоча)
- Release (budget-ключ) поверх установленного **debug**-билда → разные подписи →
  `INSTALL_FAILED_UPDATE_INCOMPATIBLE`. Сначала **удалить** старый билд, потом ставить.
- Пользователь обычно сам собирает/подписывает/пушит APK и тестит на устройстве.

## Чеклист
- [ ] новые Gson-модели — в `data.model`
- [ ] `make lint-android` зелёный
- [ ] `make bump-android` + `android/CHANGELOG.md`
- [ ] `make android-release` → apk + `apks/latest.json` обновлены
- [ ] (если нужен строгий 404 на отсутствующий apk) фронт-образ пересобран
