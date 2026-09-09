package website.msdnna.tessera.util

import android.content.pm.ServiceInfo
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.ConferenceToken
import website.msdnna.tessera.data.repository.ConfTokenResult

/** The media core's rules (#2896 §4). */
class ConferenceMediaTest {
    // ── Reading the token answer ────────────────────────────────────────────

    @Test
    fun `a granted token carries on into connecting`() {
        val token = ConferenceToken(url = "wss://sfu", token = "jwt", room = "r", identity = "u")
        val plan = confJoinPlan(ConfTokenResult.Granted(token))

        assertThat(plan.status).isEqualTo(ConfMediaStatus.CONNECTING)
        assertThat(plan.token).isEqualTo(token)
        assertThat(plan.reason).isEqualTo(ConfJoinReason.NONE)
    }

    @Test
    fun `an install without an SFU is unavailable, not an error`() {
        val plan = confJoinPlan(ConfTokenResult.NotConfigured)

        // The difference is the whole point: ERROR invites a retry button, and no
        // number of retries configures LiveKit on the server.
        assertThat(plan.status).isEqualTo(ConfMediaStatus.UNAVAILABLE)
        assertThat(plan.reason).isEqualTo(ConfJoinReason.NOT_CONFIGURED)
    }

    @Test
    fun `an ended call goes back to idle so the lobby can start it again`() {
        val plan = confJoinPlan(ConfTokenResult.Ended)

        assertThat(plan.status).isEqualTo(ConfMediaStatus.IDLE)
        assertThat(plan.reason).isEqualTo(ConfJoinReason.ENDED)
    }

    @Test
    fun `a kick is an error and keeps the cooldown`() {
        val plan = confJoinPlan(ConfTokenResult.Kicked("2026-09-05T12:30:00Z"))

        assertThat(plan.status).isEqualTo(ConfMediaStatus.ERROR)
        assertThat(plan.reason).isEqualTo(ConfJoinReason.KICKED)
        assertThat(plan.retryAfter).isEqualTo("2026-09-05T12:30:00Z")
    }

    // ── Backing off ─────────────────────────────────────────────────────────

    @Test
    fun `backoff starts at a second and doubles`() {
        assertThat(confReconnectDelay(1)).isEqualTo(1_000L)
        assertThat(confReconnectDelay(2)).isEqualTo(2_000L)
        assertThat(confReconnectDelay(3)).isEqualTo(4_000L)
        assertThat(confReconnectDelay(4)).isEqualTo(8_000L)
    }

    @Test
    fun `backoff never exceeds the ceiling, however long the call has been down`() {
        assertThat(confReconnectDelay(5)).isEqualTo(CONF_RECONNECT_MAX_MS)
        assertThat(confReconnectDelay(60)).isEqualTo(CONF_RECONNECT_MAX_MS)
        // A call left open overnight reaches attempt numbers where a plain shift
        // overflows into a negative delay — which is a busy loop against an SFU
        // that is already struggling, not a wait.
        assertThat(confReconnectDelay(1_000)).isEqualTo(CONF_RECONNECT_MAX_MS)
        assertThat(confReconnectDelay(Int.MAX_VALUE)).isGreaterThan(0L)
    }

    @Test
    fun `a zero or negative attempt still waits`() {
        assertThat(confReconnectDelay(0)).isEqualTo(CONF_RECONNECT_BASE_MS)
        assertThat(confReconnectDelay(-3)).isEqualTo(CONF_RECONNECT_BASE_MS)
    }

    @Test
    fun `we reconnect only when the call is still ours to rejoin`() {
        assertThat(shouldConfReconnect(leaving = false, ended = false)).isTrue()
        assertThat(shouldConfReconnect(leaving = true, ended = false)).isFalse()
        // Reconnecting into a room the server closed would re-open it for
        // everyone who was on their way out.
        assertThat(shouldConfReconnect(leaving = false, ended = true)).isFalse()
    }

    // ── Audio routing ───────────────────────────────────────────────────────

    @Test
    fun `a headset wins over the speaker`() {
        val all = listOf(
            ConfAudioRoute.SPEAKER,
            ConfAudioRoute.EARPIECE,
            ConfAudioRoute.WIRED_HEADSET,
            ConfAudioRoute.BLUETOOTH,
        )
        assertThat(preferredAudioRoute(all)).isEqualTo(ConfAudioRoute.BLUETOOTH)
        assertThat(preferredAudioRoute(all - ConfAudioRoute.BLUETOOTH))
            .isEqualTo(ConfAudioRoute.WIRED_HEADSET)
    }

    @Test
    fun `with nothing plugged in a conference goes to the speaker, not the earpiece`() {
        // The one place this differs from a phone call: a conference sits on a
        // desk, and starting it on the earpiece makes the app look silent.
        assertThat(preferredAudioRoute(listOf(ConfAudioRoute.EARPIECE, ConfAudioRoute.SPEAKER)))
            .isEqualTo(ConfAudioRoute.SPEAKER)
    }

    @Test
    fun `an empty device list still names an output`() {
        // The routing menu asks before the call connects, when the handler has no
        // devices to report yet.
        assertThat(preferredAudioRoute(emptyList())).isEqualTo(ConfAudioRoute.SPEAKER)
    }

