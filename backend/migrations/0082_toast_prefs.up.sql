-- Всплывающие в приложении и «Не беспокоить» (#3117, пакет I редизайна v2).
-- Настройки тостов живут рядом с тихими часами — в notification_prefs:
--
--   * toasts_enabled — главный выключатель «Показывать» карточки «Всплывающие в
--     приложении». Лента колокольчика от него не зависит.
--   * toasts — что показывать: категории «Про меня», режим и события активности
--     открытой доски, свёртка пачек и её порог, телефон. Объект JSONB, читает
--     только клиент (сервер тосты не рисует).
--   * quiet_toasts — «И всплывающие в приложении» в тихих часах: окно тишины
--     глушит и тосты, а не только внешние каналы.
--   * dnd_until — «Не беспокоить» из шапки ленты: до этого момента тосты «про
--     меня», активности и системы не показываются. NULL — выключено.
--
-- Аддитивная: старые клиенты (Android) о колонках не знают, а PUT без них их не
-- трогает (см. UpdateMyNotificationPrefs).
ALTER TABLE notification_prefs ADD COLUMN IF NOT EXISTS toasts_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE notification_prefs ADD COLUMN IF NOT EXISTS toasts jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE notification_prefs ADD COLUMN IF NOT EXISTS quiet_toasts boolean NOT NULL DEFAULT false;
ALTER TABLE notification_prefs ADD COLUMN IF NOT EXISTS dnd_until timestamptz;
