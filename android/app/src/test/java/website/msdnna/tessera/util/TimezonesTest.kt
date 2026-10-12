package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.util.TimeZone
import org.junit.Test

/**
 * The zone the phone tells the server it lives in (#2932).
 *
 * Mirrors `resolvedTimezone` in `frontend/src/utils/format.js`: an empty profile
 * zone does NOT mean UTC — it means "the device's own", a blank the backend
 * cannot re-derive (handlers/api.go callerLocation).
 */
class TimezonesTest {
    @Test
    fun `the profile zone wins over the device`() {
        assertThat(resolvedTimezone("America/New_York")).isEqualTo("America/New_York")
    }

    @Test
    fun `an empty profile zone resolves to the device zone, not a blank`() {
        val device = TimeZone.getDefault().id
        assertThat(resolvedTimezone("")).isEqualTo(device)
        assertThat(resolvedTimezone(null)).isEqualTo(device)
        assertThat(resolvedTimezone("   ")).isEqualTo(device)
    }

    @Test
    fun `a stored zone is trimmed, not re-spelled`() {
        // The server decides whether a zone loads (and falls through when it does
        // not), so an unknown value travels as written rather than being dropped
        // here — same as the web.
        assertThat(resolvedTimezone(" Europe/Moscow ")).isEqualTo("Europe/Moscow")
        assertThat(resolvedTimezone("Mars/Olympus")).isEqualTo("Mars/Olympus")
    }
}
