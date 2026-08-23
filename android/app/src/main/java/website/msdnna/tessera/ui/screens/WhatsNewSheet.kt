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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TCard
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.WhatsNewEntry

/**
 * "Что нового" after an update (#2766, web `WhatsNewModal.vue`): the curated
 * highlights of every release the user has just updated into, newest first.
 * Dismissing marks them all seen — see
 * [website.msdnna.tessera.ui.viewmodels.WhatsNewViewModel.dismissCard].
 *
 * Styled as the update prompt is (scrim + centred card), so the two post-update
 * dialogs read as one family.
 */
@Composable
fun WhatsNewSheet(releases: List<WhatsNewEntry>, onDismiss: () -> Unit) {
    if (releases.isEmpty()) return
    val c = Tessera.colors

    Box(
        Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.5f))
            .clickableNoRipple(onClick = onDismiss),
        contentAlignment = Alignment.Center,
    ) {
        // Swallow taps on the card so they don't reach the dismiss scrim.
        Box(Modifier.popupAppear(TransformOrigin.Center).padding(28.dp).clickableNoRipple {}) {
            TCard(modifier = Modifier.widthIn(max = 380.dp).testTag(TestTags.WHATS_NEW_CARD)) {
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
                                stringResource(R.string.whats_new_title),
                                color = c.text1, fontSize = 16.sp, fontWeight = FontWeight.SemiBold,
                            )
                            Text(
                                stringResource(R.string.whats_new_subtitle),
                                color = c.text3, fontSize = 12.sp,
                            )
                        }
                    }

                    // Several releases at once when the user skipped a few — the
                    // list scrolls instead of pushing the button off-screen.
                    Column(
                        Modifier.heightIn(max = 380.dp).verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(16.dp),
                    ) {
                        releases.forEach { ReleaseSection(it) }
                    }

                    TButton(
                        stringResource(R.string.common_got_it),
                        onClick = onDismiss,
                        modifier = Modifier.fillMaxWidth().testTag(TestTags.WHATS_NEW_DISMISS),
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
                release.title,
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
        release.items.forEach { item ->
            Row(Modifier.padding(bottom = 4.dp)) {
                Text("•", color = c.text3, fontSize = 13.sp)
                Spacer(Modifier.width(8.dp))
                Text(item, color = c.text2, fontSize = 13.sp, lineHeight = 18.sp)
            }
        }
    }
}
