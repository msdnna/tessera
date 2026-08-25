package website.msdnna.tessera.ui.screens

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.compose.AsyncImage
import kotlinx.coroutines.launch
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.data.model.Preferences
import website.msdnna.tessera.data.model.ProfileUpdate
import website.msdnna.tessera.data.repository.ProfileRepository
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.TDropdown
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TMenuItem
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.AccentThemes
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.countryOptions
import website.msdnna.tessera.util.timezoneOptions

/** Option labels are resolved in composition, so a language switch relabels the
 *  selects without recreating the screen (the stored values never change). */
@Composable
private fun themeModes() = listOf(
    "system" to stringResource(R.string.theme_system),
    "light" to stringResource(R.string.theme_light),
    "dark" to stringResource(R.string.theme_dark),
)

@Composable
private fun languages() = listOf(
    "ru" to stringResource(R.string.language_ru),
    "en" to stringResource(R.string.language_en),
)

@Composable
private fun timeFormats() = listOf(
    "24h" to stringResource(R.string.time_format_24h),
    "12h" to stringResource(R.string.time_format_12h),
)

@Composable
private fun weekStarts() = listOf(
    1 to stringResource(R.string.week_start_monday),
    0 to stringResource(R.string.week_start_sunday),
)

private val DateFormats = listOf(
    "dd.MM.yyyy" to "31.12.2026", "yyyy-MM-dd" to "2026-12-31",
    "MM/dd/yyyy" to "12/31/2026", "dd/MM/yyyy" to "31/12/2026",
)

/**
 * Label of the option matching [value], or the first option's when nothing matches.
 *
 * A preference value can come from another client: since #2798 the web writes named
 * date presets ("short", "medium", …) into `date_format`, which is not in [DateFormats].
 * `first { }` threw NoSuchElementException on those and took the whole screen down —
 * an unknown value must degrade to a default label, not to a crash. Android moves to
 * the same presets in stage 7 of #2796.
 */
private fun <T> labelOf(options: List<Pair<T, String>>, value: T): String =
    options.firstOrNull { it.first == value }?.second ?: options.first().second

/**
 * Account settings (U1c): profile + avatar + password, plus appearance and
 * localization preferences. Profile/password save explicitly; appearance and
 * locale persist immediately (and the theme follows via the prefs flow).
 */
@Composable
fun ProfileScreen() {
    val c = Tessera.colors
    val scope = rememberCoroutineScope()
    val ctx = LocalContext.current
    val repo = remember { ProfileRepository() }
    val prefs = AppContainer.prefs

    val user by prefs.user.collectAsStateWithLifecycle(initialValue = null)
    val p by prefs.preferences.collectAsStateWithLifecycle(initialValue = Preferences())

    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text(stringResource(R.string.settings_title), color = c.text1, fontSize = 22.sp, fontWeight = FontWeight.Bold)

        ProfileCard(user, repo, scope, ctx)
        PasswordCard(repo, scope)
        AppearanceCard(p, repo, scope)
        LocalizationCard(p, repo, scope)
    }
}

