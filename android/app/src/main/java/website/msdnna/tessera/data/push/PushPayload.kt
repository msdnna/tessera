package website.msdnna.tessera.data.push

import website.msdnna.tessera.data.realtime.DevicePush

/**
 * Decoding of an FCM data payload, kept apart from the service that receives it
 * so it can be unit-tested (a [com.google.firebase.messaging.FirebaseMessagingService]
 * can't be instantiated off-device).
 *
 * The server sends data-only messages — the fields mirror the ones the WS path
 * already carries, so both paths end up building the same [DevicePush].
 */
object PushPayload {
    /**
     * Builds a [DevicePush] from an FCM `data` map, or null when the payload
     * carries nothing worth showing. FCM values are always strings.
     */
    fun parse(data: Map<String, String>): DevicePush? {
        val body = data["body"]?.trim().orEmpty()
        if (body.isEmpty()) return null
        val title = data["title"]?.trim().orEmpty().ifEmpty { "Tessera" }
        return DevicePush(
            title = title,
            body = body,
            taskId = data["task_id"]?.trim()?.ifEmpty { null },
            id = data["notification_id"]?.trim()?.ifEmpty { null },
        )
    }
}
