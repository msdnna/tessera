package website.msdnna.tessera.ui.components

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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.data.model.BoardFavorite
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.util.Ion

/**
 * «Избранное» over «Проекты» (#3115, web `sidebar/FavoritesSection.vue`): the
 * reader's starred boards in their own order — the board's icon and colour, its
 * project after the name. Hidden while empty. A long list scrolls inside its own
 * cap so the tree below keeps its room.
 *
 * The web reorders by dragging; here the row's «⋯» has «Выше» / «Ниже» — the
 * tree's long-press drag is wired to the group/project model and a second
 * drag model for a short flat list would cost more than it gives.
 */
@Composable
internal fun SidebarFavorites(
    favorites: List<BoardFavorite>,
    onOpen: (Board) -> Unit,
    onRemove: (String) -> Unit,
    onStep: (String, Int) -> Unit,
) {
    if (favorites.isEmpty()) return
    val c = Tessera.colors
    Column(Modifier.fillMaxWidth().testTag(TestTags.SIDEBAR_FAVORITES)) {
        Text(
            stringResource(R.string.sidebar_favorites_header),
            color = c.text3,
            fontSize = 11.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.padding(start = 14.dp, end = 6.dp, top = 10.dp, bottom = 4.dp),
        )
        Column(
            Modifier.fillMaxWidth().heightIn(max = 220.dp).verticalScroll(rememberScrollState())
                .padding(start = 10.dp),
        ) {
            favorites.forEachIndexed { i, f ->
                key(f.boardId) {
                    FavoriteRow(
                        f,
                        first = i == 0,
                        last = i == favorites.lastIndex,
                        onOpen = onOpen,
                        onRemove = onRemove,
                        onStep = onStep,
                    )
                }
            }
        }
    }
}

@Composable
private fun FavoriteRow(
    f: BoardFavorite,
    first: Boolean,
    last: Boolean,
    onOpen: (Board) -> Unit,
    onRemove: (String) -> Unit,
    onStep: (String, Int) -> Unit,
) {
    val c = Tessera.colors
    var menu by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(RadiusSm))
            .testTag("${TestTags.FAVORITE_ROW}-${f.boardId}")
            .clickableNoRipple { onOpen(f.toBoard()) }
            .padding(start = 4.dp, end = 6.dp, top = 7.dp, bottom = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(20.dp), contentAlignment = Alignment.Center) {
            ProjectIcon(
                f.name, f.icon, f.color,
                size = 18.dp, iconMode = f.iconMode, fallbackGlyph = "layout_kanban_outline",
            )
        }
        Spacer(Modifier.width(9.dp))
        Text(f.name, color = c.text1, fontSize = 13.sp, maxLines = 1)
        Spacer(Modifier.width(6.dp))
        Text(f.projectName, color = c.text3, fontSize = 12.sp, maxLines = 1, modifier = Modifier.weight(1f))
        Box {
            IonIconButton(Ion.ELLIPSIS_H, onClick = { menu = true }, boxSize = 26.dp, iconSize = 17.dp, tint = c.text3)
            TDropdown(expanded = menu, onDismiss = { menu = false }) {
                if (!first) {
                    TMenuItem(stringResource(R.string.sidebar_favorite_up), onClick = {
                        menu = false
                        onStep(f.boardId, -1)
                    })
                }
                if (!last) {
                    TMenuItem(stringResource(R.string.sidebar_favorite_down), onClick = {
                        menu = false
                        onStep(f.boardId, 1)
                    })
                }
                if (!first || !last) TMenuDivider()
                TMenuItem(stringResource(R.string.sidebar_favorite_remove), icon = Ion.STAR, onClick = {
                    menu = false
                    onRemove(f.boardId)
                })
            }
        }
    }
}
