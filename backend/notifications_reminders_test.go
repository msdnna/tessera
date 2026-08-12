// Notifications (in-app feed + read state), the notification router (channels /
// routes / prefs / per-task due overrides) and personal reminders.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestRegisterDeviceChannelPushToken covers device registration around the FCM
// token: it round-trips into the channel config, a re-registration that carries
// no token keeps the stored one (a client that couldn't reach Play Services must
// not silently disable background push), and a rotated token replaces it.
func TestRegisterDeviceChannelPushToken(t *testing.T) {
	t.Parallel()
	c := signup(t)

	cfgOf := func(t *testing.T, v map[string]any) map[string]any {
		t.Helper()
		cfg, ok := v["config"].(map[string]any)
		if !ok {
			t.Fatalf("channel has no config object: %v", v)
		}
		return cfg
	}

	// First registration with a push token.
	ch := c.expect(t, c.post("/notification-devices", map[string]any{
		"device_id": "dev-1", "label": "Пиксель", "platform": "android", "fcm_token": "tok-1",
	}), http.StatusCreated)
	if cfg := cfgOf(t, ch); cfg["fcm_token"] != "tok-1" || cfg["device_id"] != "dev-1" {
		t.Fatalf("register with token: %v", cfg)
	}

	// Re-registration without a token (no Play Services this start) keeps it.
	ch = c.expect(t, c.post("/notification-devices", map[string]any{
		"device_id": "dev-1", "platform": "android",
	}), http.StatusOK)
	if cfg := cfgOf(t, ch); cfg["fcm_token"] != "tok-1" {
		t.Fatalf("re-register without token must keep it: %v", cfg)
	}

	// A rotated token overwrites the stored one.
	ch = c.expect(t, c.post("/notification-devices", map[string]any{
		"device_id": "dev-1", "platform": "android", "fcm_token": "tok-2",
	}), http.StatusOK)
	if cfg := cfgOf(t, ch); cfg["fcm_token"] != "tok-2" {
		t.Fatalf("rotated token: %v", cfg)
	}

	// A device that never had a token (browser/desktop) stores none — the sender
	// treats an empty token as permanently undeliverable.
	web := c.expect(t, c.post("/notification-devices", map[string]any{
		"device_id": "dev-web", "platform": "web",
	}), http.StatusCreated)
	if _, ok := cfgOf(t, web)["fcm_token"]; ok {
		t.Fatalf("web device should carry no fcm_token: %v", web)
	}
}

// In-app notifications: an assignment and a comment produce feed entries for the
// affected users; read / read-all maintain the unread count.
func TestNotificationsFeed(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	member := signup(t)
	s := mkStack(t, owner)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "Заметная задача")
	id := task["id"].(string)
	owner.expect(t, owner.post("/workspaces/"+s.WS+"/members", map[string]any{"email": member.Email}), http.StatusCreated)

	// Owner assigns the member → the member gets an "assigned" notification.
	if r := owner.post("/tasks/"+id+"/assignees", map[string]any{"user_id": member.UserID}); r.Status != http.StatusNoContent {
		t.Fatalf("assign: %d\n%s", r.Status, r.Body)
	}
	feed := member.get("/notifications").listBody(t)
	if len(feed) != 1 || feed[0]["kind"] != "assigned" || feed[0]["task_id"] != id {
		t.Fatalf("member feed after assign: %v", feed)
	}
	notifID := feed[0]["id"].(string)
	cnt := member.expect(t, member.get("/notifications/unread-count"), http.StatusOK)
	if cnt["count"] != float64(1) {
		t.Fatalf("unread count: %v", cnt)
	}

	// The member comments → the creator (owner) gets a "comment" notification;
	// the actor never notifies themselves.
	member.expect(t, member.post("/tasks/"+id+"/comments", map[string]any{"body": "Принято в работу"}), http.StatusCreated)
	ownerFeed := owner.get("/notifications").listBody(t)
	if len(ownerFeed) != 1 || ownerFeed[0]["kind"] != "comment" || ownerFeed[0]["task_id"] != id {
		t.Fatalf("owner feed after comment: %v", ownerFeed)
	}

	// Read one → count drops; read-all → zero.
	if r := member.post("/notifications/"+notifID+"/read", nil); r.Status != http.StatusNoContent {
		t.Fatalf("mark read: %d\n%s", r.Status, r.Body)
	}
	// (the member also received nothing from their own comment)
	cnt = member.expect(t, member.get("/notifications/unread-count"), http.StatusOK)
	if cnt["count"] != float64(0) {
		t.Fatalf("unread after read: %v", cnt)
	}
	if r := owner.post("/notifications/read-all", nil); r.Status != http.StatusNoContent {
		t.Fatalf("read-all: %d\n%s", r.Status, r.Body)
	}
	cnt = owner.expect(t, owner.get("/notifications/unread-count"), http.StatusOK)
	if cnt["count"] != float64(0) {
		t.Fatalf("unread after read-all: %v", cnt)
	}
}

