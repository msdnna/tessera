package website.msdnna.tessera.util

import android.content.res.Resources
import java.text.Collator
import java.util.Locale
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.GitlabRule
import website.msdnna.tessera.data.model.Tag

/*
 * Tag-prefix grouping helpers (web `frontend/src/utils/tagGroups.js` parity). Tags
 * follow a "<prefix>: value" / "<prefix>::value" naming convention; this derives
 * the prefix namespace, canonicalises it to a key, and groups a tag list under
 * friendly display names.
 */

/** An ordered prefix group: a stable [key], a display [label], the raw [prefix]
 *  and the member [tags] (sorted by name). */
data class TagGroup(val key: String, val label: String, val prefix: String, val tags: List<Tag>)

/** Russian-locale, case-insensitive collator so Cyrillic labels sort naturally.
 *  Локаль сортировки намеренно не следует за языком интерфейса: имена тегов — это
 *  пользовательские данные, они остаются русскими и на английском UI, а порядок
 *  групп не должен переставляться от переключения языка. */
private val ruCollator: Collator =
    Collator.getInstance(Locale.forLanguageTag("ru")).apply { strength = Collator.SECONDARY }

/** Namespace of a tag name ("T: bug" → "T: ", "effort::small" → "effort::").
 *  Returns "" when the tag has no prefix. */
fun tagNamespace(name: String): String {
    val i = name.indexOf("::")
    if (i >= 0) return name.substring(0, i + 2)
    val j = name.indexOf(": ")
    if (j >= 0) return name.substring(0, j + 2)
    return ""
}

/** Canonical prefix key — must mirror the backend (`handlers/tag_prefixes.go
 *  canonPrefix`) so a namespace derived from a tag ("S: ") lines up with what the
 *  GitLab rule editor stores ("S:"). */
fun canonPrefix(p: String): String = p.trim().lowercase()

/** Display label for a namespace: the configured friendly name, else the trimmed
 *  raw prefix (e.g. "S:"), else "Вне группы" for prefix-less tags. */
fun prefixLabel(res: Resources, ns: String, prefixNames: Map<String, String> = emptyMap()): String {
    if (ns.isEmpty()) return res.getString(R.string.tag_group_none)
    return prefixNames[canonPrefix(ns)] ?: ns.trim()
}

/** A tag name split into its namespace, bare [scope] and [label] value. */
data class TagSplit(val ns: String, val scope: String, val label: String)

/** Splits a tag name into namespace, bare scope and value (web `splitTag`):
 *  - "effort::small" → ns "effort::", scope "effort", label "small"
 *  - "T: bug"        → ns "T: ",      scope "T",      label "bug"
 *  - "urgent"        → ns "",         scope "",       label "urgent"
 *  [scope] drops the separator — [prefixLabel] keeps it, but that one titles a
 *  *group*, this one titles a pill segment. */
fun splitTag(name: String): TagSplit {
    val ns = tagNamespace(name)
    if (ns.isEmpty()) return TagSplit("", "", name.trim())
    return TagSplit(
        ns = ns,
        scope = ns.trimEnd().trimEnd(':').trim(),
        label = name.substring(ns.length).trim(),
    )
}

/** Presentation parts of a tag pill (web `tagParts`): the friendly scope name
 *  (configured label if any, else the raw prefix) and the value. A tag whose scope
 *  or value side is empty ("effort::", "::small") has no usable split — it falls
 *  back to the whole raw name so nothing silently disappears from the UI.
 *  [raw] shows the bare prefix ("T") instead of the friendly name — the device
 *  «короткие префиксы» preference. */
fun tagParts(name: String, prefixNames: Map<String, String> = emptyMap(), raw: Boolean = false): TagParts {
    val (ns, scope, label) = splitTag(name)
    if (ns.isEmpty() || scope.isEmpty() || label.isEmpty()) {
        return TagParts(scope = "", label = name.trim(), hasScope = false)
    }
    val friendly = if (raw) scope else prefixNames[canonPrefix(ns)] ?: scope
    return TagParts(scope = friendly, label = label, hasScope = true)
}

/** Scope/value pair for a tag pill; [hasScope] false means render [label] alone. */
data class TagParts(val scope: String, val label: String, val hasScope: Boolean)

/** Canonical prefixes of GitLab label rules that map a label to a NON-tag action
 *  (status / priority / board / ignore / …). Those labels aren't user tags, so they
 *  are hidden from the ADD tag-picker (web `metaPrefixesFromRules`). Only prefix-type
 *  rules qualify — regex rules can't be reduced to a stable prefix, so their tags stay. */
fun metaPrefixesFromRules(rules: List<GitlabRule>): Set<String> =
    rules.asSequence()
        .filter { it.matchType == "prefix" && it.action.isNotBlank() && it.action != "tag" }
        .map { canonPrefix(it.match) }
        .filter { it.isNotEmpty() }
        .toSet()

/** Groups a tag list by prefix namespace. Returns groups sorted alphabetically by
 *  label, with the prefix-less "Вне группы" bucket always last; tags within a group
 *  are sorted by name. [prefixNames] maps canonical prefix → friendly label.
 *  [hidePrefixes] (canonical prefixes, e.g. from [metaPrefixesFromRules]) drops those
 *  tags entirely — used to keep GitLab meta-labels out of the ADD picker. */
fun buildTagGroups(
    res: Resources,
    tags: List<Tag>,
    prefixNames: Map<String, String> = emptyMap(),
    hidePrefixes: Set<String> = emptySet(),
): List<TagGroup> {
    val groups = LinkedHashMap<String, MutableList<Tag>>()
    for (t in tags) {
        val key = canonPrefix(tagNamespace(t.name))
        if (key.isNotEmpty() && key in hidePrefixes) continue
        groups.getOrPut(key) { mutableListOf() }.add(t)
    }
    return groups.entries
        .map { (key, list) ->
            val ns = tagNamespace(list.first().name)
            TagGroup(
                key = key,
                label = prefixLabel(res, ns, prefixNames),
                prefix = ns,
                tags = list.sortedWith(compareBy(ruCollator) { it.name }),
            )
        }
        .sortedWith(
            // Prefix-less bucket ("") last; the rest by label (ru-collated).
            Comparator { a, b ->
                when {
                    a.key == "" -> 1
                    b.key == "" -> -1
                    else -> ruCollator.compare(a.label, b.label)
                }
            },
        )
}
