package main

import (
	"net/http"
	"testing"
)

// Милестоун («Этап») — сущность ПРОЕКТНАЯ: и создаётся под проектом
// (POST /projects/:id/milestones), и выбирается в модалке из этапов своего
// проекта. Но задача умеет уезжать на доску другого проекта — переносом
// (PATCH /tasks/:id/transfer) и усыновлением родителем с чужой доски
// (PATCH /tasks/:id/parent). Ни один из этих путей не трогал milestone_id.
//
// Задача уезжала в проект B и продолжала числиться за этапом проекта A:
//   - «Этапы» (ListWorkspaceMilestones) джойнит tasks по milestone_id без
//     фильтра по проекту, так что total / done / Σ estimate этапа A росли на
//     задачу, которой на доске проекта A нет;
//   - deep-link с того же экрана ведёт на доску проекта A — посчитанной задачи
//     там не найти;
//   - в модалке задачи поле «Этап» читается как «Не задан»: список вариантов
//     собран из этапов СВОЕГО проекта, и чужого id в нём нет, — то есть увидеть
//     и снять привязку через UI нельзя.
//
// Ровно тот же дефект репозиторий уже описывает для кросс-воркспейсного случая
// (ClearMilestonesAcrossWorkspaces в milestones.sql), где он чинится снятием
// привязки. Здесь — внутриворкспейсный, и потому со счётчиком, который врёт на
// глазах.
//
// Прямая привязка к этапу соседнего проекта через API (POST /tasks/:id/milestone)
// остаётся разрешённой: applyMilestone сверяет воркспейс, и на этом стоит
// TestTransferProjectSeversCrossWorkspaceRefs — он так и заводит пару, чтобы
// проверить починку на переносе проекта. Оба клиента такую пару не строят
// (в модалке выбор — только этапы своего проекта), а здесь речь про пути, по
// которым расхождение возникает САМО, без единого «неправильного» запроса.
func TestMilestoneClearedWhenTaskLeavesItsProject(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	// Второй проект того же воркспейса + его доска.
	projectB := mkProject(t, c, s.WS, s.Group, "Проект Б "+t.Name())
	boardB := mkBoard(t, c, projectB, "Доска Б")
	colB := c.get("/boards/" + boardB + "/columns").listBody(t)[0]["id"].(string)

	ms := c.expect(t, c.post("/projects/"+s.Project+"/milestones",
		map[string]any{"title": "Этап проекта А"}), http.StatusCreated)
	msID := ms["id"].(string)

	// countMilestone возвращает рассчитанный сервером rollup этапа на экране «Этапы».
	countMilestone := func(t *testing.T) (total, done float64) {
		t.Helper()
		for _, r := range c.get("/workspaces/" + s.WS + "/milestones").listBody(t) {
			if r["id"] == msID {
				return r["task_count"].(float64), r["done_count"].(float64)
			}
		}
		t.Fatalf("этап %s пропал из /workspaces/%s/milestones", msID, s.WS)
		return 0, 0
	}

	pin := func(t *testing.T, taskID string) {
		t.Helper()
		c.expect(t, c.post("/tasks/"+taskID+"/milestone",
			map[string]any{"milestone_id": msID}), http.StatusNoContent)
	}
	milestoneOf := func(t *testing.T, taskID string) any {
		t.Helper()
		return c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)["milestone_id"]
	}

	t.Run("перенос в чужой проект снимает этап", func(t *testing.T) {
		task := mkTask(t, c, s.Board, col, "уезжает переносом")["id"].(string)
		pin(t, task)
		if total, _ := countMilestone(t); total != 1 {
			t.Fatalf("до переноса task_count = %v, want 1", total)
		}

		c.expect(t, c.patch("/tasks/"+task+"/transfer", map[string]any{
			"board_id": boardB, "column_id": colB,
		}), http.StatusOK)

		if got := milestoneOf(t, task); got != nil {
			t.Fatalf("после переноса в проект Б задача всё ещё числится за этапом проекта А: milestone_id = %v", got)
		}
		if total, _ := countMilestone(t); total != 0 {
			t.Fatalf("после переноса task_count этапа = %v, want 0 — «Этапы» считают задачу, которой на доске проекта А нет", total)
		}
	})

	t.Run("перенос внутри своего проекта этап сохраняет", func(t *testing.T) {
		board2 := mkBoard(t, c, s.Project, "Вторая доска проекта А")
		col2 := c.get("/boards/" + board2 + "/columns").listBody(t)[0]["id"].(string)
		task := mkTask(t, c, s.Board, col, "переезжает внутри проекта")["id"].(string)
		pin(t, task)

		c.expect(t, c.patch("/tasks/"+task+"/transfer", map[string]any{
			"board_id": board2, "column_id": col2,
		}), http.StatusOK)

		if got := milestoneOf(t, task); got != msID {
			t.Fatalf("соседняя доска того же проекта — этап должен остаться: milestone_id = %v, want %s", got, msID)
		}
		c.expect(t, c.del("/tasks/"+task+"/milestone"), http.StatusNoContent)
	})

	t.Run("подзадачи уезжают без этапа вместе с родителем", func(t *testing.T) {
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

		for _, id := range []string{mid, deep} {
			if got := milestoneOf(t, id); got != nil {
				t.Fatalf("подзадача %s уехала в проект Б с этапом проекта А: milestone_id = %v", id, got)
			}
		}
		if total, _ := countMilestone(t); total != 0 {
			t.Fatalf("после переноса поддерева task_count этапа = %v, want 0", total)
		}
	})

	t.Run("усыновление родителем из чужого проекта снимает этап", func(t *testing.T) {
		parent := mkTask(t, c, boardB, colB, "родитель в проекте Б")["id"].(string)
		child := mkTask(t, c, s.Board, col, "усыновляется")["id"].(string)
		grand := mkTask(t, c, s.Board, col, "внук усыновляемого")["id"].(string)
		c.expect(t, c.patch("/tasks/"+grand+"/parent", map[string]any{"parent_id": child}), http.StatusOK)
		pin(t, child)
		pin(t, grand)

		c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)

		for _, id := range []string{child, grand} {
			if got := milestoneOf(t, id); got != nil {
				t.Fatalf("задача %s переехала на доску проекта Б с этапом проекта А: milestone_id = %v", id, got)
			}
		}
		if total, _ := countMilestone(t); total != 0 {
			t.Fatalf("после усыновления task_count этапа = %v, want 0", total)
		}
	})

}
