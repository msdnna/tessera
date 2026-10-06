package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.common.truth.Truth.assertWithMessage
import org.junit.Test

class MiscUtilTest {
    /** Есть ли в строке суррогат без своей пары (такой рисуется ромбом с вопросом). */
    private fun hasLoneSurrogate(s: String): Boolean = s.indices.any { i ->
        val c = s[i]
        when {
            c.isHighSurrogate() -> i + 1 >= s.length || !s[i + 1].isLowSurrogate()
            c.isLowSurrogate() -> i == 0 || !s[i - 1].isHighSurrogate()
            else -> false
        }
    }

    // ── diffSegments ─────────────────────────────────────────────────────────
    @Test
    fun `diffSegments equal strings single unchanged segment`() {
        val segs = diffSegments("abc", "abc")
        assertThat(segs).containsExactly(DiffSegment("abc", false))
    }

    @Test
    fun `diffSegments null handled as empty`() {
        assertThat(diffSegments(null, null)).containsExactly(DiffSegment("", false))
        assertThat(diffSegments(null, "x")).containsExactly(DiffSegment("x", true))
    }

    @Test
    fun `diffSegments common prefix and suffix around changed middle`() {
        val segs = diffSegments("hello world", "hello brave world")
        assertThat(segs).containsExactly(
            DiffSegment("hello ", false),
            DiffSegment("brave ", true),
            DiffSegment("world", false),
        ).inOrder()
    }

    @Test
    fun `diffSegments pure append`() {
        val segs = diffSegments("foo", "foobar")
        assertThat(segs).containsExactly(
            DiffSegment("foo", false),
            DiffSegment("bar", true),
        ).inOrder()
    }

    @Test
    fun `diffSegments cyrillic`() {
        val segs = diffSegments("привет мир", "привет добрый мир")
        assertThat(segs.first { it.changed }.text).isEqualTo("добрый ")
    }

    // Эмодзи — суррогатная пара, а соседи одного блока делят половину: общая
    // половина не должна оказаться по другую сторону границы, иначе экран
    // рисует ромбы с вопросом вместо символа.
    @Test
    fun `diffSegments keeps a swapped emoji whole when prefix shares its high half`() {
        val segs = diffSegments("Готово 😀", "Готово 😃")
        assertThat(segs).containsExactly(
            DiffSegment("Готово ", false),
            DiffSegment("😃", true),
        ).inOrder()
    }

    @Test
    fun `diffSegments keeps a swapped emoji whole when suffix shares its low half`() {
        val segs = diffSegments("𝄞 такт", "𝔞 такт")
        assertThat(segs).containsExactly(
            DiffSegment("𝔞", true),
            DiffSegment(" такт", false),
        ).inOrder()
    }

    @Test
    fun `diffSegments never splits a segment mid-pair`() {
        for ((base, cur) in listOf("👍 ок" to "👎 ок", "😀" to "😃", "a😀b" to "a😃b")) {
            val segs = diffSegments(base, cur)
            assertWithMessage("склейка %s", cur).that(segs.joinToString("") { it.text }).isEqualTo(cur)
            for (seg in segs) {
                assertWithMessage("одинокий суррогат в %s", seg.text)
                    .that(hasLoneSurrogate(seg.text)).isFalse()
            }
        }
    }

    // ── toggleTaskMarker ─────────────────────────────────────────────────────
    @Test
    fun `toggleTaskMarker empty source unchanged`() {
        assertThat(toggleTaskMarker("", 0)).isEmpty()
    }

    @Test
    fun `toggleTaskMarker flips unchecked to checked`() {
        assertThat(toggleTaskMarker("- [ ] task", 0)).isEqualTo("- [x] task")
    }

    @Test
    fun `toggleTaskMarker flips checked to unchecked`() {
        assertThat(toggleTaskMarker("- [x] done", 0)).isEqualTo("- [ ] done")
        assertThat(toggleTaskMarker("- [X] done", 0)).isEqualTo("- [ ] done")
    }

