package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupProperties
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.MentionItem
import website.msdnna.tessera.util.roleLabel

/**
 * The card behind an @-mention chip (web `MentionCard.vue`), shown on tap —
 * there is no hover on a phone. Deliberately neutral: only the avatar carries
 * the accent gradient, as it does everywhere else.
 *
 * A GitLab-only user has no Tessera account, so there is neither an email nor a
 * workspace role to show — name, handle and avatar are all such a person has.
 */
@Composable
fun MentionCardPopup(item: MentionItem, onDismiss: () -> Unit) {
    val c = Tessera.colors
    Popup(
        popupPositionProvider = BelowAnchorPositionProvider(),
        onDismissRequest = onDismiss,
        properties = PopupProperties(focusable = true),
    ) {
        Row(
            Modifier
                .popupAppear()
                .softShadow(RoundedCornerShape(RadiusMd), elevation = 6.dp)
                .clip(RoundedCornerShape(RadiusMd))
                .background(c.surface)
                .border(1.dp, c.border, RoundedCornerShape(RadiusMd))
                .widthIn(min = 200.dp, max = 300.dp)
                .padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            val name = item.display.ifBlank { item.insert }
            MemberAvatar(
                size = 44.dp,
                name = name,
                userId = item.avatarUserId,
                avatarUrl = item.avatarSrc,
                muted = item.gitlab,
            )
            Spacer(Modifier.width(12.dp))
            Column {
                Text(
                    name,
                    color = c.text1,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                // A GitLab user is identified by their login, a member by their email.
                val handle = if (item.gitlab) "@${item.username ?: item.insert}" else item.email
                if (handle.isNotBlank()) {
                    Text(handle, color = c.text2, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                val role = if (item.gitlab) "" else roleLabel(LocalResources.current, item.role)
                if (role.isNotBlank()) {
                    Text(role, color = c.text3, fontSize = 11.sp, modifier = Modifier.padding(top = 3.dp))
                }
            }
        }
    }
}
