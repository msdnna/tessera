package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"tessera/internal/recur"
)

// Месячное/годовое правило несёт серверный якорь дня месяца (`day`, плюс `month`
// для годового). Он существует ровно для одного: короткий месяц ВРЕМЕННО
// сдвигает срок («каждое 31-е» в феврале показано как 28-е), и без якоря
// следующее вычисление взяло бы за основу склампленный день — серия навсегда
// уехала бы на 28-е. Редакторы этот якорь не держат: DueEditor собирает правило
// из своих контролов и `day` не шлёт вовсе, так что перенос якоря со старого
// правила — ЕДИНСТВЕННЫЙ механизм, который серию удерживает.
//
// Условие переноса было «инстант срока побайтово тот же» (`sameTime`). Но день
// календаря и время суток в редакторе — два РАЗНЫХ контрола: рядом с сеткой
// месяца стоит отдельный n-time-picker. Пользователь, открывший февральскую
// карточку и передвинувший напоминание с 09:00 на 10:00, календарь не трогал —
// а якорь переписывался на 28, и «каждое 31-е» тихо и навсегда становилось
// «каждое 28-е». Это ровно тот «unrelated edit», против которого якорь и заведён.
//
// Границу держим по КАЛЕНДАРНОМУ ДНЮ в зоне правила: тот же день — якорь
// сохраняем, другой день — пользователь действительно перенёс серию, якорь
// перевыводим.
func TestAnchorSurvivesTimeOfDayEdit(t *testing.T) {
	const msk = "Europe/Moscow"
	loc, err := time.LoadLocation(msk)
	if err != nil {
		t.Skipf("нет tzdata для %s: %v", msk, err)
	}

	raw := func(s string) *json.RawMessage {
		m := json.RawMessage(s)
		return &m
	}
	at := func(y int, m time.Month, d, hh, mm int) *time.Time {
		v := time.Date(y, m, d, hh, mm, 0, 0, loc)
		return &v
	}
	// То, что реально шлёт клиент: правило из контролов редактора, без `day`.
	fromEditor := func(freq string) *json.RawMessage {
		return raw(`{"freq":"` + freq + `","interval":1,"tz":"` + msk + `"}`)
	}
	anchorOf := func(t *testing.T, stored *json.RawMessage) recur.Rule {
		t.Helper()
		if stored == nil {
			t.Fatalf("правило не сохранилось вовсе")
		}
		r, ok := recur.Parse(stored)
		if !ok {
			t.Fatalf("сохранённое правило не разбирается: %s", string(*stored))
		}
		return r
	}

	t.Run("правка времени суток не трогает якорь 31-го", func(t *testing.T) {
		// Хранимое состояние: «каждое 31-е», февраль склампил срок на 28-е.
		prev := raw(`{"freq":"monthly","interval":1,"day":31,"tz":"` + msk + `"}`)
		prevDue := at(2026, time.February, 28, 9, 0)
		// Пользователь двигает ТОЛЬКО время суток: 09:00 → 10:00.
		reqDue := at(2026, time.February, 28, 10, 0)

		got := anchorOf(t, recurrenceToStore(fromEditor("monthly"), reqDue, prev, prevDue))
		if got.Day != 31 {
			t.Fatalf("якорь = %d, ожидался 31: правка времени суток стёрла день серии", got.Day)
		}
	})

	t.Run("после правки времени серия возвращается на 31-е", func(t *testing.T) {
		prev := raw(`{"freq":"monthly","interval":1,"day":31,"tz":"` + msk + `"}`)
		prevDue := at(2026, time.February, 28, 9, 0)
		reqDue := at(2026, time.February, 28, 10, 0)

		rule := anchorOf(t, recurrenceToStore(fromEditor("monthly"), reqDue, prev, prevDue))
		next, ok := rule.Next(*reqDue)
		if !ok {
			t.Fatalf("Next ok=false")
		}
		if day := next.In(loc).Format("2006-01-02 15:04"); day != "2026-03-31 10:00" {
			t.Fatalf("следующее повторение = %s, ожидалось 2026-03-31 10:00", day)
		}
	})

	t.Run("годовое 29 февраля переживает правку времени", func(t *testing.T) {
		prev := raw(`{"freq":"yearly","interval":1,"day":29,"month":2,"tz":"` + msk + `"}`)
		prevDue := at(2027, time.February, 28, 9, 0) // не високосный — склампило
		reqDue := at(2027, time.February, 28, 10, 0)

		got := anchorOf(t, recurrenceToStore(fromEditor("yearly"), reqDue, prev, prevDue))
		if got.Day != 29 || got.Month != 2 {
			t.Fatalf("якорь = %02d.%02d, ожидался 29.02", got.Day, got.Month)
		}
	})

	// Контроли: перенос серии на другой день по-прежнему перевыводит якорь, а
	// нетронутый срок — по-прежнему его сохраняет.
	t.Run("контроль: смена календарного дня перевыводит якорь", func(t *testing.T) {
		prev := raw(`{"freq":"monthly","interval":1,"day":31,"tz":"` + msk + `"}`)
		prevDue := at(2026, time.February, 28, 9, 0)
		reqDue := at(2026, time.March, 2, 9, 0) // пользователь реально перенёс

		got := anchorOf(t, recurrenceToStore(fromEditor("monthly"), reqDue, prev, prevDue))
		if got.Day != 2 {
			t.Fatalf("якорь = %d, ожидался 2: явный перенос обязан перевести серию", got.Day)
		}
	})

	t.Run("контроль: нетронутый срок сохраняет якорь", func(t *testing.T) {
		prev := raw(`{"freq":"monthly","interval":1,"day":31,"tz":"` + msk + `"}`)
		due := at(2026, time.February, 28, 9, 0)

		got := anchorOf(t, recurrenceToStore(fromEditor("monthly"), due, prev, due))
		if got.Day != 31 {
			t.Fatalf("якорь = %d, ожидался 31", got.Day)
		}
	})

	t.Run("контроль: смена частоты перевыводит якорь", func(t *testing.T) {
		prev := raw(`{"freq":"weekly","interval":1,"tz":"` + msk + `"}`)
		prevDue := at(2026, time.February, 28, 9, 0)
		reqDue := at(2026, time.February, 28, 10, 0)

		got := anchorOf(t, recurrenceToStore(fromEditor("monthly"), reqDue, prev, prevDue))
		if got.Day != 28 {
			t.Fatalf("якорь = %d, ожидался 28: у прошлого правила другая частота", got.Day)
		}
	})

	// День календаря читается в зоне ПРАВИЛА, как и всё остальное в recur: в MSK
	// 23:30 и 00:30 — разные дни, хотя в UTC оба ещё 28-е февраля. Правка времени
	// через полночь — это перенос на другой день, и якорь обязан перевестись.
	t.Run("контроль: время через полночь — это другой день", func(t *testing.T) {
		prev := raw(`{"freq":"monthly","interval":1,"day":31,"tz":"` + msk + `"}`)
		prevDue := at(2026, time.February, 28, 23, 30)
		reqDue := at(2026, time.March, 1, 0, 30)

		got := anchorOf(t, recurrenceToStore(fromEditor("monthly"), reqDue, prev, prevDue))
		if got.Day != 1 {
			t.Fatalf("якорь = %d, ожидался 1: 00:30 MSK — уже следующий день", got.Day)
		}
	})
}
