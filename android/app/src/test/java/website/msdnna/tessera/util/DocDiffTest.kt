package website.msdnna.tessera.util

import com.google.common.truth.Truth.assertThat
import com.google.gson.JsonElement
import com.google.gson.JsonParser
import org.junit.Test
import website.msdnna.tessera.data.model.DocumentVersion

/**
 * Block-level comparison of two versions (#2731, §6 of #2894) — the same cases
 * the web spec `ut-docDiff.spec.js` covers, because the two panels must not
 * describe the same rollback differently.
 *
 * What is worth guarding here is not "does it print a diff" but the three
 * judgements a journal is read for: an edited block keeps its old wording, a
 * dragged one is *moved* rather than deleted-and-added, and a deletion stays
 * where it happened instead of being swept to the end.
 */
class DocDiffTest {
    /** A paragraph with a stable block id — the anchor the diff is built on. */
    private fun p(id: String, text: String) =
        """{"type":"paragraph","attrs":{"id":"$id"},"content":[{"type":"text","text":"$text"}]}"""

    /** A paragraph the way imported content arrives: no id to match on. */
    private fun anon(text: String) =
        """{"type":"paragraph","content":[{"type":"text","text":"$text"}]}"""

    private fun doc(vararg blocks: String): JsonElement =
        JsonParser.parseString("""{"type":"doc","content":[${blocks.joinToString(",")}]}""")

    private fun statuses(rows: List<DocDiffRow>) = rows.map { it.status }

    private fun texts(rows: List<DocDiffRow>) = rows.map { it.text }

    @Test
    fun `an untouched document is unchanged`() {
        val d = doc(p("a", "первый"), p("b", "второй"))
        val rows = diffDocs(d, d)
        assertThat(statuses(rows)).containsExactly(DocDiffStatus.SAME, DocDiffStatus.SAME).inOrder()
        assertThat(docDiffSummary(rows).identical).isTrue()
    }

    @Test
    fun `an edited block is changed and keeps its previous wording`() {
        val rows = diffDocs(doc(p("a", "было"), p("b", "общий")), doc(p("a", "стало"), p("b", "общий")))
        assertThat(statuses(rows)).containsExactly(DocDiffStatus.CHANGED, DocDiffStatus.SAME).inOrder()
        assertThat(rows[0].prevText).isEqualTo("было")
        assertThat(rows[0].text).isEqualTo("стало")
    }

    @Test
    fun `a dragged paragraph is moved, not deleted and added`() {
        val rows = diffDocs(
            doc(p("a", "первый"), p("b", "второй"), p("c", "третий")),
            doc(p("b", "второй"), p("a", "первый"), p("c", "третий")),
        )
        assertThat(statuses(rows))
            .containsExactly(DocDiffStatus.MOVED, DocDiffStatus.MOVED, DocDiffStatus.SAME)
            .inOrder()
        val summary = docDiffSummary(rows)
        assertThat(summary.added).isEqualTo(0)
        assertThat(summary.removed).isEqualTo(0)
    }

    @Test
    fun `blocks are not moved just because one above them was deleted`() {
        val rows = diffDocs(
            doc(p("a", "первый"), p("b", "второй"), p("c", "третий")),
            doc(p("b", "второй"), p("c", "третий")),
        )
        assertThat(statuses(rows))
            .containsExactly(DocDiffStatus.REMOVED, DocDiffStatus.SAME, DocDiffStatus.SAME)
            .inOrder()
        assertThat(docDiffSummary(rows).moved).isEqualTo(0)
    }

    @Test
    fun `a deletion is placed where it happened, not at the end`() {
        val rows = diffDocs(
            doc(p("a", "первый"), p("b", "второй"), p("c", "третий")),
            doc(p("a", "первый"), p("c", "третий")),
        )
        assertThat(statuses(rows))
            .containsExactly(DocDiffStatus.SAME, DocDiffStatus.REMOVED, DocDiffStatus.SAME)
            .inOrder()
        assertThat(texts(rows)).containsExactly("первый", "второй", "третий").inOrder()
    }

    @Test
    fun `two deletions after the same block keep their original order`() {
        val rows = diffDocs(
            doc(p("a", "первый"), p("b", "второй"), p("c", "третий"), p("d", "четвёртый")),
            doc(p("a", "первый"), p("d", "четвёртый")),
        )
        assertThat(texts(rows)).containsExactly("первый", "второй", "третий", "четвёртый").inOrder()
        assertThat(statuses(rows))
            .containsExactly(
                DocDiffStatus.SAME,
                DocDiffStatus.REMOVED,
                DocDiffStatus.REMOVED,
                DocDiffStatus.SAME,
            )
            .inOrder()
    }

    @Test
    fun `a deletion above every surviving block goes back to the top`() {
        val rows = diffDocs(doc(p("a", "вступление"), p("b", "основное")), doc(p("b", "основное")))
        assertThat(statuses(rows)).containsExactly(DocDiffStatus.REMOVED, DocDiffStatus.SAME).inOrder()
        assertThat(texts(rows)).containsExactly("вступление", "основное").inOrder()
    }

    @Test
    fun `a new block counts as added`() {
        val rows = diffDocs(doc(p("a", "первый")), doc(p("a", "первый"), p("b", "новый абзац")))
        assertThat(statuses(rows)).containsExactly(DocDiffStatus.SAME, DocDiffStatus.ADDED).inOrder()
        val summary = docDiffSummary(rows)
        assertThat(summary.added).isEqualTo(1)
        assertThat(summary.identical).isFalse()
    }

