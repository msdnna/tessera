package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.GitlabMember
import website.msdnna.tessera.data.model.Member

/** The roster behind @-mentions: what the picker inserts, and who a tapped chip
 *  resolves to (port of the web `utils/mentions.js`). */
@RunWith(RobolectricTestRunner::class)
class MentionsTest {
    private fun res(language: String): Resources =
        ApplicationProvider.getApplicationContext<Context>().withLanguage(language).resources

    private val member = Member(userId = "u1", role = "admin", email = "e@p.dev", name = "Евгений Полянский")
    private val linked = member.copy(glUsername = "e.polyansky")

    @Test
    fun `member without gitlab inserts their name`() {
        val items = buildMentionItems(listOf(member), emptyList())
        assertEquals(1, items.size)
        assertEquals("Евгений Полянский", items[0].insert)
        assertEquals("Евгений Полянский", items[0].display)
        assertEquals("", items[0].hint)
        assertNull(items[0].username)
    }

    @Test
    fun `linked member inserts the gitlab login and keeps the readable name`() {
        val items = buildMentionItems(listOf(linked), emptyList())
        assertEquals("e.polyansky", items[0].insert)
        assertEquals("Евгений Полянский", items[0].display)
        assertEquals("@e.polyansky", items[0].hint)
        assertEquals("u1", items[0].avatarUserId)
    }

    @Test
    fun `a gitlab roster row lends its login to the member it maps to`() {
        val gl = GitlabMember(glUserId = 7, glUsername = "e.polyansky", glName = "Evgeny", tesseraUserId = "u1")
        val items = buildMentionItems(listOf(member), listOf(gl))
        // Folded into the member — one row, not two.
        assertEquals(1, items.size)
        assertEquals("e.polyansky", items[0].insert)
        assertEquals("Евгений Полянский", items[0].display)
        assertTrue(!items[0].gitlab)
    }

    @Test
    fun `a gitlab user with no tessera account is listed on their own`() {
        val gl = GitlabMember(glUserId = 8, glUsername = "outsider", glName = "Out Sider")
        val items = buildMentionItems(listOf(member), listOf(gl))
        assertEquals(2, items.size)
        val only = items[1]
        assertEquals("outsider", only.insert)
        assertEquals("Out Sider", only.display)
        assertTrue(only.gitlab)
        assertNull(only.avatarUserId)
    }

    @Test
    fun `a gitlab row pointing at a stranger is not folded into anybody`() {
        val gl = GitlabMember(glUserId = 9, glUsername = "elsewhere", glName = "", tesseraUserId = "u-other")
        val items = buildMentionItems(listOf(member), listOf(gl))
        assertEquals(2, items.size)
        // No gl_name — the login stands in for the display name.
        assertEquals("elsewhere", items[1].display)
    }

    @Test
    fun `a chip resolves by login, by inserted text and by display name`() {
        val items = buildMentionItems(listOf(linked), listOf(GitlabMember(glUserId = 8, glUsername = "outsider")))
        assertEquals("u1", resolveMention(items, "e.polyansky")?.avatarUserId)
        // A chip written before the account was linked still carries the name.
        assertEquals("u1", resolveMention(items, "Евгений Полянский")?.avatarUserId)
        // The leading @ and case are both tolerated.
        assertEquals("u1", resolveMention(items, "@E.Polyansky")?.avatarUserId)
        assertTrue(resolveMention(items, "outsider")!!.gitlab)
    }

    @Test
    fun `a handle nobody owns resolves to nothing`() {
        val items = buildMentionItems(listOf(linked), emptyList())
        assertNull(resolveMention(items, "root"))
        assertNull(resolveMention(items, "  "))
    }

    /**
     * Кого уведомит комментарий — должно совпадать с тем, чей чип нарисован.
     *
     * Таблица снята с самого рендерера: разметка прогнана через marked 18 и через
     * дословную копию `highlightMentions`/`replaceOutsideCode` из `RichContent.kt`,
     * ожидания здесь — его чипы. Расхождение опасно в обе стороны: «@Иван» в
     * примере кода нарисован текстом и никого не зовёт, а имя, набранное руками,
     * рисует чип и обязано позвать.
     */
    private val roster = listOf(
        Member(userId = "u1", name = "Иван"),
        Member(userId = "u2", name = "Иван Петров"),
        Member(userId = "u3", name = "root"),
        Member(userId = "u4", name = "Ян"),
        linked.copy(userId = "u5"),
    )

    private fun ids(src: String, gl: List<GitlabMember> = emptyList()) =
        mentionIdsIn(src, buildMentionItems(roster, gl))