    @Test
    fun `toggleTaskMarker targets nth marker only`() {
        val src = "- [ ] a\n- [ ] b\n- [ ] c"
        assertThat(toggleTaskMarker(src, 1)).isEqualTo("- [ ] a\n- [x] b\n- [ ] c")
    }

    @Test
    fun `toggleTaskMarker supports numbered and star markers`() {
        assertThat(toggleTaskMarker("1. [ ] x", 0)).isEqualTo("1. [x] x")
        assertThat(toggleTaskMarker("* [ ] y", 0)).isEqualTo("* [x] y")
    }

    @Test
    fun `toggleTaskMarker out-of-range index leaves source intact`() {
        val src = "- [ ] only"
        assertThat(toggleTaskMarker(src, 5)).isEqualTo(src)
    }

    @Test
    fun `toggleTaskMarker ignores non-marker lines`() {
        val src = "plain text\nno checkbox"
        assertThat(toggleTaskMarker(src, 0)).isEqualTo(src)
    }

    // `index` is the position of the tapped <input> among the boxes RichContent's
    // WebView actually drew (marked.js renders them, the page counts them off the
    // DOM) — not the index-th line that looks like a marker. The table below is the
    // mapping marked v18.0.4 (the copy in app/src/main/assets/richcontent) implies:
    // measured by flipping each candidate line and observing which rendered box
    // moved. taskMarkerLines has to reproduce it, or a tap rewrites a different line
    // than the one the finger landed on.
    private val markedTruth = listOf(
        Triple("simple", "- [ ] a\n- [x] b\n- [ ] c", listOf(0, 1, 2)),
        Triple("fenced sample is text, not a box", "- [ ] real\n\n```\n- [ ] sample\n```\n\n- [ ] second", listOf(0, 6)),
        Triple("tilde fence", "~~~\n- [ ] sample\n~~~\n- [x] real", listOf(3)),
        Triple("fence keeps its info string", "```js\n- [ ] s\n```\n- [ ] r", listOf(3)),
        Triple("unclosed fence swallows the rest", "- [ ] a\n```\n- [ ] b", listOf(0)),
        Triple("fence indented inside a list", "- [ ] parent\n  ```\n  - [ ] sample\n  ```\n- [ ] second", listOf(0, 4)),
        Triple("four-space block is code", "text\n\n    - [ ] sample\n\n- [ ] real", listOf(4)),
        Triple("a leading tab is code too", "\t- [ ] tabbed", emptyList()),
        Triple("four spaces under a list is a nested item", "- [ ] a\n\n    - [ ] four", listOf(0, 2)),
        Triple("six spaces under a list is code", "- [ ] parent\n\n      code here\n\n- [ ] second", listOf(0, 4)),
        Triple("quoted item is drawn as a box", "> - [ ] quoted\n\n- [ ] plain", listOf(0, 2)),
        Triple("quoted list", "> - [ ] q1\n> - [x] q2", listOf(0, 1)),
        Triple("no space after the bracket is literal text", "- [ ]x\n- [ ] real", listOf(1)),
        Triple("an empty item is no item", "- [ ]", emptyList()),
        Triple("nested item follows its parent", "- [ ] parent\n  - [ ] child\n- [x] sibling", listOf(0, 1, 2)),
        Triple("deep indent is still an item", "- [ ] a\n    - [ ] deep", listOf(0, 1)),
        Triple("numbered markers", "1. [ ] one\n2) [x] two", listOf(0, 1)),
        Triple("star and plus markers", "* [ ] s\n+ [x] p", listOf(0, 1)),
        Triple("upper-case tick", "- [X] up", listOf(0)),
        // A literal `- [ ]x` still opens a *list*, so four spaces under it is a
        // nested item and not code — the indent that starts code is measured from
        // the open item's content column, not from the left margin.
        Triple("four spaces under a plain item", "- item\n\n    - [ ] under", listOf(2)),
        Triple("code between literal brackets and real boxes", "- [ ]x\n\ntext\n\n    - [ ] indented\n\n- [ ] first\n- [x] second", listOf(6, 7)),
    )

