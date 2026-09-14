package website.msdnna.tessera.util

import java.util.TimeZone

/**
 * Зона, которую клиент сообщает серверу на ручках, отвечающих ПО ДНЯМ (#2932).
 *
 * Зеркало `resolvedTimezone` из `frontend/src/utils/format.js`. Пустой преф зоны
 * значит не UTC, а «зона самого клиента», и сервер этот пробел восстановить не
 * может — `callerLocation` (backend/handlers/api.go) поэтому сначала читает
 * `?tz=` запроса и лишь потом профиль. На рендер зона на телефоне не влияет
 * (даты и так рисуются в зоне устройства); влияет она ровно на то, что считает
 * сервер: плитки «Просрочено/Сегодня/На неделе» и день, в который попадает
 * «/due сегодня» из комментария.
 *
 * Незнакомое значение уезжает как записано: решает, грузится ли зона, сервер, и
 * при неудаче он падает на следующий источник — так же, как на вебе.
 */
fun resolvedTimezone(profileTz: String?): String {
    val tz = profileTz?.trim().orEmpty()
    if (tz.isNotEmpty()) return tz
    return runCatching { TimeZone.getDefault().id }.getOrDefault("")
}
