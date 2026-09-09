package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test

/**
 * The screen-share stage's rules (#2896 §8).
 *
 * Every question the room asks about sharing is answered here rather than in the
 * composable, and the reason is the seam this feature is built on: the stage
 * lives on the room socket and the pixels live on the SFU, so «показываю» is a
 * fact about two connections at once. Deciding that in a composable would make
 * it untestable without an SFU, a projection and a display.
 */
class ConferenceScreenTest {
    // ── the three facts that make a share ────────────────────────────────

    @Test
    fun `pixels going out with the stage in hand is a share`() {
        assertThat(confShareState(want = true, presenting = true, capturing = true))
            .isEqualTo(ConfShare.SHARING)
    }

    @Test
    fun `asking while somebody else presents puts us in the line`() {
        assertThat(confShareState(want = true, presenting = false, capturing = false))
            .isEqualTo(ConfShare.QUEUED)
    }

    @Test
    fun `the stage arriving without a capture is a turn, not a share`() {
        // The gap the web does not have: the consent dialog sits in it, and
        // collapsing this into SHARING would offer «Остановить показ» for a
        // capture that never started.
        assertThat(confShareState(want = true, presenting = true, capturing = false))
            .isEqualTo(ConfShare.MY_TURN)
    }

    @Test
    fun `capturing without the stage is not a share`() {
        // A host preempted us. The pixels are still being captured for the
        // moment it takes the room to notice — painting that as SHARING is how
        // the toolbar ends up offering to stop a share nobody receives.
        assertThat(confShareState(want = true, presenting = false, capturing = true))
            .isEqualTo(ConfShare.OFF)
    }

    @Test
    fun `not wanting the stage is off however the room reads`() {
        assertThat(confShareState(want = false, presenting = false, capturing = false))
            .isEqualTo(ConfShare.OFF)
        // Even holding the stage: it is about to be released, and «Очередь дошла
        // до вас» for somebody who cancelled is a notice about nothing.
        assertThat(confShareState(want = false, presenting = true, capturing = false))
            .isEqualTo(ConfShare.OFF)
    }

    // ── what the button does ─────────────────────────────────────────────

    @Test
    fun `the one button means four different things`() {
        assertThat(confSharePress(ConfShare.OFF)).isEqualTo(ConfSharePress.REQUEST)
        assertThat(confSharePress(ConfShare.QUEUED)).isEqualTo(ConfSharePress.CANCEL)
        assertThat(confSharePress(ConfShare.MY_TURN)).isEqualTo(ConfSharePress.CAPTURE)
        assertThat(confSharePress(ConfShare.SHARING)).isEqualTo(ConfSharePress.STOP)
    }

    @Test
    fun `a free stage costs one press and a queued one does not`() {
        // Pressing onto a free stage may run into the consent dialog directly,
        // as it does on the web.
        assertThat(confShareAutoCapture(stageTaken = false)).isTrue()
        // Pressing into a queue must not: the turn may come in ten minutes, and
        // a full-screen capture warning that old reads as the app helping itself.
        assertThat(confShareAutoCapture(stageTaken = true)).isFalse()
    }

    @Test
    fun `the control is tied to the room socket and not to the media session`() {
        assertThat(confCanShare(connected = true)).isTrue()
        assertThat(confCanShare(connected = false)).isFalse()
    }

    // ── the line ─────────────────────────────────────────────────────────

    @Test
    fun `our place in the queue is 1-based`() {
        val queue = listOf("conn-a", "conn-me", "conn-c")

        assertThat(confQueuePosition(queue, "conn-a")).isEqualTo(1)
        assertThat(confQueuePosition(queue, "conn-me")).isEqualTo(2)
    }

    @Test
    fun `not being in the queue is a zero and not a first place`() {
        assertThat(confQueuePosition(listOf("conn-a"), "conn-me")).isEqualTo(0)
        assertThat(confQueuePosition(emptyList(), "conn-me")).isEqualTo(0)
    }

    @Test
    fun `a connection without an id is nowhere in the line`() {
        // The welcome has not arrived yet. Matching a blank against a blank
        // would put a phone that never asked at the head of the queue.
        assertThat(confQueuePosition(listOf("", "conn-a"), "")).isEqualTo(0)
    }

    // ── the line above the tiles ─────────────────────────────────────────

