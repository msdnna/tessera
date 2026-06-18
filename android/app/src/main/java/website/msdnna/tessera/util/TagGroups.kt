package website.msdnna.tessera.util

import java.text.Collator
import java.util.Locale
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

/** Russian-locale, case-insensitive collator so Cyrillic labels sort naturally. */
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
fun prefixLabel(ns: String, prefixNames: Map<String, String> = emptyMap()): String {
    if (ns.isEmpty()) return "Вне группы"
    return prefixNames[canonPrefix(ns)] ?: ns.trim()
}

/** Groups a tag list by prefix namespace. Returns groups sorted alphabetically by
 *  label, with the prefix-less "Вне группы" bucket always last; tags within a group
 *  are sorted by name. [prefixNames] maps canonical prefix → friendly label. */
fun buildTagGroups(tags: List<Tag>, prefixNames: Map<String, String> = emptyMap()): List<TagGroup> {
    val groups = LinkedHashMap<String, MutableList<Tag>>()
    for (t in tags) {
        val key = canonPrefix(tagNamespace(t.name))
        groups.getOrPut(key) { mutableListOf() }.add(t)
    }
    return groups.entries
        .map { (key, list) ->
            val ns = tagNamespace(list.first().name)
            TagGroup(
                key = key,
                label = prefixLabel(ns, prefixNames),
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
