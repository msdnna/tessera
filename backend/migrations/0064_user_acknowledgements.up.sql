-- Per-user "seen / acknowledged" flags keyed by an opaque string. One generic
-- store backs several one-shot UX affordances so none of them needs its own
-- table or column: the What's-New changelog modal (key `whatsnew:<webVersion>`),
-- the sidebar spotlight hints (`spotlight:<feature>`) and the future Get-Started
-- onboarding (`getstarted:<step>`). A row's mere existence means "acknowledged";
-- ack_at records when, for debugging/analytics only.
CREATE TABLE user_acknowledgements (
    user_id uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key     text        NOT NULL,
    ack_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);
