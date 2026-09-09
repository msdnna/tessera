package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.BuildConfig
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.LocalDateFormat
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.ChangelogSheet
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.VersionStamp
import website.msdnna.tessera.util.WhatsNewEntry
import website.msdnna.tessera.util.longDate
import website.msdnna.tessera.util.versionLines

/**
 * "Что нового" after an update (#2766, web `WhatsNewModal.vue`): the curated
 * highlights of every release the user has just updated into, newest first.
 * Dismissing marks them all seen — see
 * [website.msdnna.tessera.ui.viewmodels.WhatsNewViewModel.dismissCard].
 *
 * The same card doubles as the **full changelog** opened by hand from the version
 * stamp in the sidebar footer (#2858, `sheet.history`). One composable for both:
 * they render identically and only the wording, the entry list and what dismissing
 * means differ — which is decided in [website.msdnna.tessera.util.changelogSheet],
 * not here.
 *
 * The build stamps ([app], [api]) are shown **only** in history mode (#2859): the
 * post-update card is an interruption and a commit hash is noise there, while the
 * hand-opened changelog is exactly where someone goes to ask "what am I running".
 * Android has no hover, so unlike the web — where this detail lives in a tooltip —
 * it needs a surface of its own, and this is the one the same tap already opens.
 *
 * Styled as the update prompt is (scrim + centred card), so the two post-update
 * dialogs read as one family.
 */
@Composable
fun WhatsNewSheet(
    sheet: ChangelogSheet?,
    onDismiss: () -> Unit,
    app: VersionStamp = VersionStamp(BuildConfig.VERSION_NAME, BuildConfig.GIT_COMMIT, BuildConfig.BUILD_DATE),
    api: VersionStamp? = null,
) {
    if (sheet == null || sheet.entries.isEmpty()) return
    val releases = sheet.entries
    val c = Tessera.colors

    Box(
        Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.5f))
            .clickableNoRipple(onClick = onDismiss),
        contentAlignment = Alignment.Center,
    ) {
        // Swallow taps on the card so they don't reach the dismiss scrim.
        //
        // The tag rides on THIS box, not on the card inside it: `clickable` merges
        // its subtree, and `testTag` is the one property that is never merged
        // upward — a tag on a plain descendant of a merging node simply vanishes
        // from the merged tree, and a spec waiting on it times out as if the sheet
        // had never opened. So it goes on a node that merges on its own.
        Box(
            Modifier.popupAppear(TransformOrigin.Center).padding(28.dp)
                .clickableNoRipple {}
                .testTag(TestTags.WHATS_NEW_CARD),
        ) {
            TCard(modifier = Modifier.widthIn(max = 380.dp)) {
                Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier.size(38.dp).clip(CircleShape).background(accentGradient(c.primary)),
                            contentAlignment = Alignment.Center,
                        ) {
                            IonIcon(Ion.STAR, size = 20.dp, tint = c.onPrimary)
                        }
                        Spacer(Modifier.width(12.dp))
                        Column {
                            Text(
                                stringResource(
                                    if (sheet.history) R.string.whats_new_history_title else R.string.whats_new_title,
                                ),
                                color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold,
                            )
                            Text(
                                stringResource(
                                    if (sheet.history) R.string.whats_new_history_subtitle else R.string.whats_new_subtitle,
                                ),
                                color = c.text3, fontSize = 12.sp,
                            )
                        }
                    }

                    // Several releases at once when the user skipped a few — the
                    // list scrolls instead of pushing the button off-screen. In
                    // history mode it is the whole changelog, so the cap matters
                    // even more; the build stamp rides inside the same scroll so
                    // it can't push the button out either.
                    Column(
                        Modifier.heightIn(max = 380.dp).verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(16.dp),
                    ) {
                        releases.forEach { ReleaseSection(it) }
                        if (sheet.history) BuildStampSection(app, api)
                    }

                    TButton(
                        stringResource(if (sheet.history) R.string.common_close else R.string.common_got_it),
                        onClick = onDismiss,
                        modifier = Modifier.fillMaxWidth().testTag(TestTags.WHATS_NEW_DISMISS),
                    )
                }
            }
        }
    }
}

/**
 * Which build is actually running — the app's own stamp and, when the server has
 * answered, the API's (#2859). Faint and last in the list: it is a footnote for
 * bug reports, not something to read before the changelog.
 *
 * The date follows the profile's `date_format` preset like every other date in the
 * app (#2857), and each line is dropped rather than shown empty when the value is
 * missing — a dev backend reports no commit at all.
 */
@Composable
private fun BuildStampSection(app: VersionStamp, api: VersionStamp?) {
    val c = Tessera.colors
    val res = LocalResources.current
    val fmt = LocalDateFormat.current
    val appLabel = stringResource(R.string.version_client)
    val apiLabel = stringResource(R.string.version_server)
    val commitFmt = stringResource(R.string.version_commit)
    val builtFmt = stringResource(R.string.version_built)

    val groups = listOf(appLabel to app, apiLabel to api).map { (label, stamp) ->
        versionLines(
            label = label,
            stamp = stamp,
            commitLabel = { commitFmt.format(it) },
            builtLabel = { builtFmt.format(it) },
            formatDate = { longDate(res, it, fmt) },
        )
    }.filter { it.isNotEmpty() }
    if (groups.isEmpty()) return

    Column(
        // Merges on its own so the tag survives: the card above it already merges
        // (see [WhatsNewSheet]), and merging stops at a node that merges itself —
        // without this the anchor would be swallowed along with the rest.
        Modifier.fillMaxWidth().semantics(mergeDescendants = true) {}.testTag(TestTags.WHATS_NEW_BUILD),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        groups.forEach { lines ->
            Column {
                lines.forEachIndexed { i, line ->
                    Text(
                        line,
                        color = if (i == 0) c.text2 else c.text3,
                        fontSize = if (i == 0) 12.sp else 11.sp,
                        fontWeight = if (i == 0) FontWeight.SemiBold else FontWeight.Normal,
                    )
                }
            }
        }
    }
}

@Composable
private fun ReleaseSection(release: WhatsNewEntry) {
    val c = Tessera.colors
    Column {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                stringResource(release.titleRes),
                color = c.text1,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f, fill = false),
            )
            Spacer(Modifier.width(8.dp))
            Text(
                release.version,
                color = c.text3,
                fontSize = 11.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier
                    .clip(RoundedCornerShape(RadiusSm))
                    .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
                    .padding(horizontal = 7.dp, vertical = 1.dp),
            )
        }
        Spacer(Modifier.size(6.dp))
        stringArrayResource(release.itemsRes).forEach { item ->
            Row(Modifier.padding(bottom = 4.dp)) {
                Text("•", color = c.text3, fontSize = 13.sp)
                Spacer(Modifier.width(8.dp))
                Text(item, color = c.text2, fontSize = 13.sp, lineHeight = 18.sp)
            }
        }
    }
}
