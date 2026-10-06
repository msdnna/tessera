package handlers

import "testing"

// Окно дайджеста — сделка «немного опоздания в обмен на одно сообщение вместо
// десяти», и заключать её можно только на потоке активности. Уведомление,
// которое само называет момент, батчинг переносит ЗА этот момент (#2932):
// напоминание на 14:00 при часовом окне звонит в 14:59, «предупреди за 15 минут»
// приезжает после срока, приглашение — в уже кончившийся звонок. Приглашение
// проверяется здесь, а не интеграционно: поднимать конференцию ради одной
// развилки дороже, чем стоит сама развилка.
func TestTimedKindsIgnoreTheDigestWindow(t *testing.T) {
	const window = 60

	for _, kind := range []string{"reminder", "due_soon", kindConferenceInvite} {
		if got := digestMinutesFor(kind, window); got != 0 {
			t.Errorf("%s: окно %d мин осталось в силе (got %d) — доставка уедет за названный момент", kind, window, got)
		}
	}
	// Поток активности окно по-прежнему собирает — иначе освобождение
	// таймированных видов отключило бы дайджест целиком.
	for _, kind := range []string{"assigned", "comment", "mention", "updated", "moved", "archived", "integration_sync"} {
		if got := digestMinutesFor(kind, window); got != window {
			t.Errorf("%s: окно дайджеста потеряно (got %d, want %d)", kind, got, window)
		}
	}
	// Выключенный дайджест остаётся выключенным для всех.
	if got := digestMinutesFor("comment", 0); got != 0 {
		t.Errorf("выключенный дайджест: got %d, want 0", got)
	}
}
