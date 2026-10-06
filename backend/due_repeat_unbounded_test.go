// Срок предупреждения и интервал повтора уведомлений о сроке принимались без
// верхней границы, и переполнение int64 в их арифметике переворачивало смысл
// обеих настроек.
package main

import (
	"net/http"
	"testing"
	"time"
)

// TestDueLeadAndRepeatAreBounded пинит, что обе ручки отказывают интервалу,
// которого никто не мог иметь в виду.
//
// due_lead_minutes и due_repeat_minutes — int32, и до фикса перед записью стоял
// только кламп снизу (`< 0 → 0`) — и в PUT /notification-prefs, и в
// PATCH /tasks/:id/due-notify. Клиенты предлагают минуты и дни, но принималось
// любое число, и дальше dueShouldFire считал `time.Duration(x) * time.Minute`,
// которое с 153722868 переполняет int64 и выходит ОТРИЦАТЕЛЬНЫМ. Каждое поле
// после этого ломается в сторону, обратную запрошенной:
//   - repeat: следующий повтор — last_fired_at + интервал, с отрицательным он
//     всегда в прошлом. «Повторять раз в 4084 года» (int32-максимум) становится
//     срабатыванием на КАЖДОМ тике сканера — сообщение в минуту, пока задача
//     просрочена.
//   - lead: окно — due − lead, с отрицательным оно уезжает ЗА срок. «Предупреди
//     за 4084 года» становится молчанием до трёх суток после дедлайна.
//
// Тот же класс, что уже закрытые digest_minutes и quiet_tz (#2932): значение,
// принятое молча, позже даёт не то поведение, которое рисует экран. Пер-задачный
// override — вторая дверь к той же арифметике, и он побеждает пользовательский
// дефолт, поэтому проверяются обе.
func TestDueLeadAndRepeatAreBounded(t *testing.T) {
	t.Parallel()
	c := signup(t)

	prefs := func(lead, repeat int) map[string]any {
		return map[string]any{
			"due_enabled": true, "due_lead_minutes": lead, "due_repeat_minutes": repeat,
			"reminder_enabled": true, "quiet_enabled": false,
			"quiet_start_minutes": 1320, "quiet_end_minutes": 480,
			"quiet_tz": "UTC", "digest_minutes": 0,
		}
	}

	// Контроль: то, что предлагают клиенты, сохраняется и возвращается.
	p := c.expect(t, c.put("/notification-prefs", prefs(60, 15)), http.StatusOK)
	if p["due_lead_minutes"] != float64(60) || p["due_repeat_minutes"] != float64(15) {
		t.Fatalf("обычные значения должны проходить round-trip, got: %v", p)
	}

	// Находка: год — потолок. 525601 — первое запрещённое, 153722868 — первое
	// переполняющее, 2147483647 — int32-максимум (повтор каждую минуту).
	for _, m := range []int{525601, 153722868, 1000000000, 2147483647} {
		if r := c.put("/notification-prefs", prefs(m, 15)); r.Status != http.StatusBadRequest {
			t.Fatalf("due_lead_minutes=%d принято со статусом %d: предупреждение уедет за срок\n%s",
				m, r.Status, r.Body)
		}
		if r := c.put("/notification-prefs", prefs(60, m)); r.Status != http.StatusBadRequest {
			t.Fatalf("due_repeat_minutes=%d принято со статусом %d: повтор станет долбёжкой "+
				"каждую минуту\n%s", m, r.Status, r.Body)
		}
	}

	// Отказ не должен ничего двигать: пользователь живёт по последнему хорошему.
	p = c.expect(t, c.get("/notification-prefs"), http.StatusOK)
	if p["due_lead_minutes"] != float64(60) || p["due_repeat_minutes"] != float64(15) {
		t.Fatalf("отклонённый PUT изменил сохранённые настройки: %v", p)
	}

	// Контроль: годовая граница проходит ПО себе, а не под собой.
	p = c.expect(t, c.put("/notification-prefs", prefs(525600, 525600)), http.StatusOK)
	if p["due_lead_minutes"] != float64(525600) {
		t.Fatalf("годовой лид должен оставаться легальным, got: %v", p)
	}
	c.expect(t, c.put("/notification-prefs", prefs(60, 15)), http.StatusOK)

	// Вторая дверь — пер-задачный override.
	s := mkStack(t, c)
	id := mkTask(t, c, s.Board, s.col(t, 0), "Продлить домен")["id"].(string)
	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"due_date": time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
	}), http.StatusOK)

	for _, m := range []int{525601, 153722868, 2147483647} {
		if r := c.patch("/tasks/"+id+"/due-notify", map[string]any{
			"enabled": true, "lead_minutes": m, "repeat_minutes": 15,
		}); r.Status != http.StatusBadRequest {
			t.Fatalf("lead_minutes=%d на задаче принято со статусом %d\n%s", m, r.Status, r.Body)
		}
		if r := c.patch("/tasks/"+id+"/due-notify", map[string]any{
			"enabled": true, "lead_minutes": 60, "repeat_minutes": m,
		}); r.Status != http.StatusBadRequest {
			t.Fatalf("repeat_minutes=%d на задаче принято со статусом %d: просроченная задача "+
				"будет слать сообщение в минуту\n%s", m, r.Status, r.Body)
		}
	}

	// Контроль: обычный override по-прежнему сохраняется.
	task := c.expect(t, c.patch("/tasks/"+id+"/due-notify", map[string]any{
		"enabled": true, "lead_minutes": 60, "repeat_minutes": 15,
	}), http.StatusOK)
	if task["due_repeat_minutes"] != float64(15) {
		t.Fatalf("обычный override должен сохраняться, got: %v", task)
	}
}
