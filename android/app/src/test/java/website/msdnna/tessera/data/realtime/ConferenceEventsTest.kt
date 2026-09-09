package website.msdnna.tessera.data.realtime

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * How conference broadcasts are read (#2896 §1).
 *
 * The eleven server-side `conference.*` types collapse into four reactions, and
 * getting the collapse wrong is invisible in review: a recording event routed to
 * LIST repaints the whole screen every time an egress worker reports in, and a
 * join routed to LIST does the same on every arrival. Both still "work".
 */
class ConferenceEventsTest {
    private val ws = "ws-1"

    private fun classify(type: String, scope: String = ws) = classifyConfEvent(type, scope, ws)

    @Test
    fun `lifecycle events repaint the list`() {
        // These are exactly the types the Go side broadcasts for the meeting
        // itself — a missing one here means a call that appears only on reload.
        for (type in listOf(
            "conference.created",
            "conference.updated",
            "conference.started",
            "conference.ended",
            "conference.deleted",
        )) {
            assertThat(classify(type)).isEqualTo(ConfEvent.LIST)
        }
    }

    @Test
    fun `roster events touch only the participants panel`() {
        for (type in listOf(
            "conference.participant.joined",
            "conference.participant.left",
            "conference.participant.invited",
        )) {
            assertThat(classify(type)).isEqualTo(ConfEvent.PARTICIPANTS)
        }
    }

    @Test
    fun `recording events do not reload the conference`() {
        for (type in listOf(
            "conference.recording.started",
            "conference.recording.finished",
            "conference.recording.deleted",
        )) {
            assertThat(classify(type)).isEqualTo(ConfEvent.RECORDING)
        }
    }

    @Test
    fun `another workspace on the same socket is not our event`() {
        // The socket carries every workspace the user belongs to, so scope is a
        // filter, not a formality: without it a colleague's standup in another
        // workspace would repaint the open list.
        assertThat(classifyConfEvent("conference.started", "ws-2", ws)).isEqualTo(ConfEvent.OTHER)
        assertThat(classify("task.created")).isEqualTo(ConfEvent.OTHER)
        assertThat(classify("document.updated")).isEqualTo(ConfEvent.OTHER)
    }

    @Test
    fun `the end of our own call is told apart from someone else's`() {
        // Leaving a room because a *different* conference ended is the bug this
        // guards: the id has to match, and an event without one never counts.
        assertThat(isConfEnded("conference.ended", "c-1", "c-1")).isTrue()
        assertThat(isConfEnded("conference.ended", "c-1", "c-2")).isFalse()
        assertThat(isConfEnded("conference.ended", "c-1", null)).isFalse()
        assertThat(isConfEnded("conference.started", "c-1", "c-1")).isFalse()
    }
}