// Delivery channels: webhook CRUD + the synchronous /test send against a local
// HTTP server (which must receive the sample POST; success flips `verified`).
func TestNotificationChannels(t *testing.T) {
	t.Parallel()
	c := signup(t)

	var mu sync.Mutex
	var bodies [][]byte
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		mu.Lock()
		if r.Method == http.MethodPost {
			bodies = append(bodies, b)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	// Create.
	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": "Мой хук", "config": map[string]any{"url": hook.URL},
	}), http.StatusCreated)
	chID := ch["id"].(string)
	if ch["type"] != "webhook" || ch["enabled"] != true || ch["verified"] != false || ch["has_secret"] != false {
		t.Fatalf("channel create: %v", ch)
	}
	// Config without a url / unknown type → 400.
	if r := c.post("/notification-channels", map[string]any{"type": "webhook", "label": "Без url"}); r.Status != http.StatusBadRequest {
		t.Fatalf("webhook without url: %d", r.Status)
	}
	if r := c.post("/notification-channels", map[string]any{"type": "pigeon"}); r.Status != http.StatusBadRequest {
		t.Fatalf("unknown type: %d", r.Status)
	}

	// Test-send: the hook receives one POST with the sample message.
	res := c.expect(t, c.post("/notification-channels/"+chID+"/test", nil), http.StatusOK)
	if res["ok"] != true {
		t.Fatalf("channel test: %v", res)
	}
	mu.Lock()
	got := len(bodies)
	var payload map[string]any
	if got > 0 {
		_ = json.Unmarshal(bodies[0], &payload)
	}
	mu.Unlock()
	if got != 1 || payload["kind"] != "test" || payload["title"] != "Проверка канала" {
		t.Fatalf("webhook received %d posts, payload %v", got, payload)
	}
	// A successful test marks the channel verified.
	list := c.get("/notification-channels").listBody(t)
	if v := byID(t, list, chID); v["verified"] != true {
		t.Fatalf("channel not verified after test: %v", v)
	}

	// Update (full-ish replace: config must be re-sent — omitting it fails
	// validation for the stored type).
	up := c.expect(t, c.patch("/notification-channels/"+chID, map[string]any{
		"label": "Хук v2", "config": map[string]any{"url": hook.URL}, "enabled": false,
	}), http.StatusOK)
	if up["label"] != "Хук v2" || up["enabled"] != false {
		t.Fatalf("channel update: %v", up)
	}
	if r := c.patch("/notification-channels/"+chID, map[string]any{"label": "x"}); r.Status != http.StatusBadRequest {
		t.Fatalf("update without config url: %d\n%s", r.Status, r.Body)
	}

	// Secret erase: set an auth_header, then clear_secret wipes it (an empty
	// secret map alone would keep the stored one).
	up = c.expect(t, c.patch("/notification-channels/"+chID, map[string]any{
		"config": map[string]any{"url": hook.URL}, "secret": map[string]any{"auth_header": "Bearer sekret"},
	}), http.StatusOK)
	if up["has_secret"] != true {
		t.Fatalf("set auth_header: %v", up)
	}
	up = c.expect(t, c.patch("/notification-channels/"+chID, map[string]any{
		"config": map[string]any{"url": hook.URL}, "clear_secret": true,
	}), http.StatusOK)
	if up["has_secret"] != false {
		t.Fatalf("clear webhook secret: %v", up)
	}

	// A channel whose secret is required (telegram) rejects clear_secret with 400,
	// and the stored secret survives.
	tg := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "telegram", "label": "Тг", "config": map[string]any{"chat_id": "42"},
		"secret": map[string]any{"bot_token": "123:ABC"},
	}), http.StatusCreated)
	tgID := tg["id"].(string)
	if r := c.patch("/notification-channels/"+tgID, map[string]any{
		"config": map[string]any{"chat_id": "42"}, "clear_secret": true,
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("clear required secret must 400: %d\n%s", r.Status, r.Body)
	}
	if v := byID(t, c.get("/notification-channels").listBody(t), tgID); v["has_secret"] != true {
		t.Fatalf("telegram secret must survive rejected clear: %v", v)
	}
	if r := c.del("/notification-channels/" + tgID); r.Status != http.StatusNoContent {
		t.Fatalf("delete telegram channel: %d\n%s", r.Status, r.Body)
	}

	// Delete.
	if r := c.del("/notification-channels/" + chID); r.Status != http.StatusNoContent {
		t.Fatalf("delete channel: %d\n%s", r.Status, r.Body)
	}
	if list = c.get("/notification-channels").listBody(t); len(list) != 0 {
		t.Fatalf("channels after delete: %v", list)
	}
}

