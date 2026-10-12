package handlers

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Тихие часы 22:00–08:00 UTC — тот же профиль, на котором проверялись остальные
// сюжеты окна дайджеста.
func quietNight() quietFn {
	return func(t time.Time) (time.Time, bool) {
		mins := t.Hour()*60 + t.Minute()
		if mins < 1320 && mins >= 480 {
			return time.Time{}, false
		}
		end := time.Date(t.Year(), t.Month(), t.Day(), 0, 480, 0, 0, time.UTC)
		if !end.After(t) {
			end = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 480, 0, 0, time.UTC)
		}
		return end, true
	}
}

// digest_minutes приходит как int32 и до фикса складывался в БД с одним лишь
// max(0, …). Окно, которого ни один клиент не предлагает, уходило прямо в
// `time.Duration(dm) * time.Minute`, и дальше арифметика ломалась двумя разными
// способами: большое-но-валидное значение уносит слот на десятилетия вперёд
// (уведомление не отложено, а проглочено), а с 153722868 умножение переполняет
// int64, окно становится ОТРИЦАТЕЛЬНЫМ — Truncate для неположительной
// длительности ничего не делает, Add идёт назад, и слот падает в 2017 год.
// Строка, созревшая в прошлом, забирается первым же тиком: доставка уходит
// немедленно, выбрасывая якорь тихих часов, который эта же функция только что
// поставила (#2932).
func TestDigestWindowIsBounded(t *testing.T) {
	now := time.Date(2026, 9, 20, 23, 0, 0, 0, time.UTC) // внутри объявленной тишины
	quiet := quietNight()
	ch := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	morning := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)

	// Находка: слот обязан остаться в будущем и вне тишины при любом сохранённом
	// значении. int32-максимум — переполнение, 1e9 — «год 2215», 100000 — семь
	// недель; все три сегодня принимаются ручкой prefs.
	for _, dm := range []int32{100000, 153722868, 1000000000, 2147483647} {
		next, group := digestSlot(now, quiet, dm, ch)
		if next.Before(now) {
			t.Errorf("digest_minutes=%d: слот %s в ПРОШЛОМ (сейчас %s) — строка уйдёт немедленно, "+
				"в тихие часы", dm, next.Format(time.RFC3339), now.Format(time.RFC3339))
			continue
		}
		if _, inQuiet := quiet(next); inQuiet {
			t.Errorf("digest_minutes=%d: слот %s внутри объявленной тишины 22:00–08:00",
				dm, next.Format(time.RFC3339))
		}
		// Отложить дольше суток окно дайджеста не имеет права: это уже не пачка,
		// а потерянное уведомление.
		if next.After(morning.Add(maxDigestMinutes * time.Minute)) {
			t.Errorf("digest_minutes=%d: слот %s дальше суток от конца тишины — уведомление проглочено",
				dm, next.Format(time.RFC3339))
		}
		if group == "" {
			t.Errorf("digest_minutes=%d: группа свода пуста — строка перестала батчиться", dm)
		}
	}

	// Контроль: настоящее окно считается в точности как раньше. Приход в 23:00
	// внутри тишины → якорь 08:00, часовая сетка → 09:00, ключ группы несёт этот
	// момент.
	next, group := digestSlot(now, quiet, 60, ch)
	want := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("часовое окно: слот %s, want %s", next.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if group != ch.String()+":"+strconv.FormatInt(want.Unix(), 10) {
		t.Errorf("часовое окно: ключ группы %q не несёт момент слота", group)
	}

	// Контроль: суточный свод остаётся достижимым — граница проходит по нему, а
	// не под ним.
	if next, _ := digestSlot(now, quiet, maxDigestMinutes, ch); next.Before(now) {
		t.Errorf("суточное окно: слот %s в прошлом", next.Format(time.RFC3339))
	}

	// Контроль: выключенный дайджест по-прежнему отдаёт голый якорь без группы.
	if next, group := digestSlot(now, quiet, 0, ch); !next.Equal(morning) || group != "" {
		t.Errorf("выключенный дайджест: слот %s группа %q, want %s и пусто",
			next.Format(time.RFC3339), group, morning.Format(time.RFC3339))
	}
}
