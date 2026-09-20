// Закрытие повторяющейся задачи в её истории. Задачу можно закрыть двумя
// способами — галочкой в модалке (PATCH /tasks/:id) и перетаскиванием в
// done-колонку (PATCH /tasks/:id/move) — и на повторяющейся задаче они
// рассказывают «Историю» по-разному, хотя произошло одно и то же.
package main

import (
	"net/http"
	"testing"
	"time"
)

// Галочка на повторяющейся задаче — это закрытие очередного повторения, и
// журнал обязан записать именно его.
//
// updateTaskCore считает журнал диффом «задача до запроса → задача после», а
// «после» к этому моменту уже прошло через recurAdvance: повторение снимает
// completed_at и сдвигает срок. Из этого дважды выходит неправда: события
// `completed` нет вовсе (оба конца диффа открыты), а сдвиг срока и старта
// попадает в журнал как правка ПОЛЬЗОВАТЕЛЯ («изменил срок») — он же уезжает
// участникам уведомлением. Сам шаг повторения журнал и так пишет отдельной
// строкой `recurred` с новой датой.
//
// Перетаскивание в «Готово» той же задачи историю пишет правильно: applyMove
// логирует `completed` ДО повторения. Так что на одной карточке два способа
// закрыть её дают разную историю — этот тест держит их вместе.
func TestRecurringCompletionIsJournalled(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)
	done := boardDone(t, c, s.Board)
	if done == "" {
		t.Fatal("засеянная доска должна приезжать с done-колонкой")
	}

	due := time.Now().AddDate(0, 0, 2).UTC().Truncate(time.Second)
	start := due.AddDate(0, 0, -1)

	// mkRecurring отдаёт ежедневную задачу со сроком, стартом и правилом
	// повторения по закрытию — ровно то, что собирает DueEditor.
	mkRecurring := func(t *testing.T, title string) string {
		t.Helper()
		task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": todo,
			"due_date": rfc3339(due), "start_date": rfc3339(start),
		}), http.StatusCreated)
		id := task["id"].(string)
		c.expect(t, c.patch("/tasks/"+id, map[string]any{
			"due_date": rfc3339(due), "start_date": rfc3339(start),
			"recurrence": map[string]any{"freq": "daily", "interval": 1},
		}), http.StatusOK)
		return id
	}

	t.Run("галочкой", func(t *testing.T) {
		id := mkRecurring(t, "Полить цветы")
		adv := c.expect(t, c.patch("/tasks/"+id, map[string]any{"completed": true}), http.StatusOK)
		if adv["completed_at"] != nil {
			t.Fatalf("задача не перепланировалась — фикстура сломана: %v", adv)
		}
		if n := countEvents(t, c, id, "recurred"); n != 1 {
			t.Fatalf("записей recurred = %d, want 1", n)
		}
		if n := countEvents(t, c, id, "completed"); n != 1 {
			t.Fatalf("записей completed = %d, want 1: закрытие повторения не попало "+
				"в «Историю» вовсе — карточку закрывали, а журнал этого не знает", n)
		}
		if n := countEvents(t, c, id, "due"); n != 0 {
			t.Fatalf("записей due = %d, want 0: шаг повторения записан как правка срока "+
				"ПОЛЬЗОВАТЕЛЕМ — срок никто не трогал, а участникам ушло «изменил срок»", n)
		}
		if n := countEvents(t, c, id, "start"); n != 0 {
			t.Fatalf("записей start = %d, want 0: сдвиг окна повторением записан "+
				"как правка старта пользователем", n)
		}
		// Открывать задачу повторение не «переоткрывает»: она и не была закрыта
		// для читателя истории — закрылась и сразу уехала на новый срок.
		if n := countEvents(t, c, id, "reopened"); n != 0 {
			t.Fatalf("записей reopened = %d, want 0: %v", n, "повторение — не переоткрытие")
		}
	})

	t.Run("перетаскиванием в готово", func(t *testing.T) {
		id := mkRecurring(t, "Вынести мусор")
		moved := c.expect(t, c.patch("/tasks/"+id+"/move",
			map[string]any{"column_id": done}), http.StatusOK)
		if moved["completed_at"] != nil {
			t.Fatalf("задача не перепланировалась — фикстура сломана: %v", moved)
		}
		if n := countEvents(t, c, id, "completed"); n != 1 {
			t.Fatalf("записей completed = %d, want 1", n)
		}
		if n := countEvents(t, c, id, "recurred"); n != 1 {
			t.Fatalf("записей recurred = %d, want 1", n)
		}
	})
}
