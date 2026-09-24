package website.msdnna.tessera.util

import android.content.Context
import android.content.res.Resources
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import java.io.IOException
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import retrofit2.HttpException
import website.msdnna.tessera.ui.UiText
import website.msdnna.tessera.ui.resolve

/**
 * Ошибки вызовов API едут до экрана как [UiText] (#2803, волна 12), поэтому тест
 * идёт под Robolectric и проверяет обе локали: подстановка кода в «Ошибка %1$d»
 * и наличие английского перевода иначе ломались бы только в рантайме.
 */
@RunWith(RobolectricTestRunner::class)
class ErrorsTest {
    private fun res(language: String): Resources =
        ApplicationProvider.getApplicationContext<Context>().withLanguage(language).resources

    private val ru: Resources get() = res("ru")
    private val en: Resources get() = res("en")

    private fun httpException(code: Int, body: String): HttpException {
        val raw = Response.Builder()
            .code(code)
            .message("err")
            .protocol(Protocol.HTTP_1_1)
            .request(Request.Builder().url("http://localhost/").build())
            .build()
        val errBody = body.toResponseBody("application/json".toMediaType())
        return HttpException(retrofit2.Response.error<Any>(errBody, raw.newBuilder().code(code).build()))
    }

    // ── humanizeError ────────────────────────────────────────────────────────
    @Test
    fun `humanizeError null and blank yield null`() {
        assertThat(humanizeError(null)).isNull()
        assertThat(humanizeError("")).isNull()
        assertThat(humanizeError("   ")).isNull()
    }

    @Test
    fun `humanizeError maps sentinels case-insensitively`() {
        assertThat(humanizeError("invalid credentials")?.resolve(ru)).isEqualTo("Неверный email или пароль")
        assertThat(humanizeError("INVALID CREDENTIALS")?.resolve(ru)).isEqualTo("Неверный email или пароль")
        assertThat(humanizeError("email already registered")?.resolve(ru)).isEqualTo("Этот email уже зарегистрирован")
        assertThat(humanizeError("forbidden")?.resolve(ru)).isEqualTo("Недостаточно прав")
    }

    @Test
    fun `humanizeError sentinels speak English too`() {
        assertThat(humanizeError("invalid credentials")?.resolve(en)).isEqualTo("Wrong email or password")
        assertThat(humanizeError("forbidden")?.resolve(en)).isEqualTo("Not enough permissions")
    }

    /** Оба конца правила пароля (#2932): форма зеркалит его сама, но отказ с
     *  сервера всё равно должен приходить на языке читателя. */
    @Test
    fun `humanizeError maps both ends of the password rule`() {
        assertThat(humanizeError("password is too short")?.resolve(ru))
            .isEqualTo("Пароль слишком короткий — минимум 8 символов")
        assertThat(humanizeError("password is too long")?.resolve(ru))
            .isEqualTo("Пароль слишком длинный — не больше 72 байт")
        assertThat(humanizeError("password is too long")?.resolve(en))
            .isEqualTo("Password is too long — at most 72 bytes")
    }

    /**
     * Отказ ограничителя частоты. До него доходят, не сделав ничего
     * неправильного: вход throttled на десять попыток в минуту, и ключ считается
     * И по IP, И по адресу, так что офис за одним NAT делит один бюджет. Фраза
     * должна называть выход — подождать, — а не только факт (#2932).
     */
    @Test
    fun `humanizeError maps the rate-limit refusal`() {
        assertThat(humanizeError("too many requests")?.resolve(ru))
            .isEqualTo("Слишком много попыток — подождите минуту и попробуйте снова")
        assertThat(humanizeError("TOO MANY REQUESTS")?.resolve(ru))
            .isEqualTo("Слишком много попыток — подождите минуту и попробуйте снова")
        assertThat(humanizeError("too many requests")?.resolve(en))
            .isEqualTo("Too many attempts — wait a minute and try again")
    }

    /** Две границы сохранённого вида доски: правило клиент не зеркалит, а
     *  сохранение вида на телефоне есть, так что обе встречаются обычным путём. */
    @Test
    fun `humanizeError maps both saved-view bounds`() {
        assertThat(humanizeError("view limit reached")?.resolve(ru))
            .isEqualTo("Сохранённых видов доски слишком много — удалите ненужный, чтобы добавить новый")
        assertThat(humanizeError("view config is too large")?.resolve(ru))
            .isEqualTo("Настройки вида не помещаются в сохранение")
        assertThat(humanizeError("view limit reached")?.resolve(en))
            .isEqualTo("Too many saved views on this board — delete one to add another")
    }

