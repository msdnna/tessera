package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceStatus

/**
 * The decisions the conference list makes on its own (#2896 §2): which time a
 * row describes itself by, what the retention box is holding, and whether the
 * schedule form is fit to send.
 */
class ConferencesTest {
    private fun conf(
        status: String,
        scheduledAt: String? = null,
        startedAt: String? = null,
        endedAt: String? = null,
    ) = Conference(
        id = "c1",
        title = "Standup",
        status = status,
        scheduledAt = scheduledAt,
        startedAt = startedAt,
        endedAt = endedAt,
    )

    @Test
    fun `a finished call is described by its end`() {
        val line = conferenceTimeLine(
            conf(
                ConferenceStatus.ENDED,
                scheduledAt = "2026-09-05T09:00:00Z",
                startedAt = "2026-09-05T09:02:00Z",
                endedAt = "2026-09-05T09:30:00Z",
            ),
        )
        assertThat(line).isEqualTo(ConfTimeLine(ConfTimeKind.ENDED, "2026-09-05T09:30:00Z"))
    }

    @Test
    fun `a live call is described by its start`() {
        val line = conferenceTimeLine(
            conf(
                ConferenceStatus.LIVE,
                scheduledAt = "2026-09-05T09:00:00Z",
                startedAt = "2026-09-05T09:02:00Z",
            ),
        )
        assertThat(line).isEqualTo(ConfTimeLine(ConfTimeKind.STARTED, "2026-09-05T09:02:00Z"))
    }

    /**
     * The regression this whole helper exists for: a reusable room (#2879) keeps
     * `started_at` from yesterday while sitting back in `scheduled`. Reading the
     * stamps before the status would label today's idle standup «Началась:
     * вчера» — a row that is wrong every morning and never crashes.
     */
    @Test
    fun `a paused room shows its plan, not yesterday's start`() {
        val line = conferenceTimeLine(
            conf(
                ConferenceStatus.SCHEDULED,
                scheduledAt = "2026-09-06T09:00:00Z",
                startedAt = "2026-09-05T09:02:00Z",
            ),
        )
        assertThat(line).isEqualTo(ConfTimeLine(ConfTimeKind.SCHEDULED, "2026-09-06T09:00:00Z"))
    }

    /** A call with no plan and no history has nothing to say about time. */
    @Test
    fun `a room with no times at all falls back to none`() {
        assertThat(conferenceTimeLine(conf(ConferenceStatus.SCHEDULED)).kind).isEqualTo(ConfTimeKind.NONE)
    }

    /** An ended call whose end stamp is missing still has its plan to show. */
    @Test
    fun `an ended call without an end stamp falls through to its plan`() {
        val line = conferenceTimeLine(conf(ConferenceStatus.ENDED, scheduledAt = "2026-09-05T09:00:00Z"))
        assertThat(line).isEqualTo(ConfTimeLine(ConfTimeKind.SCHEDULED, "2026-09-05T09:00:00Z"))
    }

    // ── retention box ─────────────────────────────────────────────────────

    @Test
    fun `an empty box means the server's own default`() {
        assertThat(parseTtlDays("")).isEqualTo(TtlInput.Days(RECORDING_TTL_DEFAULT))
        assertThat(parseTtlDays("   ")).isEqualTo(TtlInput.Days(RECORDING_TTL_DEFAULT))
    }

    @Test
    fun `zero is keep-indefinitely, not expire-immediately`() {
        assertThat(parseTtlDays("0")).isEqualTo(TtlInput.Days(0))
    }

    @Test
    fun `a number is taken as written`() {
        assertThat(parseTtlDays(" 90 ")).isEqualTo(TtlInput.Days(90))
    }

    /** A typo must not silently become the default — «3O» and 30 differ only
     *  once the recording the user wanted is already gone. */
    @Test
    fun `a typo is refused rather than defaulted`() {
        assertThat(parseTtlDays("3O")).isEqualTo(TtlInput.Invalid)
        assertThat(parseTtlDays("-1")).isEqualTo(TtlInput.Invalid)
        assertThat(parseTtlDays("99999999999")).isEqualTo(TtlInput.Invalid)
    }

    // ── the schedule form as a whole ──────────────────────────────────────
    //
    // These stand in for a spec driving the rendered dialog, which this
    // environment cannot run: its text fields keep the composition from ever
    // reaching idle and even `setContent` dies on `AppNotIdleException`. Since
    // the dialog's submit button is both enabled by [conferenceDraft] and sends
    // exactly what it returns, the two cannot disagree — which is the part a
    // rendered spec would have been guarding.

    @Test
    fun `a nameless form is not submittable`() {
        assertThat(conferenceDraft("", "", null, "30")).isNull()
        assertThat(conferenceDraft("   ", "", null, "30")).isNull()
    }

    @Test
    fun `a named form sends its trimmed title and the retention on screen`() {
        assertThat(conferenceDraft("  Standup  ", "  ежедневная  ", null, "30"))
            .isEqualTo(ConferenceDraft("Standup", "ежедневная", null, 30))
    }

    /** A retention the backend would answer 400 to blocks the button, so the
     *  user gets the hint under the box instead of an error toast. */
    @Test
    fun `a refused retention blocks the whole form`() {
        assertThat(conferenceDraft("Standup", "", null, "-7")).isNull()
        assertThat(conferenceDraft("Standup", "", null, "0"))
            .isEqualTo(ConferenceDraft("Standup", "", null, 0))
    }

    /** An empty picker means «start it whenever», and travels as null rather
     *  than as a blank string the backend would reject. */
    @Test
    fun `a blank start time travels as no start time`() {
        assertThat(conferenceDraft("Standup", "", "   ", "30")?.scheduledAtIso).isNull()
        assertThat(conferenceDraft("Standup", "", "2026-09-05T09:00:00Z", "30")?.scheduledAtIso)
            .isEqualTo("2026-09-05T09:00:00Z")
    }
}
