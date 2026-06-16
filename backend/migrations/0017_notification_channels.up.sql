-- Notification channels & routing (Phase A): per-user delivery destinations
-- (email / telegram / webhook / …) plus an Alertmanager-style routing layer that
-- decides which notifications reach which channels, and an outbox queue that the
-- background delivery worker drains. Additive — safe on the live DB.
--
-- Loose coupling (see CLAUDE.md): the channel `type` is a free string, not an
-- enum, so a new provider needs no schema change; secrets are encrypted at rest
-- exactly like GitLab PATs (AES-256-GCM, base64 in secret_enc).

-- Per-user delivery channel. `config` holds non-secret, user-visible settings
-- (telegram chat_id, webhook url, target email …); `secret_enc` holds the
-- encrypted secret blob (telegram bot token, webhook auth header …) and is never
-- returned to the client.
CREATE TABLE notification_channels (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       text NOT NULL,                       -- 'email' | 'telegram' | 'webhook' | … (extensible)
    label      text NOT NULL DEFAULT '',            -- user-facing name
    config     jsonb NOT NULL DEFAULT '{}'::jsonb,  -- non-secret settings (returned to client)
    secret_enc text NOT NULL DEFAULT '',            -- AES-256-GCM(secret JSON), base64; never returned
    enabled    boolean NOT NULL DEFAULT true,
    verified   boolean NOT NULL DEFAULT false,      -- a successful test send flips this on
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notification_channels_user ON notification_channels (user_id);

-- Per-user routing rule. Ordered by `position`; the dispatcher uses the first
-- enabled rule whose `matcher` matches an event and fans the notification out to
-- its `channel_ids` (or drops it when options.mute is set). An empty matcher
-- matches everything (catch-all). In-app notifications are unconditional and not
-- modelled here — these rules only gate external channels.
CREATE TABLE notification_routes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position    double precision NOT NULL DEFAULT 0,
    matcher     jsonb NOT NULL DEFAULT '{}'::jsonb, -- {kinds:[...], workspace_id}
    channel_ids uuid[] NOT NULL DEFAULT '{}',
    options     jsonb NOT NULL DEFAULT '{}'::jsonb, -- {mute: bool}  (quiet_hours land in Phase B)
    enabled     boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notification_routes_user ON notification_routes (user_id, position);

-- Outbox queue: one row per (notification, channel) the dispatcher decided to
-- deliver. The background worker claims pending rows that are due, attempts the
-- send, and either marks them sent or reschedules with backoff (failed after the
-- attempt cap). Decouples external delivery from the request path (async, retry,
-- survives restart).
CREATE TABLE notification_deliveries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id uuid NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel_id      uuid NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'pending', -- pending | sending | sent | failed
    attempts        integer NOT NULL DEFAULT 0,
    last_error      text NOT NULL DEFAULT '',
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notification_deliveries_due ON notification_deliveries (next_attempt_at) WHERE status = 'pending';
