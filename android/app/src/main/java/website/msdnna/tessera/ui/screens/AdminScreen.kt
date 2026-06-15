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
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.compose.AsyncImage
import kotlinx.coroutines.launch
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.AdminUser
import website.msdnna.tessera.data.model.SetActiveRequest
import website.msdnna.tessera.data.model.SetAdminRequest
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
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
            .onFailure { error = it.message ?: "Не удалось загрузить пользователей" }
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
                        .makeText(ctx, "Ссылка для сброса пароля скопирована", android.widget.Toast.LENGTH_SHORT)
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
            Text("Администрирование", color = c.text1, fontSize = 20.sp, fontWeight = FontWeight.Bold)
        }
        Text("Пользователи экземпляра — ${users.size}", color = c.text3, fontSize = 13.sp)
        Spacer(Modifier.height(12.dp))
        TTextField(value = query, onValueChange = { query = it }, placeholder = "Поиск по имени или почте")
        TFormError(error, modifier = Modifier.padding(top = 6.dp))
        Spacer(Modifier.height(10.dp))

        when {
            loading -> Text("Загрузка…", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(top = 20.dp))

            filtered.isEmpty() -> Text("Никого не найдено", color = c.text3, fontSize = 13.sp, modifier = Modifier.padding(top = 20.dp))

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
            title = "Снять администратора",
            message = "Снять права администратора у ${u.name}?",
            confirmText = "Снять",
            onConfirm = {
                setAdmin(u, false)
                confirmAdmin = null
            },
            onDismiss = { confirmAdmin = null },
        )
    }
    confirmActive?.let { u ->
        TConfirmDialog(
            title = "Деактивировать аккаунт",
            message = "Деактивировать ${u.name}? Пользователь не сможет войти.",
            confirmText = "Деактивировать",
            onConfirm = {
                setActive(u, false)
                confirmActive = null
            },
            onDismiss = { confirmActive = null },
        )
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
                    if (isMe) Badge("вы", c.text3)
                    if (!u.active) Badge("деактивирован", TesseraDanger)
                    if (!u.emailVerified) Badge("не подтверждён", c.text3)
                }
                Text(u.email, color = c.text3, fontSize = 12.sp, maxLines = 1)
            }
        }
        Spacer(Modifier.height(10.dp))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TButton("Сброс пароля", kind = TButtonKind.Secondary, enabled = !busy, icon = Ion.LINK, onClick = onCopyReset)
            if (!isMe) {
                TButton(
                    if (u.isAdmin) "Снять админа" else "Сделать админом",
                    kind = TButtonKind.Secondary,
                    enabled = !busy,
                    onClick = onToggleAdmin,
                )
                TButton(
                    if (u.active) "Деактивировать" else "Активировать",
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
