package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/notify"
)

// The v2 notifications (#3048) carry what each client needs to word them and
// to open what they are about, and the catalogue for external deliveries words
// them in both languages.
func TestV2NotificationPayloads(t *testing.T) {
	ms := db.Milestone{ID: uuid.New(), ProjectID: uuid.New(), Title: " Спринт 41 "}
	closed := msgMilestoneClosed("Илья", ms, db.MilestoneCloseSummaryRow{
		ProjectName: "Tessera Web", ProjectSlug: "tessera-web", TaskCount: 22, DoneCount: 19,
	})
	p := decode(t, closed)
	if p["event"] != evMilestoneClosed || p["title"] != "Спринт 41" || p["done"] != float64(19) ||
		p["total"] != float64(22) || p["project_slug"] != "tessera-web" || p["milestone_id"] != ms.ID.String() {
		t.Fatalf("milestone_closed payload = %v", p)
	}
	if got := notify.Sentence(p, "ru", ""); got != "Илья закрыл(а) этап «Спринт 41» — выполнено 19 из 22" {
		t.Fatalf("ru = %q", got)
	}
	if got := notify.Sentence(p, "en", ""); got != "Илья closed stage “Спринт 41” — 19 of 22 done" {
		t.Fatalf("en = %q", got)
	}
	link := notifyLink("https://app", db.Notification{Payload: closed.json()})
	if link != "https://app/project/tessera-web/stage/"+ms.ID.String() {
		t.Fatalf("stage link = %q", link)
	}

	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	soon := decode(t, msgConferenceStarting("Синк", "c-1", at, 5))
	if soon["conference_id"] != "c-1" || soon["minutes"] != float64(5) || soon["scheduled_at"] != "2026-10-21T12:00:00Z" {
		t.Fatalf("conference_starting payload = %v", soon)
	}
	if got := notify.Sentence(soon, "ru", ""); got != "Через 5 мин начнётся встреча «Синк»" {
		t.Fatalf("ru = %q", got)
	}

	n := int64(418)
	conflict := decode(t, msgSyncConflict("gitlab", "GitLab", &n, "Экспорт", []string{"description", "state"}))
	if got := notify.Sentence(conflict, "ru", ""); got != "GitLab · конфликт синхронизации в #418 (описание и статус): изменено в обоих местах" {
		t.Fatalf("ru = %q", got)
	}
	if conflict["provider"] != "gitlab" {
		t.Fatalf("provider = %v", conflict["provider"])
	}

	doc := db.Document{ID: uuid.New(), Slug: "plan", Title: "План релиза"}
	dm := msgDocMention("Анна", doc, "@Мария глянь")
	dp := decode(t, dm)
	if dp["name"] != "План релиза" || dp["quote"] != "@Мария глянь" || dp["document_slug"] != "plan" {
		t.Fatalf("document_mention payload = %v", dp)
	}
	if got := notify.Sentence(dp, "ru", ""); got != "Анна упомянул(а) вас в документе «План релиза» «@Мария глянь»" {
		t.Fatalf("ru = %q", got)
	}
	if got := notifyLink("https://app/", db.Notification{Payload: dm.json()}); got != "https://app/documents/plan" {
		t.Fatalf("document link = %q", got)
	}

	conf := db.Conference{ID: uuid.New(), Title: "Стендап"}
	cm := decode(t, msgConfMention("Анна", conf, "ок"))
	if got := notify.Sentence(cm, "en", ""); got != "Анна mentioned you in the chat of “Стендап” “ок”" {
		t.Fatalf("en = %q", got)
	}
}

func TestQuietExemptOnlyWhenAsked(t *testing.T) {
	off := db.NotificationPref{}
	on := db.NotificationPref{QuietAllowUrgent: true}
	if quietExempt(off, kindMention, 4) {
		t.Fatal("exempt with the switch off")
	}
	for _, c := range []struct {
		kind string
		prio int32
		want bool
	}{
		{kindMention, 0, true},
		{kindConferenceSoon, 0, true},
		{"assigned", urgentPriority, true},
		{"assigned", 3, false},
		{kindReminder, 0, false},
	} {
		if got := quietExempt(on, c.kind, c.prio); got != c.want {
			t.Errorf("quietExempt(%s, %d) = %v, want %v", c.kind, c.prio, got, c.want)
		}
	}
}
