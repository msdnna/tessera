package website.msdnna.tessera.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
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
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import website.msdnna.tessera.R
import website.msdnna.tessera.data.AppContainer
import website.msdnna.tessera.data.model.Task
import website.msdnna.tessera.data.model.WorkspaceCommand
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmPopover
import website.msdnna.tessera.ui.components.TSwitch
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.TagChip
import website.msdnna.tessera.ui.components.TesseraLoader
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.components.popupAppear
import website.msdnna.tessera.ui.theme.PriorityLabels
import website.msdnna.tessera.ui.theme.RadiusLg
import website.msdnna.tessera.ui.theme.RadiusMd
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.theme.TesseraDanger
import website.msdnna.tessera.ui.theme.accentGradient
import website.msdnna.tessera.ui.viewmodels.BoardUiState
import website.msdnna.tessera.ui.viewmodels.BoardViewModel
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.buildTagGroups
import website.msdnna.tessera.util.canonCommandKey
import website.msdnna.tessera.util.isValidCommandKey
import website.msdnna.tessera.util.parseHexColor

private val TagPalette = listOf(
    "#7c5cff", "#2f80ed", "#0eb0a9", "#18a058", "#f0a020", "#e0533d", "#eb2f96", "#9aa0aa",
)

/** Tag manager: create, recolour, rename and delete workspace tags. */
@Composable
fun TagManagerModal(state: BoardUiState, vm: BoardViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    var newName by remember { mutableStateOf("") }
    // Which tag is in inline-edit; a tap on empty modal space clears it (cancel).
    var editingId by remember { mutableStateOf<String?>(null) }
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.popupAppear(TransformOrigin.Center).fillMaxWidth().clip(RoundedCornerShape(RadiusLg))
                .background(c.surface).clickableNoRipple { editingId = null }.padding(18.dp),
        ) {
            Text(
                stringResource(R.string.tags_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(12.dp))

            if (state.tagList.isEmpty()) {
                Text(stringResource(R.string.tags_empty), color = c.text3, fontSize = 13.sp)
            } else {
                val groups = buildTagGroups(LocalResources.current, state.tagList, state.prefixNames)
                val showHeaders = groups.size > 1
                Column(Modifier.heightIn(max = 340.dp).verticalScroll(rememberScrollState())) {
                    groups.forEach { g ->
                        if (showHeaders) {
                            Text(
                                g.label.uppercase(),
                                color = c.text3,
                                fontSize = 10.sp,
                                fontWeight = FontWeight.SemiBold,
                                letterSpacing = 0.4.sp,
                                modifier = Modifier.padding(top = 8.dp, bottom = 2.dp),
                            )
                        }
                        g.tags.forEach { tag ->
                            TagRow(
                                tag,
                                vm,
                                prefixNames = state.prefixNames,
                                showScope = !showHeaders,
                                editing = editingId == tag.id,
                                onEdit = { editingId = tag.id },
                                onDone = { editingId = null },
                            )
                        }
                    }
                }
                if (groups.any { it.key.isNotEmpty() }) PrefixModeRow()
            }

            Spacer(Modifier.height(14.dp))
            Text(
                stringResource(R.string.tags_new),
                color = c.text3,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
            )
            Spacer(Modifier.height(6.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.weight(1f)) {
                    TTextField(
                        value = newName,
                        onValueChange = { newName = it },
                        placeholder = stringResource(R.string.tags_name_hint),
                    )
                }
                Spacer(Modifier.width(8.dp))
                TButton(
                    stringResource(R.string.common_create),
                    enabled = newName.isNotBlank(),
                    onClick = {
                        vm.createTagStandalone(newName.trim(), TagPalette.first())
                        newName = ""
                    },
                )
            }

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TButton(stringResource(R.string.common_done), onClick = onDismiss)
            }
        }
    }
}

/**
 * Editor for the workspace's custom command dictionary — the `/`-popup entries
 * that are only text (they never execute; the built-in list below is what the
 * backend actually runs). Saved as the complete desired state in one PUT, like
 * the tag-prefix editor.
 *
 * Non-managers still get the modal, read-only: knowing which commands exist is
 * useful to everyone, and hiding the built-in reference behind a role would be
 * pointless — the popup suggests them anyway.
 */
