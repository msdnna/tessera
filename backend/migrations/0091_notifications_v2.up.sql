-- Уведомления v2 (#3033): исключения тихих часов, ответ на приглашение во
-- встречу и «встреча через 5 минут».
--
--   * notification_prefs.quiet_allow_urgent — «Только срочное и упоминания»
--     (#3050): в тихие часы внешние каналы всё равно получают упоминания, срочные
--     задачи и встречу, которая вот-вот начнётся; остальное ждёт конца окна.
--   * conference_participants.rsvp / rsvp_at — «Принять / Отклонить» (#3051).
--     pending — приглашение без ответа; хозяин встречи отвечает «да» сам.
--   * conferences.soon_notified_for — для какого scheduled_at уже ушло «встреча
--     через 5 минут» (#3048): перенос встречи снимает отметку сам, сравнением.
--
-- Аддитивная: старые клиенты о колонках не знают, PUT настроек без нового поля
-- его не трогает (см. UpdateMyNotificationPrefs).
ALTER TABLE notification_prefs ADD COLUMN IF NOT EXISTS quiet_allow_urgent boolean NOT NULL DEFAULT false;

ALTER TABLE conference_participants ADD COLUMN IF NOT EXISTS rsvp text NOT NULL DEFAULT 'pending'
    CHECK (rsvp IN ('pending', 'accepted', 'declined'));
ALTER TABLE conference_participants ADD COLUMN IF NOT EXISTS rsvp_at timestamptz;
-- Хозяин встречи и те, кто уже входил в неё, ответили делом.
UPDATE conference_participants SET rsvp = 'accepted', rsvp_at = COALESCE(joined_at, invited_at)
WHERE role = 'host' OR joined_at IS NOT NULL;

ALTER TABLE conferences ADD COLUMN IF NOT EXISTS soon_notified_for timestamptz;
-- Встречи, время которых уже наступило, не должны звенеть задним числом.
UPDATE conferences SET soon_notified_for = scheduled_at
WHERE scheduled_at IS NOT NULL AND scheduled_at <= now();