@Composable
private fun ProfileCard(
    user: website.msdnna.tessera.data.model.User?,
    repo: ProfileRepository,
    scope: kotlinx.coroutines.CoroutineScope,
    ctx: android.content.Context,
) {
    val c = Tessera.colors
    var name by remember(user?.id) { mutableStateOf(user?.name ?: "") }
    var last by remember(user?.id) { mutableStateOf(user?.lastName ?: "") }
    var first by remember(user?.id) { mutableStateOf(user?.firstName ?: "") }
    var middle by remember(user?.id) { mutableStateOf(user?.middleName ?: "") }
    var company by remember(user?.id) { mutableStateOf(user?.company ?: "") }
    var jobTitle by remember(user?.id) { mutableStateOf(user?.jobTitle ?: "") }
    var bio by remember(user?.id) { mutableStateOf(user?.bio ?: "") }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var avatarBust by remember { mutableStateOf(0L) }
    var verifySent by remember { mutableStateOf(false) }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        uri ?: return@rememberLauncherForActivityResult
        scope.launch {
            error = null
            runCatching {
                val bytes = ctx.contentResolver.openInputStream(uri)?.use { it.readBytes() } ?: return@launch
                val mime = ctx.contentResolver.getType(uri)
                repo.uploadAvatar(bytes, "avatar", mime)
                avatarBust = System.currentTimeMillis()
            }.onFailure { error = it.message }
        }
    }

    TCard {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(
                stringResource(R.string.settings_profile_title),
                color = c.text1,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                AvatarPreview(user, avatarBust)
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    TButton(
                        stringResource(R.string.settings_avatar_upload),
                        onClick = { picker.launch("image/*") },
                        kind = TButtonKind.Secondary,
                        icon = Ion.IMAGE,
                    )
                    if (!user?.avatarUrl.isNullOrBlank()) {
                        TButton(stringResource(R.string.settings_avatar_remove), onClick = {
                            scope.launch { runCatching { repo.deleteAvatar() }.onFailure { error = it.message } }
                        }, kind = TButtonKind.Ghost)
                    }
                    Text(stringResource(R.string.settings_avatar_hint), color = c.text3, fontSize = 11.sp)
                }
            }
            ReadonlyField(stringResource(R.string.settings_email_label), user?.email ?: "")
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (user?.emailVerified == true) {
                    IonIcon(Ion.CHECK_CIRCLE, size = 15.dp, tint = c.primary, gradient = true)
                    Text(stringResource(R.string.settings_email_verified), color = c.text3, fontSize = 12.sp)
                } else {
                    Text(stringResource(R.string.settings_email_unverified), color = c.text3, fontSize = 12.sp)
                    if (verifySent) {
                        Text(stringResource(R.string.settings_email_sent), color = c.text3, fontSize = 12.sp)
                    } else {
                        TButton(stringResource(R.string.settings_email_send), kind = TButtonKind.Secondary, onClick = {
                            scope.launch {
                                runCatching { repo.resendVerification() }
                                verifySent = true
                            }
                        })
                    }
                }
            }
            TTextField(
                name,
                { name = it },
                label = stringResource(R.string.settings_display_name),
                placeholder = stringResource(R.string.settings_display_name_placeholder),
            )
            TTextField(last, { last = it }, label = stringResource(R.string.settings_last_name))
            TTextField(first, { first = it }, label = stringResource(R.string.settings_first_name))
            TTextField(middle, { middle = it }, label = stringResource(R.string.settings_middle_name))
            TTextField(company, { company = it }, label = stringResource(R.string.settings_company))
            TTextField(jobTitle, { jobTitle = it }, label = stringResource(R.string.settings_job_title))
            TTextField(bio, { bio = it }, label = stringResource(R.string.settings_bio), singleLine = false)
            TFormError(error)
            TButton(
                stringResource(R.string.settings_save_profile),
                loading = saving,
                onClick = {
                    scope.launch {
                        saving = true
                        error = null
                        runCatching {
                            repo.updateProfile(ProfileUpdate(name, last, first, middle, bio, company, jobTitle))
                        }.onFailure { error = it.message }
                        saving = false
                    }
                },
            )
        }
    }
}

@Composable
private fun AvatarPreview(user: website.msdnna.tessera.data.model.User?, bust: Long) {
    val c = Tessera.colors
    val url = user?.avatarUrl?.takeIf { it.isNotBlank() }?.let { "${RetrofitClient.serverRoot}$it?t=$bust" }
    Box(Modifier.size(72.dp).clip(CircleShape).background(accentGradient(c.primary)), contentAlignment = Alignment.Center) {
        if (url != null) {
            AsyncImage(model = url, contentDescription = null, modifier = Modifier.size(72.dp).clip(CircleShape))
        } else {
            Text(initials(user?.name ?: user?.email ?: "?"), color = c.onPrimary, fontSize = 24.sp, fontWeight = FontWeight.SemiBold)
        }
    }
}

