package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import org.junit.Test
import website.msdnna.tessera.data.model.GitlabRule
import website.msdnna.tessera.data.model.Tag

class TagGroupsTest {
    private fun tag(name: String) = Tag(id = name, name = name)

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
        assertThat(prefixLabel("")).isEqualTo("Вне группы")
    }

    @Test
    fun `prefixLabel uses friendly name then trimmed prefix`() {
        assertThat(prefixLabel("S: ", mapOf("s:" to "Статус"))).isEqualTo("Статус")
        assertThat(prefixLabel("S: ")).isEqualTo("S:")
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
        val groups = buildTagGroups(tags)
        // labels: "A:", "Статус:", then "Вне группы" last
        assertThat(groups.map { it.label }).containsExactly("A:", "Статус:", "Вне группы").inOrder()
        // last bucket is the prefix-less one
        assertThat(groups.last().key).isEmpty()
    }

    @Test
    fun `buildTagGroups sorts tags within group by name`() {
        val tags = listOf(tag("Статус: готово"), tag("Статус: в работе"), tag("Статус: архив"))
        val group = buildTagGroups(tags).first()
        assertThat(group.tags.map { it.name }).containsExactly(
            "Статус: архив", "Статус: в работе", "Статус: готово",
        ).inOrder()
    }

    @Test
    fun `buildTagGroups honours friendly prefix names`() {
        val tags = listOf(tag("S: a"))
        val groups = buildTagGroups(tags, prefixNames = mapOf("s:" to "Статус"))
        assertThat(groups.first().label).isEqualTo("Статус")
        assertThat(groups.first().key).isEqualTo("s:")
        assertThat(groups.first().prefix).isEqualTo("S: ")
    }

    @Test
    fun `buildTagGroups hidePrefixes drops matching tags`() {
        val tags = listOf(tag("S: a"), tag("T: b"), tag("plain"))
        val groups = buildTagGroups(tags, hidePrefixes = setOf("s:"))
        assertThat(groups.flatMap { it.tags }.map { it.name })
            .containsExactly("T: b", "plain")
    }

    @Test
    fun `buildTagGroups empty input`() {
        assertThat(buildTagGroups(emptyList())).isEmpty()
    }
}
