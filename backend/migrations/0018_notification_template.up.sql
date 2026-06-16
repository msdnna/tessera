-- Per-channel message template (Phase B): a Go text/template rendered with the
-- notification's data to build the delivered message. Empty = use the built-in
-- default. Additive — safe on the live DB.
ALTER TABLE notification_channels ADD COLUMN template text NOT NULL DEFAULT '';