    @Test
    fun `a free stage and an empty queue say nothing at all`() {
        val line = confStageLine(ConfShare.OFF, stageName = "", stageIsMine = false, position = 0, waiting = 0)

        // The normal state of a call. A permanent «никто не показывает экран» is
        // a line people stop reading before it has anything to say.
        assertThat(line.notice).isEqualTo(ConfStageNotice.NONE)
        assertThat(line.name).isEmpty()
        assertThat(line.position).isEqualTo(0)
    }

    @Test
    fun `our own share outranks everything else on the line`() {
        val line = confStageLine(ConfShare.SHARING, stageName = "Я", stageIsMine = true, position = 0, waiting = 2)

        assertThat(line.notice).isEqualTo(ConfStageNotice.YOU_PRESENT)
        assertThat(line.waiting).isEqualTo(2)
    }

    @Test
    fun `the turn arriving is said before whose screen was on the stage`() {
        val line = confStageLine(ConfShare.MY_TURN, stageName = "Аня", stageIsMine = false, position = 0, waiting = 1)

        assertThat(line.notice).isEqualTo(ConfStageNotice.STAGE_YOURS)
    }

    @Test
    fun `somebody else on the stage is named`() {
        val line = confStageLine(ConfShare.QUEUED, stageName = "Аня", stageIsMine = false, position = 2, waiting = 3)

        assertThat(line.notice).isEqualTo(ConfStageNotice.OTHER_PRESENTS)
        assertThat(line.name).isEqualTo("Аня")
        assertThat(line.position).isEqualTo(2)
    }

    @Test
    fun `only the named notice carries a name`() {
        val line = confStageLine(ConfShare.SHARING, stageName = "Аня", stageIsMine = false, position = 0, waiting = 0)

        // Carried separately precisely so «Экран показывает » cannot be printed
        // with a blank where a person should be.
        assertThat(line.name).isEmpty()
    }

    @Test
    fun `waiting behind a stage that just freed still says we are waiting`() {
        val line = confStageLine(ConfShare.QUEUED, stageName = "", stageIsMine = false, position = 1, waiting = 1)

        assertThat(line.notice).isEqualTo(ConfStageNotice.WAITING)
    }

    @Test
    fun `our own stage is not reported as somebody else's`() {
        // The same person on a phone and a laptop: the laptop holds the stage,
        // and this phone must not be told «Экран показывает Вы».
        val line = confStageLine(ConfShare.OFF, stageName = "Я", stageIsMine = true, position = 0, waiting = 0)

        assertThat(line.notice).isEqualTo(ConfStageNotice.NONE)
    }

    // ── holding on to the stage ──────────────────────────────────────────

    @Test
    fun `the heartbeat is three beats to the server's own TTL`() {
        // Taken from the welcome rather than from a constant here: a client that
        // drifted out of step with the TTL would have the stage collected out
        // from under a presenter who is still presenting.
        assertThat(confShareHeartbeatMs(30_000)).isEqualTo(10_000)
        assertThat(confShareHeartbeatMs(9_000)).isEqualTo(3_000)
    }

    @Test
    fun `an absurdly short TTL does not turn the heartbeat into a busy loop`() {
        assertThat(confShareHeartbeatMs(600)).isEqualTo(1_000)
        assertThat(confShareHeartbeatMs(0)).isEqualTo(1_000)
    }

    // ── losing it ────────────────────────────────────────────────────────

    @Test
    fun `a snapshot newer than our request taking the stage away is a preemption`() {
        assertThat(confSharePreempted(capturing = true, presenting = false, seq = 5, askedAt = 4)).isTrue()
    }

    @Test
    fun `the server not having answered yet is not a preemption`() {
        // The whole reason the counter exists: acting on this would stop the
        // capture in the same breath as starting it.
        assertThat(confSharePreempted(capturing = true, presenting = false, seq = 4, askedAt = 4)).isFalse()
    }

    @Test
    fun `nothing is preempted while we still hold the stage`() {
        assertThat(confSharePreempted(capturing = true, presenting = true, seq = 9, askedAt = 4)).isFalse()
    }

    @Test
    fun `a phone that is not capturing has nothing to lose`() {
        assertThat(confSharePreempted(capturing = false, presenting = false, seq = 9, askedAt = 4)).isFalse()
    }
}