    /** Опечатка в текущем пароле — единственный отказ из этого каталога, до
     *  которого обычный пользователь доходит обычным путём. Печатается прямо в
     *  форме (ProfileScreen, TFormError), и до #2932 — по-английски. */
    @Test
    fun `humanizeError maps the wrong current password`() {
        assertThat(humanizeError("current password is incorrect")?.resolve(ru))
            .isEqualTo("Текущий пароль неверен")
        assertThat(humanizeError("current password is incorrect")?.resolve(en))
            .isEqualTo("Current password is incorrect")
    }

    /** Каталог обязан совпадать с вебовским (`frontend/src/utils/errors.js`) —
     *  один и тот же сервер отвечает обоим клиентам, и разойтись они могут
     *  только молча. Эти два сентинела веб знал, а телефон — нет. */
    @Test
    fun `humanizeError knows the sentinels the web catalogue has`() {
        assertThat(humanizeError("job is already running")?.resolve(ru))
            .isEqualTo("Задача уже выполняется — дождитесь её завершения")
        assertThat(humanizeError("too many approvers")?.resolve(ru))
            .isEqualTo("В маршруте согласования слишком много участников")
    }

    /** Четыре отказа членства в пространстве плюс ранг, который остаётся после
     *  них (#2932). Модалка участников на телефоне гасит свою строку так же, как
     *  веб, поэтому сюда доходит только устаревшая вкладка — момент, когда сырая
     *  английская фраза нужна меньше всего. */
    @Test
    fun `humanizeError maps the workspace membership refusals`() {
        assertThat(humanizeError("cannot change the workspace owner's role")?.resolve(ru))
            .isEqualTo("Роль владельца пространства менять нельзя")
        assertThat(humanizeError("cannot remove the workspace owner")?.resolve(ru))
            .isEqualTo("Владельца пространства нельзя удалить")
        assertThat(humanizeError("cannot change your own role")?.resolve(ru))
            .isEqualTo("Свою роль изменить нельзя — попросите другого администратора")
        assertThat(humanizeError("cannot remove yourself from the workspace")?.resolve(ru))
            .isEqualTo("Себя нельзя удалить из пространства — попросите другого администратора")
        assertThat(humanizeError("requires admin or owner")?.resolve(ru))
            .isEqualTo("Нужны права администратора или владельца пространства")
        assertThat(humanizeError("requires admin")?.resolve(ru))
            .isEqualTo("Нужны права администратора")
    }

    /** Обе локали заполнены — англоязычный читатель не должен получить русскую
     *  строку из-за половинчатой правки. */
    @Test
    fun `humanizeError phrases the membership refusals in English too`() {
        assertThat(humanizeError("cannot change your own role")?.resolve(en))
            .isEqualTo("You cannot change your own role — ask another admin")
        assertThat(humanizeError("cannot remove yourself from the workspace")?.resolve(en))
            .isEqualTo("You cannot remove yourself from the workspace — ask another admin")
    }

    @Test
    fun `humanizeError validator email`() {
        val raw = "Key: 'Req.Email' Error:Field validation for 'Email' failed on the 'email' tag"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Введите корректный email")
    }

    @Test
    fun `humanizeError validator required only`() {
        val raw = "Key: 'Req.Name' Error:Field validation for 'Name' failed on the 'required' tag"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Заполните все обязательные поля")
    }

    @Test
    fun `humanizeError validator min`() {
        val raw = "Key: 'Req.Pw' Error:Field validation for 'Pw' failed on the 'min' tag"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Значение слишком короткое")
    }

    @Test
    fun `humanizeError validation without recognisable tag falls back`() {
        val raw = "Key: 'X' validation something weird"
        assertThat(humanizeError(raw)?.resolve(ru)).isEqualTo("Проверьте правильность заполнения полей")
    }

    @Test
    fun `humanizeError network hints`() {
        assertThat(humanizeError("dial tcp: connection timeout")?.resolve(ru))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
        assertThat(humanizeError("econnrefused 127.0.0.1")?.resolve(ru))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
        assertThat(humanizeError("unable to resolve host example.com")?.resolve(ru))
            .isEqualTo("Нет связи с сервером. Проверьте подключение.")
    }

