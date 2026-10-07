-- Этапы и теги для окна «Проект» и страницы этапа (#2971: #2992, #3027, #3028).
--
--   * milestones.closed_at — когда этап закрыли. Нужен метрикам страницы этапа
--     (темп «сделано за этап» по прошлым этапам, конец burn-up у закрытого) и
--     подписи «закрыт 11 окт.». Ставит UpdateMilestone при переходе в closed,
--     снимает при возврате в active. Состояний по-прежнему два, как в GitLab;
--     «Запланирован» клиенты выводят из start_date в будущем.
--     Бэкфилл уже закрытых — оценкой: время последней правки строки (закрывают
--     обычно последней правкой; точнее взять неоткуда).
--   * tags.source — откуда тег: user (создан в Tessera) или gitlab (пришёл
--     лейблом из синка). Провайдеро-нейтральный шов, как task_tags.source.
--     Бэкфилл: тег, который синк хоть раз вешал на задачу (task_tags.source =
--     'gitlab'), считается пришедшим из GitLab.
--   * частичный индекс по событиям смены этапа — страница этапа считает по ним
--     изменение объёма и burn-up.
--
-- Аддитивная и повторяемая: каждый шаг защищён IF [NOT] EXISTS, бэкфиллы
-- трогают только пустые значения.

ALTER TABLE milestones ADD COLUMN IF NOT EXISTS closed_at timestamptz;
UPDATE milestones SET closed_at = updated_at
WHERE state = 'closed' AND closed_at IS NULL;

ALTER TABLE tags ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'user';
ALTER TABLE tags DROP CONSTRAINT IF EXISTS tags_source_check;
ALTER TABLE tags ADD CONSTRAINT tags_source_check CHECK (source IN ('user', 'gitlab'));
UPDATE tags g SET source = 'gitlab'
WHERE g.source = 'user'
  AND EXISTS (SELECT 1 FROM task_tags tt WHERE tt.tag_id = g.id AND tt.source = 'gitlab');

CREATE INDEX IF NOT EXISTS idx_task_events_milestone
    ON task_events (created_at)
    WHERE kind IN ('milestone', 'milestone_removed');