    // ── What may be published ───────────────────────────────────────────────

    @Test
    fun `the microphone needs the user, the permission and the host to agree`() {
        val want = ConfMediaWant(mic = true, cam = false)
        assertThat(publishableMedia(want, ConfMediaGrant(mic = true)).mic).isTrue()
        assertThat(publishableMedia(want, ConfMediaGrant(mic = false)).mic).isFalse()
        assertThat(publishableMedia(ConfMediaWant(mic = false), ConfMediaGrant(mic = true)).mic)
            .isFalse()
    }

    @Test
    fun `a host's force-mute outranks the user's own toggle`() {
        val state = publishableMedia(
            ConfMediaWant(mic = true),
            ConfMediaGrant(mic = true),
            forceMuted = true,
        )
        // Applied to the track, not just to the button: the roster is republished
        // on every change, and a client that only greyed out its control would
        // keep transmitting until somebody happened to press it.
        assertThat(state.mic).isFalse()
    }

    @Test
    fun `a refused camera leaves the call running with audio`() {
        val state = publishableMedia(
            ConfMediaWant(mic = true, cam = true),
            ConfMediaGrant(mic = true, cam = false),
        )
        assertThat(state.cam).isFalse()
        assertThat(state.mic).isTrue()
    }

    @Test
    fun `only the permissions the call actually intends to use are asked for`() {
        assertThat(confRequiredPermissions(ConfMediaWant(mic = true, cam = false)))
            .containsExactly(android.Manifest.permission.RECORD_AUDIO)
        assertThat(confRequiredPermissions(ConfMediaWant(mic = true, cam = true)))
            .containsExactly(
                android.Manifest.permission.RECORD_AUDIO,
                android.Manifest.permission.CAMERA,
            )
        // Joining to listen asks for nothing, and a listener should not see a
        // microphone prompt they have no use for.
        assertThat(confRequiredPermissions(ConfMediaWant(mic = false, cam = false))).isEmpty()
    }

    // ── Tiles ───────────────────────────────────────────────────────────────

    @Test
    fun `a muted camera shows the avatar, not a black rectangle`() {
        // The SDK mutes a camera in place and keeps both the publication and the
        // track object, so "has a track" alone answers yes for somebody who
        // turned their camera off (#2890 on the web).
        assertThat(showsVideo(hasTrack = true, muted = true, subscribed = true)).isFalse()
        assertThat(showsVideo(hasTrack = true, muted = false, subscribed = true)).isTrue()
        assertThat(showsVideo(hasTrack = true, muted = false, subscribed = false)).isFalse()
        assertThat(showsVideo(hasTrack = false, muted = false, subscribed = true)).isFalse()
    }

    // ── The foreground service ──────────────────────────────────────────────

    @Test
    fun `the call service covers the connecting window too`() {
        // Someone who taps «Войти» and immediately switches away would otherwise
        // have the connection killed halfway through.
        assertThat(needsCallService(ConfMediaStatus.CONNECTING)).isTrue()
        assertThat(needsCallService(ConfMediaStatus.LIVE)).isTrue()
    }

    @Test
    fun `no call, no service`() {
        assertThat(needsCallService(ConfMediaStatus.IDLE)).isFalse()
        assertThat(needsCallService(ConfMediaStatus.ERROR)).isFalse()
        assertThat(needsCallService(ConfMediaStatus.UNAVAILABLE)).isFalse()
    }

    @Test
    fun `the service type never claims a permission we do not hold`() {
        // The crash this exists for: the room screen connects first and asks for
        // the microphone second, and the camera is only ever asked for from the
        // toolbar — so at the moment the service starts, the camera is not ours.
        // A `camera` type there is a SecurityException out of startForeground.
        assertThat(confServiceTypes(micGranted = false, camGranted = false, sdk = 34)).isEqualTo(0)
        assertThat(confServiceTypes(micGranted = true, camGranted = false, sdk = 34))
            .isEqualTo(ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE)
        assertThat(confServiceTypes(micGranted = false, camGranted = true, sdk = 34))
            .isEqualTo(ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA)
        // Both, once the toolbar's dialog has been answered: widening is allowed,
        // the service is restarted on every session change, and by then we hold it.
        assertThat(confServiceTypes(micGranted = true, camGranted = true, sdk = 34)).isEqualTo(
            ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE or ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA,
        )
    }

    @Test
    fun `types are not a concept before Q`() {
        assertThat(confServiceTypes(micGranted = true, camGranted = true, sdk = 28)).isEqualTo(0)
    }

    @Test
    fun `a listener-only call keeps its service below 14 and loses it on 14`() {
        // From 14 a zero type falls back to everything the manifest declares —
        // camera included — so «no permissions» has to mean «no service», not
        // «an untyped one».
        assertThat(confServiceAllowed(micGranted = false, camGranted = false, sdk = 34)).isFalse()
        assertThat(confServiceAllowed(micGranted = false, camGranted = false, sdk = 33)).isTrue()
        // One permission is enough: that type alone is legal.
        assertThat(confServiceAllowed(micGranted = true, camGranted = false, sdk = 34)).isTrue()
    }
}
