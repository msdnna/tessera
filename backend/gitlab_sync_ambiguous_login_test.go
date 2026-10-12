// Задача #2932: синк GitLab резолвит исполнителя issue по логину, а логин в
// oauth_identities не уникален — и до этого теста при двух совпадающих строках
// выбиралась произвольная.
//
// Схема говорит об этом прямо: UNIQUE стоит на (provider, provider_user_id),
// числовом id, а по provider_username лежит обычный (неуникальный) индекс —
// имя в GitLab меняется, номер нет. Строку с именем обновляет только следующий
// вход человека (upsertIdentity), так что между переименованием в GitLab и этим
// входом старое имя остаётся записанным за прежним владельцем. Если за то же
// время освободившийся логин займёт другой человек и войдёт в Tessera, в
// таблице окажутся ДВЕ живые строки с одним provider_username — а оба запроса
// резолва были объявлены `:one` без ORDER BY и LIMIT, то есть отдавали ту
// строку, которую первой вернёт план (сейчас это `GetUserIDsByOAuthUsername` и
// `GetUserIDsByGitlabUsername`, возвращающие список).
//
// Гвард членства, добавленный этой же задачей, здесь не спасает: оба —
// участники одного пространства, коллеги. Промах молчалив и адресен: задача
// уезжает не тому человеку (он получает её в «мои задачи» и уведомления), а
// настоящий исполнитель issue её не видит.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// mutableIDP — фейковый GitLab, у которого можно подменить отдаваемого
// пользователя между входами. Именно один сервер, а не два: сценарий про ОДИН
// инстанс (gl_base_url у обеих строк совпадает), иначе находка слилась бы с
// уже закрытой кросс-инстансной.
type mutableIDP struct {
	mu   sync.Mutex
	id   int64
	user string
	mail string
	srv  *httptest.Server
}

func newMutableIDP(t *testing.T) *mutableIDP {
	t.Helper()
	m := &mutableIDP{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-access-token"})
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		m.mu.Lock()
		id, user, mail := m.id, m.user, m.mail
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "username": user, "email": mail, "name": "GitLab User",
		})
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mutableIDP) serve(id int64, user, mail string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.id, m.user, m.mail = id, user, mail
}

