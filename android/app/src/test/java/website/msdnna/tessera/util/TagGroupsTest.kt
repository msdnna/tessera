package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.GitlabRule
import website.msdnna.tessera.data.model.Tag

@RunWith(RobolectricTestRunner::class)
class TagGroupsTest {
    private fun tag(name: String) = Tag(id = name, name = name)

    /** Ресурсы на конкретном языке — подпись бакета «без префикса» переводится. */
    private fun res(language: String): Resources =
        ApplicationProvider.getApplicationContext<Context>().withLanguage(language).resources

    private val ru: Resources get() = res("ru")

    // ── tagNamespace ─────────────────────────────────────────────────────────
    @Test
    fun `tagNamespace with colon-space`() {
        assertThat(tagNamespace("T: bug")).isEqualTo("T: ")
        assertThat(tagNamespace("Приоритет: высокий")).isEqualTo("Приоритет: ")
    }

    @Test
    fun `tagNamespace with double colon takes precedence`() {
        assertThat(tagNamespace("effort::small")).isEqualTo("effort::")
        // "::" is found before ": " even when both present
        assertThat(tagNamespace("a::b: c")).isEqualTo("a::")
    }

    @Test
    fun `tagNamespace none`() {
        assertThat(tagNamespace("bug")).isEmpty()
        assertThat(tagNamespace("")).isEmpty()
        assertThat(tagNamespace("no:space")).isEmpty() // needs ": " (colon+space)
    }

    // ── canonPrefix ──────────────────────────────────────────────────────────
    @Test
    fun `canonPrefix trims and lowercases`() {
        assertThat(canonPrefix(" S: ")).isEqualTo("s:")
        assertThat(canonPrefix("EFFORT::")).isEqualTo("effort::")
        assertThat(canonPrefix("Тег: ")).isEqualTo("тег:")
    }

    // ── prefixLabel ──────────────────────────────────────────────────────────
    @Test
    fun `prefixLabel empty is Vne gruppy`() {
        assertThat(prefixLabel(ru, "")).isEqualTo("Вне группы")
        assertThat(prefixLabel(res("en"), "")).isEqualTo("Ungrouped")
    }

    @Test
    fun `prefixLabel uses friendly name then trimmed prefix`() {
        assertThat(prefixLabel(ru, "S: ", mapOf("s:" to "Статус"))).isEqualTo("Статус")
        assertThat(prefixLabel(ru, "S: ")).isEqualTo("S:")
    }

    // ── splitTag ─────────────────────────────────────────────────────────────
    @Test
    fun `splitTag drops the separator from the scope`() {
        assertThat(splitTag("effort::small")).isEqualTo(TagSplit("effort::", "effort", "small"))
        assertThat(splitTag("T: bug")).isEqualTo(TagSplit("T: ", "T", "bug"))
        assertThat(splitTag("Приоритет: высокий")).isEqualTo(TagSplit("Приоритет: ", "Приоритет", "высокий"))
    }

    @Test
    fun `splitTag unscoped keeps the whole name as the label`() {
        assertThat(splitTag("urgent")).isEqualTo(TagSplit("", "", "urgent"))
        assertThat(splitTag("  urgent  ")).isEqualTo(TagSplit("", "", "urgent"))
        assertThat(splitTag("")).isEqualTo(TagSplit("", "", ""))
    }

    // ── tagParts ─────────────────────────────────────────────────────────────
    @Test
    fun `tagParts uses the friendly prefix name when configured`() {
        val parts = tagParts("S: готово", mapOf("s:" to "Статус"))
        assertThat(parts).isEqualTo(TagParts("Статус", "готово", true))
    }

    @Test
    fun `tagParts falls back to the bare prefix without a configured name`() {
        assertThat(tagParts("type::feature")).isEqualTo(TagParts("type", "feature", true))
    }

    @Test
    fun `tagParts raw mode ignores the friendly name`() {
        val parts = tagParts("S: готово", mapOf("s:" to "Статус"), raw = true)
        assertThat(parts).isEqualTo(TagParts("S", "готово", true))
    }

    @Test
    fun `tagParts unscoped tag has no scope segment`() {
        assertThat(tagParts("urgent")).isEqualTo(TagParts("", "urgent", false))
    }

    @Test
    fun `tagParts half-split tag falls back to the raw name`() {
        // Neither side may be empty — otherwise the pill would drop half the name.
        assertThat(tagParts("effort::")).isEqualTo(TagParts("", "effort::", false))
        assertThat(tagParts("::small")).isEqualTo(TagParts("", "::small", false))
    }

    // ── metaPrefixesFromRules ────────────────────────────────────────────────
    @Test
    fun `metaPrefixesFromRules keeps only non-tag prefix rules`() {
        val rules = listOf(
            GitlabRule(match = "S:", matchType = "prefix", action = "status"),
            GitlabRule(match = "T:", matchType = "prefix", action = "tag"), // tag → excluded
            GitlabRule(match = "P.*", matchType = "regex", action = "priority"), // regex → excluded
            GitlabRule(match = "", matchType = "prefix", action = "board"), // blank action? action non-blank but match empty
            GitlabRule(match = "B:", matchType = "prefix", action = "board"),
        )
        assertThat(metaPrefixesFromRules(rules)).containsExactly("s:", "b:")
    }

    @Test
    fun `metaPrefixesFromRules empty`() {
        assertThat(metaPrefixesFromRules(emptyList())).isEmpty()
    }

    // ── buildTagGroups ───────────────────────────────────────────────────────
    @Test
    fun `buildTagGroups sorts groups by label with ungrouped last`() {
        val tags = listOf(
            tag("bug"), // ungrouped
            tag("Статус: готово"),
            tag("Статус: в работе"),
            tag("A: one"),
        )
        val groups = buildTagGroups(ru, tags)
        // labels: "A:", "Статус:", then "Вне группы" last
        assertThat(groups.map { it.label }).containsExactly("A:", "Статус:", "Вне группы").inOrder()
        // last bucket is the prefix-less one
        assertThat(groups.last().key).isEmpty()
    }

    @Test
    fun `buildTagGroups sorts tags within group by name`() {
        val tags = listOf(tag("Статус: готово"), tag("Статус: в работе"), tag("Статус: архив"))
        val group = buildTagGroups(ru, tags).first()
        assertThat(group.tags.map { it.name }).containsExactly(
            "Статус: архив", "Статус: в работе", "Статус: готово",
        ).inOrder()
    }

    @Test
    fun `buildTagGroups honours friendly prefix names`() {
        val tags = listOf(tag("S: a"))
        val groups = buildTagGroups(ru, tags, prefixNames = mapOf("s:" to "Статус"))
        assertThat(groups.first().label).isEqualTo("Статус")
        assertThat(groups.first().key).isEqualTo("s:")
        assertThat(groups.first().prefix).isEqualTo("S: ")
    }

    @Test
    fun `buildTagGroups hidePrefixes drops matching tags`() {
        val tags = listOf(tag("S: a"), tag("T: b"), tag("plain"))
        val groups = buildTagGroups(ru, tags, hidePrefixes = setOf("s:"))
        assertThat(groups.flatMap { it.tags }.map { it.name })
            .containsExactly("T: b", "plain")
    }

    @Test
    fun `buildTagGroups empty input`() {
        assertThat(buildTagGroups(ru, emptyList())).isEmpty()
    }
}
