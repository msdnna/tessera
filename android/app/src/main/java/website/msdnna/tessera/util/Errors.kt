package website.msdnna.tessera.util

import com.google.gson.Gson
import java.io.IOException
import retrofit2.HttpException
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.ApiError
import website.msdnna.tessera.ui.UiText

private val gson = Gson()

/**
 * Maps a thrown call failure to a user-facing message, preferring the
 * backend's `{ "error": "..." }` envelope (mirrors the web client, which
 * surfaces `err.response.data.error`). The raw envelope is run through
 * [humanizeError] so bare English sentinels / validator noise become a friendly
 * string resource — matching the web's central interceptor.
 *
 * Возвращается [UiText], а не готовая строка: ошибка рождается во ViewModel, где
 * `Resources` нет, и живёт в состоянии до следующей перерисовки — готовый текст
 * застыл бы на языке момента сбоя. Незнакомое сообщение сервера едет как
 * [UiText.Raw]: переводить его нечем.
 */
fun errorMessage(t: Throwable): UiText = when (t) {
    is HttpException ->
        humanizeError(apiError(t)) ?: UiText.Res(R.string.err_http_code, listOf(t.code()))

    is IOException -> UiText.Res(R.string.err_no_connection)

    else -> humanizeError(t.message) ?: UiText.Res(R.string.err_unknown)
}

/**
 * Сырой текст ошибки — то, что сказал сервер, до всякого причёсывания.
 *
 * Нужен там, где по ошибке ветвится логика, а не только показ: сравнивать
 * приходится с английским сентинелом сервера, и делать это по локализованному
 * тексту нельзя — он меняется вместе с языком интерфейса.
 */
fun rawErrorText(t: Throwable): String? = when (t) {
    is HttpException -> apiError(t)
    else -> t.message
}

/** `{ "error": "..." }` из тела ответа, если он там есть и не пустой. */
private fun apiError(t: HttpException): String? {
    val body = runCatching { t.response()?.errorBody()?.string() }.getOrNull()
    val parsed = body?.let { runCatching { gson.fromJson(it, ApiError::class.java) }.getOrNull() }
    return parsed?.error?.takeIf { it.isNotBlank() }
}

// Bare English sentinels the backend surfaces → friendly resource. Mirrors the
// web `frontend/src/utils/errors.js` map; keep the two in sync.
private val sentinels = mapOf(
    "invalid credentials" to R.string.err_invalid_credentials,
    "invalid email or password" to R.string.err_invalid_credentials,
    // Оба конца правила пароля (#2932). Сюда доходит лишь то, что прошло мимо
    // собственной проверки формы — она зеркалит правило (util/Password.kt), —
    // но без этих двух строк отказ показывался бы по-английски в русском UI.
    "password is too short" to R.string.err_password_too_short,
    "password is too long" to R.string.err_password_too_long,
    "email already registered" to R.string.err_email_registered,
    "email already exists" to R.string.err_email_registered,
    "email already in use" to R.string.err_email_in_use,
    "user already exists" to R.string.err_user_exists,
    "user not found" to R.string.err_user_not_found,
    "invalid or expired token" to R.string.err_link_invalid,
    "invalid token" to R.string.err_link_invalid,
    "token expired" to R.string.err_link_expired,
    "account is deactivated" to R.string.err_account_deactivated,
    "account deactivated" to R.string.err_account_deactivated,
    // Опечатка в поле «текущий пароль» (#2932) — единственный отказ из этого
    // списка, до которого доходят обычным путём. Печатается прямо в форме
    // (ProfileScreen), так что английская строка оставалась на экране.
    "current password is incorrect" to R.string.err_current_password_wrong,
    // Добавление участника по адресу, за которым нет аккаунта. На вебе по этому
    // же ответу ветвится предложение отправить приглашение — но по сырой строке
    // (`rawErrorText`), а не по этой фразе.
    "no user with that email" to R.string.err_no_user_with_email,
    "job is already running" to R.string.err_job_already_running,
    "too many approvers" to R.string.err_too_many_approvers,
    // Отказ ограничителя частоты (`middleware.RateLimit`) — единственная строка
    // здесь, до которой доходят, не сделав ничего неправильного (#2932). Вход
    // throttled на десять попыток в минуту, и ключ считается И по IP, И по
    // адресу: офис за одним NAT делит один бюджет. Опечатка в пароле утром в
    // понедельник — и форма отвечала английской фразой, которая вдобавок молчит
    // о главном: что помогает просто подождать.
    "too many requests" to R.string.err_too_many_requests,
    // Две границы сохранённого вида доски. Клиент их не зеркалит, а сохранение
    // вида на телефоне есть (BoardComposer → `saveView`), так что до обеих
    // доходят обычным путём — как на вебе, откуда эта карта и списана.
    "view limit reached" to R.string.err_board_view_limit_reached,
    "view config is too large" to R.string.err_board_view_config_too_large,
    // Четыре гварда членства в пространстве и ранг, который остаётся после них
    // (#2932). Модалка участников гасит свою строку (см. MembersModal), так что
    // сюда доходит устаревшая вкладка: другой админ поменял роли, пока эта была
    // открыта.
    "cannot change the workspace owner's role" to R.string.err_cannot_change_owner_role,
    "cannot remove the workspace owner" to R.string.err_cannot_remove_owner,
    "cannot change your own role" to R.string.err_cannot_change_own_role,
    "cannot remove yourself from the workspace" to R.string.err_cannot_remove_self,
    // Предупреждение, а не отказ, и единственное здесь, что едет с кодом 200:
    // пробник канала отвечает {ok:true, warning}, когда на сервере нет SMTP.
    // Перехватчик его не видит — вызов-то успешен, — поэтому ViewModel прогоняет
    // строку через humanizeError сама.
    "smtp is not configured" to R.string.err_smtp_not_configured,
    // Отказ самого транспорта (middleware.BodyLimit), а не хендлера — оттого и
    // звучит как протокольная ошибка, а не как что-то, сделанное человеком. До
    // него доходят обычным путём: достаточно написать очень длинное описание
    // задачи (#2932).
    "request body too large" to R.string.err_body_too_large,
    "requires admin or owner" to R.string.err_requires_manager,
    "requires admin" to R.string.err_requires_admin,
    "forbidden" to R.string.err_forbidden,
    "unauthorized" to R.string.err_unauthorized,
    "not found" to R.string.err_not_found,
)

