---
name: tessera-ship
description: Release hygiene for a finished Tessera change — bump the right component VERSION, add a CHANGELOG entry, and make a conventional commit. Use after a verified backend/frontend/android fix or feature, before handing off. Does NOT push or tag (the user does that).
---

# tessera-ship

Финализирует проверенное изменение в Tessera: bump версии нужного компонента →
запись в CHANGELOG → conventional-commit. **Не пушить и не ставить теги** — это делает
пользователь сам.

Предусловие: изменение уже проверено и quality-gate зелёный
(`make lint-<comp>` + `make test-<comp>`; для backend часто ещё e2e — см. скилл tessera-e2e).

## 1. Решить: бампать ли и какой компонент

Три независимо версионируемых компонента, у каждого свой `VERSION`:
`backend/VERSION`, `frontend/VERSION`, `android/VERSION`. Версии **не синхронизируются**.

Бамп — на каждое **содержательное** изменение именно этого компонента, по semver:

| Изменение | Bump |
|-----------|------|
| Багфикс, контракт/UI не меняется | `patch` |
| Новая обратно-совместимая фича | `minor` |
| Несовместимое изменение API/контракта/схемы | `major` |

**Без бампа** (используй префиксы `docs:`/`chore:`/`refactor:`): документация,
форматирование, refactor без поведенческих изменений, CI/dev-инфра.

## 2. Bump версии

```bash
make bump-api     BUMP=patch|minor|major   # backend/VERSION
make bump-web     BUMP=patch|minor|major   # frontend/VERSION
make bump-android BUMP=patch|minor|major   # android/VERSION
```
(Под капотом — `tools/bump-version.sh <service> <bump>`; он правит ТОЛЬКО файл VERSION,
ни changelog, ни git не трогает.)

## 3. CHANGELOG (вручную — скрипт его не пишет)

- **backend / frontend** → корневой `CHANGELOG.md`, в соответствующую секцию
  `## backend` или `## frontend`, новой записью **сверху**:
  ```
  ### [<новая-версия>] — <YYYY-MM-DD>
  - Что изменилось, по-человечески (как существующие записи).
  ```
- **android** → отдельный `android/CHANGELOG.md`, тем же форматом.

Формат — Keep a Changelog; стиль см. в существующих записях файла. Дату бери реальную
текущую (не выдумывай).

## 4. Conventional commit (локально)

```bash
git add -A
git commit -m "feat(web): краткое описание (web <версия>)"
```
- Типы: `feat` / `fix` / `docs` / `chore` / `refactor` / `test`. Скоупы по компоненту:
  `api`/`backend`, `web`/`ui`, `android`. Смотри `git log` для стиля (часто `(web 0.41.5)` в конце).
- Ветка по умолчанию — `develop`. Git-identity: `msdnna` / `extracker0mail@gmail.com`
  (НЕ harness-email).
- **Не `git push`, не `git tag`** — пользователь пушит и тегает сам. Просто оставь
  локальный коммит и скажи, что готово.

## Чеклист
- [ ] quality-gate зелёный для компонента
- [ ] `make bump-<comp>` с правильным semver-уровнем
- [ ] запись в нужный CHANGELOG (корневой для api/web, `android/CHANGELOG.md` для android)
- [ ] conventional-commit локально, без push/tag
