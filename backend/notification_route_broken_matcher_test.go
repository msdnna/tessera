// matcher и options правила маршрутизации кладутся в JSONB как есть — ручка их
// не разбирает. Разбирает уже routeNotification, и делает это молча:
// `_ = json.Unmarshal(...)`. При ошибке остаётся НУЛЕВОЙ notify.Matcher, а он
// документирован как catch-all. То есть правило, чьё условие сервер прочитать не
// смог, матчит ВСЁ — и список first-match-wins отдаёт ему каждое уведомление
// пользователя. Правило «заглушить упоминания», написанное с `"kinds":"mention"`
// вместо `["mention"]`, принимается с 201 и гасит в том числе будильник (#2932).
package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// brokenMatchers — записи, на которых json.Unmarshal в notify.Matcher падает, а
// значит правило превращается в catch-all. Все три — то, что реально напишет
// рука или скрипт: строка вместо массива, массив вместо объекта, не-uuid.
var brokenMatchers = map[string]string{
	"строка вместо массива kinds": `{"kinds":"mention"}`,
	"массив вместо объекта":       `["mention"]`,
	"workspace_id не uuid":        `{"workspace_id":"домашний"}`,
}

// Находка, половина первая: ручка принимает matcher, который сама же потом не
// прочитает.
func TestBrokenMatcherIsRefusedOnCreate(t *testing.T) {
	t.Parallel()
	c := signup(t)

	for name, raw := range brokenMatchers {
		t.Run(name, func(t *testing.T) {
			r := c.post("/notification-routes", map[string]any{
				"matcher": rawJSON(raw), "channel_ids": []string{},
				"options": map[string]any{"mute": true}, "enabled": true,
			})
			if r.Status != http.StatusBadRequest {
				t.Errorf("matcher %s принят со статусом %d, ожидался 400 — такое правило станет catch-all\n%s",
					raw, r.Status, r.Body)
			}
		})
	}
}

// То же на PATCH: правку правила пишет тот же скрипт.
func TestBrokenMatcherIsRefusedOnPatch(t *testing.T) {
	t.Parallel()
	c := signup(t)

	rt := c.expect(t, c.post("/notification-routes", map[string]any{
		"matcher": map[string]any{"kinds": []string{"mention"}}, "channel_ids": []string{},
		"options": map[string]any{"mute": true}, "enabled": true,
	}), http.StatusCreated)
	rtID := rt["id"].(string)

	r := c.patch("/notification-routes/"+rtID, map[string]any{"matcher": rawJSON(`{"kinds":"mention"}`)})
	if r.Status != http.StatusBadRequest {
		t.Errorf("сломанный matcher принят правкой со статусом %d, ожидался 400\n%s", r.Status, r.Body)
	}
	// И правило не должно было при этом поменяться.
	got := byID(t, c.get("/notification-routes").listBody(t), rtID)
	m, _ := got["matcher"].(map[string]any)
	if kinds, _ := m["kinds"].([]any); len(kinds) != 1 || kinds[0] != "mention" {
		t.Errorf("matcher после отказа = %v, want kinds [mention]", got["matcher"])
	}
}

// Находка, половина вторая — последствие, ради которого всё это: строка,
// написанная в БД до отказа на входе (или чем угодно, что пишет туда JSONB),
// НЕ должна собой подменять весь список. Правило заглушало упоминания; оно не
// вправе глотать напоминание.
func TestBrokenMatcherRuleDoesNotSwallowEverything(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук напоминаний", hook, "")

	// Правило 1 (позиция 1): «заглушить упоминания», написанное с ошибкой.
	mkRawRoute(t, c.UserID, 1, `{"kinds":"mention"}`, `{"mute":true}`, nil)
	// Правило 2 (позиция 2): всё остальное — в вебхук.
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminder(ctx, t, c, "выпить таблетку")

	if n := deliveryCount(t, c.UserID, "reminder"); n == 0 {
		t.Fatalf("напоминание не доехало ни до одного канала: правило про упоминания стало catch-all и заглушило всё")
	}
}

// Контроль A: исправное правило-заглушка по-прежнему глушит свой вид. Иначе
// «пропускать нечитаемое правило» починило бы находку ценой самой фичи mute.
func TestWellFormedMuteRuleStillMutes(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук напоминаний", hook, "")

	mkRawRoute(t, c.UserID, 1, `{"kinds":["reminder"]}`, `{"mute":true}`, nil)
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminder(ctx, t, c, "заглушённое напоминание")

	if n := deliveryCount(t, c.UserID, "reminder"); n != 0 {
		t.Fatalf("строк аутбокса %d, ожидалось 0 — правило mute на вид reminder перестало глушить", n)
	}
}

// Контроль B: исправное УЗКОЕ правило чужого вида по-прежнему пропускает
// напоминание дальше по списку — то есть выше меряется поломка matcher'а, а не
// сломанный сканер/маршрутизатор.
func TestWellFormedNarrowRuleLetsOtherKindsThrough(t *testing.T) {
	ctx := context.Background()
	c := signup(t)
	hook, _ := countingHook(t)
	chID := mkWebhookChannelTmpl(t, c, "Хук напоминаний", hook, "")

	mkRawRoute(t, c.UserID, 1, `{"kinds":["mention"]}`, `{"mute":true}`, nil)
	mkRawRoute(t, c.UserID, 2, `{}`, `{}`, []string{chID})

	fireReminder(ctx, t, c, "живое напоминание")

	if n := deliveryCount(t, c.UserID, "reminder"); n == 0 {
		t.Fatalf("напоминание не доехало ни до одного канала при исправных правилах")
	}
}

// rawJSON протаскивает сырой JSON через сериализацию тела запроса.
type rawJSON string

func (r rawJSON) MarshalJSON() ([]byte, error) { return []byte(r), nil }

// mkRawRoute кладёт правило в обход ручки — так заводится строка, которую
// валидация на входе уже не пустила бы, но которая может лежать в БД.
func mkRawRoute(t *testing.T, userID string, pos float64, matcher, options string, chIDs []string) {
	t.Helper()
	uid, err := uuid.Parse(userID)
	if err != nil {
		t.Fatalf("user id %q: %v", userID, err)
	}
	ids := make([]uuid.UUID, 0, len(chIDs))
	for _, s := range chIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			t.Fatalf("channel id %q: %v", s, err)
		}
		ids = append(ids, id)
	}
	if _, err := testQueries.CreateNotificationRoute(context.Background(), db.CreateNotificationRouteParams{
		UserID: uid, Position: pos, Matcher: []byte(matcher),
		ChannelIds: ids, Options: []byte(options), Enabled: true,
	}); err != nil {
		t.Fatalf("создать правило напрямую: %v", err)
	}
}

// fireReminder заводит просроченное напоминание и делает ровно один тик сканера
// сам — иначе его разберёт воркер харнесса.
func fireReminder(ctx context.Context, t *testing.T, c *client, message string) {
	t.Helper()
	release := holdScannerLock(t)
	c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   message,
	}), http.StatusCreated)
	release()
	newScannerAPI(t).ScanNotificationsOnce(ctx)
}

// deliveryCount — сколько строк аутбокса маршрутизатор завёл на уведомления
// такого вида у пользователя.
func deliveryCount(t *testing.T, userID, kind string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*)
		   FROM notification_deliveries d
		   JOIN notifications n ON n.id = d.notification_id
		  WHERE n.user_id = $1 AND n.kind = $2`, userID, kind).Scan(&n); err != nil {
		t.Fatalf("посчитать строки аутбокса: %v", err)
	}
	return n
}