    @Test
    fun `taskMarkerLines matches what marked renders`() {
        for ((name, src, expected) in markedTruth) {
            assertWithMessage(name).that(taskMarkerLines(src)).isEqualTo(expected)
        }
    }

    @Test
    fun `toggleTaskMarker flips the line the tapped box came from`() {
        // The first box in a description that documents a checklist. Counting lines
        // rewrote the sample inside the fence and left the tapped box to snap back.
        val src = "```\n- [ ] sample\n```\n\n- [ ] real"
        assertThat(toggleTaskMarker(src, 0)).isEqualTo("```\n- [ ] sample\n```\n\n- [x] real")
    }

    @Test
    fun `toggleTaskMarker skips code blocks and literal brackets`() {
        val src = "- [ ]x\n\ntext\n\n    - [ ] indented\n\n- [ ] first\n- [x] second"
        assertThat(toggleTaskMarker(src, 0))
            .isEqualTo("- [ ]x\n\ntext\n\n    - [ ] indented\n\n- [x] first\n- [x] second")
        assertThat(toggleTaskMarker(src, 1))
            .isEqualTo("- [ ]x\n\ntext\n\n    - [ ] indented\n\n- [ ] first\n- [ ] second")
    }

    @Test
    fun `toggleTaskMarker flips a quoted item`() {
        assertThat(toggleTaskMarker("> - [ ] quoted", 0)).isEqualTo("> - [x] quoted")
    }

    @Test
    fun `toggleTaskMarker negative index leaves source intact`() {
        val src = "- [ ] only"
        assertThat(toggleTaskMarker(src, -1)).isEqualTo(src)
    }

    // ── LocaleOptions ────────────────────────────────────────────────────────
    @Test
    fun `timezoneOptions are sorted value equals label and include known zone`() {
        val tzs = timezoneOptions()
        assertThat(tzs).isNotEmpty()
        assertThat(tzs.all { it.first == it.second }).isTrue()
        assertThat(tzs.map { it.first }).contains("UTC")
        val labels = tzs.map { it.first }
        assertThat(labels).isEqualTo(labels.sorted())
    }

    @Test
    fun `countryOptions localized and sorted`() {
        val ru = countryOptions("ru")
        assertThat(ru).isNotEmpty()
        // every country has a non-blank display name
        assertThat(ru.all { it.second.isNotBlank() }).isTrue()
        // sorted by localized name, with the collator every other list uses —
        // NOT by `lowercase()`, which is a code-unit sort (see below).
        val names = ru.map { it.second }
        assertThat(names).isEqualTo(names.sortedWith(UserTextOrder))
    }

    /**
     * «ё» is U+0451, past «я» (U+044F), so a code-unit sort files the only Russian
     * country name that carries one at the far end of «Т» — between «Турция» and
     * «Уганда» — instead of among the «Те…»s where a reader scrolls for it.
     */
    @Test
    fun `countryOptions place a yo-name by its letter, not past the alphabet`() {
        val names = countryOptions("ru").map { it.second }
        val i = names.indexOf("Тёркс и Кайкос")
        assertThat(i).isAtLeast(0)
        assertThat(names.indexOf("Танзания")).isLessThan(i)
        assertThat(i).isLessThan(names.indexOf("Того"))
        assertThat(i).isLessThan(names.indexOf("Турция"))
    }

    /**
     * A hyphen precedes every letter by code unit, so «о-в …»/«о-ва …» used to sort
     * ahead of «ОАЭ»; the collator compares the letters first, as the browser does.
     */
    @Test
    fun `countryOptions ignore the hyphen ahead of the letters`() {
        val names = countryOptions("ru").map { it.second }
        assertThat(names.indexOf("ОАЭ")).isLessThan(names.indexOf("о-в Буве"))
    }
}
