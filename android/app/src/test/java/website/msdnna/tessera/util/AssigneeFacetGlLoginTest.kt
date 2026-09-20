package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.GitlabMember
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.Task

/**
 * «Исполнитель» facet vs GitLab logins — the port of `frontend/tests/ut-assignee-board-gl-login.spec.js`.
 *
 * Two halves of one hole, both of which web plugged first:
 *  - [matchesAssignee] did not resolve a Tessera person through their LINKED GitLab login,
 *    so a card carrying that login (and not their user id) matched nothing;
 *  - the menu offered only the GitLab member ROSTER, which the backend refreshes at most
 *    once an hour, while issues arrive on every pull — so a login the cards already draw
 *    had no entry to select it with.
 *
 * Together they meant a linked person's synced cards answered to NO value of the facet:
 * not to their own entry (the login is not their uuid) and not to a `gl:` one (the menu
 * drops a roster row that maps to a workspace member, so one person isn't listed twice).
 */
class AssigneeFacetGlLoginTest {
    private val clock = FilterClock(today = "2026-08-03", weekEnd = "2026-08-10")

    // Аня — участница пространства с привязанным GitLab-аккаунтом `anya`.
    private val anya = Member(userId = "u1", name = "Аня")
    private val linked = mapOf("u1" to "anya")

    // Её карточка приехала из GitLab: логин в gitlab_assignee_logins, uuid'а нет.
    private val synced = Task(
        id = "t1",
        title = "Синхронизированная",
        number = 10,
        columnId = "c1",
        gitlabAssigneeLogins = listOf("anya"),
    )

    // ── предикат ──

    @Test
    fun `a synced card matches its tessera assignee through the linked login`() {
        assertTrue(matchesAssignee(synced, setOf("u1"), linked))
        // Без карты связей — прежний мир: её собственная запись не находит карточку.
        assertFalse(matchesAssignee(synced, setOf("u1")))
        // …а голое `gl:`-значение матчит всегда.
        assertTrue(matchesAssignee(synced, setOf("gl:anya")))
    }

    @Test
    fun `the linked login does not make a stranger's card match`() {
        val other = synced.copy(gitlabAssigneeLogins = listOf("ivan"))
        assertFalse(matchesAssignee(other, setOf("u1"), linked))
        // Настоящее назначение по uuid продолжает работать и с картой.
        assertTrue(matchesAssignee(other.copy(assigneeIds = listOf("u1")), setOf("u1"), linked))
        // Пустой выбор ничего не фильтрует.
        assertTrue(matchesAssignee(other, emptySet(), linked))
    }

    @Test
    fun `the whole board pass carries the link map into the assignee facet`() {
        val r = filterBoardTasks(listOf(synced), emptyMap(), BoardFilter(assigneeIds = setOf("u1")), clock, linked)
        assertEquals(setOf("t1"), r.visibleIds)
        // Без карты карточка с доски пропадает — ровно та регрессия.
        assertTrue(
            filterBoardTasks(listOf(synced), emptyMap(), BoardFilter(assigneeIds = setOf("u1")), clock).visibleIds.isEmpty(),
        )
    }

    @Test
    fun `a synced subtask lifts its parent through the linked login`() {
        val parent = Task(id = "p1", title = "Родитель", number = 9, columnId = "c1")
        val child = synced.copy(id = "s1", parentId = "p1")
        val r = filterBoardTasks(
            listOf(parent),
            mapOf("p1" to listOf(child)),
            BoardFilter(assigneeIds = setOf("u1")),
            clock,
            linked,
        )
        assertEquals(setOf("p1"), r.visibleIds)
        assertEquals(listOf("s1"), r.subtasksByParent["p1"]?.map { it.id })
    }

    // ── логины, которых нет в ростере ──

    @Test
    fun `boardGitlabAssignees collects distinct logins across the board, sorted`() {
        val got = boardGitlabAssignees(
            listOf(
                Task(id = "1", gitlabAssigneeLogins = listOf("ivan", "anya")),
                Task(id = "2", gitlabAssigneeLogins = listOf("ivan")),
                Task(id = "3"),
            ),
        )
        assertEquals(listOf("anya", "ivan"), got.map { it.username })
        // Отображать нечего, кроме логина — имени у назначенного из GitLab на задаче нет.
        assertEquals(listOf("anya", "ivan"), got.map { it.name })
        assertEquals(listOf("", ""), got.map { it.avatarUrl })
    }

