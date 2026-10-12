package main

import (
	"net/http"
	"testing"
)

// Телефон, сменивший владельца, не должен продолжать получать пуши прежнего
// пользователя.
//
// `device_id` у Android-клиента привязан к ANDROID_ID и намеренно переживает
// logout и переустановку, а `fcm_token` принадлежит УСТАНОВКЕ приложения, не
// аккаунту: FCM доставляет по токену, и показывает уведомление тот, кто сейчас
// держит телефон в руках. Серверный logout при этом гасит только refresh-токен
// — канал типа device со всей его конфигурацией остаётся как был.
//
// Поэтому после «А вышел → Б вошёл» на одном телефоне у А оставался канал с
// живым токеном этой установки, и всё, что роутинг А отправлял на устройство,
// приезжало на экран Б. Текст там личный по определению: упоминание в
// комментарии, а для напоминания — собственная фраза пользователя. Ровно этот
// сюжет для ЛОКАЛЬНЫХ будильников уже закрыт на клиенте
// (AuthRepository.clearSession отменяет запланированные напоминания, чтобы
// «на общем устройстве не досталось тому, кто вошёл следующим») — серверная
// половина доставки осталась открытой.
//
// Чинится на регистрации, а не на выходе: выход бывает и без сети, и вовсе без
// него (аккаунт сменили после переустановки), а регистрация устройства — тот
// единственный момент, когда сервер узнаёт, что установка теперь принадлежит
// другому пользователю. Отбирается только токен: сам канал — настройка его
// владельца, и если он вернётся на этот телефон, следующая регистрация вернёт
// токен на место. Та же политика, что у dropPushToken на мёртвом токене.
func TestDeviceHandoverStripsPreviousOwnerPushToken(t *testing.T) {
	t.Parallel()
	prev := signup(t)
	next := signup(t)

	// Идентификаторы уникальны на тест: отбор токена смотрит на устройство
	// ПОПЕРЁК пользователей, так что общий «dev-1» столкнул бы параллельные тесты.
	const (
		phone     = "dev-handover-phone"
		phoneTok  = "tok-handover-install"
		tablet    = "dev-handover-tablet"
		tabletTok = "tok-handover-tablet"
	)

	cfgOf := func(t *testing.T, v map[string]any) map[string]any {
		t.Helper()
		cfg, ok := v["config"].(map[string]any)
		if !ok {
			t.Fatalf("у канала нет объекта config: %v", v)
		}
		return cfg
	}
	// channelFor возвращает канал устройства из списка каналов пользователя.
	channelFor := func(t *testing.T, c *client, deviceID string) map[string]any {
		t.Helper()
		for _, ch := range c.get("/notification-channels").listBody(t) {
			if ch["type"] == "device" && cfgOf(t, ch)["device_id"] == deviceID {
				return ch
			}
		}
		t.Fatalf("канал устройства %s пропал из /notification-channels", deviceID)
		return nil
	}

	// Прежний владелец: телефон с пуш-токеном плюс второе устройство, которое
	// из рук не уходило.
	prev.expect(t, prev.post("/notification-devices", map[string]any{
		"device_id": phone, "platform": "android", "fcm_token": phoneTok,
	}), http.StatusCreated)
	prev.expect(t, prev.post("/notification-devices", map[string]any{
		"device_id": tablet, "platform": "android", "fcm_token": tabletTok,
	}), http.StatusCreated)

	// Своя же перерегистрация токен не трогает — иначе перезапуск приложения
	// выключал бы фоновые пуши самому себе.
	again := prev.expect(t, prev.post("/notification-devices", map[string]any{
		"device_id": phone, "platform": "android", "fcm_token": phoneTok,
	}), http.StatusOK)
	if cfgOf(t, again)["fcm_token"] != phoneTok {
		t.Fatalf("перерегистрация своего же устройства потеряла токен: %v", cfgOf(t, again))
	}

	// На том же телефоне входит другой пользователь: тот же ANDROID_ID, тот же
	// токен установки.
	taken := next.expect(t, next.post("/notification-devices", map[string]any{
		"device_id": phone, "platform": "android", "fcm_token": phoneTok,
	}), http.StatusCreated)
	if cfgOf(t, taken)["fcm_token"] != phoneTok {
		t.Fatalf("новый владелец остался без токена: %v", cfgOf(t, taken))
	}

	t.Run("токен уходит у прежнего владельца", func(t *testing.T) {
		cfg := cfgOf(t, channelFor(t, prev, phone))
		if got, ok := cfg["fcm_token"]; ok && got != "" {
			t.Fatalf("телефон сменил владельца, а пуши прежнего всё ещё едут на эту установку: fcm_token = %v", got)
		}
	})

	t.Run("канал прежнего владельца остаётся на месте", func(t *testing.T) {
		ch := channelFor(t, prev, phone)
		if ch["enabled"] != true {
			t.Fatalf("канал должен пережить передачу устройства (вернётся владелец — вернётся токен): %v", ch)
		}
	})

	t.Run("чужое устройство не задето", func(t *testing.T) {
		if got := cfgOf(t, channelFor(t, prev, tablet))["fcm_token"]; got != tabletTok {
			t.Fatalf("планшет из рук не уходил, токен должен остаться: fcm_token = %v, want %s", got, tabletTok)
		}
	})
}
