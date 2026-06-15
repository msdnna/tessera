-- name: GetUserPreferences :one
SELECT * FROM user_preferences WHERE user_id = $1;

-- name: UpsertUserPreferences :one
INSERT INTO user_preferences (
    user_id, language, timezone, country, time_format, date_format,
    week_start, theme, accent, board_background, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (user_id) DO UPDATE SET
    language         = EXCLUDED.language,
    timezone         = EXCLUDED.timezone,
    country          = EXCLUDED.country,
    time_format      = EXCLUDED.time_format,
    date_format      = EXCLUDED.date_format,
    week_start       = EXCLUDED.week_start,
    theme            = EXCLUDED.theme,
    accent           = EXCLUDED.accent,
    board_background = EXCLUDED.board_background,
    updated_at       = now()
RETURNING *;

-- name: GetUserAvatar :one
SELECT content_type, bytes FROM user_avatars WHERE user_id = $1;

-- name: UserHasAvatar :one
SELECT EXISTS(SELECT 1 FROM user_avatars WHERE user_id = $1);

-- name: UpsertUserAvatar :exec
INSERT INTO user_avatars (user_id, content_type, bytes, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (user_id) DO UPDATE SET
    content_type = EXCLUDED.content_type,
    bytes        = EXCLUDED.bytes,
    updated_at   = now();

-- name: DeleteUserAvatar :exec
DELETE FROM user_avatars WHERE user_id = $1;
