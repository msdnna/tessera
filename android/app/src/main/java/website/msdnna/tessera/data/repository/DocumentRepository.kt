package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Document

/** Read-only access to workspace documents (#2735 — editing is web-only). */
class DocumentRepository {
    private val api get() = AppContainer.api()

    suspend fun list(workspaceId: String): List<Document> = api.documents(workspaceId).orEmpty()

    suspend fun get(id: String): Document = api.document(id)
}