@Composable
fun WorkspaceCommandsModal(state: BoardUiState, vm: BoardViewModel, onDismiss: () -> Unit) {
    val c = Tessera.colors
    val canManage = state.canManageCommands
    // Seed from the loaded dictionary each time the modal opens, so an aborted
    // edit doesn't stick.
    var rows by remember { mutableStateOf(state.customCommands.map { it.key to it.description }) }
    val builtinKeys = remember(state.builtinCommands) { state.builtinCommands.map { it.key }.toSet() }

    // Per-row validation, shown inline: the backend rejects the whole PUT on the
    // first bad key, so catching it here keeps the user from losing the other rows.
    // Тексты берутся заранее: `rowError` — обычная локальная функция, из неё
    // `stringResource` не вызвать.
    val errEmptyKey = stringResource(R.string.commands_err_empty_key)
    val errBadKey = stringResource(R.string.commands_err_bad_key)
    val errBuiltin = stringResource(R.string.commands_err_builtin)
    val errDuplicate = stringResource(R.string.commands_err_duplicate)
    val errTooLong = stringResource(R.string.commands_err_too_long)
    fun rowError(i: Int): String {
        val key = canonCommandKey(rows[i].first)
        return when {
            key.isEmpty() -> errEmptyKey
            !isValidCommandKey(key) -> errBadKey
            key in builtinKeys -> errBuiltin
            rows.take(i).any { canonCommandKey(it.first) == key } -> errDuplicate
            rows[i].second.length > 200 -> errTooLong
            else -> ""
        }
    }
    val errors = rows.indices.map { rowError(it) }
    val canSave = errors.all { it.isEmpty() }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier.popupAppear(TransformOrigin.Center).fillMaxWidth().clip(RoundedCornerShape(RadiusLg))
                .background(c.surface).padding(18.dp),
        ) {
            Text(
                stringResource(R.string.commands_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(6.dp))
            Text(
                stringResource(R.string.commands_hint),
                color = c.text3,
                fontSize = 11.sp,
            )
            Spacer(Modifier.height(12.dp))

            Column(Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState())) {
                rows.forEachIndexed { i, row ->
                    if (!canManage) {
                        CommandRefRow("/${row.first}", row.second)
                        return@forEachIndexed
                    }
                    Column(Modifier.fillMaxWidth().padding(bottom = 8.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text("/", color = c.text3, fontSize = 13.sp)
                            Spacer(Modifier.width(4.dp))
                            Box(Modifier.width(104.dp)) {
                                TTextField(
                                    value = row.first,
                                    onValueChange = { v ->
                                        rows = rows.toMutableList().also { it[i] = v to row.second }
                                    },
                                    placeholder = "approve",
                                )
                            }
                            Spacer(Modifier.width(8.dp))
                            Box(Modifier.weight(1f)) {
                                TTextField(
                                    value = row.second,
                                    onValueChange = { v ->
                                        rows = rows.toMutableList().also { it[i] = row.first to v }
                                    },
                                    placeholder = stringResource(R.string.commands_description_hint),
                                )
                            }
                            // Order in the list is order in the popup, so it has to be movable.
                            IonIconButton(Ion.SORT, {
                                if (i > 0) {
                                    rows = rows.toMutableList().apply {
                                        val prev = this[i - 1]
                                        this[i - 1] = this[i]
                                        this[i] = prev
                                    }
                                }
                            }, boxSize = 28.dp, iconSize = 14.dp, tint = if (i > 0) c.text2 else c.text3)
                            IonIconButton(Ion.TRASH, {
                                rows = rows.filterIndexed { j, _ -> j != i }
                            }, boxSize = 28.dp, iconSize = 14.dp, tint = c.text3)
                        }
                        if (errors[i].isNotEmpty()) {
                            Text(errors[i], color = TesseraDanger, fontSize = 11.sp, modifier = Modifier.padding(top = 2.dp))
                        }
                    }
                }
                if (rows.isEmpty()) {
                    Text(stringResource(R.string.commands_empty), color = c.text3, fontSize = 13.sp)
                }
            }

            if (canManage) {
                Spacer(Modifier.height(4.dp))
                TButton(
                    stringResource(R.string.commands_add),
                    kind = TButtonKind.Secondary,
                    enabled = rows.size < 50,
                    onClick = { rows = rows + ("" to "") },
                    modifier = Modifier.height(34.dp),
                )
            } else {
                Spacer(Modifier.height(4.dp))
                Text(stringResource(R.string.commands_readonly), color = c.text3, fontSize = 11.sp)
            }

            if (state.builtinCommands.isNotEmpty()) {
                Spacer(Modifier.height(14.dp))
                Text(
                    stringResource(R.string.commands_builtin),
                    color = c.text3,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.Medium,
                )
                Spacer(Modifier.height(4.dp))
                Column(Modifier.heightIn(max = 200.dp).verticalScroll(rememberScrollState())) {
                    state.builtinCommands.forEach { cmd -> CommandRefRow("/${cmd.key}", cmd.description) }
                }
            }

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                if (canManage) {
                    TButton(stringResource(R.string.common_cancel), kind = TButtonKind.Secondary, onClick = onDismiss)
                    Spacer(Modifier.width(8.dp))
                    TButton(
                        stringResource(R.string.common_save),
                        enabled = canSave,
                        onClick = {
                            vm.saveCustomCommands(
                                rows.map { WorkspaceCommand(canonCommandKey(it.first), it.second.trim()) },
                                onDismiss,
                            )
                        },
                    )
                } else {
                    TButton(stringResource(R.string.common_done), onClick = onDismiss)
                }
            }
        }
    }
}

