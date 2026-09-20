package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.GitlabMember
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.Task

/**
 * «Автор» facet vs a GitLab person who is not a member of THIS workspace — the port of
 * `frontend/tests/cx-author-facet-nonmember-login.spec.js`.
 *
 * A synced issue carries its author as a bare login (`gitlab_author`, `created_by` null), so
 * the facet offers a Tessera user id for a workspace member and `gl:<login>` for everyone
 * else. A login a linked Tessera row already covers is left out of the GitLab bucket, or one
 * person is listed twice — their own entry matches the login through `glLoginByUserId`.
 *
 * That skip set used to be the WHOLE of `glLoginByUserId` (#2932), and the map is the GitLab
 * *project* roster keyed by Tessera user id: the backend resolves those ids by joining
 * `oauth_identities` / `gitlab_credentials` on the login alone, with no membership check. So
 * it also covers accounts that are not members here — someone who linked GitLab in another
 * workspace, or who has since left this one — and those got no entry at all: no Tessera row
 * (not in the member list) and no `gl:` row either, while their issues answered a `gl:` value
 * perfectly well. Exactly the hole closed one facet over for «Исполнитель».
 */
class AuthorFacetNonMemberTest {
    // Аня — участница пространства с привязанным GitLab-аккаунтом `anya`.
    private val anya = Member(userId = "u1", name = "Аня")

    // Петя привязал GitLab, но в ЭТОМ пространстве не состоит: ростер знает его
    // tessera_user_id (джойн по логину, без проверки членства), а списка участников — нет.
    private val linked = mapOf("u1" to "anya", "u9" to "petya")

    private fun glMember(login: String, name: String = login) =
        GitlabMember(glUsername = login, glName = name)

    private fun issueBy(login: String, id: String = "t-$login") = Task(
        id = id,
        title = "Issue by $login",
        columnId = "c1",
        gitlabAuthor = login,
    )

    private fun options(
        members: List<Member> = listOf(anya),
        roster: List<GitlabMember> = emptyList(),
        tasks: List<Task> = emptyList(),
        selected: Set<String> = emptySet(),
    ) = authorFacetGitlabOptions(members, roster, tasks, linked, selected).map { it.username }

    // ── находка ──

    @Test
    fun `a roster login whose account is not a member here is offered`() {
        assertEquals(listOf("petya"), options(roster = listOf(glMember("petya", "Петя"))))
    }

    @Test
    fun `a board-only author whose account is not a member here is offered`() {
        // Ростер — часовой кэш, так что автора может не быть и в нём: тогда логин
        // известен только по карточкам (boardGitlabAuthors).
        assertEquals(listOf("petya"), options(tasks = listOf(issueBy("petya"))))
    }

    @Test
    fun `the display name comes from the card when the roster has nothing`() {
        val named = issueBy("petya").copy(gitlabAuthorName = "Петя П.")
        val opts = authorFacetGitlabOptions(listOf(anya), emptyList(), listOf(named), linked, emptySet())
        assertEquals(listOf("Петя П."), opts.map { it.name })
    }

    // ── контроли: правило, ради которого пропуск и существует ──

    @Test
    fun `a linked member is still listed once, not twice`() {
        assertEquals(
            emptyList<String>(),
            options(roster = listOf(glMember("anya", "Аня")), tasks = listOf(issueBy("anya"))),
        )
    }

    @Test
    fun `a login carried by both the roster and the board is offered once`() {
        assertEquals(
            listOf("petya"),
            options(
                roster = listOf(glMember("petya", "Петя")),
                tasks = listOf(issueBy("petya"), issueBy("petya", id = "t2")),
            ),
        )
    }

    @Test
    fun `an active chip is not offered again`() {
        assertEquals(
            emptyList<String>(),
            options(roster = listOf(glMember("petya")), selected = setOf("gl:petya")),
        )
    }

    @Test
    fun `a blank login is never an entry`() {
        assertEquals(emptyList<String>(), options(roster = listOf(glMember("", ""))))
    }

    @Test
    fun `an empty workspace roster skips nothing`() {
        // Без участников пропускать некого: обе записи должны доехать до меню.
        assertEquals(
            listOf("anya", "petya"),
            options(members = emptyList(), roster = listOf(glMember("anya"), glMember("petya"))),
        )
    }

    // ── вторая половина фасета: выбираемое значение должно и фильтровать ──

    @Test
    fun `the offered value is the one that matches the card`() {
        val issue = issueBy("petya")
        assertTrue(matchesAuthor(issue, setOf("gl:petya")))
        // uuid не-участника алиасом логина не становится: карта — ростер, а не список
        // членства, так что матч через неё нарисовал бы карточку человеку, которого
        // эта доска и назвать-то не может.
        assertFalse(matchesAuthor(issue, setOf("u1"), linked))
    }

    @Test
    fun `the assignee facet keeps following the same rule`() {
        // Оба фасета ходят через один helper; у исполнителя правило было верным — оно
        // не должно было поехать.
        val opts = assigneeFacetGitlabOptions(
            listOf(anya),
            listOf(glMember("anya", "Аня"), glMember("petya", "Петя")),
            emptyList(),
            linked,
            emptySet(),
        )
        assertEquals(listOf("petya"), opts.map { it.username })
    }
}
