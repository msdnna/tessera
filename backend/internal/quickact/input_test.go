package quickact

import (
	"testing"
	"time"
)

// Даты пишут руками, а значит — без ведущих нулей. Go-шные layout'ы "02" и "01"
// фиксированной ширины, поэтому «1.9» раньше доходило до пользователя как
// «не понимаю дату», хотя «01.09» работало.
func TestParseDateAcceptsUnpaddedNumbers(t *testing.T) {
	now := time.Date(2026, 9, 20, 15, 4, 5, 0, time.UTC)
	cases := []struct {
		in   string
		want string
	}{
		{"1.9", "2026-09-01"},
		{"14.8", "2026-08-14"},
		{"1.9.2026", "2026-09-01"},
		{"4/9/2026", "2026-09-04"},
		{"2026-8-14", "2026-08-14"},
		// Контроль: форматы, работавшие и раньше, читаются так же.
		{"2026-08-14", "2026-08-14"},
		{"14.08.2026", "2026-08-14"},
		{"14.08", "2026-08-14"},
		{"04/09/2026", "2026-09-04"},
	}
	for _, c := range cases {
		got, err := ParseDate(c.in, now)
		if err != nil {
			t.Errorf("ParseDate(%q) = ошибка %v, хотели %s", c.in, err, c.want)
			continue
		}
		if got.Format("2006-01-02") != c.want {
			t.Errorf("ParseDate(%q) = %s, хотели %s", c.in, got.Format("2006-01-02"), c.want)
		}
		if h, m, s := got.Clock(); h != 0 || m != 0 || s != 0 {
			t.Errorf("ParseDate(%q) = %s, хотели полночь", c.in, got)
		}
		if got.Location() != now.Location() {
			t.Errorf("ParseDate(%q) в зоне %v, хотели %v", c.in, got.Location(), now.Location())
		}
	}
}

// Дополнение нулями не должно превратить невозможную дату в соседнюю: валидацию
// по-прежнему делает time.ParseInLocation, а не наша арифметика.
func TestParseDateStillRejectsImpossibleDays(t *testing.T) {
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	for _, in := range []string{"31.02", "31.2.2026", "2026-13-01", "32.1.2026", "0.9", "1.0", "не дата", "14.08.26"} {
		if got, err := ParseDate(in, now); err == nil {
			t.Errorf("ParseDate(%q) = %s, хотели ошибку", in, got.Format("2006-01-02"))
		}
	}
}

// Длинное число — это не «первые девять цифр и остаток». Бескрайний `\d{1,9}`
// резал вставленный id на две ссылки, и вторая попадала в задачу №1, которая
// есть в каждом пространстве.
func TestParseRefsIgnoresOverlongNumbers(t *testing.T) {
	if refs, err := ParseRefs("#12345678901"); err == nil {
		t.Errorf("ParseRefs(#12345678901) = %v, хотели ошибку", refs)
	}
	if refs, err := ParseRefs("12345678901 12345678902"); err == nil {
		t.Errorf("ParseRefs(два длинных) = %v, хотели ошибку", refs)
	}
	// Длинное число рядом с настоящим номером не должно рождать лишних ссылок.
	refs, err := ParseRefs("#2591 #12345678901")
	if err != nil {
		t.Fatalf("ParseRefs = ошибка %v", err)
	}
	if len(refs) != 1 || refs[0] != 2591 {
		t.Errorf("ParseRefs = %v, хотели [2591]", refs)
	}
}

// Контроль: обычные ссылки читаются как читались.
func TestParseRefsStillReadsOrdinaryNumbers(t *testing.T) {
	refs, err := ParseRefs("#2591, 2592 #2591")
	if err != nil {
		t.Fatalf("ParseRefs = ошибка %v", err)
	}
	if len(refs) != 2 || refs[0] != 2591 || refs[1] != 2592 {
		t.Errorf("ParseRefs = %v, хотели [2591 2592]", refs)
	}
	if refs, err := ParseRefs("999999999"); err != nil || len(refs) != 1 || refs[0] != 999999999 {
		t.Errorf("ParseRefs(999999999) = %v, %v — девять цифр всё ещё номер", refs, err)
	}
}

// Словарь воркспейса принимает ключ, начинающийся с цифры (commandKeyRe), и
// отказ прямо обещает «латиница, цифры». Парсер обязан такой ключ видеть — иначе
// превью молчит о команде, которую сам же показал в попапе.
func TestParseRecognisesDigitLedCustomKey(t *testing.T) {
	res := Parse("/1on1 с Петей", []string{"1on1"})
	if len(res.Custom) != 1 || res.Custom[0].Key != "1on1" {
		t.Fatalf("Custom = %+v, хотели одну команду 1on1", res.Custom)
	}
	if res.Custom[0].Arg != "с Петей" {
		t.Errorf("Arg = %q, хотели «с Петей»", res.Custom[0].Arg)
	}
	// Словарная команда остаётся в тексте — это её единственная работа.
	if res.Rest != "/1on1 с Петей" {
		t.Errorf("Rest = %q, хотели строку целиком", res.Rest)
	}
}

// Контроль: строка с цифрой, которой нет в словаре, по-прежнему просто текст.
func TestParseLeavesUnknownDigitLedLineAsText(t *testing.T) {
	res := Parse("/2 часа на это", nil)
	if len(res.Cmds) != 0 || len(res.Custom) != 0 {
		t.Fatalf("Cmds=%+v Custom=%+v, хотели пусто", res.Cmds, res.Custom)
	}
	if res.Rest != "/2 часа на это" {
		t.Errorf("Rest = %q, хотели строку целиком", res.Rest)
	}
}
