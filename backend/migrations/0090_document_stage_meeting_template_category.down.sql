ALTER TABLE document_templates DROP COLUMN IF EXISTS category;
DROP INDEX IF EXISTS idx_documents_conference;
ALTER TABLE documents DROP COLUMN IF EXISTS conference_id;
DROP INDEX IF EXISTS idx_documents_milestone;
ALTER TABLE documents DROP COLUMN IF EXISTS milestone_id;