    /** Незнакомое сообщение сервера — [UiText.Raw]: переводить его нечем, и в
     *  обеих локалях оно должно доехать до экрана дословно. */
    @Test
    fun `humanizeError unknown short message passes through as raw`() {
        val out = humanizeError("Что-то пошло не так")
        assertThat(out).isEqualTo(UiText.Raw("Что-то пошло не так"))
        assertThat(out?.resolve(en)).isEqualTo("Что-то пошло не так")
    }

    // ── errorMessage ─────────────────────────────────────────────────────────
    @Test
    fun `errorMessage IOException`() {
        assertThat(errorMessage(IOException("boom")).resolve(ru)).isEqualTo("Нет соединения с сервером")
        assertThat(errorMessage(IOException("boom")).resolve(en)).isEqualTo("No connection to the server")
    }

    @Test
    fun `errorMessage generic throwable humanized`() {
        assertThat(errorMessage(RuntimeException("user not found")).resolve(ru)).isEqualTo("Пользователь не найден")
        assertThat(errorMessage(RuntimeException()).resolve(ru)).isEqualTo("Неизвестная ошибка")
    }

    @Test
    fun `errorMessage HttpException with error envelope`() {
        val ex = httpException(400, """{"error":"invalid credentials"}""")
        assertThat(errorMessage(ex).resolve(ru)).isEqualTo("Неверный email или пароль")
    }

    @Test
    fun `errorMessage HttpException with empty envelope falls back to code`() {
        val ex = httpException(500, """{"error":""}""")
        assertThat(errorMessage(ex).resolve(ru)).isEqualTo("Ошибка 500")
        assertThat(errorMessage(httpException(500, """{"error":""}""")).resolve(en)).isEqualTo("Error 500")
    }

    @Test
    fun `errorMessage HttpException with non-json body falls back to code`() {
        val ex = httpException(503, "gateway down")
        assertThat(errorMessage(ex).resolve(ru)).isEqualTo("Ошибка 503")
    }

    // ── rawErrorText ─────────────────────────────────────────────────────────

    /** По сырому тексту ветвится логика (приглашение вместо добавления участника),
     *  и он обязан остаться английским ответом сервера, а не переводом. */
    @Test
    fun `rawErrorText keeps the server wording`() {
        val ex = httpException(404, """{"error":"no user with this email"}""")
        assertThat(rawErrorText(ex)).isEqualTo("no user with this email")
        assertThat(rawErrorText(RuntimeException("boom"))).isEqualTo("boom")
        assertThat(rawErrorText(httpException(500, "gateway down"))).isNull()
    }

    // ── отказы редактора каналов уведомлений (#2932) ─────────────────────────

