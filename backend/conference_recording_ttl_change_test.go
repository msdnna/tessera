package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// Меняем срок хранения записей у конференции, где записи уже есть (#2932).
//
// `recording_ttl_days` — это политика хранения конференции, и подсказка под полем
// обещает именно её: «записи этой конференции удалятся автоматически через
// столько дней». Но дата смерти файла (`expires_at`) ставится один-единственный
// раз — в момент, когда запись завершилась, — и больше не пересматривается.
// Поэтому смена срока действовала только на записи, которых ещё нет:
//
//   - укоротили срок (нужно, чтобы старое перестало лежать) — уже снятое лежит
//     дальше по старой, длинной дате, и в списке так и написано «удалится через
//     месяц»;
//   - удлинили срок (нужно, чтобы запись дожила до разбора) — она всё равно
//     исчезнет завтра.
//
// Свипер про это знает больше, чем handler: его собственный комментарий бережёт
// воркер от «тысячи файлов на первом прогоне ПОСЛЕ смены TTL» — прогона, которого
// при неперештампованных строках не бывает вовсе.

// finishedRecording кладёт конференции завершённую запись так, как её оставляет
// реальный конец эгресса: через тот же FinishConferenceRecording, то есть с
// expires_at, посчитанным от действовавшего на тот момент срока.
func finishedRecording(t *testing.T, confID uuid.UUID, by uuid.UUID, ttlDays int32) db.ConferenceRecording {
	t.Helper()
	ctx := context.Background()
	rec, err := testQueries.CreateConferenceRecording(ctx, db.CreateConferenceRecordingParams{
		ConferenceID: confID,
		EgressID:     "EG_" + uuid.NewString(),
		FilePath:     "rec/" + confID.String() + "/" + uuid.NewString() + ".mp4",
		FileName:     "rec-2026-09-21-0100.mp4",
		StartedBy:    &by,
	})
	if err != nil {
		t.Fatalf("create recording: %v", err)
	}
	done, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID: rec.ID, Status: "completed", SizeBytes: 1024, DurationSec: 42, TtlDays: ttlDays,
	})
	if err != nil {
		t.Fatalf("finish recording: %v", err)
	}
	return done
}

// recordingByID вытаскивает запись из списка конференции — ровно то, что видит
// экран: строку с датой «удалится …».
func recordingByID(t *testing.T, c *client, confID, recID string) map[string]any {
	t.Helper()
	list := c.get("/conferences/" + confID + "/recordings")
	if list.Status != http.StatusOK {
		t.Fatalf("list recordings: status %d\n%s", list.Status, list.Body)
	}
	for _, r := range list.listBody(t) {
		if r["id"] == recID {
			return r
		}
	}
	t.Fatalf("запись %s пропала из списка конференции", recID)
	return nil
}

func expiresAt(t *testing.T, rec map[string]any) *time.Time {
	t.Helper()
	raw, ok := rec["expires_at"].(string)
	if !ok || raw == "" {
		return nil
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatalf("parse expires_at %q: %v", raw, err)
	}
	return &at
}

// patchTTL меняет срок хранения записей конференции. PATCH здесь full-replace по
// названию, поэтому оно передаётся вместе с новым сроком.
func patchTTL(t *testing.T, c *client, confID, title string, ttl int) {
	t.Helper()
	c.expect(t, c.patch("/conferences/"+confID, map[string]any{
		"title": title, "recording_ttl_days": ttl,
	}), http.StatusOK)
}

// TestRecordingTTLShortenedAppliesToExistingRecordings — укоротили срок хранения,
// и уже снятая запись должна умереть по новому сроку, а не по тому, что
// действовал в день съёмки.
func TestRecordingTTLShortenedAppliesToExistingRecordings(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Летучка", "recording_ttl_days": 30}), http.StatusCreated)
	confID := conf["id"].(string)

	rec := finishedRecording(t, mustUUID(t, confID), mustUUID(t, owner.UserID), 30)
	if rec.ExpiresAt == nil {
		t.Fatalf("запись со сроком 30 дней завершилась без expires_at")
	}

	patchTTL(t, owner, confID, "Летучка", 1)

	got := expiresAt(t, recordingByID(t, owner, confID, rec.ID.String()))
	if got == nil {
		t.Fatalf("после укорочения срока запись стала бессрочной")
	}
	// Срок считается от конца записи, а не от момента правки: «хранить сутки»
	// про запись вчерашнего дня означает, что её уже пора убрать.
	want := rec.EndedAt.Add(24 * time.Hour)
	if got.Sub(want).Abs() > time.Minute {
		t.Fatalf("expires_at = %s, want ≈ %s (конец записи + новый срок); старая дата была %s",
			got.Format(time.RFC3339), want.Format(time.RFC3339), rec.ExpiresAt.Format(time.RFC3339))
	}
}

