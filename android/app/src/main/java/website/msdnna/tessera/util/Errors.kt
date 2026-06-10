package website.msdnna.tessera.util

import com.google.gson.Gson
import java.io.IOException
import retrofit2.HttpException
import website.msdnna.tessera.data.model.ApiError

private val gson = Gson()

/**
 * Maps a thrown call failure to a user-facing message, preferring the
 * backend's `{ "error": "..." }` envelope (mirrors the web client, which
 * surfaces `err.response.data.error`).
 */
fun errorMessage(t: Throwable): String = when (t) {
    is HttpException -> {
        val body = runCatching { t.response()?.errorBody()?.string() }.getOrNull()
        val parsed = body?.let { runCatching { gson.fromJson(it, ApiError::class.java) }.getOrNull() }
        parsed?.error?.takeIf { it.isNotBlank() } ?: "Ошибка ${t.code()}"
    }

    is IOException -> "Нет соединения с сервером"

    else -> t.message ?: "Неизвестная ошибка"
}