// TestGitlabSyncAmbiguousLoginIsNotGuessed проходит весь путь: человек входит
// под логином, переименовывается в GitLab, логин занимает другой человек и тоже
// входит — после чего issue назначается на этот логин. Резолв обязан признать
// имя неоднозначным и не назначать никого: человек из GitLab остаётся на
// карточке внешним (display-only) исполнителем — ветка, которая в
// reconcileAssignees уже есть.
func TestGitlabSyncAmbiguousLoginIsNotGuessed(t *testing.T) {
	// Не parallel: configureIDP флипает instance-wide строку провайдера.
	n := userSeq.Add(1)
	login := fmt.Sprintf("gl-dup-%d", n)
	oldID, newID := 950000+int64(n), 960000+int64(n)
	oldEmail := fmt.Sprintf("gl-dup-old-%d@test.local", n)
	newEmail := fmt.Sprintf("gl-dup-new-%d@test.local", n)

	idp := newMutableIDP(t)
	configureIDP(t, idp.srv.URL)

	// Первый вход: логин принадлежит прежнему владельцу.
	idp.serve(oldID, login, oldEmail)
	oldUser := signInID(t, "первый вход")

	// Переименование в GitLab + логин занимает новый человек и входит. Строку
	// прежнего владельца это не трогает — он в Tessera пока не заходил.
	idp.serve(newID, login, newEmail)
	newUser := signInID(t, "вход нового владельца логина")
	if newUser == oldUser {
		t.Fatalf("оба входа попали в одну учётку %s — сценарий не собрался", oldUser)
	}

	// Ровно то состояние, из-за которого находка и существует.
	var rows int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_identities WHERE provider = 'gitlab' AND provider_username = $1`,
		login).Scan(&rows); err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if rows != 2 {
		t.Fatalf("строк с логином %q = %d, want 2 (сценарий не собрался)", login, rows)
	}

	// Оба — участники одного пространства, так что гвард членства обоих пропустит.
	owner := signup(t)
	makeAdmin(t, owner)
	s := mkStack(t, owner)
	inviteByEmail(t, owner, s.WS, oldEmail)
	inviteByEmail(t, owner, s.WS, newEmail)

	f := newFakeGitlab(t, fmt.Sprintf("gl-dup-owner-%d", n), fmt.Sprintf("grp-dup-%d", n))
	f.addIssue(glIssue{IID: 1, Title: "Задача на спорный логин", AssigneeLogins: []string{login}})
	f.members = []glMember{{ID: newID, Username: login, Name: "Новый владелец логина"}}

	connectGitlab(t, owner, f)
	integ := createIntegration(t, owner, s.WS, s.Board, f, nil)
	triggerSync(t, owner, s.WS, integ["id"].(string))
	waitSyncRuns(t, owner, s.WS, 1)

	tasks := owner.get("/boards/" + s.Board + "/tasks").listBody(t)
	task := taskByIid(t, tasks, 1)
	if ids, _ := task["assignee_ids"].([]any); len(ids) != 0 {
		who := "прежний владелец логина " + oldUser
		if len(ids) > 0 && ids[0] == newUser {
			who = "нынешний владелец логина " + newUser
		}
		t.Fatalf("assignee_ids = %v: логин %q резолвился в конкретного человека (%s), "+
			"хотя за ним числятся две живые личности GitLab (#%d и #%d) — выбор сделан планом запроса, а не данными",
			ids, login, who, oldID, newID)
	}

	// Исполнитель issue при этом не исчезает с карточки — остаётся внешним, под
	// своим GitLab-именем. Иначе «фикс» просто терял бы данные issue.
	detail := owner.expect(t, owner.get("/tasks/"+task["id"].(string)), http.StatusOK)
	ext, _ := detail["gitlab_assignees"].([]any)
	if len(ext) != 1 {
		t.Fatalf("gitlab_assignees = %v, want один внешний исполнитель %q", ext, login)
	}
	if row, _ := ext[0].(map[string]any); row["gl_username"] != login {
		t.Fatalf("внешний исполнитель = %v, want gl_username=%q", row, login)
	}
}

// TestGitlabSyncUnambiguousLoginStillResolves — контрвес: одна личность на логин
// резолвится как и раньше. Без него «фикс» мог бы просто перестать назначать
// кого-либо, и первый тест этого не заметил бы.
func TestGitlabSyncUnambiguousLoginStillResolves(t *testing.T) {
	// Не parallel: configureIDP флипает instance-wide строку провайдера.
	n := userSeq.Add(1)
	login := fmt.Sprintf("gl-solo-%d", n)
	glID := 970000 + int64(n)
	email := fmt.Sprintf("gl-solo-%d@test.local", n)

	idp := newMutableIDP(t)
	configureIDP(t, idp.srv.URL)
	idp.serve(glID, login, email)
	member := signInID(t, "вход единственного владельца логина")

	owner := signup(t)
	makeAdmin(t, owner)
	s := mkStack(t, owner)
	inviteByEmail(t, owner, s.WS, email)

	f := newFakeGitlab(t, fmt.Sprintf("gl-solo-owner-%d", n), fmt.Sprintf("grp-solo-%d", n))
	f.addIssue(glIssue{IID: 1, Title: "Задача на однозначный логин", AssigneeLogins: []string{login}})
	f.members = []glMember{{ID: glID, Username: login, Name: "Владелец логина"}}

	connectGitlab(t, owner, f)
	integ := createIntegration(t, owner, s.WS, s.Board, f, nil)
	triggerSync(t, owner, s.WS, integ["id"].(string))
	waitSyncRuns(t, owner, s.WS, 1)

	tasks := owner.get("/boards/" + s.Board + "/tasks").listBody(t)
	task := taskByIid(t, tasks, 1)
	ids, _ := task["assignee_ids"].([]any)
	if len(ids) != 1 || ids[0] != member {
		t.Fatalf("assignee_ids = %v, want [%s]: однозначный логин перестал резолвиться в своего юзера", ids, member)
	}
}

// signInID гоняет OAuth-вход целиком и возвращает uuid учётки, в которую он привёл.
func signInID(t *testing.T, what string) string {
	t.Helper()
	frag := gitlabSignIn(t)
	tok := frag.Get("access_token")
	if tok == "" {
		t.Fatalf("%s не прошёл: %v", what, frag)
	}
	id, _ := whoami(t, tok)["id"].(string)
	if id == "" {
		t.Fatalf("%s не вернул учётку: %v", what, frag)
	}
	return id
}

// inviteByEmail зовёт в пространство по адресу: у заведённых через OAuth
// пользователей нет *client, которого ждёт addMemberTo.
func inviteByEmail(t *testing.T, owner *client, wsID, email string) {
	t.Helper()
	owner.expect(t, owner.post("/workspaces/"+wsID+"/members",
		map[string]any{"email": email}), http.StatusCreated)
}
