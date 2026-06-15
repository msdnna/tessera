package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupProperties
import website.msdnna.tessera.data.model.Notification
import website.msdnna.tessera.ui.components.BelowAnchorPositionProvider
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.components.softShadow
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.NotificationUiState
import website.msdnna.tessera.util.whenLabel

/**
 * The notification feed dropdown (web `WorkspaceTools` bell popover). Place as a
 * sibling of the bell inside a `Box`; it anchors below it. Tapping an item marks
 * it read and, when it points at a task, opens it.
 */
@Composable
fun NotificationsPanel(
    expanded: Boolean,
    state: NotificationUiState,
    onItemClick: (Notification) -> Unit,
    onMarkAll: () -> Unit,
    onDismiss: () -> Unit,
) {
    if (!expanded) return
    val c = Tessera.colors
    val provider = remember { BelowAnchorPositionProvider() }
    Popup(
        popupPositionProvider = provider,
        onDismissRequest = onDismiss,
        properties = PopupProperties(focusable = true),
    ) {
        Column(
            Modifier
                .popupAppear()
                .width(300.dp)
                .softShadow(RoundedCornerShape(RadiusMd), elevation = 6.dp)
                .clip(RoundedCornerShape(RadiusMd))
                .background(c.surface)
                .border(1.dp, c.border, RoundedCornerShape(RadiusMd)),
        ) {
            Row(
                Modifier.fillMaxWidth().padding(start = 14.dp, end = 8.dp, top = 10.dp, bottom = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Уведомления", color = c.text1, fontSize = 14.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                if (state.unread > 0) {
                    Text(
                        "Прочитать все",
                        color = c.primary,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.clickableNoRipple(onClick = onMarkAll).padding(6.dp),
                    )
                }
            }
            HorizontalDivider(color = c.border)

            if (state.items.isEmpty()) {
                Text(
                    "Пока тихо",
                    color = c.text3,
                    fontSize = 13.sp,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(vertical = 26.dp),
                )
            } else {
                Column(Modifier.heightIn(max = 380.dp).verticalScroll(rememberScrollState())) {
                    state.items.forEach { item ->
                        NotificationItem(item, onClick = { onItemClick(item) })
                    }
                }
            }
        }
    }
}

@Composable
private fun NotificationItem(item: Notification, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        Modifier
            .fillMaxWidth()
            .then(if (item.isUnread) Modifier.background(accentGradient(c.primary.copy(alpha = 0.10f))) else Modifier)
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.Top,
    ) {
        if (item.isUnread) {
            Box(
                Modifier.padding(top = 5.dp, end = 8.dp).size(7.dp)
                    .clip(CircleShape).background(accentGradient(c.primary)),
            )
        }
        Column(Modifier.weight(1f)) {
            Text(
                item.text,
                color = c.text1,
                fontSize = 13.sp,
                fontWeight = if (item.isUnread) FontWeight.SemiBold else FontWeight.Normal,
            )
            val time = whenLabel(item.createdAt)
            if (time.isNotEmpty()) {
                Spacer(Modifier.padding(top = 2.dp))
                Text(time, color = c.text3, fontSize = 11.sp)
            }
        }
    }
}
