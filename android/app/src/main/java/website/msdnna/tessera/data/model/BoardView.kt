package website.msdnna.tessera.data.model

import com.google.gson.annotations.SerializedName

/**
 * A saved board view (server-side, per-user, cross-device). [config] mirrors the
 * web `KanbanBoard` view schema verbatim — camelCase keys, **not** snake_case —
 * so a view saved on web applies on Android and vice versa.
 */
data class BoardView(
    @SerializedName("id") val id: String = "",
    @SerializedName("board_id") val boardId: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("config") val config: BoardViewConfig = BoardViewConfig(),
)

/** The toolbar state a view captures. Keys match the web (camelCase) for interop. */
data class BoardViewConfig(
    @SerializedName("layout") val layout: String = "board", // board | list | calendar
    @SerializedName("groupMode") val groupMode: String = "status", // status | tag
    @SerializedName("tagPrefix") val tagPrefix: String = "",
    @SerializedName("sortLevels") val sortLevels: List<SortLevel> = emptyList(),
    @SerializedName("subtasksExpanded") val subtasksExpanded: Boolean = false,
    @SerializedName("autoSort") val autoSort: Boolean = false,
    @SerializedName("filters") val filters: BoardViewFilters = BoardViewFilters(),
)

/** One level of the multi-level sort. [field] ∈ priority|due|title|number; [dir] ∈ asc|desc. */
data class SortLevel(
    @SerializedName("field") val field: String = "priority",
    @SerializedName("dir") val dir: String = "asc",
)

data class BoardViewFilters(
    @SerializedName("priorities") val priorities: List<Int> = emptyList(),
    @SerializedName("assignees") val assignees: List<String> = emptyList(),
    @SerializedName("tags") val tags: List<String> = emptyList(),
    @SerializedName("statuses") val statuses: List<String> = emptyList(),
    @SerializedName("due") val due: String = "", // "" | overdue | today | week | has | none
    @SerializedName("q") val q: String = "",
)

data class SaveBoardViewRequest(
    @SerializedName("name") val name: String,
    @SerializedName("config") val config: BoardViewConfig,
)
