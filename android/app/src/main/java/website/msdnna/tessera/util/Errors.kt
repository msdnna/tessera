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
    "forbidden" to R.string.err_forbidden,
    "unauthorized" to R.string.err_unauthorized,
    "not found" to R.string.err_not_found,
)

private val validatorTag = Regex("""failed on the '(\w+)' tag""")

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
