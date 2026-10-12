package website.msdnna.tessera.util

import com.google.gson.Gson
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.data.model.UpdateCommentRequest

/**
 * Правка комментария и упоминания (#2932).
 *
 * Редактор правки получает тот же роутер @-кандидатов, что и основной композер,
 * то есть дописанное правкой «@Аня, посмотри» рисует резолвнутый чип — а PATCH
 * уходил одним только body, и человеку не говорили ничего. Уведомляет сервер
 * ровно тех, кого ему назвали, поэтому здесь пиннится обе половины контракта:
 * дельта (кого правка ДОБАВИЛА) и то, что она вообще уезжает на провод.
 */
@RunWith(RobolectricTestRunner::class)
class MentionEditDeltaTest {
    private val anya = Member(userId = "u-anya", role = "member", email = "anya@p.dev", name = "Аня")
    private val boris = Member(userId = "u-boris", role = "member", email = "boris@p.dev", name = "Боря")
    private val roster = buildMentionItems(listOf(anya, boris), emptyList())

    @Test
    fun `an edit that names someone new reports them`() {
        val added = addedMentionIds("Черновик готов", "Черновик готов. @Аня, посмотри", roster)
        assertEquals(listOf("u-anya"), added)
    }

    @Test
    fun `fixing a typo does not ping the same person again`() {
        // Оба тела упоминают Аню — добавленных нет, иначе каждая правка была бы
        // новым уведомлением для всех названных.
        val added = addedMentionIds(
            "Черновик готов. @Аня, посмотри",
            "Черновик готов. @Аня, посмотри его",
            roster,
        )
        assertEquals(emptyList<String>(), added)
    }

    @Test
    fun `only the person the edit added is reported`() {
        val added = addedMentionIds(
            "@Аня, посмотри",
            "@Аня, посмотри. @Боря, тебя тоже касается",
            roster,
        )
        assertEquals(listOf("u-boris"), added)
    }

    @Test
    fun `a handle inside code names nobody`() {
        // Та же граница, что и у отправки: чипа нет — значит и обещания, что
        // человеку сказали, нет (mentionIdsIn читает то, что отрисуется).
        val added = addedMentionIds("Черновик готов", "Черновик готов\n\n```\n@Аня\n```", roster)
        assertEquals(emptyList<String>(), added)
    }

    @Test
    fun `dropping a mention notifies nobody`() {
        // Упоминание убрали — сказать об этом некому; дельта считается только
        // вперёд.
        val added = addedMentionIds("@Аня, посмотри", "Посмотрю сам", roster)
        assertEquals(emptyList<String>(), added)
    }

    @Test
    fun `the request carries the ids under the name the server reads`() {
        // @SerializedName не проверяется компилятором: опечатка здесь — это не
        // ошибка, а молчаливое «упомянутых нет».
        val json = Gson().toJson(UpdateCommentRequest("Черновик готов. @Аня", listOf("u-anya")))
        assertEquals("""{"body":"Черновик готов. @Аня","mentions":["u-anya"]}""", json)
    }

    @Test
    fun `an edit that names nobody still sends a list`() {
        // Галочка в чеклисте ходит этим же PATCH: тело переписано, упоминаний нет.
        val json = Gson().toJson(UpdateCommentRequest("- [x] собрать смету"))
        assertEquals("""{"body":"- [x] собрать смету","mentions":[]}""", json)
    }
}