// TestRecordingTTLExtendedAppliesToExistingRecordings — обратная сторона: срок
// продлили, и запись, которой оставалось меньше суток, обязана дожить.
func TestRecordingTTLExtendedAppliesToExistingRecordings(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Разбор", "recording_ttl_days": 1}), http.StatusCreated)
	confID := conf["id"].(string)

	rec := finishedRecording(t, mustUUID(t, confID), mustUUID(t, owner.UserID), 1)

	patchTTL(t, owner, confID, "Разбор", 90)

	got := expiresAt(t, recordingByID(t, owner, confID, rec.ID.String()))
	if got == nil {
		t.Fatalf("после продления срока запись стала бессрочной")
	}
	want := rec.EndedAt.Add(90 * 24 * time.Hour)
	if got.Sub(want).Abs() > time.Minute {
		t.Fatalf("expires_at = %s, want ≈ %s (конец записи + новый срок)",
			got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

// TestRecordingTTLZeroKeepsExistingRecordingsForever — «0 — хранить без срока»
// это решение о записях конференции, а не только о будущих.
func TestRecordingTTLZeroKeepsExistingRecordingsForever(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Демо", "recording_ttl_days": 7}), http.StatusCreated)
	confID := conf["id"].(string)

	rec := finishedRecording(t, mustUUID(t, confID), mustUUID(t, owner.UserID), 7)

	patchTTL(t, owner, confID, "Демо", 0)

	if got := expiresAt(t, recordingByID(t, owner, confID, rec.ID.String())); got != nil {
		t.Fatalf("после «хранить без срока» у записи осталась дата удаления %s", got.Format(time.RFC3339))
	}
}

// TestRecordingTTLChangeLeavesRunningRecordingAlone — запись, которая прямо
// сейчас пишется, срок получает в момент завершения и по действующему тогда
// правилу. Перештамповка не должна лезть в активную строку: там ещё нет
// ended_at, от которого считается дата, а сама она — та самая строка, которой
// свипер обязан не касаться, пока файл пишется.
func TestRecordingTTLChangeLeavesRunningRecordingAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := signup(t)
	s := mkStack(t, owner)
	conf := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Эфир", "recording_ttl_days": 30}), http.StatusCreated)
	confID := conf["id"].(string)

	by := mustUUID(t, owner.UserID)
	rec, err := testQueries.CreateConferenceRecording(ctx, db.CreateConferenceRecordingParams{
		ConferenceID: mustUUID(t, confID),
		EgressID:     "EG_" + uuid.NewString(),
		FilePath:     "rec/" + confID + "/" + uuid.NewString() + ".mp4",
		FileName:     "rec-live.mp4",
		StartedBy:    &by,
	})
	if err != nil {
		t.Fatalf("create recording: %v", err)
	}

	patchTTL(t, owner, confID, "Эфир", 1)

	live := recordingByID(t, owner, confID, rec.ID.String())
	if live["status"] != "active" {
		t.Fatalf("активная запись стала %q", live["status"])
	}
	if got := expiresAt(t, live); got != nil {
		t.Fatalf("незавершённой записи проставили дату удаления %s", got.Format(time.RFC3339))
	}

	// И закрывается она уже по новому сроку — TTL читается на закрытии.
	done, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID: rec.ID, Status: "completed", SizeBytes: 1, DurationSec: 1, TtlDays: 1,
	})
	if err != nil {
		t.Fatalf("finish recording: %v", err)
	}
	if done.ExpiresAt == nil {
		t.Fatalf("завершённая запись осталась без expires_at")
	}
}
