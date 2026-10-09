-- Старые адреса проекта (#3012, пакет A редизайна v2). Смена адреса проекта
-- раньше рвала все выданные ссылки; теперь прежний slug 30 дней ведёт на новый.
--
--   * slug — прежний адрес. PRIMARY KEY: один старый адрес ведёт ровно в один
--     проект. Повторный уход с того же адреса перезаписывает строку.
--   * project_id — куда ведёт. Удаление проекта уносит его алиасы.
--   * expires_at — до какого момента адрес ведёт. Читатели фильтруют по нему
--     сами, воркер trash_purge только подметает просроченное.
--
-- Алиас не занимает адрес: slug живого проекта (projects.slug, уникален на
-- инстанс) всегда важнее. Проект, занявший адрес, стирает алиас с ним.
CREATE TABLE IF NOT EXISTS project_slug_aliases (
    slug        text PRIMARY KEY,
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_project_slug_aliases_project ON project_slug_aliases (project_id);
CREATE INDEX IF NOT EXISTS idx_project_slug_aliases_expires ON project_slug_aliases (expires_at);
