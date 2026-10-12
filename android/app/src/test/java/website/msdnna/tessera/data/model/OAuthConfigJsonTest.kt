package website.msdnna.tessera.data.model

import com.google.common.truth.Truth.assertThat
import com.google.gson.Gson
import com.google.gson.JsonParser
import org.junit.Test

/**
 * Тело `PUT /admin/oauth/gitlab` — ПОЛНАЯ замена (backend/handlers/oauth_admin.go
 * SetOAuthConfig): сервер собирает ряд провайдера из полей тела, и ключ, которого
 * в теле нет, Go связывает нулевым значением его типа.
 *
 * Для `sudo_writeback` это значит «выключить»: карточка OAuth на телефоне этот
 * ключ не везла вовсе, поэтому админ, сохранивший с телефона хоть что-нибудь —
 * хоть не изменив ни одного поля, — тихо снимал запись от имени действующего
 * пользователя, включённую из веба. Ни отказа, ни следа в интерфейсе; заметно
 * это только по тому, что обратная запись в GitLab начинает идти от сервисного
 * аккаунта (handlers/gitlab.go useSudo).
 *
 * Отсюда две проверки: ключ обязан быть В КАЖДОМ теле (а не только когда включён
 * — Gson не выбрасывает false, но выбросил бы nullable-поле), и ответ сервера
 * обязан читаться обратно, иначе карточке нечего везти назад.
 */
class OAuthConfigJsonTest {
    private val gson = Gson()

    private fun request(sudo: Boolean) = OAuthConfigRequest(
        clientId = "abc",
        clientSecret = "",
        glBaseUrl = "https://gitlab.example.com",
        enabled = true,
        orgMap = JsonParser.parseString("{}"),
        serviceToken = "",
        sudoWriteback = sudo,
    )

    @Test
    fun `a save carries sudo_writeback even when it is off`() {
        val body = JsonParser.parseString(gson.toJson(request(sudo = false))).asJsonObject

        assertThat(body.keySet()).contains("sudo_writeback")
        assertThat(body["sudo_writeback"].asBoolean).isFalse()
    }

    @Test
    fun `a save carries sudo_writeback when it is on`() {
        val body = JsonParser.parseString(gson.toJson(request(sudo = true))).asJsonObject

        assertThat(body["sudo_writeback"].asBoolean).isTrue()
    }

    @Test
    fun `the stored flag is read back from the server's answer`() {
        val wire = """
            {"provider":"gitlab","client_id":"abc","gl_base_url":"https://gitlab.example.com",
             "enabled":true,"org_map":{},"has_secret":true,"has_service_token":true,
             "sudo_writeback":true}
        """.trimIndent()

        val cfg = gson.fromJson(wire, OAuthConfig::class.java)

        assertThat(cfg.sudoWriteback).isTrue()
    }

    @Test
    fun `an answer without the flag reads as off`() {
        // Инстанс, где провайдера ещё нет: сервер отдаёт пустой вид, и карточка
        // не должна включать переключатель на пустом месте.
        val cfg = gson.fromJson("""{"provider":"gitlab","org_map":{}}""", OAuthConfig::class.java)

        assertThat(cfg.sudoWriteback).isFalse()
    }
}
