package main

import (
	"net/http"
	"testing"
)

// Задача, уехавшая на доску ДРУГОГО проекта, теряет этап молча (#2932).
//
// Само снятие — правильное и давно сделанное: этап проектный, и TransferTask /
// SetTaskParent / MoveSubtreeToBoard гасят milestone_id, когда целевая доска
// стоит в другом проекте (иначе «Этапы» продолжали бы считать задачу, которой на
// доске этого проекта нет). Но пара milestone / milestone_removed появилась
// ПОЗЖЕ, и в журнал этот путь не попал.
//
// Получалось единственное место, где поле задачи обнуляется без единого слова в
// «Истории», — и обнуляется НЕ по просьбе: читатель перетащил карточку на доску
// соседнего проекта, «Этап» опустел сам. Ровно от этого пару и заводили: перенос
// между этапами двигает прогресс двух этапов сразу, и «кто и когда» должно быть
// записано. Всё остальное, что переезд снимает по дороге, строчку оставляет —
// `reopened` при выезде из колонки завершения, `untagged`, `attachment_removed`.
//
// Запись несёт ЗАГОЛОВОК этапа, как и обычное снятие: id после переезда в задаче
// уже нет, а этап может быть и переименован, и удалён.
func TestMilestoneLostToProjectMoveIsJournalled(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	// Второй проект того же воркспейса + его доска: перенос разрешён только внутри
	// воркспейса, а границу проекта он при этом пересекает.
	projectB := mkProject(t, c, s.WS, s.Group, "Проект Б "+t.Name())
	boardB := mkBoard(t, c, projectB, "Доска Б")
	colB := c.get("/boards/" + boardB + "/columns").listBody(t)[0]["id"].(string)

	msID := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Релиз А"}), http.StatusCreated)["id"].(string)

	pin := func(t *testing.T, taskID string) {
		t.Helper()
		c.expect(t, c.post("/tasks/"+taskID+"/milestone",
			map[string]any{"milestone_id": msID}), http.StatusNoContent)
	}

	t.Run("перенос на доску чужого проекта пишет milestone_removed", func(t *testing.T) {
		task := mkTask(t, c, s.Board, col, "уезжает переносом")["id"].(string)
		pin(t, task)

		c.expect(t, c.patch("/tasks/"+task+"/transfer", map[string]any{
			"board_id": boardB, "column_id": colB,
		}), http.StatusOK)

		if data := lastTagEvent(t, c, task, "milestone_removed"); data["title"] != "Релиз А" {
			t.Fatalf("журнал переноса = %v, want заголовок потерянного этапа", data)
		}
	})

	t.Run("подзадачи получают запись каждая", func(t *testing.T) {
		top := mkTask(t, c, s.Board, col, "родитель")["id"].(string)
		mid := mkTask(t, c, s.Board, col, "ребёнок")["id"].(string)
		deep := mkTask(t, c, s.Board, col, "внук")["id"].(string)
		c.expect(t, c.patch("/tasks/"+mid+"/parent", map[string]any{"parent_id": top}), http.StatusOK)
		c.expect(t, c.patch("/tasks/"+deep+"/parent", map[string]any{"parent_id": mid}), http.StatusOK)
		for _, id := range []string{top, mid, deep} {
			pin(t, id)
		}

		c.expect(t, c.patch("/tasks/"+top+"/transfer", map[string]any{
			"board_id": boardB, "column_id": colB,
		}), http.StatusOK)

		// Поддерево переезжает одним движением, но этап теряет каждая задача по
		// отдельности — и история у каждой своя.
		for _, id := range []string{top, mid, deep} {
			if data := lastTagEvent(t, c, id, "milestone_removed"); data["title"] != "Релиз А" {
				t.Fatalf("журнал задачи %s = %v, want заголовок потерянного этапа", id, data)
			}
		}
	})

	t.Run("усыновление родителем из чужого проекта пишет milestone_removed", func(t *testing.T) {
		parent := mkTask(t, c, boardB, colB, "родитель в проекте Б")["id"].(string)
		child := mkTask(t, c, s.Board, col, "усыновляется")["id"].(string)
		grand := mkTask(t, c, s.Board, col, "внук усыновляемого")["id"].(string)
		c.expect(t, c.patch("/tasks/"+grand+"/parent", map[string]any{"parent_id": child}), http.StatusOK)
		pin(t, child)
		pin(t, grand)

		c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)

		for _, id := range []string{child, grand} {
			if data := lastTagEvent(t, c, id, "milestone_removed"); data["title"] != "Релиз А" {
				t.Fatalf("журнал задачи %s = %v, want заголовок потерянного этапа", id, data)
			}
		}
	})

	// Негативный контроль: переезд, который этап НЕ снимает, и строчки писать не
	// должен. Иначе «История» врала бы про потерю в обратную сторону — соседняя
	// доска того же проекта этап сохраняет.
	t.Run("соседняя доска того же проекта записи не пишет", func(t *testing.T) {
		board2 := mkBoard(t, c, s.Project, "Вторая доска проекта А")
		col2 := c.get("/boards/" + board2 + "/columns").listBody(t)[0]["id"].(string)
		task := mkTask(t, c, s.Board, col, "переезжает внутри проекта")["id"].(string)
		pin(t, task)

		c.expect(t, c.patch("/tasks/"+task+"/transfer", map[string]any{
			"board_id": board2, "column_id": col2,
		}), http.StatusOK)

		if n := countEvents(t, c, task, "milestone_removed"); n != 0 {
			t.Fatalf("записей milestone_removed после переезда внутри проекта = %d, want 0", n)
		}
	})

	// И подзадача, у которой этапа не было, ничего не теряет — запись пишется по
	// строкам, которые этап реально несли, а не по всему поддереву.
	t.Run("задача без этапа записи не получает", func(t *testing.T) {
		top := mkTask(t, c, s.Board, col, "родитель с этапом")["id"].(string)
		bare := mkTask(t, c, s.Board, col, "ребёнок без этапа")["id"].(string)
		c.expect(t, c.patch("/tasks/"+bare+"/parent", map[string]any{"parent_id": top}), http.StatusOK)
		pin(t, top)

		c.expect(t, c.patch("/tasks/"+top+"/transfer", map[string]any{
			"board_id": boardB, "column_id": colB,
		}), http.StatusOK)

		if n := countEvents(t, c, bare, "milestone_removed"); n != 0 {
			t.Fatalf("записей milestone_removed у задачи без этапа = %d, want 0", n)
		}
	})
}