/** One read-only command row: the mono `/key` plus its description. */
@Composable
private fun CommandRefRow(key: String, description: String) {
    val c = Tessera.colors
    Row(Modifier.fillMaxWidth().padding(vertical = 3.dp)) {
        Text(
            key,
            color = c.text2,
            fontSize = 12.sp,
            fontFamily = FontFamily.Monospace,
            modifier = Modifier.width(120.dp),
        )
        Text(description, color = c.text3, fontSize = 12.sp, modifier = Modifier.weight(1f))
    }
}

/**
 * «Короткие префиксы» — a device preference (not synced) that swaps the friendly
 * prefix name on scoped tag pills for the bare prefix ("T"), for people who want
 * shorter chips. Web parity: `tessera_tag_prefix_mode` in localStorage. Only shown
 * when the workspace actually has scoped tags.
 */
@Composable
private fun PrefixModeRow() {
    val c = Tessera.colors
    val scope = rememberCoroutineScope()
    val mode by AppContainer.prefs.tagPrefixMode.collectAsStateWithLifecycle(initialValue = "name")
    Row(
        Modifier.fillMaxWidth().padding(top = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                stringResource(R.string.tags_prefix_mode),
                color = c.text2,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
            )
            Text(stringResource(R.string.tags_prefix_mode_hint), color = c.text3, fontSize = 11.sp)
        }
        Spacer(Modifier.width(8.dp))
        TSwitch(
            checked = mode == "raw",
            onCheckedChange = { on -> scope.launch { AppContainer.prefs.setTagPrefixMode(if (on) "raw" else "name") } },
        )
    }
}

/**
 * One tag row (web `TagManager` parity): the tag's own badge, tapped to open an
 * inline editor (rename field + colour swatches); a trash button stays alongside.
 * [editing] is owned by the modal so a tap outside the row cancels it.
 */
@Composable
private fun TagRow(
    tag: website.msdnna.tessera.data.model.Tag,
    vm: BoardViewModel,
    prefixNames: Map<String, String>,
    showScope: Boolean,
    editing: Boolean,
    onEdit: () -> Unit,
    onDone: () -> Unit,
) {
    val c = Tessera.colors
    var nameEdit by remember(tag.id, editing) { mutableStateOf(tag.name) }
    var confirmDelete by remember { mutableStateOf(false) }
    fun commit(color: String) = vm.updateTag(tag.id, nameEdit.trim().ifBlank { tag.name }, color)

    Column(Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (editing) {
                Box(Modifier.weight(1f)) {
                    TTextField(
                        value = nameEdit,
                        onValueChange = { nameEdit = it },
                        placeholder = stringResource(R.string.tags_rename_hint),
                    )
                }
                Spacer(Modifier.width(6.dp))
                IonIconButton(
                    Ion.CHECK,
                    onClick = {
                        commit(tag.color)
                        onDone()
                    },
                    boxSize = 30.dp,
                    iconSize = 16.dp,
                    tint = c.primary,
                )
            } else {
                Box(Modifier.weight(1f), contentAlignment = Alignment.CenterStart) {
                    TagChip(
                        name = tag.name,
                        color = tag.color,
                        big = true,
                        modifier = Modifier.clickableNoRipple { onEdit() },
                        prefixNames = prefixNames,
                        showScope = showScope,
                    )
                }
            }
            Box {
                IonIconButton(Ion.TRASH, onClick = { confirmDelete = true }, boxSize = 30.dp, iconSize = 15.dp, tint = c.text3)
                TConfirmPopover(
                    expanded = confirmDelete,
                    message = stringResource(R.string.tags_delete_confirm),
                    onConfirm = {
                        confirmDelete = false
                        vm.deleteTag(tag.id)
                    },
                    onDismiss = { confirmDelete = false },
                )
            }
        }
        if (editing) {
            FlowRow(
                Modifier.padding(top = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                TagPalette.forEach { hex ->
                    val selected = tag.color.equals(hex, ignoreCase = true)
                    Box(
                        Modifier.size(22.dp).clip(CircleShape).background(accentGradient(parseHexColor(hex, c.text3)))
                            .then(if (selected) Modifier.border(2.dp, c.text1, CircleShape) else Modifier)
                            .clickableNoRipple { commit(hex) },
                    )
                }
            }
        }
    }
}
