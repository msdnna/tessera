package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import java.io.IOException
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Test
import retrofit2.HttpException
import website.msdnna.tessera.data.model.ConferenceRecording
import website.msdnna.tessera.data.model.RecordingStatus

/**
 * The recordings' rules (#2896 §8).
 *
 * Nothing here captures anything — an egress worker on the server joins the call
 * and writes the file. What this side owes the user is an order that makes the
 * list findable, a gate that matches who the server will actually obey, and an
 * honest word for an install where that worker was never deployed.
 */
class ConferenceRecordingsTest {
    private fun rec(
        id: String = "r1",
        status: String = RecordingStatus.COMPLETED,
        startedAt: String = "2026-09-05T10:00:00Z",
        durationSec: Int = 0,
        fileName: String = "",
    ) = ConferenceRecording(
        id = id,
        conferenceId = "c1",
        status = status,
        startedAt = startedAt,
        durationSec = durationSec,
        fileName = fileName,
    )

    private fun http(code: Int): HttpException {
        val raw = Response.Builder()
            .code(code)
            .message("err")
            .protocol(Protocol.HTTP_1_1)
            .request(Request.Builder().url("http://localhost/").build())
            .build()
        val body = "{}".toResponseBody("application/json".toMediaType())
        return HttpException(retrofit2.Response.error<Any>(body, raw))
    }

    // ── why a start was refused ──────────────────────────────────────────

    @Test
    fun `a server without the egress worker is unavailable, not a retry`() {
        // The one refusal this feature was asked for by name: an install that
        // never deployed the worker answers every start this way, and «попробуйте
        // ещё раз» would have people pressing it for the rest of the call.
        assertThat(confRecordingFailure(http(502))).isEqualTo(ConfRecordingFailure.UNAVAILABLE)
        assertThat(confRecordingFailure(http(503))).isEqualTo(ConfRecordingFailure.UNAVAILABLE)
    }

    @Test
    fun `a refusal by role and a race with another moderator read differently`() {
        assertThat(confRecordingFailure(http(403))).isEqualTo(ConfRecordingFailure.FORBIDDEN)
        assertThat(confRecordingFailure(http(409))).isEqualTo(ConfRecordingFailure.CONFLICT)
    }

    @Test
    fun `anything else falls through to the server's own sentence`() {
        assertThat(confRecordingFailure(http(500))).isEqualTo(ConfRecordingFailure.OTHER)
        assertThat(confRecordingFailure(http(404))).isEqualTo(ConfRecordingFailure.OTHER)
        // A dead network never reached the server, so it says nothing about one.
        assertThat(confRecordingFailure(IOException("offline"))).isEqualTo(ConfRecordingFailure.OTHER)
    }

    // ── who may press it ─────────────────────────────────────────────────

    @Test
    fun `a member gets no control at all`() {
        // Hidden rather than greyed out: the toolbar is already two rows deep on
        // a phone, and a dead button advertises a capability nobody but a
        // moderator has.
        assertThat(confRecordingPress(canModerate = false, running = false))
            .isEqualTo(ConfRecordingPress.NONE)
        assertThat(confRecordingPress(canModerate = false, running = true))
            .isEqualTo(ConfRecordingPress.NONE)
    }

    @Test
    fun `a moderator starts a quiet call and stops a running one`() {
        assertThat(confRecordingPress(canModerate = true, running = false))
            .isEqualTo(ConfRecordingPress.START)
        assertThat(confRecordingPress(canModerate = true, running = true))
            .isEqualTo(ConfRecordingPress.STOP)
    }

    // ── the order of the list ────────────────────────────────────────────

    @Test
    fun `newest first`() {
        val rows = listOf(
            rec("old", startedAt = "2026-09-01T10:00:00Z"),
            rec("new", startedAt = "2026-09-05T10:00:00Z"),
            rec("mid", startedAt = "2026-09-03T10:00:00Z"),
        )

        assertThat(confRecordingOrder(rows).map { it.id })
            .containsExactly("new", "mid", "old").inOrder()
    }

