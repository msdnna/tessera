package website.msdnna.tessera.update

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.AckRequest

/**
 * Thin wrapper over the acknowledgement + version endpoints. Sits in `update/`
 * next to [UpdateRepository] and [WhatsNewEntries]: both answer "what changed
 * since the build the user had".
 */
class WhatsNewRepository {
    private val api get() = AppContainer.api()

    /** Every key this user has acknowledged, on any client. */
    suspend fun acknowledged(): Set<String> = api.acknowledgements().orEmpty().mapTo(mutableSetOf()) { it.key }

    /** Records a key as seen. Idempotent server-side — a repeat keeps the first timestamp. */
    suspend fun ack(key: String) {
        api.acknowledge(AckRequest(key))
    }

    /** The API's own version (`0.85.1`), or "" when the server doesn't answer. */
    suspend fun apiVersion(): String = api.apiVersion().api
}