    // Formatting is content: a heading that became level 3, a link that now
    // points elsewhere. The flattened text is identical in both cases.
    @Test
    fun `a change the plain text does not show is still a change`() {
        val before = doc("""{"type":"heading","attrs":{"id":"h","level":2},"content":[{"type":"text","text":"Раздел"}]}""")
        val after = doc("""{"type":"heading","attrs":{"id":"h","level":3},"content":[{"type":"text","text":"Раздел"}]}""")
        assertThat(statuses(diffDocs(before, after))).containsExactly(DocDiffStatus.CHANGED)
    }

    // A server that serialises the same node with its keys in another order has
    // changed nothing, and a diff that says otherwise turns every rollback into
    // "весь документ изменён".
    @Test
    fun `key order in the stored node is not an edit`() {
        val before = doc("""{"type":"paragraph","attrs":{"id":"a"},"content":[{"type":"text","text":"текст"}]}""")
        val after = doc("""{"attrs":{"id":"a"},"content":[{"text":"текст","type":"text"}],"type":"paragraph"}""")
        assertThat(statuses(diffDocs(before, after))).containsExactly(DocDiffStatus.SAME)
    }

    @Test
    fun `blocks without ids fall back to position`() {
        val rows = diffDocs(doc(anon("первый"), anon("второй")), doc(anon("первый"), anon("другой")))
        assertThat(statuses(rows)).containsExactly(DocDiffStatus.SAME, DocDiffStatus.CHANGED).inOrder()
    }

    // Imported content and content written in the editor live in the same
    // document all the time — a `.docx` brought in and then edited. "Moved" is
    // measured against two lists of surviving blocks, and building them by
    // different rules shifts the indices and reports untouched paragraphs as
    // dragged.
    @Test
    fun `an untouched block is not moved in a document with imported blocks`() {
        val d = doc(anon("импортированный абзац"), p("a", "дописанный абзац"))
        assertThat(statuses(diffDocs(d, d)))
            .containsExactly(DocDiffStatus.SAME, DocDiffStatus.SAME)
            .inOrder()
    }

    @Test
    fun `an empty document against a filled one`() {
        val empty = JsonParser.parseString("""{"type":"doc","content":[]}""")
        assertThat(statuses(diffDocs(empty, doc(p("a", "первая строка")))))
            .containsExactly(DocDiffStatus.ADDED)
        assertThat(diffDocs(doc(p("a", "первая строка")), empty)).hasSize(1)
    }

    // A body that never arrived, or one the server answered with `null`: the
    // journal must show "нет изменений", not fall over.
    @Test
    fun `a missing body compares to nothing`() {
        assertThat(diffDocs(null, null)).isEmpty()
        assertThat(diffDocs(null, doc(p("a", "текст")))).hasSize(1)
    }

    @Test
    fun `nested and media blocks flatten into readable text`() {
        val blocks = docDiffBlocks(
            doc(
                """{"type":"bulletList","attrs":{"id":"l"},"content":[
                     {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"раз"}]}]},
                     {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"два"}]}]}]}""",
                """{"type":"image","attrs":{"id":"i","src":"/api/documents/asset?x=1","alt":"схема"}}""",
                """{"type":"image","attrs":{"id":"j"}}""",
            ),
            imageLabel = "изображение",
        )
        assertThat(blocks[0].text).isEqualTo("раз два")
        // An image has no text of its own; its alt is what a reader can act on,
        // and one with neither alt nor src is still a block worth naming.
        assertThat(blocks[1].text).isEqualTo("схема")
        assertThat(blocks[2].text).isEqualTo("изображение")
    }

    // ── the journal's own ordering ────────────────────────────────────────────

    private fun version(id: String, revision: Int) =
        DocumentVersion(id = id, revision = revision)

    private val older = version("v1", 3)
    private val newer = version("v2", 7)
    private val bodies = mapOf<String, JsonElement?>(
        "v1" to doc(p("a", "было")),
        "v2" to doc(p("a", "стало")),
    )

    /**
     * The comparison reads old → new whichever of the two was picked from the
     * list: a journal is walked backwards in time, and a diff that flipped with
     * the selection would show yesterday's text as the addition.
     */
    @Test
    fun `the older version is always the left-hand side`() {
        val fromOld = docVersionDiff(selected = older, baseline = newer, bodies = bodies)
        val fromNew = docVersionDiff(selected = newer, baseline = older, bodies = bodies)
        assertThat(fromOld.map { it.prevText }).containsExactly("было")
        assertThat(fromOld.map { it.text }).containsExactly("стало")
        assertThat(fromNew).isEqualTo(fromOld)
    }

    /** Until both bodies are in hand the panel has nothing to say — and an empty
     *  comparison must not be dressed up as "изменений нет". */
    @Test
    fun `a half-loaded comparison yields nothing`() {
        assertThat(docVersionDiff(older, newer, bodies = mapOf("v1" to bodies["v1"]))).isEmpty()
        assertThat(docVersionDiff(null, newer, bodies = bodies)).isEmpty()
        assertThat(docVersionDiff(older, null, bodies = bodies)).isEmpty()
    }
}
