package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.compose.AsyncImage
import com.google.gson.GsonBuilder
import com.google.gson.JsonParser
import kotlinx.coroutines.launch
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.AdminUser
import website.msdnna.tessera.data.model.OAuthConfigRequest
import website.msdnna.tessera.data.model.SetActiveRequest
import website.msdnna.tessera.data.model.SetAdminRequest
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion

/**
 * Global admin panel (U3): list every account on the instance and manage it —
 * copy a password-reset link, grant/revoke admin, activate/deactivate. Mirrors
 * the web `/admin` page. Every action re-checks `is_admin` on the server; the
 * sidebar only surfaces this screen to admins.
 */
@Composable
fun AdminScreen() {
    val c = Tessera.colors
    val scope = rememberCoroutineScope()
    val clipboard = LocalClipboardManager.current
    val ctx = LocalContext.current
    // Тост показывается вне композиции, поэтому строку берём из ресурсов, которые
    // подменил AppLocale, — LocalContext остаётся на системной локали.
    val res = LocalResources.current
    val api = AppContainer.api()
    val me by AppContainer.prefs.user.collectAsStateWithLifecycle(initialValue = null)

    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var query by remember { mutableStateOf("") }
    var busyId by remember { mutableStateOf("") }
    val users = remember { mutableStateListOf<AdminUser>() }
    // Pending destructive confirm: (user, makeActive?/makeAdmin?) — null = none.
    var confirmActive by remember { mutableStateOf<AdminUser?>(null) }
    var confirmAdmin by remember { mutableStateOf<AdminUser?>(null) }

    suspend fun reload() {
        loading = true
        error = null
        runCatching { api.adminUsers().orEmpty() }
            .onSuccess {
                users.clear()
                users.addAll(it)
            }
            .onFailure { error = it.message ?: res.getString(R.string.admin_load_failed) }
        loading = false
    }

    LaunchedEffect(Unit) { reload() }

    fun replace(updated: AdminUser) {
        val i = users.indexOfFirst { it.id == updated.id }
        if (i >= 0) users[i] = updated
    }

    fun setActive(u: AdminUser, active: Boolean) {
        busyId = u.id
        scope.launch {
            runCatching { api.adminSetActive(u.id, SetActiveRequest(active)) }
                .onSuccess { replace(u.copy(active = active)) }
                .onFailure { error = it.message }
            busyId = ""
        }
    }

    fun setAdmin(u: AdminUser, admin: Boolean) {
        busyId = u.id
        scope.launch {
            runCatching { api.adminSetAdmin(u.id, SetAdminRequest(admin)) }
                .onSuccess { replace(u.copy(isAdmin = admin)) }
                .onFailure { error = it.message }
            busyId = ""
        }
    }

    fun copyResetLink(u: AdminUser) {
        busyId = u.id
        scope.launch {
            runCatching { api.adminResetLink(u.id).link }
                .onSuccess { link ->
                    // The backend returns a path-only link when PUBLIC_URL is unset —
                    // qualify it against the server root so the copied value is usable.
                    val full = if (link.startsWith("http")) link else RetrofitClient.serverRoot + link
                    clipboard.setText(AnnotatedString(full))
                    android.widget.Toast
                        .makeText(ctx, res.getString(R.string.admin_reset_link_copied), android.widget.Toast.LENGTH_SHORT)
                        .show()
                }
                .onFailure { error = it.message }
            busyId = ""
        }
    }

    val filtered = users.filter {
        val q = query.trim().lowercase()
        q.isEmpty() || it.name.lowercase().contains(q) || it.email.lowercase().contains(q)
    }

    Column(Modifier.fillMaxSize().background(c.bg).padding(horizontal = 14.dp)) {
        Row(Modifier.padding(top = 14.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            IonIcon(Ion.SHIELD_CHECKMARK, size = 22.dp, tint = c.primary)
            Spacer(Modifier.width(8.dp))
            Text(stringResource(R.string.nav_admin), color = c.text1, fontSize = 20.sp, fontWeight = FontWeight.Bold)
        }
        Spacer(Modifier.height(12.dp))
        OAuthConfigCard()
        Spacer(Modifier.height(14.dp))
        Text(stringResource(R.string.admin_users_count, users.size), color = c.text3, fontSize = 13.sp)
        Spacer(Modifier.height(12.dp))
        TTextField(value = query, onValueChange = { query = it }, placeholder = stringResource(R.string.admin_search_placeholder))
        TFormError(error, modifier = Modifier.padding(top = 6.dp))
        Spacer(Modifier.height(10.dp))

        when {
            loading -> Text(stringResource(R.string.common_loading), color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(top = 20.dp))

            filtered.isEmpty() -> Text(stringResource(R.string.admin_nobody_found), color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(top = 20.dp))

            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                items(filtered, key = { it.id }) { u ->
                    UserRow(
                        u = u,
                        isMe = u.id == me?.id,
                        busy = busyId == u.id,
                        onCopyReset = { copyResetLink(u) },
                        onToggleAdmin = { if (u.isAdmin) confirmAdmin = u else setAdmin(u, true) },
                        onToggleActive = { if (u.active) confirmActive = u else setActive(u, true) },
                    )
                }
            }
        }
    }

    confirmAdmin?.let { u ->
        TConfirmDialog(
            title = stringResource(R.string.admin_demote_title),
            message = stringResource(R.string.admin_demote_message, u.name),
            confirmText = stringResource(R.string.admin_demote_confirm),
            onConfirm = {
                setAdmin(u, false)
                confirmAdmin = null
            },
            onDismiss = { confirmAdmin = null },
        )
    }
    confirmActive?.let { u ->
        TConfirmDialog(
            title = stringResource(R.string.admin_deactivate_title),
            message = stringResource(R.string.admin_deactivate_message, u.name),
            confirmText = stringResource(R.string.admin_deactivate),
            onConfirm = {
                setActive(u, false)
                confirmActive = null
            },
            onDismiss = { confirmActive = null },
        )
    }
}

