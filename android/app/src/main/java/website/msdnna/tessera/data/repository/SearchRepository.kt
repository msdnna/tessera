package website.msdnna.tessera.data.repository

import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.SearchResults

/** Workspace-scoped global search over task titles/descriptions + note text. */
class SearchRepository {
    private val api get() = AppContainer.api()

    suspend fun search(workspaceId: String, query: String): SearchResults =
        api.search(workspaceId, query)
}
