// Задача #2932: синк GitLab сопоставляет исполнителя issue с Tessera-юзером по
// ИМЕНИ, глобально — и до этого теста ставил его в задачу, не спросив, состоит ли
// он в пространстве этой задачи.
//
// Инвариант «исполнитель задачи — участник её пространства» в проекте заявлен в
// трёх местах: AddTaskAssignee проверяет членство у пришедшего в теле user_id,
// notifyMentions/notifyThread молча пропускают не-участника, а TransferProject
// после переезда вычищает оставшихся StripNonMemberAssignees'ом. Синк из этого
// списка выпадал, хотя пишет в ту же таблицу.
//
// Цена промаха — не косметика: taskDetail отдаёт assignees через
// ListTaskAssignees, а тот SELECT'ит u.email и u.name. То есть команда, которая
// всего лишь привязала свою доску к GitLab-проекту, получала почту и имя
// человека, никогда в это пространство не входившего.
package main

import (
	"net/http"
	"strings"
	"testing"
)

// TestGitlabSyncAssigneeMustBeWorkspaceMember: issue назначен на GitLab-логин,
// который принадлежит Tessera-юзеру из ДРУГОГО пространства. После синка он не
// должен стать исполнителем задачи; вместо этого ему место во внешних
// (display-only) исполнителях — ветка, которая в reconcileAssignees уже есть для
// GitLab-людей без аккаунта в Tessera.
//
// Обратный случай — исполнитель-участник резолвится в настоящего юзера —
// закреплён в TestGitlabSyncFlow, так что дублировать его здесь нечем.
func TestGitlabSyncAssigneeMustBeWorkspaceMember(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	makeAdmin(t, owner)
	s := mkStack(t, owner)

	// Посторонний: обычный юзер Tessera со своим личным пространством, в s.WS его
	// нет и не было. GitLab-имя ему даёт привязка к своему же инстансу — резолв
	// смотрит только на gl_username, так что второй инстанс тут не лазейка, а
	// ровно тот случай, ради которого имя и хранится: gitlab.com рядом с
	// внутренним self-hosted.
	stranger := signup(t)
	strangerGL := newFakeGitlab(t, "gl-outsider-stranger", "grp-outsider-own")
	connectGitlab(t, stranger, strangerGL)

	f := newFakeGitlab(t, "gl-outsider-owner", "grp-outsider")
	f.addIssue(glIssue{
		IID: 1, Title: "Задача на постороннего",
		AssigneeLogins: []string{strangerGL.username},
	})
	f.members = []glMember{{ID: 7, Username: strangerGL.username, Name: "Посторонний"}}

	connectGitlab(t, owner, f)
	integ := createIntegration(t, owner, s.WS, s.Board, f, nil)
	triggerSync(t, owner, s.WS, integ["id"].(string))
	waitSyncRuns(t, owner, s.WS, 1)

	tasks := owner.get("/boards/" + s.Board + "/tasks").listBody(t)
	task := taskByIid(t, tasks, 1)
	if ids, _ := task["assignee_ids"].([]any); len(ids) != 0 {
		t.Fatalf("assignee_ids = %v, посторонний %s попал в исполнители задачи чужого пространства",
			ids, stranger.UserID)
	}

	// Та же строка с другой стороны: карточка задачи. Здесь и видно, что утекает
	// не uuid, а почта — ListTaskAssignees отдаёт u.email.
	detail := owner.expect(t, owner.get("/tasks/"+task["id"].(string)), http.StatusOK)
	if as, _ := detail["assignees"].([]any); len(as) != 0 {
		t.Fatalf("карточка задачи отдала исполнителей %v; почта постороннего (%s) видна команде, которая его не звала",
			as, stranger.Email)
	}
	if strings.Contains(string(owner.get("/tasks/"+task["id"].(string)).Body), stranger.Email) {
		t.Fatalf("почта постороннего %s встречается в ответе /tasks/:id", stranger.Email)
	}

	// Человек из GitLab при этом не исчезает с карточки: он остаётся внешним
	// исполнителем, с именем и аватаркой из GitLab. Иначе «фикс» просто терял бы
	// данные issue.
	ext, _ := detail["gitlab_assignees"].([]any)
	if len(ext) != 1 {
		t.Fatalf("gitlab_assignees = %v, want один внешний исполнитель %q", ext, strangerGL.username)
	}
	row, _ := ext[0].(map[string]any)
	if row["gl_username"] != strangerGL.username {
		t.Fatalf("внешний исполнитель = %v, want gl_username=%q", row, strangerGL.username)
	}
}