private fun initials(name: String): String {
    val s = name.trim()
    if (s.isEmpty()) return "?"
    val w = s.split(Regex("\\s+")).filter { it.isNotEmpty() }
    return (if (w.size >= 2) "${w[0].first()}${w[1].first()}" else s.take(2)).uppercase()
}

@Composable
private fun PasswordCard(repo: ProfileRepository, scope: kotlinx.coroutines.CoroutineScope) {
    val c = Tessera.colors
    var current by remember { mutableStateOf("") }
    var next by remember { mutableStateOf("") }
    var confirm by remember { mutableStateOf("") }
    var saving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var done by remember { mutableStateOf(false) }
    val valid = current.isNotBlank() && next.length >= 8 && next == confirm

    TCard {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(
                stringResource(R.string.settings_security_title),
                color = c.text1,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
            )
            TTextField(current, { current = it }, label = stringResource(R.string.settings_password_current), isPassword = true)
            TTextField(next, { next = it }, label = stringResource(R.string.settings_password_new), isPassword = true)
            TTextField(confirm, { confirm = it }, label = stringResource(R.string.settings_password_repeat), isPassword = true)
            TFormError(error)
            if (done) Text(stringResource(R.string.settings_password_changed), color = c.primary, fontSize = 13.sp)
            TButton(
                stringResource(R.string.settings_password_submit),
                enabled = valid,
                loading = saving,
                onClick = {
                    scope.launch {
                        saving = true
                        error = null
                        done = false
                        runCatching { repo.changePassword(current, next) }
                            .onSuccess {
                                current = ""
                                next = ""
                                confirm = ""
                                done = true
                            }
                            .onFailure { error = it.message }
                        saving = false
                    }
                },
            )
        }
    }
}

@Composable
private fun AppearanceCard(p: Preferences, repo: ProfileRepository, scope: kotlinx.coroutines.CoroutineScope) {
    val c = Tessera.colors
    fun save(next: Preferences) = scope.launch { repo.savePreferences(next) }

    TCard {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(
                stringResource(R.string.settings_appearance_title),
                color = c.text1,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
            )
            val themes = themeModes()
            SelectRow(stringResource(R.string.settings_theme), labelOf(themes, p.theme), themes) { save(p.copy(theme = it)) }
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(stringResource(R.string.settings_accent), color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    AccentThemes.forEach { t ->
                        Box(
                            Modifier.size(28.dp).clip(CircleShape).background(accentGradient(t.primary))
                                .then(
                                    if (t.key == p.accent) {
                                        Modifier.border(2.dp, c.text1, CircleShape)
                                    } else {
                                        Modifier
                                    },
                                )
                                .clickableNoRipple { save(p.copy(accent = t.key)) },
                        )
                    }
                }
            }
            TTextField(
                p.boardBackground,
                { save(p.copy(boardBackground = it)) },
                label = stringResource(R.string.settings_board_background),
                placeholder = stringResource(R.string.settings_board_background_placeholder),
            )
        }
    }
}

@Composable
private fun LocalizationCard(p: Preferences, repo: ProfileRepository, scope: kotlinx.coroutines.CoroutineScope) {
    val c = Tessera.colors
    fun save(next: Preferences) = scope.launch { repo.savePreferences(next) }

    TCard {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(
                stringResource(R.string.settings_locale_title),
                color = c.text1,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
            )
            val langs = languages()
            val weeks = weekStarts()
            val times = timeFormats()
            SelectRow(stringResource(R.string.settings_language), labelOf(langs, p.language), langs) {
                save(p.copy(language = it))
            }
            SelectRow(stringResource(R.string.settings_week_start), labelOf(weeks, p.weekStart), weeks) {
                save(p.copy(weekStart = it))
            }
            SelectRow(stringResource(R.string.settings_time_format), labelOf(times, p.timeFormat), times) {
                save(p.copy(timeFormat = it))
            }
            SelectRow(stringResource(R.string.settings_date_format), labelOf(DateFormats, p.dateFormat), DateFormats) {
                save(p.copy(dateFormat = it))
            }
            val tzOptions = remember { timezoneOptions() }
            val countryOpts = remember(p.language) { countryOptions(p.language) }
            SearchableSelectRow(
                stringResource(R.string.settings_timezone),
                p.timezone,
                tzOptions,
                stringResource(R.string.settings_timezone_placeholder),
            ) { save(p.copy(timezone = it)) }
            SearchableSelectRow(
                stringResource(R.string.settings_country),
                p.country,
                countryOpts,
                stringResource(R.string.settings_country_placeholder),
            ) { save(p.copy(country = it)) }
            Text(stringResource(R.string.settings_locale_hint), color = c.text3, fontSize = 12.sp)
        }
    }
}