    @Test
    fun `mentions are read the way the renderer draws them`() {
        val table = listOf(
            // текст → кого зовём
            "Привет @Иван, глянь" to listOf("u1"),
            "@Иван глянь" to listOf("u1"),
            "```\n@Иван\n```" to emptyList(),
            "```sh\nsudo -u @root ls\n```" to emptyList(),
            "команда `@Иван` тут" to emptyList(),
            "    @Иван\n" to emptyList(),
            "(@Иван) глянь" to listOf("u1"),
            "*@Иван* глянь" to listOf("u1"),
            "**@Иван** глянь" to listOf("u1"),
            "@Иван, глянь" to listOf("u1"),
            "mail@root тут" to emptyList(),
            "@Янис привет" to listOf("u4"),
            "@Иван Петров глянь" to listOf("u2"),
            "> @Иван глянь" to listOf("u1"),
            "- @Иван глянь" to listOf("u1"),
            "[@Иван](http://x) глянь" to listOf("u1"),
            "[тут](http://x/@Иван) глянь" to emptyList(),
            "@e.polyansky глянь" to listOf("u5"),
            "@Иван и снова @Иван" to listOf("u1"),
            "# @Иван глянь" to listOf("u1"),
            "| кто |\n| --- |\n| @Иван |" to listOf("u1"),
            "строка\n@Иван глянь" to listOf("u1"),
            "<b>@Иван</b> глянь" to listOf("u1"),
            "_@Иван_ глянь" to listOf("u1"),
            "~~@Иван~~ глянь" to listOf("u1"),
            "| кто |\n| --- |\n|@Иван|" to listOf("u1"),
            "mail_@root тут" to emptyList(),
            "«@Иван» глянь" to emptyList(),
            "\"@Иван\" глянь" to emptyList(),
            "-@Иван глянь" to emptyList(),
            "/@Иван глянь" to emptyList(),
            "@@Иван глянь" to emptyList(),
            "кому:@Иван глянь" to emptyList(),
            "строка\n\n@Иван глянь" to listOf("u1"),
            ">@Иван глянь" to listOf("u1"),
            "-   @Иван глянь" to listOf("u1"),
            "http://x/@Иван глянь" to emptyList(),
            ".@Иван глянь" to emptyList(),
            "<http://x/@Иван> глянь" to emptyList(),
            "\\@Иван глянь" to listOf("u1"),
            "строка\n    @Иван глянь" to listOf("u1"),
            "строка\n\n    @Иван глянь" to emptyList(),
            "- пункт\n\n    @Иван глянь" to listOf("u1"),
            "- пункт\n\n        @Иван глянь" to emptyList(),
            "```\nкод\n```\n@Иван глянь" to listOf("u1"),
            "~~~\n@Иван\n~~~" to emptyList(),
            "```\n@Иван глянь" to emptyList(),
            "``@Иван`` глянь" to emptyList(),
            "`@Иван глянь" to emptyList(),
            "> ```\n> @Иван\n> ```" to emptyList(),
        )
        table.forEach { (src, expected) ->
            assertEquals("разметка: ${src.replace("\n", "\\n")}", expected, ids(src))
        }
    }

    /** Порядок — как в тексте, повторы схлопнуты: список уходит на сервер как есть. */
    @Test
    fun `mentions keep the order they are written in, without repeats`() {
        assertEquals(listOf("u2", "u1", "u4"), ids("@Иван Петров, @Иван и @Ян — и снова @Иван"))
    }

    /**
     * Логин из ростера GitLab — то, что вставляет пикер и что рисует чип, даже
     * когда на строке участника своего `gl_username` нет. Раньше уведомление
     * искало только имя и `gl_username` участника, так что чип был, а звонка не было.
     */
    @Test
    fun `a login borrowed from the gitlab roster notifies its owner`() {
        val gl = GitlabMember(glUserId = 7, glUsername = "i.petrov", glName = "Ivan", tesseraUserId = "u2")
        assertEquals(listOf("u2"), ids("@i.petrov глянь", listOf(gl)))
    }

    /** У гостя из GitLab нет аккаунта в Tessera — звать некого, текст остаётся ему. */
    @Test
    fun `a gitlab-only handle notifies nobody`() {
        val gl = GitlabMember(glUserId = 8, glUsername = "outsider", glName = "Out Sider")
        assertEquals(emptyList<String>(), ids("@outsider глянь", listOf(gl)))
    }

    @Test
    fun `roles are spelled out, unknown ones kept verbatim`() {
        val ru = res("ru")
        assertEquals("Владелец", roleLabel(ru, "owner"))
        assertEquals("Админ", roleLabel(ru, "admin"))
        assertEquals("Участник", roleLabel(ru, "member"))
        assertEquals("guest", roleLabel(ru, "guest"))
        assertEquals("", roleLabel(ru, null))
        assertEquals("Owner", roleLabel(res("en"), "owner"))
    }
}