    @Test
    fun `a running recording is pinned above everything finished`() {
        val rows = listOf(
            rec("newest", startedAt = "2026-09-05T12:00:00Z"),
            // Started yesterday and still going — by date alone it sinks into the
            // middle of the list, which is where nobody finds it.
            rec("live", status = RecordingStatus.ACTIVE, startedAt = "2026-09-04T09:00:00Z"),
        )

        assertThat(confRecordingOrder(rows).map { it.id }).containsExactly("live", "newest").inOrder()
    }

    @Test
    fun `a row the server sent no date for still lands in the list`() {
        val rows = listOf(rec("dated", startedAt = "2026-09-05T10:00:00Z"), rec("undated", startedAt = ""))

        // Last rather than dropped: an unparseable date is a reason to sort it
        // badly, not a reason to hide a file somebody is looking for.
        assertThat(confRecordingOrder(rows).map { it.id }).containsExactly("dated", "undated").inOrder()
    }

    // ── which rows have bytes behind them ────────────────────────────────

    @Test
    fun `only a finished recording can be opened`() {
        assertThat(confRecordingDownloadable(rec(status = RecordingStatus.COMPLETED))).isTrue()
        // Shown in the list — «запись не удалась» is what people came to find
        // out — but there is no file to hand the system viewer.
        assertThat(confRecordingDownloadable(rec(status = RecordingStatus.FAILED))).isFalse()
        assertThat(confRecordingDownloadable(rec(status = RecordingStatus.ACTIVE))).isFalse()
    }

    @Test
    fun `a row without an id is not downloadable whatever its status says`() {
        assertThat(confRecordingDownloadable(rec(id = "", status = RecordingStatus.COMPLETED))).isFalse()
    }

    // ── what it is saved as ──────────────────────────────────────────────

    @Test
    fun `the server's name is used when it sent one`() {
        assertThat(confRecordingFileName(rec(fileName = "planerka.mp4"))).isEqualTo("planerka.mp4")
    }

    @Test
    fun `a nameless recording is saved under its id and never blank`() {
        // A blank would write a directory rather than a file.
        assertThat(confRecordingFileName(rec(id = "abc", fileName = ""))).isEqualTo("recording-abc.mp4")
    }

    @Test
    fun `separators in a name from the other side are stripped`() {
        // The name comes from the server; a `..\/` in it would put the file
        // somewhere the cache directory is not.
        assertThat(confRecordingFileName(rec(fileName = "../etc/passwd")))
            .isEqualTo(".._etc_passwd")
        assertThat(confRecordingFileName(rec(fileName = "a\\b.mp4"))).isEqualTo("a_b.mp4")
    }

    // ── how long it ran ──────────────────────────────────────────────────

    @Test
    fun `a duration is split into hours, minutes and seconds`() {
        val length = confRecordingLength(rec(durationSec = 3725))

        assertThat(length).isNotNull()
        assertThat(length!!.hours).isEqualTo(1)
        assertThat(length.minutes).isEqualTo(2)
        assertThat(length.seconds).isEqualTo(5)
    }

    @Test
    fun `under an hour there are no hours`() {
        val length = confRecordingLength(rec(durationSec = 90))!!

        assertThat(length.hours).isEqualTo(0)
        assertThat(length.minutes).isEqualTo(1)
        assertThat(length.seconds).isEqualTo(30)
    }

    @Test
    fun `a recording still being written has no length yet`() {
        // «00:00» next to a live red dot reads as a recording that is broken.
        assertThat(confRecordingLength(rec(status = RecordingStatus.ACTIVE, durationSec = 0))).isNull()
        assertThat(confRecordingLength(rec(status = RecordingStatus.ACTIVE, durationSec = 42))).isNull()
    }

    @Test
    fun `a finished recording the server gave no duration for shows none`() {
        assertThat(confRecordingLength(rec(durationSec = 0))).isNull()
    }
}
