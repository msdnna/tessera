-- Tessera-hosted assets mirrored into GitLab's upload store (task #2713). Additive.
--
-- When a task becomes a GitLab issue, its description points at Tessera-relative
-- asset URLs ("/api/uploads/<name>"), which GitLab resolves against its OWN origin
-- and renders as dead links. The fix uploads the bytes to the project
-- (POST /projects/:id/uploads) and rewrites the link to the "/uploads/<sha>/<file>"
-- URL GitLab hands back.
--
-- This table is what keeps that fix from poisoning the write-back conflict detector.
-- title_desc conflicts compare `tasks.description` (Tessera URLs) against the issue
-- body pulled from GitLab (GitLab URLs); rewriting on push alone would make those two
-- strings differ forever, so every description edit would re-upload every image and
-- pile up duplicates in GitLab's store. The map is therefore read in BOTH directions:
-- outbound it makes the upload idempotent (an asset already mirrored is reused), and
-- inbound rewriteAssets turns our own mirrored URL back into "/api/uploads/<name>",
-- so the description round-trips to itself byte-for-byte.
--
-- source_key is provider-neutral by shape: "/api/uploads/<name>" for inline media,
-- "att:<attachment_id>" for a task attachment.

CREATE TABLE gitlab_uploads (
    integration_id uuid NOT NULL REFERENCES gitlab_integrations(id) ON DELETE CASCADE,
    source_key     text NOT NULL,
    gl_url         text NOT NULL,            -- project-relative, "/uploads/<sha>/<file>"
    gl_markdown    text NOT NULL DEFAULT '', -- GitLab's own suggested markdown for the upload
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (integration_id, source_key)
);

-- The inbound direction looks the row up by the GitLab URL (see rewriteAssets).
CREATE INDEX idx_gitlab_uploads_glurl ON gitlab_uploads (integration_id, gl_url);
