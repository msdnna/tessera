package website.msdnna.tessera.data.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.core.stringSetPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.model.User

val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "tessera")

/**
 * DataStore-backed app state: session tokens, the current user, the server
 * override and theme prefs. Online-first means this holds session + settings
 * only — no domain data is persisted here.
 */
class AppPreferences(private val context: Context) {
    private object Keys {
        val SERVER_URL = stringPreferencesKey("server_url")
        val AUTH_TOKEN = stringPreferencesKey("auth_token")
        val REFRESH_TOKEN = stringPreferencesKey("refresh_token")
        val USER_ID = stringPreferencesKey("user_id")
        val USER_EMAIL = stringPreferencesKey("user_email")
        val USER_NAME = stringPreferencesKey("user_name")
        val IS_ADMIN = booleanPreferencesKey("is_admin")
        val ACCENT_KEY = stringPreferencesKey("accent_key")
        val DARK_MODE = booleanPreferencesKey("dark_mode")
        val CURRENT_WORKSPACE = stringPreferencesKey("current_workspace")

        // Restored-on-launch UI state (online-first: UI state only, no domain data).
        val EXPANDED_GROUPS = stringSetPreferencesKey("expanded_groups")
        val EXPANDED_PROJECTS = stringSetPreferencesKey("expanded_projects")
        val LAST_DEST = stringPreferencesKey("last_dest")
    }

    /** User-set server override; falls back to the build's default base URL. */
    val serverUrl: Flow<String> = context.dataStore.data
        .map { it[Keys.SERVER_URL]?.takeIf { url -> url.isNotBlank() } ?: BuildConfig.DEFAULT_BASE_URL }

    val authToken: Flow<String> = context.dataStore.data.map { it[Keys.AUTH_TOKEN] ?: "" }
    val refreshToken: Flow<String> = context.dataStore.data.map { it[Keys.REFRESH_TOKEN] ?: "" }

    val user: Flow<User?> = context.dataStore.data.map { prefs ->
        val id = prefs[Keys.USER_ID] ?: return@map null
        if (id.isBlank()) return@map null
        User(
            id = id,
            email = prefs[Keys.USER_EMAIL] ?: "",
            name = prefs[Keys.USER_NAME] ?: "",
            isAdmin = prefs[Keys.IS_ADMIN] ?: false,
        )
    }

    val accentKey: Flow<String> = context.dataStore.data.map { it[Keys.ACCENT_KEY] ?: "purple" }
    val darkMode: Flow<Boolean> = context.dataStore.data.map { it[Keys.DARK_MODE] ?: false }

    suspend fun setServerUrl(url: String) {
        context.dataStore.edit {
            if (url.isBlank()) it.remove(Keys.SERVER_URL) else it[Keys.SERVER_URL] = url.trim()
        }
    }

    suspend fun setSession(tokens: Pair<String, String>, user: User?) {
        context.dataStore.edit { prefs ->
            prefs[Keys.AUTH_TOKEN] = tokens.first
            prefs[Keys.REFRESH_TOKEN] = tokens.second
            if (user != null) {
                prefs[Keys.USER_ID] = user.id
                prefs[Keys.USER_EMAIL] = user.email
                prefs[Keys.USER_NAME] = user.name
                prefs[Keys.IS_ADMIN] = user.isAdmin
            }
        }
    }

    suspend fun setTokens(access: String, refresh: String) {
        context.dataStore.edit { prefs ->
            prefs[Keys.AUTH_TOKEN] = access
            if (refresh.isNotBlank()) prefs[Keys.REFRESH_TOKEN] = refresh
        }
    }

    suspend fun setUser(user: User) {
        context.dataStore.edit { prefs ->
            prefs[Keys.USER_ID] = user.id
            prefs[Keys.USER_EMAIL] = user.email
            prefs[Keys.USER_NAME] = user.name
            prefs[Keys.IS_ADMIN] = user.isAdmin
        }
    }

    suspend fun clearSession() {
        context.dataStore.edit { prefs ->
            prefs.remove(Keys.AUTH_TOKEN)
            prefs.remove(Keys.REFRESH_TOKEN)
            prefs.remove(Keys.USER_ID)
            prefs.remove(Keys.USER_EMAIL)
            prefs.remove(Keys.USER_NAME)
            prefs.remove(Keys.IS_ADMIN)
        }
    }

    val currentWorkspaceId: Flow<String> = context.dataStore.data.map { it[Keys.CURRENT_WORKSPACE] ?: "" }

    suspend fun setCurrentWorkspaceId(id: String) {
        context.dataStore.edit { it[Keys.CURRENT_WORKSPACE] = id }
    }

    /** Per-board saved view (filter/sort/group), serialised JSON. "" if none. */
    suspend fun boardViewJson(boardId: String): String =
        context.dataStore.data.first()[stringPreferencesKey("view_$boardId")] ?: ""

    suspend fun setBoardViewJson(boardId: String, json: String) {
        context.dataStore.edit { it[stringPreferencesKey("view_$boardId")] = json }
    }

    /** Sidebar tree expand state, restored on launch (web parity: `useTreeExpand`). */
    val expandedGroups: Flow<Set<String>> = context.dataStore.data.map { it[Keys.EXPANDED_GROUPS] ?: emptySet() }
    val expandedProjects: Flow<Set<String>> = context.dataStore.data.map { it[Keys.EXPANDED_PROJECTS] ?: emptySet() }

    suspend fun setExpandedGroups(ids: Set<String>) {
        context.dataStore.edit { it[Keys.EXPANDED_GROUPS] = ids }
    }

    suspend fun setExpandedProjects(ids: Set<String>) {
        context.dataStore.edit { it[Keys.EXPANDED_PROJECTS] = ids }
    }

    /**
     * The last active top-level destination, restored on launch. Encoded as
     * `"home"` / `"notes"` / `"reminders"` / `"board:<boardId>"`.
     */
    val lastDest: Flow<String> = context.dataStore.data.map { it[Keys.LAST_DEST] ?: "" }

    suspend fun setLastDest(dest: String) {
        context.dataStore.edit { it[Keys.LAST_DEST] = dest }
    }

    suspend fun setAccentKey(key: String) {
        context.dataStore.edit { it[Keys.ACCENT_KEY] = key }
    }

    suspend fun setDarkMode(dark: Boolean) {
        context.dataStore.edit { it[Keys.DARK_MODE] = dark }
    }
}
