package website.msdnna.tessera.i18n

import com.google.common.truth.Truth.assertWithMessage
import java.io.File
import org.junit.Test

/**
 * Храповик сырого текста исключения (#2932). Отказ, показанный как `e.message`, —
 * это не причина, а её обёртка: у retrofit получается «HTTP 400 Bad Request», у
 * OkHttp — «Unable to resolve host …». Обе строки английские, обе ничего не
 * объясняют, и обе выглядят нормально, пока сервер не откажет по-настоящему.
 *
 * Канон один — [website.msdnna.tessera.util.errorMessage]: он достаёт `{"error":…}`
 * из тела ответа и причёсывает его через `humanizeError`, как это делает
 * перехватчик веба (`frontend/src/api/index.js`). Сам он и разбирает `t.message`,
 * поэтому `util/Errors.kt` — единственное место, где обращение к нему уместно.
 *
 * Тест сравнивает список файлов-нарушителей **точно**: новый — красный тест,
 * вычищенный обязан исчезнуть из [ALLOWED]. Класс уже ловили дважды подряд —
 * сначала на экране профиля, потом на админском, — поэтому и появился храповик.
 */
class RawThrowableTextTest {
    @Test
    fun `nobody but the humanizer reads a throwable message`() {
        val root = sourceRoot()
        val actual = root.walkTopDown()
            .filter { it.isFile && it.extension == "kt" }
            .map { it.relativeTo(root).path.replace(File.separatorChar, '/') to it }
            .filter { (path, _) -> path != HUMANIZER }
            .filter { (_, file) -> rawThrowableReads(file.readText()).isNotEmpty() }
            .map { (path, _) -> path }
            .toSortedSet()

        assertWithMessage(
            "здесь показывают сырой текст исключения — проведите отказ через errorMessage(t), " +
                "иначе вместо причины сервера пользователь увидит «HTTP 400 Bad Request»",
        ).that(actual - ALLOWED).isEmpty()
        assertWithMessage(
            "эти файлы уже на errorMessage — уберите их из ALLOWED, иначе храповик перестанет их сторожить",
        ).that(ALLOWED - actual).isEmpty()
    }

    /** Сканер смотрит на получателя, а не на слово `message`: у списка сообщений
     *  чата оно своё и к исключениям отношения не имеет. */
    @Test
    fun `a chat message list is not a throwable`() {
        val source = """
            val last = state.messages.lastOrNull()
            val text = message.body
            val why = e.message
        """.trimIndent()
        assertWithMessage("сканер спутал сообщения чата с текстом исключения")
            .that(rawThrowableReads(source)).containsExactly("e.message")
    }

    private companion object {
        /** Сам канон: он и обязан разбирать `t.message` — больше это делать некому. */
        const val HUMANIZER = "website/msdnna/tessera/util/Errors.kt"

        /**
         * Движок звонка: его ошибки приходят не от нашего сервера, а от SDK LiveKit,
         * и состояние держит их готовой строкой (`ConfMediaState.error: String`).
         * Перевод на `UiText` — отдельная правка со своей поверхностью.
         */
        val ALLOWED = sortedSetOf("website/msdnna/tessera/data/conference/ConferenceEngine.kt")
    }
}

/**
 * Обращения к `message` исключения: получатель — одно из привычных имён пойманного
 * throwable (`it` в `onFailure`, `e`/`t` в `catch`). Не регулярка по слову
 * `message`: так в улов попали бы сообщения чата и комментарии.
 */
internal fun rawThrowableReads(source: String): List<String> =
    THROWABLE_MESSAGE.findAll(stripComments(source)).map { it.value }.toList()

private val THROWABLE_MESSAGE = Regex("""\b(it|e|t|err|ex|cause|throwable|failure)\.message\b""")

/** Комментарии вырезаем: они и объясняют, почему `e.message` читать нельзя. */
private fun stripComments(source: String): String {
    val out = StringBuilder(source.length)
    var i = 0
    while (i < source.length) {
        when {
            source.startsWith("//", i) -> {
                while (i < source.length && source[i] != '\n') i++
            }

            source.startsWith("/*", i) -> {
                i += 2
                while (i + 1 < source.length && !(source[i] == '*' && source[i + 1] == '/')) i++
                i = minOf(i + 2, source.length)
            }

            else -> {
                out.append(source[i])
                i++
            }
        }
    }
    return out.toString()
}