private val validatorTag = Regex("""failed on the '(\w+)' tag""")

// Отказы редактора каналов уведомлений (#2932). В отличие от большинства строк
// выше это не редкость из устаревшей вкладки: правила каналов клиент не зеркалит,
// так что до них доходят обычным путём — при настройке канала. У каждого есть
// переменная часть (тип, недостающее поле, потолок), поэтому они шаблоны, а не
// строки таблицы. Тип перечислён явно: на проводе это закрытый набор, а свободное
// `(\w+) channel requires …` переписало бы обычную английскую фразу с теми же
// тремя словами. Зеркало `frontend/src/utils/errors.js` — держать в синхроне.
private val channelTypeUnknown = Regex("""^unknown channel type: "(.*)"$""")
private val channelRequiresField =
    Regex("""^(email|telegram|webhook|shoutrrr|device) channel requires ([a-z_]+)$""")
private val templateTooLong = Regex("""^template exceeds (\d+) characters$""")

// Два отказа, которые называют свой потолок В БАЙТАХ, чтобы каждый клиент
// перевёл число в единицы читателя (#2932). Веб так и делает; телефон печатал
// английскую фразу с семизначным счётчиком посреди русского экрана.
//
// Первый — валидатор документа (`handlers/document_schema.go`): он всплывает в
// строке автосохранения редактора, и читателю нужно «4 МБ» и понимание, что
// страницу надо разделить. Второй — чат конференции, отказ на СУММУ вложений
// (`MaxChatTotalBytes`): композер правило зеркалит (util/ConferenceChat.kt), так
// что сюда доходит только вторая вкладка или второй пикер.
private val docContentTooLarge = Regex("""^content exceeds (\d+) bytes$""")
private val chatAttachmentsTooLarge = Regex("""^attachments exceed (\d+) bytes$""")

// Своя тут только обёртка: хвост — слова самого парсера Go, они называют
// сломанное выражение и перевода не имеют, поэтому едут дословно и с исходным
// регистром (из `msg`, а не из приведённой к нижнему копии).
private const val TEMPLATE_ERROR_PREFIX = "template error: "