    /** Правила каналов клиент не зеркалит: до этих отказов доходят обычным
     *  путём, настраивая канал. Раньше сервер слал их зашитыми русскими фразами,
     *  и в английском UI они печатались дословно. */
    @Test
    fun `humanizeError words the channel refusals`() {
        assertThat(humanizeError("""unknown channel type: "carrier-pigeon"""")?.resolve(ru))
            .isEqualTo("Сервер не умеет каналы типа «carrier-pigeon»")
        assertThat(humanizeError("email channel requires address")?.resolve(ru))
            .isEqualTo("Каналу «email» нужно поле address")
        assertThat(humanizeError("telegram channel requires bot_token")?.resolve(en))
            .isEqualTo("The “telegram” channel needs bot_token")
        assertThat(humanizeError("template exceeds 8192 characters")?.resolve(ru))
            .isEqualTo("Шаблон длиннее 8192 символов — сократите его")
    }

    /** Хвост — слова парсера Go: перевода у них нет, и регистр им приходится
     *  сохранять, хотя сопоставление идёт по строке в нижнем. */
    @Test
    fun `humanizeError labels a template error but keeps the parser's words`() {
        assertThat(humanizeError("template error: template: msg:1: unclosed action")?.resolve(ru))
            .isEqualTo("Ошибка в шаблоне: template: msg:1: unclosed action")
    }

    /** Предупреждение едет с кодом 200, мимо перехватчика — его переводит сама
     *  ViewModel пробника. */
    @Test
    fun `humanizeError translates the SMTP warning`() {
        assertThat(humanizeError("smtp is not configured")?.resolve(ru))
            .isEqualTo("SMTP на сервере не настроен — письмо записано в лог, но не отправлено")
        assertThat(humanizeError("smtp is not configured")?.resolve(en))
            .isEqualTo("SMTP is not configured on the server — the email was logged, not sent")
    }

    // ── отказы «слишком большой» (#2932) ─────────────────────────────────────

    /** Транспортный отказ `middleware.BodyLimit`. Веб его переводит, телефон —
     *  нет, хотя доходит до него обычным путём: длинное описание задачи. */
    @Test
    fun `humanizeError maps the transport's body limit`() {
        assertThat(humanizeError("request body too large")?.resolve(ru))
            .isEqualTo("Слишком большой запрос — сократите текст или вложение")
        assertThat(humanizeError("request body too large")?.resolve(en))
            .isEqualTo("The request is too large — shorten the text or the attachment")
    }

    /** Валидатор документа называет свой потолок в БАЙТАХ, чтобы клиент перевёл
     *  его в единицы читателя. Редактор на телефоне сохраняет через тот же
     *  `PATCH /documents/:id/content`, и до правки печатал «content exceeds
     *  4194304 bytes» дословно. */
    @Test
    fun `humanizeError words the document ceiling in megabytes`() {
        assertThat(humanizeError("content exceeds 4194304 bytes")?.resolve(ru))
            .isEqualTo("Документ больше 4 МБ — разделите его на несколько страниц")
        assertThat(humanizeError("content exceeds 4194304 bytes")?.resolve(en))
            .isEqualTo("The document is larger than 4 MB — split it into several pages")
    }

    /** То же для суммы вложений чата конференции (`MaxChatTotalBytes`). Дробный
     *  потолок остаётся дробным — округлять его вниз значило бы соврать. */
    @Test
    fun `humanizeError words the chat batch ceiling in megabytes`() {
        assertThat(humanizeError("attachments exceed 26214400 bytes")?.resolve(ru))
            .isEqualTo("Вложения весят больше 25 МБ — отправьте их несколькими сообщениями")
        assertThat(humanizeError("attachments exceed 26214400 bytes")?.resolve(en))
            .isEqualTo("The attachments add up to more than 25 MB — send them in several messages")
    }

    /** Разделитель дробного потолка — локали читателя, а не `Locale.US`: тот же
     *  файл во вкладке «Файлы» и в чате звонка подписан «1,5 МБ»
     *  ([attachmentSizeText], [confFileSize], web `formatFileSize`), и отказ не
     *  может писать его иначе внутри одного русского интерфейса. */
    @Test
    fun `humanizeError writes the fractional ceiling in the reader's separator`() {
        assertThat(humanizeError("attachments exceed 1572864 bytes")?.resolve(ru))
            .isEqualTo("Вложения весят больше 1,5 МБ — отправьте их несколькими сообщениями")
        assertThat(humanizeError("attachments exceed 1572864 bytes")?.resolve(en))
            .isEqualTo("The attachments add up to more than 1.5 MB — send them in several messages")
    }

    /** Нераспознанное число едет как пришло: соврать про размер хуже, чем
     *  показать байты. Регекс пропустит только цифры, так что сюда доходит лишь
     *  то, что не влезло в `Long`. */
    @Test
    fun `humanizeError passes an unparsable ceiling through`() {
        val huge = "9".repeat(25)
        assertThat(humanizeError("content exceeds $huge bytes")?.resolve(ru))
            .isEqualTo("Документ больше $huge МБ — разделите его на несколько страниц")
    }

    /** Негативные контроли: шаблоны узкие и не переписывают обычную фразу,
     *  в которой случайно оказались те же слова. */
    @Test
    fun `humanizeError leaves lookalike sentences alone`() {
        assertThat(humanizeError("the channel requires maintenance")?.resolve(ru))
            .isEqualTo("the channel requires maintenance")
        assertThat(humanizeError("template exceeds expectations")?.resolve(ru))
            .isEqualTo("template exceeds expectations")
        assertThat(humanizeError("unknown channel type")?.resolve(ru))
            .isEqualTo("unknown channel type")
        assertThat(humanizeError("content exceeds expectations")?.resolve(ru))
            .isEqualTo("content exceeds expectations")
        assertThat(humanizeError("the attachments exceed 10 bytes of patience")?.resolve(ru))
            .isEqualTo("the attachments exceed 10 bytes of patience")
    }

    // ── isAuthError ──────────────────────────────────────────────────────────
    @Test
    fun `isAuthError only for 401 and 403`() {
        assertThat(isAuthError(httpException(401, "{}"))).isTrue()
        assertThat(isAuthError(httpException(403, "{}"))).isTrue()
        assertThat(isAuthError(httpException(500, "{}"))).isFalse()
        assertThat(isAuthError(IOException())).isFalse()
    }
}
