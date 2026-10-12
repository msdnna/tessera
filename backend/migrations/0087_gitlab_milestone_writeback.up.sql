-- Этапы GitLab — запись из Tessera и конфликты (#3010: #3023, #3024, #3025).
--
-- До этого этап с привязкой к GitLab был в Tessera только для чтения (0039):
-- синк переписывал его поля значениями GitLab. Теперь его можно править и
-- закрывать в Tessera — правка уходит в GitLab (когда у привязки включена
-- запись), а синк сверяет три версии, как у задач (gitlab_links.gl_snapshot,
-- 0037). Всё GitLab-специфичное — в таблице связи, ядро `milestones` не
-- трогаем.
--
--   * gl_snapshot   — база для трёхсторонней сверки: поля этапа в GitLab на
--                     момент последней удачной синхронизации
--                     {title, description, start, due, state}; даты — YYYY-MM-DD.
--   * push_error    — почему последняя запись в GitLab не прошла ('' — прошла);
--                     полоса этапа показывает «не синхронизировано · повторить».
--   * push_fields   — какие поля не удалось записать (подпись индикатора).
--   * push_actor_id — от чьего имени повторять запись (синк повторяет сам).
--   * conflict      — поля, изменённые и в Tessera, и в GitLab после базы:
--                     {fields:[{field, base, ours, theirs}], detected_at};
--                     '{}' — конфликта нет. Решается в окне конфликтов.
--
-- Бэкфилл базы: до этой миграции этап с привязкой правился только синком,
-- поэтому его текущие поля и есть последнее, что пришло из GitLab.
-- Аддитивная и повторяемая.

ALTER TABLE gitlab_milestone_links
    ADD COLUMN IF NOT EXISTS gl_snapshot   jsonb  NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS push_error    text   NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS push_fields   text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS push_actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS conflict      jsonb  NOT NULL DEFAULT '{}'::jsonb;

UPDATE gitlab_milestone_links l
SET gl_snapshot = jsonb_build_object(
        'title', m.title,
        'description', m.description,
        'start', COALESCE(to_char(m.start_date AT TIME ZONE 'UTC', 'YYYY-MM-DD'), ''),
        'due', COALESCE(to_char(m.due_date AT TIME ZONE 'UTC', 'YYYY-MM-DD'), ''),
        'state', m.state)
FROM milestones m
WHERE m.id = l.milestone_id AND l.gl_snapshot = '{}'::jsonb;
