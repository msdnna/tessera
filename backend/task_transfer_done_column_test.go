package main

import (
	"net/http"
	"testing"
)

// Завершённость следует за колонкой — это правило доски: applyMove закрывает
// карточку, въехавшую в done-колонку, и ОТКРЫВАЕТ ту, что из неё выехала.
// Перенос на другую доску (PATCH /tasks/:id/transfer) колонку меняет тоже, но
// мимо applyMove: он просто переписывает board_id/column_id, а подзадачам —
// через MoveSubtreeToBoard.
//
// Поэтому завершённая карточка приезжала в ПЕРВУЮ колонку чужой доски всё ещё
// закрытой (оба клиента шлют только board_id: TaskModal.vue, TaskDetailViewModel.kt):
// перечёркнутая в «К работе», посчитанная в «Готово» — и правило reopen её уже
// не достанет, потому что оно срабатывает только для задачи, СТОЯЩЕЙ в
// done-колонке. Разгрести это можно было лишь ритуалом «затащить в «Готово» и
// вытащить обратно».
//
// Условие починки — исходная колонка, а не сам флаг: задачу, закрытую в любой
// другой колонке (или на доске без done-колонки вовсе), внутридосочный перенос
// тоже не трогает, и перенос не должен выдумывать связь, которой на доске нет.
func TestTransferOutOfDoneColumnReopensTask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	src := mkStack(t, c)
	srcTodo := src.col(t, 0)
	srcDone := boardDone(t, c, src.Board)
	if srcDone == "" {
		t.Fatal("засеянная доска должна приезжать с done-колонкой")
	}

	// Вторая доска того же проекта: у неё своя лестница колонок и своя
	// done-колонка, а перенос без column_id кладёт карточку в первую.
	dstBoard := mkBoard(t, c, src.Project, "Доска-приёмник "+t.Name())
	dstCols := c.get("/boards/" + dstBoard + "/columns").listBody(t)
	dstTodo := dstCols[0]["id"].(string)
	dstDone := boardDone(t, c, dstBoard)

	completedOf := func(t *testing.T, taskID string) any {
		t.Helper()
		return c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)["completed_at"]
	}
	// closeByColumn закрывает задачу так, как это делает читатель: перетаскиванием
	// в done-колонку своей доски.
	closeByColumn := func(t *testing.T, taskID string) {
		t.Helper()
		moved := c.expect(t, c.patch("/tasks/"+taskID+"/move",
			map[string]any{"column_id": srcDone}), http.StatusOK)
		if moved["completed_at"] == nil {
			t.Fatalf("задача %s не закрылась в done-колонке — фикстура сломана", taskID)
		}
	}
	hasEvent := func(t *testing.T, taskID, kind string) bool {
		t.Helper()
		for _, e := range c.get("/tasks/" + taskID + "/events").listBody(t) {
			if e["kind"] == kind {
				return true
			}
		}
		return false
	}

	t.Run("из done-колонки в первую колонку чужой доски — открывается", func(t *testing.T) {
		task := mkTask(t, c, src.Board, srcTodo, "готовая уезжает")["id"].(string)
		closeByColumn(t, task)

		out := c.expect(t, c.patch("/tasks/"+task+"/transfer",
			map[string]any{"board_id": dstBoard}), http.StatusOK)

		if out["column_id"] != dstTodo {
			t.Fatalf("перенос без column_id положил карточку в %v, want первую колонку %s", out["column_id"], dstTodo)
		}
		if out["completed_at"] != nil {
			t.Fatalf("ответ переноса всё ещё держит completed_at = %v", out["completed_at"])
		}
		if got := completedOf(t, task); got != nil {
			t.Fatalf("карточка стоит в «К работе» чужой доски и числится завершённой: completed_at = %v", got)
		}
		if !hasEvent(t, task, "reopened") {
			t.Fatal("завершённость сняли молча — в журнале задачи нет записи reopened")
		}
	})

	t.Run("подзадачи из done-колонки открываются вместе с родителем", func(t *testing.T) {
		top := mkTask(t, c, src.Board, srcTodo, "родитель")["id"].(string)
		mid := mkTask(t, c, src.Board, srcTodo, "ребёнок")["id"].(string)
		deep := mkTask(t, c, src.Board, srcTodo, "внук")["id"].(string)
		c.expect(t, c.patch("/tasks/"+mid+"/parent", map[string]any{"parent_id": top}), http.StatusOK)
		c.expect(t, c.patch("/tasks/"+deep+"/parent", map[string]any{"parent_id": mid}), http.StatusOK)
		// Закрыт только внук — и закрыт колонкой: MoveSubtreeToBoard утащит его
		// в первую колонку доски-приёмника вместе с родителем.
		closeByColumn(t, deep)

		c.expect(t, c.patch("/tasks/"+top+"/transfer",
			map[string]any{"board_id": dstBoard}), http.StatusOK)

		if got := completedOf(t, deep); got != nil {
			t.Fatalf("внук уехал в «К работе» чужой доски завершённым: completed_at = %v", got)
		}
	})

	t.Run("завершённая вне done-колонки флаг сохраняет", func(t *testing.T) {
		task := mkTask(t, c, src.Board, srcTodo, "закрыта галочкой")["id"].(string)
		c.expect(t, c.patch("/tasks/"+task, map[string]any{"completed": true}), http.StatusOK)
		if completedOf(t, task) == nil {
			t.Fatal("галочка «завершена» не проставилась — фикстура сломана")
		}

		c.expect(t, c.patch("/tasks/"+task+"/transfer",
			map[string]any{"board_id": dstBoard}), http.StatusOK)

		if completedOf(t, task) == nil {
			t.Fatal("перенос снял завершённость с задачи, которая в done-колонке не стояла")
		}
	})

	t.Run("в done-колонку целевой доски — остаётся закрытой", func(t *testing.T) {
		if dstDone == "" {
			t.Skip("у доски-приёмника нет done-колонки")
		}
		task := mkTask(t, c, src.Board, srcTodo, "из готового в готовое")["id"].(string)
		closeByColumn(t, task)

		c.expect(t, c.patch("/tasks/"+task+"/transfer",
			map[string]any{"board_id": dstBoard, "column_id": dstDone}), http.StatusOK)

		if completedOf(t, task) == nil {
			t.Fatal("карточка приехала в «Готово» чужой доски и там открылась")
		}
	})
}