// Scheduling prefs (GET defaults → PUT round-trip) and routing-rule CRUD, plus
// the per-task due-notify override.
func TestNotificationPrefsRoutesDueNotify(t *testing.T) {
	t.Parallel()
	c := signup(t)

	// Defaults for a user who never customised.
	p := c.expect(t, c.get("/notification-prefs"), http.StatusOK)
	if p["due_enabled"] != true || p["due_lead_minutes"] != float64(60) || p["reminder_enabled"] != true {
		t.Fatalf("default prefs: %v", p)
	}

	p = c.expect(t, c.put("/notification-prefs", map[string]any{
		"due_enabled": false, "due_lead_minutes": 120, "due_repeat_minutes": 15,
		"reminder_enabled": false, "quiet_enabled": true,
		"quiet_start_minutes": 1320, "quiet_end_minutes": 480,
		"quiet_tz": "Europe/Moscow", "digest_minutes": 5,
	}), http.StatusOK)
	if p["due_enabled"] != false || p["due_lead_minutes"] != float64(120) ||
		p["quiet_enabled"] != true || p["quiet_tz"] != "Europe/Moscow" || p["digest_minutes"] != float64(5) {
		t.Fatalf("prefs round-trip: %v", p)
	}
	p = c.expect(t, c.get("/notification-prefs"), http.StatusOK)
	if p["due_repeat_minutes"] != float64(15) {
		t.Fatalf("prefs persisted: %v", p)
	}

	// Routes need a channel to target.
	ch := c.expect(t, c.post("/notification-channels", map[string]any{
		"type": "webhook", "label": "Для маршрута", "config": map[string]any{"url": "http://127.0.0.1:1/hook"},
	}), http.StatusCreated)
	chID := ch["id"].(string)

	route := c.expect(t, c.post("/notification-routes", map[string]any{
		"matcher": map[string]any{"kinds": []string{"assigned"}}, "channel_ids": []string{chID},
	}), http.StatusCreated)
	routeID := route["id"].(string)
	if ids, _ := route["channel_ids"].([]any); len(ids) != 1 || ids[0] != chID {
		t.Fatalf("route channels: %v", route)
	}
	if route["enabled"] != true {
		t.Fatalf("route default enabled: %v", route)
	}
	// A route pointing at someone else's / nonexistent channel → 400.
	if r := c.post("/notification-routes", map[string]any{
		"channel_ids": []string{"00000000-0000-0000-0000-000000000001"},
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("route with bogus channel: %d", r.Status)
	}

	list := c.get("/notification-routes").listBody(t)
	if len(list) != 1 || list[0]["id"] != routeID {
		t.Fatalf("route list: %v", list)
	}

	up := c.expect(t, c.patch("/notification-routes/"+routeID, map[string]any{
		"matcher": map[string]any{}, "channel_ids": []string{chID},
		"options": map[string]any{"mute": true}, "enabled": false, "position": route["position"],
	}), http.StatusOK)
	if up["enabled"] != false {
		t.Fatalf("route update: %v", up)
	}

	if r := c.del("/notification-routes/" + routeID); r.Status != http.StatusNoContent {
		t.Fatalf("delete route: %d\n%s", r.Status, r.Body)
	}
	if list = c.get("/notification-routes").listBody(t); len(list) != 0 {
		t.Fatalf("routes after delete: %v", list)
	}

	// Per-task due-notify override (null field = inherit; here set explicitly).
	s := mkStack(t, c)
	id := mkTask(t, c, s.Board, s.col(t, 0), "Со сроком")["id"].(string)
	task := c.expect(t, c.patch("/tasks/"+id+"/due-notify", map[string]any{
		"enabled": false, "lead_minutes": 30, "repeat_minutes": 10,
	}), http.StatusOK)
	if task["due_notify_enabled"] != false || task["due_lead_minutes"] != float64(30) ||
		task["due_repeat_minutes"] != float64(10) {
		t.Fatalf("due-notify override: %v", task)
	}
}

// Personal reminders: CRUD, owner-only access.
func TestRemindersCRUD(t *testing.T) {
	t.Parallel()
	c := signup(t)
	stranger := signup(t)

	at := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	rem := c.expect(t, c.post("/reminders", map[string]any{
		"remind_at": at.Format(time.RFC3339), "message": "Позвонить в банк",
	}), http.StatusCreated)
	remID := rem["id"].(string)
	if rem["message"] != "Позвонить в банк" || rem["done"] != false {
		t.Fatalf("reminder create: %v", rem)
	}
	if !parseTS(t, rem["remind_at"]).Equal(at) {
		t.Fatalf("remind_at round-trip: %v", rem["remind_at"])
	}
	// remind_at is required.
	if r := c.post("/reminders", map[string]any{"message": "без времени"}); r.Status != http.StatusBadRequest {
		t.Fatalf("reminder without time: %d", r.Status)
	}

	list := c.get("/reminders").listBody(t)
	if len(list) != 1 || list[0]["id"] != remID {
		t.Fatalf("reminder list: %v", list)
	}

	// Update (full replace) + done toggle.
	at2 := at.Add(24 * time.Hour)
	up := c.expect(t, c.patch("/reminders/"+remID, map[string]any{
		"remind_at": at2.Format(time.RFC3339), "message": "Перенесено", "done": true,
	}), http.StatusOK)
	if up["message"] != "Перенесено" || up["done"] != true || !parseTS(t, up["remind_at"]).Equal(at2) {
		t.Fatalf("reminder update: %v", up)
	}

	// Another user can't touch it.
	if r := stranger.patch("/reminders/"+remID, map[string]any{
		"remind_at": at2.Format(time.RFC3339), "message": "чужое",
	}); r.Status != http.StatusForbidden {
		t.Fatalf("foreign reminder patch: %d", r.Status)
	}
	if r := stranger.del("/reminders/" + remID); r.Status != http.StatusForbidden {
		t.Fatalf("foreign reminder delete: %d", r.Status)
	}

	if r := c.del("/reminders/" + remID); r.Status != http.StatusNoContent {
		t.Fatalf("delete reminder: %d\n%s", r.Status, r.Body)
	}
	if list = c.get("/reminders").listBody(t); len(list) != 0 {
		t.Fatalf("reminders after delete: %v", list)
	}
}
