package website.msdnna.tessera.util

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import website.msdnna.tessera.data.model.CommandDef
import website.msdnna.tessera.data.model.WorkspaceCommand

/** Quick-action editor helpers (web `utils/commands.js` parity). */
class CommandsTest {
    private val builtin = listOf(
        CommandDef(key = "assign", aliases = listOf("assignee"), arg = "user", description = "Назначить исполнителя"),
        CommandDef(key = "due", aliases = listOf("due_date"), arg = "date", description = "Установить срок"),
        CommandDef(key = "close", aliases = listOf("done"), arg = "none", description = "Закрыть задачу"),
    )
    private val custom = listOf(WorkspaceCommand("approve", "Одобрить план"))
    private val items = commandItems(builtin, custom)

    @Test
    fun `a key is canonicalised to its storage form`() {
        assertEquals("approve", canonCommandKey(" /Approve "))
        assertEquals("approve", canonCommandKey("//approve"))
        assertEquals("", canonCommandKey(null))
        assertEquals("", canonCommandKey("  /  "))
    }

    @Test
    fun `key validation mirrors the backend regex`() {
        assertTrue(isValidCommandKey("approve"))
        assertTrue(isValidCommandKey("/Re-Check_2"))
        // Must start with a letter or digit, latin only, 32 chars max.
        assertFalse(isValidCommandKey("-approve"))
        assertFalse(isValidCommandKey("одобрить"))
        assertFalse(isValidCommandKey("a".repeat(33)))
        assertFalse(isValidCommandKey(""))
    }

    @Test
    fun `the slash trigger only fires at the start of a line`() {
        assertEquals(SlashQuery(start = 0, query = ""), detectSlashQuery("/"))
        assertEquals(SlashQuery(start = 6, query = "clo"), detectSlashQuery("текст\n/clo"))
        // Paths, dates and ratios are ordinary task text — the popup must stay shut.
        assertNull(detectSlashQuery("cd /home"))
        assertNull(detectSlashQuery("src/utils"))
        assertNull(detectSlashQuery("24/7"))
        // Only an open query counts: anything after the word ends it.
        assertNull(detectSlashQuery("/close done"))
    }

    @Test
    fun `built-ins come first, custom entries after`() {
        assertEquals(listOf("assign", "due", "close", "approve"), items.map { it.key })
        assertTrue(items.first { it.key == "close" }.builtin)
        assertFalse(items.last().builtin)
        // A custom entry never takes an argument, and its example is its own key.
        assertEquals("none", items.last().arg)
        assertEquals("/approve", items.last().example)
    }

    @Test
    fun `matching spans key, aliases and description`() {
        assertEquals(listOf("assign"), matchCommands(items, "assignee").map { it.key })
        assertEquals(listOf("due"), matchCommands(items, "срок").map { it.key })
        assertEquals(listOf("close"), matchCommands(items, "DONE").map { it.key })
        // An empty query lists everything, capped at the limit.
        assertEquals(4, matchCommands(items, "").size)
        assertEquals(2, matchCommands(items, null, limit = 2).size)
        assertTrue(matchCommands(items, "неттакого").isEmpty())
    }

    @Test
    fun `insert text leaves the caret where the next input belongs`() {
        // Argument commands wait for it after a space; argument-less ones end the
        // line so the next command can follow immediately.
        assertEquals("/assign ", commandInsertText(items.first { it.key == "assign" }))
        assertEquals("/close\n", commandInsertText(items.first { it.key == "close" }))
        // Custom entries are plain text, never arguments — even if a built-in
        // shares the argument kind name.
        assertEquals("/approve\n", commandInsertText(items.last()))
        assertEquals("", commandInsertText(null))
    }

    @Test
    fun `the preview gate spots a command line anywhere in the body`() {
        assertTrue(hasCommandLine("/close"))
        assertTrue(hasCommandLine("Готово\n  /close"))
        assertFalse(hasCommandLine("посмотри src/utils"))
        assertFalse(hasCommandLine("/2026 — не команда"))
        assertFalse(hasCommandLine(""))
        assertFalse(hasCommandLine(null))
    }
}