/**
 * A labelled value row that opens a searchable dialog of [options] (value to
 * label) — for long lists like time zones / countries. Shows all on open and
 * filters by the typed query.
 */
@Composable
private fun SearchableSelectRow(
    label: String,
    value: String,
    options: List<Pair<String, String>>,
    placeholder: String,
    onSelect: (String) -> Unit,
) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    val current = options.firstOrNull { it.first == value }?.second ?: value
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(label, color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        Row(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surface)
                .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                .clickableNoRipple { open = true }
                .padding(horizontal = 12.dp, vertical = 11.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                current.ifBlank { placeholder },
                color = if (current.isBlank()) c.placeholder else c.text1,
                fontSize = 14.sp,
                modifier = Modifier.weight(1f),
            )
            IonIcon(Ion.CHEVRON_DOWN, size = 16.dp, tint = c.text3)
        }
    }
    if (open) {
        var query by remember { mutableStateOf("") }
        val filtered = remember(query, options) {
            if (query.isBlank()) {
                options
            } else {
                options.filter { it.second.contains(query, ignoreCase = true) || it.first.contains(query, ignoreCase = true) }
            }
        }
        Dialog(onDismissRequest = { open = false }) {
            Column(
                Modifier.popupAppear(TransformOrigin.Center).fillMaxWidth().clip(RoundedCornerShape(RadiusLg))
                    .background(c.surface).padding(16.dp),
            ) {
                Text(label, color = c.text1, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.height(10.dp))
                TTextField(query, { query = it }, placeholder = stringResource(R.string.settings_search_placeholder))
                Spacer(Modifier.height(8.dp))
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 320.dp)) {
                    items(filtered, key = { it.first }) { (value, lbl) ->
                        Text(
                            lbl,
                            color = c.text1,
                            fontSize = 14.sp,
                            modifier = Modifier.fillMaxWidth().clickableNoRipple {
                                onSelect(value)
                                open = false
                            }.padding(vertical = 11.dp, horizontal = 4.dp),
                        )
                    }
                }
            }
        }
    }
}

/** A labelled value row that opens a dropdown of [options] (value to label). */
@Composable
private fun <T> SelectRow(label: String, current: String, options: List<Pair<T, String>>, onSelect: (T) -> Unit) {
    val c = Tessera.colors
    var open by remember { mutableStateOf(false) }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(label, color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        Box {
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surface)
                    .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                    .clickableNoRipple { open = true }
                    .padding(horizontal = 12.dp, vertical = 11.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(current, color = c.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
                IonIcon(Ion.CHEVRON_DOWN, size = 16.dp, tint = c.text3)
            }
            TDropdown(expanded = open, onDismiss = { open = false }) {
                options.forEach { (value, lbl) ->
                    TMenuItem(lbl, onClick = {
                        open = false
                        onSelect(value)
                    })
                }
            }
        }
    }
}

@Composable
private fun ReadonlyField(label: String, value: String) {
    val c = Tessera.colors
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(label, color = c.text2, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        Box(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusMd)).background(c.surfaceAlt)
                .border(1.dp, c.border, RoundedCornerShape(RadiusMd)).padding(horizontal = 12.dp, vertical = 11.dp),
        ) { Text(value, color = c.text3, fontSize = 14.sp) }
    }
}
