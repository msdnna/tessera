package website.msdnna.tessera.util

import com.google.gson.Gson
import java.io.IOException
import retrofit2.HttpException
import website.msdnna.tessera.data.model.ApiError

private val gson = Gson()

/**
 * Maps a thrown call failure to a user-facing message, preferring the
 * backend's `{ "error": "..." }` envelope (mirrors the web client, which
 * surfaces `err.response.data.error`). The raw envelope is run through
 * [humanizeError] so bare English sentinels / validator noise become friendly
 * Russian — matching the web's central interceptor.
 */
fun errorMessage(t: Throwable): String = when (t) {
    is HttpException -> {
        val body = runCatching { t.response()?.errorBody()?.string() }.getOrNull()
        val parsed = body?.let { runCatching { gson.fromJson(it, ApiError::class.java) }.getOrNull() }
        parsed?.error?.takeIf { it.isNotBlank() }?.let(::humanizeError) ?: "Ошибка ${t.code()}"
    }

    is IOException -> "Нет соединения с сервером"

    else -> humanizeError(t.message) ?: "Неизвестная ошибка"
}

// Bare English sentinels the backend surfaces → friendly Russian. Mirrors the
// web `frontend/src/utils/errors.js` map; keep the two in sync.
private val sentinels = mapOf(
    "invalid credentials" to "Неверный email или пароль",
    "invalid email or password" to "Неверный email или пароль",
    "email already registered" to "Этот email уже зарегистрирован",
    "email already exists" to "Этот email уже зарегистрирован",
    "email already in use" to "Этот email уже используется",
    "user already exists" to "Пользователь с таким email уже существует",
    "user not found" to "Пользователь не найден",
    "invalid or expired token" to "Ссылка недействительна или устарела",
    "invalid token" to "Ссылка недействительна или устарела",
    "token expired" to "Срок действия ссылки истёк",
    "account is deactivated" to "Аккаунт деактивирован",
    "account deactivated" to "Аккаунт деактивирован",
    "forbidden" to "Недостаточно прав",
    "unauthorized" to "Нужно войти заново",
    "not found" to "Не найдено",
)

private val validatorTag = Regex("""failed on the '(\w+)' tag""")

// Collapse a gin/validator error ("...failed on the 'X' tag") into one friendly
// sentence based on which validation tags failed.
private fun fromValidator(raw: String): String? {
    val tags = validatorTag.findAll(raw).map { it.groupValues[1] }.toList()
    if (tags.isEmpty()) return null
    return when {
        "email" in tags -> "Введите корректный email"
        tags.all { it == "required" } -> "Заполните все обязательные поля"
        "min" in tags -> "Значение слишком короткое"
        "max" in tags -> "Значение слишком длинное"
        else -> "Проверьте правильность заполнения полей"
    }
}

/**
 * Turns a raw backend/network error string into friendly Russian. Recognised
 * sentinels and validator noise are mapped; short unknown messages pass through;
 * null/blank yields null so callers can fall back to their own default. Mirrors
 * the web `humanizeError`.
 */
fun humanizeError(raw: String?): String? {
    val msg = raw?.trim()
    if (msg.isNullOrEmpty()) return null
    val low = msg.lowercase()
    sentinels[low]?.let { return it }
    if (low.startsWith("key:") && low.contains("validation")) {
        return fromValidator(msg) ?: "Проверьте правильность заполнения полей"
    }
    if (low.contains("timeout") || low.contains("econnrefused") ||
        low.contains("failed to connect") || low.contains("unable to resolve host")
    ) {
        return "Нет связи с сервером. Проверьте подключение."
    }
    return msg
}

/**
 * True when a call failed because the session is invalid/forbidden (expired,
 * revoked, or the account was deactivated) — a 401/403 from the API. Used at
 * startup to tell "re-login" apart from "server unreachable".
 */
fun isAuthError(t: Throwable): Boolean = t is HttpException && (t.code() == 401 || t.code() == 403)