/**
 * Admin config for the GitLab OAuth provider (login-with-GitLab) — mirrors the web
 * admin panel's OAuth card. Collapsible; loads lazily on first expand. The client
 * secret and instance-wide service token are write-only: an empty field keeps the
 * stored value (a placeholder reports whether one is set). `org_map` is edited as
 * raw JSON (group-path → {workspace_id, admins, users}).
 */
@Composable
private fun OAuthConfigCard() {
    val c = Tessera.colors
    val scope = rememberCoroutineScope()
    val ctx = LocalContext.current
    // Как и в списке аккаунтов: тост уходит наружу из композиции, строки — из
    // подменённых AppLocale ресурсов, а не из системного LocalContext.
    val res = LocalResources.current
    val api = AppContainer.api()
    val pretty = remember { GsonBuilder().setPrettyPrinting().create() }

    var expanded by remember { mutableStateOf(false) }
    var loaded by remember { mutableStateOf(false) }
    var loading by remember { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }

    var glBaseUrl by remember { mutableStateOf("") }
    var clientId by remember { mutableStateOf("") }
    var clientSecret by remember { mutableStateOf("") }
    var serviceToken by remember { mutableStateOf("") }
    var enabled by remember { mutableStateOf(false) }
    var orgMapText by remember { mutableStateOf("{}") }
    var hasSecret by remember { mutableStateOf(false) }
    var hasServiceToken by remember { mutableStateOf(false) }

    suspend fun load() {
        loading = true
        error = null
        runCatching { api.getOAuthConfig() }
            .onSuccess { cfg ->
                glBaseUrl = cfg.glBaseUrl
                clientId = cfg.clientId
                enabled = cfg.enabled
                hasSecret = cfg.hasSecret
                hasServiceToken = cfg.hasServiceToken
                orgMapText = pretty.toJson(cfg.orgMap ?: JsonParser.parseString("{}"))
                clientSecret = ""
                serviceToken = ""
                loaded = true
            }
            .onFailure { error = it.message ?: res.getString(R.string.admin_oauth_load_failed) }
        loading = false
    }

    LaunchedEffect(expanded) { if (expanded && !loaded) load() }

    fun save() {
        val orgMapEl = runCatching { JsonParser.parseString(orgMapText.ifBlank { "{}" }) }.getOrNull()
        if (orgMapEl == null || !orgMapEl.isJsonObject) {
            error = res.getString(R.string.admin_oauth_orgmap_invalid)
            return
        }
        saving = true
        error = null
        scope.launch {
            runCatching {
                api.setOAuthConfig(
                    OAuthConfigRequest(
                        clientId = clientId.trim(),
                        clientSecret = clientSecret,
                        glBaseUrl = glBaseUrl.trim(),
                        enabled = enabled,
                        orgMap = orgMapEl,
                        serviceToken = serviceToken,
                    ),
                )
            }
                .onSuccess { cfg ->
                    hasSecret = cfg.hasSecret
                    hasServiceToken = cfg.hasServiceToken
                    clientSecret = ""
                    serviceToken = ""
                    orgMapText = pretty.toJson(cfg.orgMap ?: JsonParser.parseString("{}"))
                    android.widget.Toast
                        .makeText(ctx, res.getString(R.string.admin_oauth_saved), android.widget.Toast.LENGTH_SHORT)
                        .show()
                }
                .onFailure { error = it.message ?: res.getString(R.string.admin_oauth_save_failed) }
            saving = false
        }
    }

    val callbackUrl = RetrofitClient.serverRoot + "/api/auth/gitlab/callback"

    TCard(Modifier.fillMaxWidth()) {
        Column {
            Row(
                Modifier.fillMaxWidth().clickableNoRipple { expanded = !expanded },
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IonIcon(Ion.GITLAB, size = 18.dp, tint = c.primary)
                Spacer(Modifier.width(8.dp))
                Text(
                    stringResource(R.string.admin_oauth_title),
                    color = c.text1,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                )
                if (enabled) Badge(stringResource(R.string.admin_oauth_badge_on), c.primary)
                Spacer(Modifier.width(6.dp))
                IonIcon(if (expanded) Ion.CHEVRON_DOWN else Ion.CHEVRON_FORWARD, size = 18.dp, tint = c.text3)
            }

            if (expanded) {
                Spacer(Modifier.height(12.dp))
                if (loading) {
                    Text(stringResource(R.string.common_loading), color = c.text3, fontSize = 13.sp)
                } else {
                    val stored = stringResource(R.string.admin_oauth_secret_stored)
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        TTextField(glBaseUrl, { glBaseUrl = it }, label = "GitLab URL", placeholder = "https://gitlab.example.com")
                        TTextField(clientId, { clientId = it }, label = "Client ID")
                        TTextField(
                            clientSecret,
                            { clientSecret = it },
                            label = "Client Secret",
                            placeholder = if (hasSecret) stored else "",
                            isPassword = true,
                        )
                        TTextField(
                            serviceToken,
                            { serviceToken = it },
                            label = stringResource(R.string.admin_oauth_service_token),
                            placeholder = if (hasServiceToken) stored else "glpat-…",
                            isPassword = true,
                        )
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(stringResource(R.string.admin_oauth_enabled), color = c.text2, fontSize = 14.sp, modifier = Modifier.weight(1f))
                            TSwitch(checked = enabled, onCheckedChange = { enabled = it })
                        }
                        TTextField(
                            orgMapText,
                            { orgMapText = it },
                            label = stringResource(R.string.admin_oauth_org_map),
                            singleLine = false,
                        )
                        Text(stringResource(R.string.admin_oauth_callback_hint), color = c.text3, fontSize = 12.sp)
                        Text(callbackUrl, color = c.text2, fontSize = 12.sp)
                        TFormError(error)
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            TButton(stringResource(R.string.common_save), enabled = !saving, loading = saving, onClick = ::save)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun UserRow(
    u: AdminUser,
    isMe: Boolean,
    busy: Boolean,
    onCopyReset: () -> Unit,
    onToggleAdmin: () -> Unit,
    onToggleActive: () -> Unit,
) {
    val c = Tessera.colors
    Column(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(c.cardSurface)
            .padding(12.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            AdminAvatar(u)
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(u.name, color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                    if (u.isAdmin) Badge("admin", c.primary)
                    if (isMe) Badge(stringResource(R.string.admin_badge_you), c.text3)
                    if (!u.active) Badge(stringResource(R.string.admin_badge_inactive), TesseraDanger)
                    if (!u.emailVerified) Badge(stringResource(R.string.admin_badge_unverified), c.text3)
                }
                Text(u.email, color = c.text3, fontSize = 12.sp, maxLines = 1)
            }
        }
        Spacer(Modifier.height(10.dp))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TButton(stringResource(R.string.admin_reset_password), kind = TButtonKind.Secondary, enabled = !busy, icon = Ion.LINK, onClick = onCopyReset)
            if (!isMe) {
                TButton(
                    stringResource(if (u.isAdmin) R.string.admin_demote else R.string.admin_promote),
                    kind = TButtonKind.Secondary,
                    enabled = !busy,
                    onClick = onToggleAdmin,
                )
                TButton(
                    stringResource(if (u.active) R.string.admin_deactivate else R.string.admin_activate),
                    kind = if (u.active) TButtonKind.Ghost else TButtonKind.Primary,
                    enabled = !busy,
                    onClick = onToggleActive,
                )
            }
        }
    }
}

@Composable
private fun AdminAvatar(u: AdminUser) {
    val c = Tessera.colors
    // avatarUrl is set (server-side) only when the account actually has an avatar;
    // otherwise show gradient initials (no needless 404 round-trip).
    val url = u.avatarUrl.takeIf { it.isNotBlank() }?.let { RetrofitClient.serverRoot + it }
    Box(
        Modifier.size(38.dp).clip(CircleShape).background(accentGradient(c.primary)),
        contentAlignment = Alignment.Center,
    ) {
        if (url != null) {
            AsyncImage(model = url, contentDescription = null, modifier = Modifier.size(38.dp).clip(CircleShape))
        } else {
            Text(initialsOf(u.name), color = c.onPrimary, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
        }
    }
}

@Composable
private fun Badge(text: String, color: Color) {
    Box(
        Modifier
            .clip(RoundedCornerShape(8.dp))
            .background(SolidColor(color.copy(alpha = 0.16f)))
            .padding(horizontal = 7.dp, vertical = 1.dp),
    ) {
        Text(text, color = color, fontSize = 11.sp, fontWeight = FontWeight.Medium)
    }
}

private fun initialsOf(name: String): String {
    val s = name.trim()
    if (s.isEmpty()) return "?"
    val parts = s.split(Regex("\\s+")).filter { it.isNotBlank() }
    return if (parts.size >= 2) "${parts[0].first()}${parts[1].first()}".uppercase() else s.take(2).uppercase()
}
