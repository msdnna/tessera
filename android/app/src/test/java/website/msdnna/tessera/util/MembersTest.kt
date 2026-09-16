package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.Member

/**
 * Правила диалога участников (#2932) — зеркало веб-`utils/members.js`.
 *
 * Сервер применяет отказы в фиксированном порядке: сначала `requireManager`
 * (он вообще не смотрит, чья это строка), затем владелец пространства, затем
 * сам вызывающий. Порядок здесь тот же, и проверяется он именно как порядок:
 * рядовой участник, смотрящий на собственную строку, должен получить VIEWER, а
 * владелец на своей — OWNER, а не SELF.
 */
class MembersTest {
    private val owner = Member(userId = "u-owner", role = "owner", name = "Оля")
    private val admin = Member(userId = "u-admin", role = "admin", name = "Аня")
    private val plain = Member(userId = "u-plain", role = "member", name = "Петя")
    private val roster = listOf(owner, admin, plain)

    @Test
    fun `владелец и админ проходят requireManager, участник — нет`() {
        assertThat(canManageMembers(roster, "u-owner")).isTrue()
        assertThat(canManageMembers(roster, "u-admin")).isTrue()
        assertThat(canManageMembers(roster, "u-plain")).isFalse()
    }

    @Test
    fun `чужого в ростере нет — управлять нечем`() {
        assertThat(canManageMembers(roster, "u-stranger")).isFalse()
        // Профиль ещё не прочитан: пустой id не должен совпасть с пустым userId
        // строки, иначе роль «первого попавшегося» станет моей.
        assertThat(canManageMembers(listOf(Member(userId = "", role = "owner")), "")).isFalse()
    }

    @Test
    fun `у рядового участника заперты все строки, включая свою`() {
        val locks = roster.map { memberRowLock(it, meId = "u-plain", canManage = false) }
        assertThat(locks).containsExactly(MemberLock.VIEWER, MemberLock.VIEWER, MemberLock.VIEWER)
    }

    @Test
    fun `у менеджера заперты владелец и он сам`() {
        assertThat(memberRowLock(owner, meId = "u-admin", canManage = true)).isEqualTo(MemberLock.OWNER)
        assertThat(memberRowLock(admin, meId = "u-admin", canManage = true)).isEqualTo(MemberLock.SELF)
        assertThat(memberRowLock(plain, meId = "u-admin", canManage = true)).isEqualTo(MemberLock.NONE)
    }

    @Test
    fun `владелец на своей строке слышит правило владельца`() {
        assertThat(memberRowLock(owner, meId = "u-owner", canManage = true)).isEqualTo(MemberLock.OWNER)
    }

    @Test
    fun `дрейф без гварда идёт в сторону лишней кнопки, а не спрятанной`() {
        assertThat(memberRowLock(plain)).isEqualTo(MemberLock.NONE)
        assertThat(canChangeMemberRole(plain)).isTrue()
        assertThat(canRemoveMember(plain)).isTrue()
    }

    @Test
    fun `строка называет себя, даже когда заперта`() {
        // Блокировка у рядового участника на всех строках одна, а подпись —
        // нет: иначе ростер из четырёх «Участник» скажет читателю меньше, чем
        // говорил до фикса.
        assertThat(memberRowBadge(owner, "u-plain")).isEqualTo(MemberBadge.OWNER)
        assertThat(memberRowBadge(plain, "u-plain")).isEqualTo(MemberBadge.YOU)
        assertThat(memberRowBadge(admin, "u-plain")).isEqualTo(MemberBadge.ROLE)
    }

    @Test
    fun `владелец, смотрящий на себя, остаётся владельцем, а не «это вы»`() {
        assertThat(memberRowBadge(owner, "u-owner")).isEqualTo(MemberBadge.OWNER)
    }

    @Test
    fun `роль и корзина живут по одному и тому же правилу`() {
        for (m in roster) {
            val lock = memberRowLock(m, meId = "u-admin", canManage = true)
            assertThat(canChangeMemberRole(m, "u-admin", true)).isEqualTo(lock == MemberLock.NONE)
            assertThat(canRemoveMember(m, "u-admin", true)).isEqualTo(lock == MemberLock.NONE)
        }
    }

    @Test
    fun `срок приглашения решает, живая строка или мёртвая`() {
        // Граница взята у веба (`until.getTime() <= Date.now()`), а не у сервера:
        // тот отвергает токен строго ПОСЛЕ `expires_at`, но расхождение в один
        // тик подписи не касается, а совпасть с соседним клиентом она обязана.
        val now = 1_760_000_000_000L
        assertThat(inviteExpired(millisToUtcIso(now - 60_000), now)).isTrue()
        assertThat(inviteExpired(millisToUtcIso(now), now)).isTrue()
        assertThat(inviteExpired(millisToUtcIso(now + 60_000), now)).isFalse()
    }

    @Test
    fun `нечитаемый срок не объявляют истёкшим`() {
        // Пустая и битая строка дают «не истекло» — подпись тогда не рисуется
        // вовсе. Иначе одно только отсутствие даты гасило бы живое приглашение.
        assertThat(inviteExpired(null)).isFalse()
        assertThat(inviteExpired("")).isFalse()
        assertThat(inviteExpired("2026-09-23")).isFalse()
    }
}
