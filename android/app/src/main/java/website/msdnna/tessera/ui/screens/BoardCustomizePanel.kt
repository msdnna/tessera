package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.Board
import website.msdnna.tessera.ui.components.ColumnScopePicker
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel

// Оба списка остаются значениями уровня файла, но держат id ресурса, а не готовый
// текст: с текстом подписи вычислились бы один раз при загрузке класса и застыли
// бы на языке первого рендера. Строка разрешается в месте отрисовки.

/** Card density presets: config key → label (web cardSize). */
private val CARD_SIZES = listOf(
    "compact" to R.string.board_card_size_compact,
    "medium" to R.string.board_card_size_medium,
    "large" to R.string.board_card_size_large,
)

/** Per-field visibility toggles: fieldVis key → label (web FIELDS, same order). */
private val CARD_FIELDS = listOf(
    "priority" to R.string.task_prop_priority,
    "due" to R.string.task_prop_due,
    "assignee" to R.string.board_field_assignee,
    "tags" to R.string.task_prop_tags,
    "estimate" to R.string.task_prop_estimate,
    "milestone" to R.string.task_prop_milestone,
    "description" to R.string.task_tab_description,
    "number" to R.string.board_field_number,
    "gitlab" to R.string.board_field_gitlab,
)

/**
 * The board customization panel (web BoardCustomizePanel): card density, pill
 * layout, per-field visibility, empty-field placeholders, auto-collapse of empty
 * columns and subtask expansion. All settings persist to the board view config
 * (cross-device with web). Board name/icon/color live elsewhere (not yet ported).
 */
@Composable
fun BoardCustomizePanel(
    state: BoardUiState,
    vm: BoardViewModel,
    board: Board,
    onUpdateBoard: (icon: String, color: String, iconMode: String) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = Tessera.colors
    // Empty status columns whose explicit override should clear when auto-collapse
    // turns on (web watch parity) — mirrors the VM's isLaneCollapsed rule.
    val emptyLaneIds = state.columns.filter { state.tasksIn(it.id).isEmpty() }.map { it.id }.toSet()
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.fillMaxWidth(0.94f).fillMaxHeight(0.9f)
                .clip(RoundedCornerShape(RadiusLg))
                .background(c.surface)
                .padding(20.dp),
        ) {
            Text(
                stringResource(R.string.board_customize_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(14.dp))
            Column(Modifier.weight(1f).verticalScroll(rememberScrollState())) {
                SectionLabel(stringResource(R.string.board_customize_appearance))
                ColumnScopePicker(
                    color = board.color,
                    icon = board.icon,
                    onColor = { onUpdateBoard(board.icon, it, board.iconMode) },
                    onIcon = { onUpdateBoard(it, board.color, board.iconMode) },
                    iconMode = board.iconMode,
                    onIconMode = { onUpdateBoard(board.icon, board.color, it) },
                    fallbackIcon = "layout_kanban_outline",
                )

                SectionLabel(stringResource(R.string.board_customize_card_size))
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CARD_SIZES.forEach { (key, labelRes) ->
                        SizeChip(stringResource(labelRes), active = state.cardSize == key, modifier = Modifier.weight(1f)) {
                            vm.setCardSize(key)
                        }
                    }
                }

                SectionLabel(stringResource(R.string.board_customize_fields))
                ToggleRow(stringResource(R.string.board_customize_stack), state.stackFields) { vm.setStackFields(it) }
                ToggleRow(stringResource(R.string.board_customize_show_empty), state.showEmpty) { vm.setShowEmpty(it) }
                CARD_FIELDS.forEach { (key, labelRes) ->
                    ToggleRow(stringResource(labelRes), state.fieldOn(key)) { vm.setFieldVisible(key, it) }
                }

                SectionLabel(stringResource(R.string.board_customize_columns))
                ToggleRow(stringResource(R.string.board_customize_auto_collapse), state.autoCollapseEmpty) {
                    vm.setAutoCollapseEmpty(it, emptyLaneIds)
                }
                ToggleRow(stringResource(R.string.board_customize_subtasks), state.subtasksExpanded) {
                    vm.toggleSubtasksExpanded()
                }
            }
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton(stringResource(R.string.common_done), onClick = onDismiss)
            }
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Spacer(Modifier.height(16.dp))
    Text(
        text.uppercase(),
        color = Tessera.colors.text3,
        fontSize = 11.sp,
        fontWeight = FontWeight.SemiBold,
        letterSpacing = 0.4.sp,
    )
    Spacer(Modifier.height(8.dp))
}

@Composable
private fun ToggleRow(label: String, checked: Boolean, onChange: (Boolean) -> Unit) {
    Row(
        Modifier.fillMaxWidth().padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = Tessera.colors.text1, fontSize = 14.sp, modifier = Modifier.weight(1f))
        TSwitch(checked = checked, onCheckedChange = onChange)
    }
}

@Composable
private fun SizeChip(label: String, active: Boolean, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val c = Tessera.colors
    Row(
        modifier
            .clip(RoundedCornerShape(RadiusSm))
            .background(if (active) accentGradient(c.primary) else SolidColor(c.surfaceAlt))
            .clickableNoRipple(onClick = onClick)
            .padding(vertical = 9.dp),
        horizontalArrangement = Arrangement.Center,
    ) {
        Text(
            label,
            color = if (active) c.onPrimary else c.text2,
            fontSize = 13.sp,
            fontWeight = if (active) FontWeight.SemiBold else FontWeight.Normal,
        )
    }
}