/**
 * Потолок из сообщения сервера — в мебибайтах, как его печатает веб
 * (`formatFileSize`): дробная часть только когда она есть, так что 4194304
 * читается «4», а не «4.0». Нераспознанное число едет как пришло — соврать про
 * размер хуже, чем показать байты.
 *
 * Число возвращается [UiText.Decimal], а не готовой строкой: разделитель — дело
 * локали читателя, а здесь, во ViewModel, она ещё не видна. Считать её
 * `Locale.getDefault()` нельзя — язык профиля живёт в ресурсах композиции
 * ([AppLocale]), а телефон вполне может стоять на другом; ровно этот раскол уже
 * разбирали в [attachmentSizeText]. До правки потолок печатался
 * `Locale.US`-точкой посреди русской фразы: «Документ больше 1.5 МБ», тогда как
 * тот же файл во вкладке «Файлы», в документах и в чате звонка — «1,5 МБ».
 */
private fun megabytes(raw: String): UiText {
    val bytes = raw.toLongOrNull() ?: return UiText.Raw(raw)
    return UiText.Decimal(bytes.toDouble() / (1L shl 20))
}

// Разбор пары «слишком большой» отказов. Отдельной функцией по той же причине,
// что и [channelRefusal] ниже: в [humanizeError] это были бы ранние возвраты
// подряд на одну тему.
private fun tooLargeRefusal(low: String): UiText? {
    docContentTooLarge.find(low)?.let {
        return UiText.Res(R.string.err_doc_content_too_large, listOf(megabytes(it.groupValues[1])))
    }
    chatAttachmentsTooLarge.find(low)?.let {
        return UiText.Res(
            R.string.err_chat_attachments_too_large,
            listOf(megabytes(it.groupValues[1])),
        )
    }
    return null
}

// Разбор той самой четвёрки. Вынесен из [humanizeError] отдельной функцией: там
// это были бы четыре ранних возврата подряд на одну тему.
private fun channelRefusal(msg: String, low: String): UiText? {
    channelTypeUnknown.find(low)?.let {
        return UiText.Res(R.string.err_channel_type_unknown, listOf(it.groupValues[1]))
    }
    channelRequiresField.find(low)?.let {
        return UiText.Res(
            R.string.err_channel_requires_field,
            listOf(it.groupValues[1], it.groupValues[2]),
        )
    }
    templateTooLong.find(low)?.let {
        return UiText.Res(R.string.err_template_too_long, listOf(it.groupValues[1]))
    }
    if (low.startsWith(TEMPLATE_ERROR_PREFIX)) {
        return UiText.Res(
            R.string.err_template_error,
            listOf(msg.substring(TEMPLATE_ERROR_PREFIX.length)),
        )
    }
    return null
}

// Collapse a gin/validator error ("...failed on the 'X' tag") into one friendly
// sentence based on which validation tags failed.
private fun fromValidator(raw: String): Int? {
    val tags = validatorTag.findAll(raw).map { it.groupValues[1] }.toList()
    if (tags.isEmpty()) return null
    return when {
        "email" in tags -> R.string.err_invalid_email
        tags.all { it == "required" } -> R.string.err_required_fields
        "min" in tags -> R.string.err_value_too_short
        "max" in tags -> R.string.err_value_too_long
        else -> R.string.err_check_fields
    }
}

/**
 * Turns a raw backend/network error string into a friendly [UiText]. Recognised
 * sentinels and validator noise map to resources; short unknown messages pass
 * through as [UiText.Raw]; null/blank yields null so callers can fall back to
 * their own default. Mirrors the web `humanizeError`.
 */
fun humanizeError(raw: String?): UiText? {
    val msg = raw?.trim()
    if (msg.isNullOrEmpty()) return null
    val low = msg.lowercase()
    sentinels[low]?.let { return UiText.Res(it) }
    // Одним возвратом на обе группы шаблонов: ReturnCount этой функции detekt
    // считает по лимиту, и «ещё один ранний return» её краснит.
    (tooLargeRefusal(low) ?: channelRefusal(msg, low))?.let { return it }
    if (low.startsWith("key:") && low.contains("validation")) {
        return UiText.Res(fromValidator(msg) ?: R.string.err_check_fields)
    }
    if (low.contains("timeout") || low.contains("econnrefused") ||
        low.contains("failed to connect") || low.contains("unable to resolve host")
    ) {
        return UiText.Res(R.string.err_offline)
    }
    return UiText.Raw(msg)
}

/**
 * True when a call failed because the session is invalid/forbidden (expired,
 * revoked, or the account was deactivated) — a 401/403 from the API. Used at
 * startup to tell "re-login" apart from "server unreachable".
 */
fun isAuthError(t: Throwable): Boolean = t is HttpException && (t.code() == 401 || t.code() == 403)
