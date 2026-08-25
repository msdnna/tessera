package handlers

import (
	"encoding/json"
	"testing"

	"tessera/internal/db"
)

// notifySentence is the seam between a stored row and the outbound wording: the
// payload wins when it is there, the legacy Russian text when it isn't.
func TestNotifySentence(t *testing.T) {
	legacy := "Алиса назначил вам задачу #42 «Починить чайник»"
	payload := func(v map[string]any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}

	cases := []struct {
		name string
		n    db.Notification
		lang string
		want string
	}{
		{
			name: "payload renders in the reader's language",
			n: db.Notification{Kind: "assigned", Text: legacy, Payload: payload(map[string]any{
				"event": evAssigned, "actor": "Алиса", "task_number": 42, "title": "Починить чайник",
			})},
			lang: "en",
			want: "Алиса assigned task #42 to you “Починить чайник”",
		},
		{
			name: "russian reader gets the russian sentence",
			n: db.Notification{Kind: "assigned", Text: legacy, Payload: payload(map[string]any{
				"event": evAssigned, "actor": "Алиса", "task_number": 42, "title": "Починить чайник",
			})},
			lang: "ru",
			want: legacy,
		},
		{
			name: "pre-0065 row keeps its stored text",
			n:    db.Notification{Kind: "assigned", Text: legacy},
			lang: "en",
			want: legacy,
		},
		{
			name: "empty payload object keeps its stored text",
			n:    db.Notification{Kind: "assigned", Text: legacy, Payload: json.RawMessage(`{}`)},
			lang: "en",
			want: legacy,
		},
		{
			name: "unreadable payload never loses the notification",
			n:    db.Notification{Kind: "assigned", Text: legacy, Payload: json.RawMessage(`{oops`)},
			lang: "en",
			want: legacy,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := notifySentence(c.n, c.lang); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}
