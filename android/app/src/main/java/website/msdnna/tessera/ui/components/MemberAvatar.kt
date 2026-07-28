package website.msdnna.tessera.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import website.msdnna.tessera.data.api.RetrofitClient
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient

/**
 * A circular member avatar (shared across the board/filter/modal): the uploaded
 * image (Tessera [userId] → `/api/users/{id}/avatar`, or an explicit GitLab
 * [avatarUrl]) over a gradient/grey disc, falling back to initials. [muted] renders
 * a flat grey disc (unlinked GitLab users).
 */
@Composable
fun MemberAvatar(size: Dp, name: String, userId: String? = null, avatarUrl: String? = null, muted: Boolean = false) {
    val c = Tessera.colors
    val url = avatarUrl?.takeIf { it.isNotBlank() }
        ?: userId?.takeIf { it.isNotBlank() }?.let { "${RetrofitClient.serverRoot}/api/users/$it/avatar" }
    Box(
        Modifier.size(size).clip(CircleShape)
            .background(if (muted) SolidColor(c.text3) else accentGradient(c.primary)),
        contentAlignment = Alignment.Center,
    ) {
        // Initials underneath; the image overlays and shows through if it fails to
        // load (e.g. an unreachable GitLab avatar) rather than leaving a blank disc.
        Text(
            avatarInitials(name),
            color = if (muted) Color.White else c.onPrimary,
            fontSize = (size.value * 0.42f).sp,
            fontWeight = FontWeight.SemiBold,
        )
        if (url != null) {
            AsyncImage(model = url, contentDescription = null, modifier = Modifier.size(size).clip(CircleShape))
        }
    }
}

/** Up to two initials from a name, mirroring the card/modal avatars ("a.fokin" → AF,
 *  "Name Last" → NL, "msdnna" → MS). */
fun avatarInitials(name: String): String {
    val s = name.trim()
    if (s.isEmpty()) return "?"
    if (s.contains('.')) {
        val parts = s.split('.').map { it.trim() }.filter { it.isNotEmpty() }
        if (parts.size >= 2) return "${parts[0].first()}${parts[1].first()}".uppercase()
    }
    val words = s.split(Regex("\\s+")).filter { it.isNotEmpty() }
    if (words.size >= 2) return "${words[0].first()}${words[1].first()}".uppercase()
    return s.take(2).uppercase()
}
