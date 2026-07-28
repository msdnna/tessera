package website.msdnna.tessera.data.preferences

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.core.stringSetPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.google.gson.Gson
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.data.model.Preferences as UserPrefs
import website.msdnna.tessera.data.model.User

val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "tessera")

/**
 * DataStore-backed app state: session tokens, the current user, the server
 * override and theme prefs. Online-first means this holds session + settings
 * only — no domain data is persisted here.
 */
class AppPreferences(private val context: Context) {
    private val gson = Gson()

    private object Keys {
        val SERVER_URL = stringPreferencesKey("server_url")
        val AUTH_TOKEN = stringPreferencesKey("auth_token")
        val REFRESH_TOKEN = stringPreferencesKey("refresh_token")
        val USER_ID = stringPreferencesKey("user_id")
        val USER_EMAIL = stringPreferencesKey("user_email")
        val USER_NAME = stringPreferencesKey("user_name")
        val IS_ADMIN = booleanPreferencesKey("is_admin")
        val USER_JSON = stringPreferencesKey("user_json") // full profile (avatar/legal name/…)
        val PREFS_JSON = stringPreferencesKey("prefs_json") // full user preferences
        val ACCENT_KEY = stringPreferencesKey("accent_key")
        val DARK_MODE = booleanPreferencesKey("dark_mode")
        val CURRENT_WORKSPACE = stringPreferencesKey("current_workspace")

        // Restored-on-launch UI state (online-first: UI state only, no domain data).
        val EXPANDED_GROUPS = stringSetPreferencesKey("expanded_groups")
        val EXPANDED_PROJECTS = stringSetPreferencesKey("expanded_projects")
        val LAST_DEST = stringPreferencesKey("last_dest")
        val DEVICE_ID = stringPreferencesKey("device_id")
        val RECENT_ASSIGNEES = stringPreferencesKey("recent_assignees")
    }

    /** Stable id for the notification "device" channel so this device is routable and
     *  the backend upserts a single row (not a new duplicate on every re-login).
     *
     *  Anchored to the hardware `ANDROID_ID` (survives logout/reinstall until a factory
     *  reset), which keeps the id identical even if DataStore is cleared. Falls back to a
     *  persisted random UUID when ANDROID_ID is missing/unreliable. Once resolved the id
     *  is persisted, so an existing install keeps whatever id it already registered. */
    suspend fun ensureDeviceId(): String {
        val existing = context.dataStore.data.first()[Keys.DEVICE_ID]
        if (!existing.isNullOrBlank()) return existing
        val id = "android-" + (stableHardwareId() ?: java.util.UUID.randomUUID().toString())
        context.dataStore.edit { it[Keys.DEVICE_ID] = id }
        return id
    }

    /** Settings.Secure.ANDROID_ID, unless it's blank or the well-known buggy emulator
     *  constant (`9774d56d682e549c`) that isn't unique across devices. */
    private fun stableHardwareId(): String? {
        val androidId = try {
            android.provider.Settings.Secure.getString(
                context.contentResolver,
                android.provider.Settings.Secure.ANDROID_ID,
            )
        } catch (_: Exception) {
            null
        }
        return androidId?.takeIf { it.isNotBlank() && it != "9774d56d682e549c" }
    }

    /** User-set server override; falls back to the build's default base URL. */
    val serverUrl: Flow<String> = context.dataStore.data
        .map { it[Keys.SERVER_URL]?.takeIf { url -> url.isNotBlank() } ?: BuildConfig.DEFAULT_BASE_URL }

    val authToken: Flow<String> = context.dataStore.data.map { it[Keys.AUTH_TOKEN] ?: "" }
    val refreshToken: Flow<String> = context.dataStore.data.map { it[Keys.REFRESH_TOKEN] ?: "" }

    val user: Flow<User?> = context.dataStore.data.map { prefs ->
        // Prefer the full JSON profile; fall back to the legacy per-field keys
        // (installs upgraded from before the profile fields existed).
        prefs[Keys.USER_JSON]?.let { json ->
            runCatching { gson.fromJson(json, User::class.java) }.getOrNull()?.takeIf { it.id.isNotBlank() }
        }?.let { return@map it }
        val id = prefs[Keys.USER_ID] ?: return@map null
        if (id.isBlank()) return@map null
        User(
            id = id,
            email = prefs[Keys.USER_EMAIL] ?: "",
            name = prefs[Keys.USER_NAME] ?: "",
            isAdmin = prefs[Keys.IS_ADMIN] ?: false,
        )
    }

    /** Full user preferences (cache of the server's; defaults until hydrated). */
    val preferences: Flow<UserPrefs> = context.dataStore.data.map { prefs ->
        prefs[Keys.PREFS_JSON]?.let { runCatching { gson.fromJson(it, UserPrefs::class.java) }.getOrNull() } ?: UserPrefs()
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
                prefs[Keys.USER_JSON] = gson.toJson(user)
            }
        }
    }

    /** Caches the full preferences and keeps the theme-apply keys (accent/dark) in
     *  sync so the existing theme path follows them. theme 'dark' → dark on. */
    suspend fun setPreferences(p: UserPrefs) {
        context.dataStore.edit { prefs ->
            prefs[Keys.PREFS_JSON] = gson.toJson(p)
            prefs[Keys.ACCENT_KEY] = p.accent
            prefs[Keys.DARK_MODE] = p.theme == "dark"
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
            prefs[Keys.USER_JSON] = gson.toJson(user)
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
            prefs.remove(Keys.USER_JSON)
            prefs.remove(Keys.PREFS_JSON)
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

    /** Cross-board MRU of recently-assigned user ids (web `tessera_recent_assignees`),
     *  surfacing the people you actually use first in the on-card assignee picker.
     *  Stored newest-first as a newline-joined string (ids are UUIDs). */
    val recentAssignees: Flow<List<String>> = context.dataStore.data.map {
        it[Keys.RECENT_ASSIGNEES].orEmpty().split('\n').filter { id -> id.isNotBlank() }
    }

    suspend fun bumpRecentAssignee(id: String) {
        if (id.isBlank()) return
        context.dataStore.edit { prefs ->
            val cur = prefs[Keys.RECENT_ASSIGNEES].orEmpty().split('\n').filter { it.isNotBlank() }
            prefs[Keys.RECENT_ASSIGNEES] = (listOf(id) + cur.filter { it != id }).take(30).joinToString("\n")
        }
    }

    suspend fun setAccentKey(key: String) {
        context.dataStore.edit { it[Keys.ACCENT_KEY] = key }
    }

    suspend fun setDarkMode(dark: Boolean) {
        context.dataStore.edit { it[Keys.DARK_MODE] = dark }
    }
}
