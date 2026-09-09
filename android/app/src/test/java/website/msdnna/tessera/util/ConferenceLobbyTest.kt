package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Conference
import website.msdnna.tessera.data.model.ConferenceParticipant
import website.msdnna.tessera.data.model.ConferenceStatus
import website.msdnna.tessera.data.model.Member

/**
 * The lobby's rules (#2896 §3). Every control on that screen is one of these
 * answers, so they are checked here rather than through the rendered lobby —
 * which needs a repository, a socket and a workspace before it says anything.
 */
class ConferenceLobbyTest {
    private val me = "u-me"

    private fun conference(
        status: String = ConferenceStatus.LIVE,
        createdBy: String? = "u-other",
    ) = Conference(id = "c1", workspaceId = "w1", status = status, createdBy = createdBy)

    private fun seat(
        userId: String,
        role: String = "member",
        joined: String? = null,
        left: String? = null,
        name: String? = null,
    ) = ConferenceParticipant(
        conferenceId = "c1",
        userId = userId,
        role = role,
        invitedAt = "2026-09-05T08:00:00Z",
        joinedAt = joined,
        leftAt = left,
        userName = name,
    )

    private fun member(userId: String, name: String = userId) =
        Member(userId = userId, role = "member", email = "$userId@t.local", name = name)

    // ── presence ──────────────────────────────────────────────────────────

    @Test
    fun `a seat with an open join is presence`() {
        val roster = listOf(seat(me, joined = "2026-09-05T09:00:00Z"))
        assertThat(isInConfRoom(roster, me)).isTrue()
    }

    /**
     * The one that decides which button the screen shows. A row left over from
     * an earlier session has both stamps set; reading `joined_at` alone would
     * offer «Выйти» to somebody who has not been in the call since Tuesday, and
     * hide the join button that is the only way back in.
     */
    @Test
    fun `a seat left behind by an earlier session is not presence`() {
        val roster = listOf(seat(me, joined = "2026-09-04T09:00:00Z", left = "2026-09-04T09:40:00Z"))
        assertThat(isInConfRoom(roster, me)).isFalse()
    }

    @Test
    fun `an invitation nobody acted on is not presence`() {
        assertThat(isInConfRoom(listOf(seat(me)), me)).isFalse()
    }

    /** Signed out, or a roster that has not loaded — no blank id matches a row. */
    @Test
    fun `a blank identity matches nobody`() {
        assertThat(confSeat(listOf(seat("")), "")).isNull()
        assertThat(isInConfRoom(listOf(seat("")), "")).isFalse()
    }

    // ── moderation ────────────────────────────────────────────────────────

    @Test
    fun `the creator may end their own call`() {
        val conf = conference(createdBy = me)
        assertThat(canModerateConference(conf, emptyList(), me, "member")).isTrue()
    }

    @Test
    fun `a host inside the call may end it`() {
        val roster = listOf(seat(me, role = "host", joined = "2026-09-05T09:00:00Z"))
        assertThat(canModerateConference(conference(), roster, me, "member")).isTrue()
    }

    @Test
    fun `a workspace owner or admin may end somebody else's call`() {
        assertThat(canModerateConference(conference(), emptyList(), me, "owner")).isTrue()
        assertThat(canModerateConference(conference(), emptyList(), me, "admin")).isTrue()
    }

    /**
     * The gate itself: a plain member who is merely *in* the call must not be
     * offered «Завершить». The server answers 403, so a button here would only
     * ever produce a bug report.
     */
    @Test
    fun `a plain member in the room may not end the call`() {
        val roster = listOf(seat(me, joined = "2026-09-05T09:00:00Z"))
        assertThat(canModerateConference(conference(), roster, me, "member")).isFalse()
    }

    // ── the two roster lists ──────────────────────────────────────────────

    @Test
    fun `the room lists hosts first and then by name`() {
        val roster = confRoster(
            listOf(
                seat("u2", joined = "1", name = "Яна"),
                seat("u3", joined = "1", name = "Ада"),
                seat("u1", role = "host", joined = "1", name = "Пётр"),
                seat("u4", name = "Не пришёл"),
            ),
        )
        assertThat(roster.map { it.userId }).containsExactly("u1", "u3", "u2").inOrder()
    }

    @Test
    fun `invited lists an outstanding invitation`() {
        assertThat(confInvited(listOf(seat("u2", name = "Ада"))).map { it.userId }).containsExactly("u2")
    }

    /**
     * Somebody who came and left is *not* still being called. Listing them under
     * «Приглашены» would report an invitation that nobody is ringing.
     */
    @Test
    fun `invited drops whoever attended an earlier session`() {
        val roster = listOf(seat("u2", joined = "2026-09-04T09:00:00Z", left = "2026-09-04T09:40:00Z"))
        assertThat(confInvited(roster)).isEmpty()
    }

    // ── the invite picker ─────────────────────────────────────────────────

    @Test
    fun `the picker skips whoever already holds a seat`() {
        val members = listOf(member("u1", "Ада"), member("u2", "Пётр"), member("u3", "Яна"))
        val roster = listOf(
            seat("u1", joined = "2026-09-05T09:00:00Z"),
            seat("u2"),
        )
        assertThat(confInvitable(members, roster).map { it.userId }).containsExactly("u3")
    }

    /**
     * …but offers again anyone who joined and left: re-inviting them is a fresh
     * call, which is the backend's own rule for the notification (#2875).
     */
    @Test
    fun `the picker offers somebody who joined and left again`() {
        val roster = listOf(seat("u1", joined = "2026-09-04T09:00:00Z", left = "2026-09-04T09:40:00Z"))
        assertThat(confInvitable(listOf(member("u1")), roster).map { it.userId }).containsExactly("u1")
    }

    @Test
    fun `an ended call cannot be invited to`() {
        assertThat(canInviteToConference(conference(status = ConferenceStatus.ENDED))).isFalse()
        assertThat(canInviteToConference(conference(status = ConferenceStatus.SCHEDULED))).isTrue()
    }

    // ── labels ────────────────────────────────────────────────────────────

    /** Join/leave answer the *raw* row, which carries no joined-in name. */
    @Test
    fun `a nameless row falls back to the email and then the id`() {
        assertThat(seat("u1", name = "Ада").label()).isEqualTo("Ада")
        assertThat(
            ConferenceParticipant(userId = "u1", userEmail = "a@t.local").label(),
        ).isEqualTo("a@t.local")
        assertThat(ConferenceParticipant(userId = "u1").label()).isEqualTo("u1")
    }
}
