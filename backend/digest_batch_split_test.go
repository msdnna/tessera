// Окно дайджеста существует ради одного: вместо потока отдельных сообщений
// прислать один свод. Собирает его drainDeliveries — но только из тех строк,
// которые вернул ClaimPendingDeliveries, а тот берёт «до notifyBatchSize строк
// ORDER BY next_attempt_at», ничего не зная про группы. Группа, не поместившаяся
// в пачку целиком, уезжает двумя сообщениями: часть сейчас, часть следующим
// тиком. И это не редкий случай: слоты лежат на общей для всех пользователей
// сетке (digestSlot округляет до границы окна), так что в один и тот же момент
// созревают группы многих пользователей и пачка режет их по живому (#2932).
package main

import "testing"

// Строк в группе больше, чем notifyBatchSize (20): один пользователь, одно окно,
// бурст уведомлений — свод обязан остаться одним сообщением.
func TestDigestLargerThanClaimBatchStaysOneMessage(t *testing.T) {
	hook, posts := countingHook(t)

	c := signup(t)
	wsID := mkWorkspace(t, c, "Дайджест: группа больше пачки")
	chID := mkWebhookChannelTmpl(t, c, "Хук большого свода", hook, "")

	texts := make([]string, 0, 25)
	for i := 0; i < 25; i++ {
		texts = append(texts, "уведомление бурста")
	}
	ids := pendingDeliveries(t, c.UserID, wsID, chID, "дайджест:большой-слот", texts...)
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })

	if got := posts(); got != 1 {
		t.Fatalf("свод из %d строк ушёл %d сообщением(ями), want 1: пачка аутбокса разрезала группу\n%s",
			len(ids), got, dumpDeliveries(t))
	}
	for i, id := range ids {
		if st, lastErr := deliveryOutcome(t, id); st != "sent" {
			t.Errorf("строка %d свода: статус %q (%s), want sent", i+1, st, lastErr)
		}
	}
}

// Тот же разрез, но в реальной форме: у каждого пользователя группа сама по себе
// маленькая, а созревают они одновременно — слот считается от границы окна, а не
// от момента уведомления. Вдвоём строки не влезают в пачку, и чей-то свод
// приезжает двумя письмами.
func TestDigestsOfTwoUsersDoNotSplitEachOther(t *testing.T) {
	hookA, postsA := countingHook(t)
	hookB, postsB := countingHook(t)

	a := signup(t)
	wsA := mkWorkspace(t, a, "Дайджест: сосед по слоту A")
	chA := mkWebhookChannelTmpl(t, a, "Хук A", hookA, "")

	b := signup(t)
	wsB := mkWorkspace(t, b, "Дайджест: сосед по слоту B")
	chB := mkWebhookChannelTmpl(t, b, "Хук B", hookB, "")

	texts := make([]string, 0, 11)
	for i := 0; i < 11; i++ {
		texts = append(texts, "уведомление слота")
	}
	idsA := pendingDeliveries(t, a.UserID, wsA, chA, "дайджест:общий-слот-A", texts...)
	idsB := pendingDeliveries(t, b.UserID, wsB, chB, "дайджест:общий-слот-B", texts...)

	all := append(append([]string{}, idsA...), idsB...)
	drainDeliveriesUntil(t, func() bool { return allSettled(t, all) })

	if got := postsA(); got != 1 {
		t.Errorf("свод A ушёл %d сообщением(ями), want 1\n%s", got, dumpDeliveries(t))
	}
	if got := postsB(); got != 1 {
		t.Errorf("свод B ушёл %d сообщением(ями), want 1\n%s", got, dumpDeliveries(t))
	}
}

// Контроль: одиночные строки (без группы) пачка по-прежнему разбирает и
// доставляет каждую отдельным сообщением — claim по группам не должен их
// склеивать.
func TestUngroupedDeliveriesStayIndividual(t *testing.T) {
	hook, posts := countingHook(t)

	c := signup(t)
	wsID := mkWorkspace(t, c, "Без дайджеста: три отдельных")
	chID := mkWebhookChannelTmpl(t, c, "Хук одиночек", hook, "")

	ids := pendingDeliveries(t, c.UserID, wsID, chID, "", "первое", "второе", "третье")
	drainDeliveriesUntil(t, func() bool { return allSettled(t, ids) })

	if got := posts(); got != 3 {
		t.Fatalf("три строки без дайджеста ушли %d сообщением(ями), want 3\n%s", got, dumpDeliveries(t))
	}
	for i, id := range ids {
		if st, _ := deliveryOutcome(t, id); st != "sent" {
			t.Errorf("строка %d: статус %q, want sent", i+1, st)
		}
	}
}
