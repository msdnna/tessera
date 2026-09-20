// Окно дайджеста принималось без верхней границы, и переполнение int64 в его
// арифметике уводило доставку в прошлое — мимо тихих часов.
package main

import (
	"net/http"
	"testing"
)

// TestDigestMinutesIsBounded пинит, что ручка prefs отказывает окну, которого
// никто не мог иметь в виду.
//
// digest_minutes — int32, и до фикса перед записью стояло только max(0, …).
// Клиенты предлагают 5/15/30/60, но PUT принимал любое число, и дальше
// digestSlot считал `time.Duration(dm) * time.Minute`:
//   - 100000 отодвигает следующий слот на семь недель, 1e9 — в год 2215.
//     Уведомление не отложено, оно проглочено: в аутбоксе строка вечно pending,
//     и ничто об этом не сообщает.
//   - с 153722868 умножение переполняет int64, окно выходит ОТРИЦАТЕЛЬНЫМ.
//     Truncate для неположительной длительности — no-op, Add идёт назад, и при
//     int32-максимуме слот падает в 2017 год. Строка, созревшая в прошлом,
//     забирается первым же тиком: письмо уходит немедленно, выбрасывая якорь
//     тихих часов, который digestSlot только что поставил, — то есть в 23:00
//     уезжает ровно то сообщение, что окно отложило до 08:00 (#2932).
//
// Тот же класс, что уже закрытая «неизвестная зона принималась с 200»: второе
// поле той же ручки, тоже сохранялось молча и тоже превращало настройку в ложь,
// которую оба клиента подтверждают.
func TestDigestMinutesIsBounded(t *testing.T) {
	t.Parallel()
	c := signup(t)

	prefs := func(digest int) map[string]any {
		return map[string]any{
			"due_enabled": true, "due_lead_minutes": 60, "due_repeat_minutes": 0,
			"reminder_enabled": true, "quiet_enabled": true,
			"quiet_start_minutes": 1320, "quiet_end_minutes": 480,
			"quiet_tz": "UTC", "digest_minutes": digest,
		}
	}

	// Контроль: окно, которое предлагают клиенты, сохраняется и возвращается.
	p := c.expect(t, c.put("/notification-prefs", prefs(60)), http.StatusOK)
	if p["digest_minutes"] != float64(60) {
		t.Fatalf("часовое окно должно проходить round-trip, got: %v", p)
	}

	// Находка: окна больше суток быть не может. 1441 — первое запрещённое,
	// 100000 — «семь недель», 153722868 — первое переполняющее, 2147483647 —
	// int32-максимум (слот в 2017-м).
	for _, dm := range []int{1441, 100000, 153722868, 1000000000, 2147483647} {
		if r := c.put("/notification-prefs", prefs(dm)); r.Status != http.StatusBadRequest {
			t.Fatalf("digest_minutes=%d принято со статусом %d: доставка уедет в прошлое "+
				"или не уедет никогда\n%s", dm, r.Status, r.Body)
		}
	}

	// Отказ не должен ничего двигать: окно, по которому живёт пользователь, —
	// последнее хорошее.
	p = c.expect(t, c.get("/notification-prefs"), http.StatusOK)
	if p["digest_minutes"] != float64(60) {
		t.Fatalf("отклонённый PUT изменил сохранённое окно: %v", p)
	}

	// Контроль: суточный свод остаётся достижимым — граница проходит по нему.
	p = c.expect(t, c.put("/notification-prefs", prefs(1440)), http.StatusOK)
	if p["digest_minutes"] != float64(1440) {
		t.Fatalf("суточное окно должно оставаться легальным, got: %v", p)
	}

	// Контроль: выключенный дайджест — не ошибка, это дефолт.
	p = c.expect(t, c.put("/notification-prefs", prefs(0)), http.StatusOK)
	if p["digest_minutes"] != float64(0) {
		t.Fatalf("нулевое окно должно оставаться легальным, got: %v", p)
	}
}
