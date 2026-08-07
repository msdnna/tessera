package website.msdnna.tessera.util

/*
 * Provider-neutral dictionary of "where did this record come from" sources, used by
 * the source badges (relations, and whatever gets a `source` column next). Port of
 * web `frontend/src/utils/sources.js`. Consumers never name a provider — adding the
 * second integration is one entry here.
 */

/** Display meta of a record source: a human [label] and an optional [Ion] icon key. */
data class SourceMeta(val label: String, val icon: String? = null)

private val SOURCES = mapOf(
    "user" to SourceMeta("Tessera"),
    "gitlab" to SourceMeta("GitLab", Ion.GITLAB),
)

fun sourceMeta(source: String?): SourceMeta =
    SOURCES[source] ?: SourceMeta(source?.takeIf { it.isNotBlank() } ?: "—")

/** A source that isn't the user typing it in — i.e. an integration owns the record
 *  and will re-create it on the next sync. Empty/absent source = legacy user data. */
fun isExternalSource(source: String?): Boolean = !source.isNullOrBlank() && source != "user"