    @Test
    fun `boardGitlabAssignees drops blanks and ignores the legacy name-only field`() {
        assertEquals(emptyList<GitlabAuthor>(), boardGitlabAssignees(listOf(Task(id = "1", gitlabAssigneeLogins = listOf("", " ")))))
        // `gitlabAssignees` — отображаемые имена, а фасет матчит логины: запись из имени
        // не выбрала бы ничего, поэтому пунктом меню становиться не должна.
        assertEquals(
            emptyList<GitlabAuthor>(),
            boardGitlabAssignees(listOf(Task(id = "1", gitlabAssignees = listOf("Иван П.")))),
        )
        assertEquals(emptyList<GitlabAuthor>(), boardGitlabAssignees(emptyList()))
    }

    // ── меню фасета ──

    private fun menu(
        members: List<Member> = emptyList(),
        roster: List<GitlabMember> = emptyList(),
        tasks: List<Task> = emptyList(),
        glLoginByUserId: Map<String, String> = emptyMap(),
        selected: Set<String> = emptySet(),
    ) = assigneeFacetGitlabOptions(members, roster, tasks, glLoginByUserId, selected)

    @Test
    fun `a board-seen login is offered even when the roster is empty`() {
        val newcomer = Task(id = "t1", gitlabAssigneeLogins = listOf("newcomer"))
        assertEquals(listOf("newcomer"), menu(tasks = listOf(newcomer)).map { it.username })
        // И значение, которое меню выдаст, предикат действительно матчит.
        assertTrue(matchesAssignee(newcomer, setOf("gl:newcomer")))
    }

    @Test
    fun `a login the roster already carries is not duplicated`() {
        val opts = menu(
            roster = listOf(GitlabMember(glUsername = "ivan", glName = "Иван П.")),
            tasks = listOf(Task(id = "t1", gitlabAssigneeLogins = listOf("ivan", "newcomer"))),
        )
        assertEquals(listOf("ivan", "newcomer"), opts.map { it.username })
        // У ростерной записи есть имя, у найденной на доске — только логин.
        assertEquals(listOf("Иван П.", "newcomer"), opts.map { it.name })
    }

    @Test
    fun `a linked workspace member is skipped — their tessera entry already matches`() {
        assertEquals(
            emptyList<GitlabAuthor>(),
            menu(
                members = listOf(anya),
                roster = listOf(GitlabMember(glUsername = "anya", glName = "Аня", tesseraUserId = "u1")),
                tasks = listOf(synced),
                glLoginByUserId = linked,
            ),
        )
    }

    @Test
    fun `a roster member linked to a non-member of this workspace is kept`() {
        // glLoginByUserId строится по всему ростеру, а не по участникам пространства:
        // сняв весь его разом, мы бы молча выкинули человека, которому на этой доске
        // не на какую tessera-запись опереться.
        assertEquals(
            listOf("ivan"),
            menu(
                roster = listOf(GitlabMember(glUsername = "ivan", glName = "Иван П.", tesseraUserId = "uOther")),
                glLoginByUserId = mapOf("uOther" to "ivan"),
            ).map { it.username },
        )
    }

    @Test
    fun `an already picked login is not offered again`() {
        assertEquals(
            listOf("newcomer"),
            menu(
                roster = listOf(GitlabMember(glUsername = "ivan", glName = "Иван П.")),
                tasks = listOf(Task(id = "t1", gitlabAssigneeLogins = listOf("ivan", "newcomer"))),
                selected = setOf("gl:ivan"),
            ).map { it.username },
        )
    }

    @Test
    fun `a board with no gitlab people at all leaves the bucket empty`() {
        assertEquals(
            emptyList<GitlabAuthor>(),
            menu(members = listOf(anya), tasks = listOf(Task(id = "t1", assigneeIds = listOf("u1")))),
        )
    }
}
