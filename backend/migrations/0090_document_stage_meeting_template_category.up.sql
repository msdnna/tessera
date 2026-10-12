-- Документы и шаблоны редизайна v2 (#3032): три связи, которых не было в данных.
--
--   * documents.milestone_id (#3044) — «Спринт 42» в шапке документа (макет
--     DocsPage). Одна ссылка, а не таблица связей: в макете у документа один этап,
--     как у задачи (tasks.milestone_id), и этап здесь — контекст документа, а не
--     список. SET NULL: удаление этапа не должно забирать спецификацию с собой.
--   * documents.conference_id (#3046) — документ-повестка встречи (TemplatesWin
--     «с привязкой к встрече»). Колонка на документе, а не на встрече: повестка —
--     свойство документа (её видно в его шапке), у встречи может быть несколько
--     документов (повестка, протокол), а строку встречи мы не трогаем. SET NULL —
--     удалённая встреча оставляет протокол.
--   * document_templates.category (#3045) — раздел галереи: Спринт · Продукт ·
--     Инженерия · Встречи. '' — без категории (виден в «Все»). «Мои шаблоны» —
--     это author_id, не категория. CHECK, а не enum, — как conferences.status:
--     набор будет расти, а ALTER TYPE в транзакции не везде доступен.
--
-- Обе ссылки документа живут в пределах пространства; перенос проекта
-- (TransferProject) снимает те, что оказались по разные стороны границы.
ALTER TABLE documents ADD COLUMN IF NOT EXISTS milestone_id uuid REFERENCES milestones(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_documents_milestone ON documents (milestone_id) WHERE milestone_id IS NOT NULL;

ALTER TABLE documents ADD COLUMN IF NOT EXISTS conference_id uuid REFERENCES conferences(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_documents_conference ON documents (conference_id) WHERE conference_id IS NOT NULL;

ALTER TABLE document_templates ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT ''
    CHECK (category IN ('', 'sprint', 'product', 'engineering', 'meetings'));
