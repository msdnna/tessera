package website.msdnna.tessera.ui.screens.documents

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.data.model.DocumentApproval
import website.msdnna.tessera.data.model.DocumentApprovalMode
import website.msdnna.tessera.data.model.DocumentApprovalStatus
import website.msdnna.tessera.data.model.DocumentApprovalStep
import website.msdnna.tessera.data.model.DocumentTaskLink
import website.msdnna.tessera.data.model.Member
import website.msdnna.tessera.ui.TestTags
import website.msdnna.tessera.ui.components.IonIcon
import website.msdnna.tessera.ui.components.IonIconButton
import website.msdnna.tessera.ui.components.TButton
import website.msdnna.tessera.ui.components.TButtonKind
import website.msdnna.tessera.ui.components.TConfirmDialog
import website.msdnna.tessera.ui.components.TFormError
import website.msdnna.tessera.ui.components.TTextField
import website.msdnna.tessera.ui.components.clickableNoRipple
import website.msdnna.tessera.ui.theme.RadiusSm
import website.msdnna.tessera.ui.theme.Tessera
import website.msdnna.tessera.ui.viewmodels.DocLinksState
import website.msdnna.tessera.util.DocStepState
import website.msdnna.tessera.util.Ion
import website.msdnna.tessera.util.approvalProgress
import website.msdnna.tessera.util.approvalStepState
import website.msdnna.tessera.util.canDecideNow
import website.msdnna.tessera.util.orderedApprovalSteps

/**
 * Task links and approval routes of the open document (#2732, §7 of #2894).
 *
 * Both belong in one panel, as on the web: a route is raised against the
 * document, and the tasks it came from are the reason it exists. A panel over
 * the document rather than beside it, for the reason the discussions and the
 * journal are — the web has a column to spare next to the sheet and a phone has
 * none.
 *
 * Signing is offered only to the approver whose turn it actually is
 * ([canDecideNow]): the server refuses an out-of-turn signature with 409, and a
 * button that exists in order to be refused is worse than no button.
 */
@Composable
fun DocLinksSheet(
    state: DocLinksState,
    meId: String?,
    onDismiss: () -> Unit,
    onClearAnchor: () -> Unit,
    onLink: (taskId: String) -> Unit,
    onUnlink: (linkId: String) -> Unit,
    onRaise: (title: String, mode: String, approvers: List<String>) -> Unit,
    onDecide: (approvalId: String, decision: String, comment: String) -> Unit,
    onCancel: (approvalId: String) -> Unit,
    /** Opens the linked task. Null where there is nowhere to open it — the panel
     *  then lists the links without pretending they are tappable. */
    onOpenTask: ((DocumentTaskLink) -> Unit)?,
) {
    val c = Tessera.colors
    var picking by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    var raising by remember { mutableStateOf(false) }
    var confirmUnlink by remember { mutableStateOf<String?>(null) }
    var confirmCancel by remember { mutableStateOf<String?>(null) }

    BackHandler(enabled = true) { onDismiss() }

    Column(Modifier.fillMaxSize().background(c.surface).imePadding().testTag(TestTags.DOCUMENT_LINKS)) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                stringResource(R.string.docs_links_title),
                color = c.text1,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.padding(start = 8.dp),
            )
            if (state.loading) {
                Spacer(Modifier.width(6.dp))
                Text("…", color = c.text3, fontSize = 13.sp)
            }
            Spacer(Modifier.weight(1f))
            IonIconButton(
                Ion.CLOSE,
                onClick = onDismiss,
                boxSize = 40.dp,
                modifier = Modifier.testTag(TestTags.DOCUMENT_LINKS_CLOSE),
            )
        }
        HorizontalDivider(color = c.border)

        // What a new link will hang on. Shown because the block itself is behind
        // this panel — the same reasoning as the discussions' anchor strip.
        if (state.anchorBlockId.isNotBlank()) {
            Row(
                Modifier.fillMaxWidth().background(c.surfaceAlt).padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    state.anchorQuote.ifBlank { stringResource(R.string.docs_links_anchor_fallback) },
                    color = c.text2,
                    fontSize = 12.sp,
                    fontStyle = FontStyle.Italic,
                    maxLines = 2,
                    modifier = Modifier.weight(1f).testTag(TestTags.DOCUMENT_LINKS_ANCHOR),
                )
                Spacer(Modifier.width(8.dp))
                // The tag rides the tappable box, not the label: a clickable
                // merges its children's semantics and a tag inside stops being
                // findable (#2860).
                Box(
                    Modifier.testTag(TestTags.DOCUMENT_LINKS_UNPIN)
                        .clickableNoRipple(onClick = onClearAnchor)
                        .padding(4.dp),
                ) {
                    Text(
                        stringResource(R.string.docs_links_unpin),
                        color = c.primary,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                }
            }
        }

        TFormError(state.error, Modifier.padding(horizontal = 12.dp, vertical = 4.dp))

        LazyColumn(Modifier.weight(1f).fillMaxWidth()) {
            item { SectionTitle(stringResource(R.string.docs_links_tasks)) }

            if (state.links.isEmpty() && !state.loading) {
                item {
                    EmptyLine(
                        stringResource(R.string.docs_links_empty),
                        Modifier.testTag(TestTags.DOCUMENT_LINKS_EMPTY),
                    )
                }
            }
            items(state.links, key = { it.id }) { link ->
                DocLinkRow(
                    link = link,
                    onOpen = onOpenTask,
                    onRemove = { confirmUnlink = link.id },
                )
            }

            item {
                Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 6.dp)) {
                    TButton(
                        stringResource(
                            if (state.anchorBlockId.isNotBlank()) {
                                R.string.docs_links_link_block
                            } else {
                                R.string.docs_links_link_task
                            },
                        ),
                        kind = TButtonKind.Secondary,
                        icon = Ion.LINK,
                        onClick = { picking = !picking },
                        enabled = !state.busy,
                        modifier = Modifier.testTag(TestTags.DOCUMENT_LINK_ADD),
                    )
                    if (picking) {
                        Spacer(Modifier.height(6.dp))
                        TaskPicker(
                            state = state,
                            query = query,
                            onQuery = { query = it },
                            onPick = { task ->
                                picking = false
                                query = ""
                                onLink(task.id)
                            },
                        )
                    }
                }
            }

            item { SectionTitle(stringResource(R.string.docs_links_approvals)) }

            if (state.approvals.isEmpty() && !state.loading) {
                item {
                    EmptyLine(
                        stringResource(R.string.docs_links_no_approvals),
                        Modifier.testTag(TestTags.DOCUMENT_APPROVALS_EMPTY),
                    )
                }
            }
            // Newest first, as the server returns them: the open route is the one
            // being asked about, and the closed ones are the journal behind it.
            items(state.approvals, key = { it.id }) { approval ->
                DocApprovalCard(
                    approval = approval,
                    meId = meId,
                    busy = state.busy,
                    onDecide = { decision, comment -> onDecide(approval.id, decision, comment) },
                    onCancel = { confirmCancel = approval.id },
                )
            }

            item {
                Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp)) {
                    if (!raising) {
                        TButton(
                            stringResource(R.string.docs_links_raise),
                            kind = TButtonKind.Secondary,
                            onClick = { raising = true },
                            // One open route per document — see canRaiseApproval.
                            enabled = state.canRaise && !state.busy,
                            modifier = Modifier.testTag(TestTags.DOCUMENT_APPROVAL_RAISE),
                        )
                        if (!state.canRaise) {
                            Spacer(Modifier.height(4.dp))
                            Text(
                                stringResource(R.string.docs_links_raise_disabled),
                                color = c.text3,
                                fontSize = 11.sp,
                            )
                        }
                    } else {
                        RouteComposer(
                            members = state.members,
                            busy = state.busy,
                            onSubmit = { title, mode, approvers ->
                                raising = false
                                onRaise(title, mode, approvers)
                            },
                            onCancel = { raising = false },
                        )
                    }
                }
            }
            item { Spacer(Modifier.height(16.dp)) }
        }
    }

    confirmUnlink?.let { id ->
        TConfirmDialog(
            title = stringResource(R.string.docs_links_unlink),
            message = stringResource(R.string.docs_links_unlink_confirm),
            confirmTag = TestTags.DOCUMENT_LINK_REMOVE_CONFIRM,
            onConfirm = {
                onUnlink(id)
                confirmUnlink = null
            },
            onDismiss = { confirmUnlink = null },
        )
    }

    confirmCancel?.let { id ->
        TConfirmDialog(
            title = stringResource(R.string.docs_links_cancel_route),
            message = stringResource(R.string.docs_links_cancel_confirm),
            confirmTag = TestTags.DOCUMENT_APPROVAL_CANCEL_CONFIRM,
            onConfirm = {
                onCancel(id)
                confirmCancel = null
            },
            onDismiss = { confirmCancel = null },
        )
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(
        text.uppercase(),
        color = Tessera.colors.text3,
        fontSize = 11.sp,
        fontWeight = FontWeight.SemiBold,
        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 2.dp),
    )
}

@Composable
private fun EmptyLine(text: String, modifier: Modifier = Modifier) {
    Text(
        text,
        color = Tessera.colors.text3,
        fontSize = 13.sp,
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp),
    )
}

/**
 * One linked task.
 *
 * The quote is what says *which* clause the task hangs on once that clause has
 * been rewritten; without it an anchored link degrades into a link on the whole
 * document.
 */
@Composable
private fun DocLinkRow(
    link: DocumentTaskLink,
    onOpen: ((DocumentTaskLink) -> Unit)?,
    onRemove: () -> Unit,
) {
    val c = Tessera.colors
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(
            Modifier.weight(1f)
                .clip(RoundedCornerShape(RadiusSm))
                .then(if (onOpen != null) Modifier.clickableNoRipple { onOpen(link) } else Modifier)
                .padding(horizontal = 6.dp, vertical = 8.dp)
                .testTag(TestTags.documentLinkRow(link.id)),
        ) {
            Text(
                link.taskTitle.ifBlank { stringResource(R.string.docs_links_task) },
                color = c.text1,
                fontSize = 13.sp,
                maxLines = 1,
            )
            if (link.blockId.isNotBlank()) {
                Text(
                    link.quote.ifBlank { stringResource(R.string.docs_links_fragment) },
                    color = c.text3,
                    fontSize = 11.sp,
                    maxLines = 1,
                )
            }
        }
        IonIconButton(
            Ion.CLOSE,
            onClick = onRemove,
            boxSize = 32.dp,
            modifier = Modifier.testTag(TestTags.documentLinkRemove(link.id)),
        )
    }
}

/** The task picker: search by number or title over the workspace's tasks. */
@Composable
private fun TaskPicker(
    state: DocLinksState,
    query: String,
    onQuery: (String) -> Unit,
    onPick: (website.msdnna.tessera.data.model.WorkspaceTask) -> Unit,
) {
    val c = Tessera.colors
    val candidates = state.candidates(query)

    Column(
        Modifier.fillMaxWidth()
            .clip(RoundedCornerShape(RadiusSm))
            .border(1.dp, c.border, RoundedCornerShape(RadiusSm))
            .padding(8.dp),
    ) {
        TTextField(
            value = query,
            onValueChange = onQuery,
            placeholder = stringResource(R.string.docs_links_search_hint),
            fieldTag = TestTags.DOCUMENT_LINK_QUERY,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(6.dp))
        if (candidates.isEmpty()) {
            Text(
                stringResource(R.string.docs_links_nothing_found),
                color = c.text3,
                fontSize = 12.sp,
                modifier = Modifier.padding(vertical = 6.dp),
            )
        }
        // Bounded rather than free-growing: the picker sits inside the panel's
        // own scroll, and a list of fifty rows would push everything under it
        // out of reach.
        Column(Modifier.fillMaxWidth().heightIn(max = PICKER_MAX_HEIGHT)) {
            candidates.forEach { task ->
                Row(
                    Modifier.fillMaxWidth()
                        .clip(RoundedCornerShape(RadiusSm))
                        .clickableNoRipple { onPick(task) }
                        .padding(horizontal = 6.dp, vertical = 8.dp)
                        .testTag(TestTags.documentLinkCandidate(task.id)),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("#${task.number}", color = c.text3, fontSize = 11.sp)
                    Spacer(Modifier.width(6.dp))
                    Text(task.title, color = c.text1, fontSize = 13.sp, maxLines = 1)
                }
            }
        }
    }
}

/** How tall the candidate list may grow inside the panel's own scroll. */
private val PICKER_MAX_HEIGHT = 260.dp

/**
 * One approval route.
 *
 * The open route takes the accent frame; closed ones stay neutral — the panel is
 * read to find out what is being asked of you *now*.
 */
@Composable
private fun DocApprovalCard(
    approval: DocumentApproval,
    meId: String?,
    busy: Boolean,
    onDecide: (decision: String, comment: String) -> Unit,
    onCancel: () -> Unit,
) {
    val c = Tessera.colors
    var deciding by remember { mutableStateOf(false) }
    var comment by remember { mutableStateOf("") }
    val progress = approvalProgress(approval)
    val mayDecide = canDecideNow(approval, meId)

    Column(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp)
            .clip(RoundedCornerShape(RadiusSm))
            .background(c.surfaceAlt)
            .border(
                1.dp,
                if (approval.isOpen) c.primary else c.border,
                RoundedCornerShape(RadiusSm),
            )
            .padding(10.dp)
            .testTag(TestTags.documentApprovalCard(approval.id)),
    ) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                stringResource(approvalStatusLabel(approval.status)),
                color = if (approval.status == DocumentApprovalStatus.REJECTED) REJECTED_COLOR else c.text2,
                fontSize = 11.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f).testTag(TestTags.documentApprovalStatus(approval.id)),
            )
            Text(
                stringResource(R.string.docs_links_progress, progress.signed, progress.total),
                color = c.text3,
                fontSize = 11.sp,
            )
        }
        if (approval.title.isNotBlank()) {
            Text(approval.title, color = c.text1, fontSize = 13.sp, modifier = Modifier.padding(top = 2.dp))
        }
        // Which text is being agreed: a route that cannot name its revision is a
        // signature on a moving target.
        Text(
            stringResource(
                R.string.docs_links_meta,
                approval.versionRevision,
                approval.createdByName?.takeIf { it.isNotBlank() }
                    ?: stringResource(R.string.docs_links_unknown_author),
            ),
            color = c.text3,
            fontSize = 11.sp,
        )

        Spacer(Modifier.height(4.dp))
        orderedApprovalSteps(approval).forEach { step ->
            DocApprovalStepRow(state = approvalStepState(approval, step), step = step)
        }

        if (mayDecide) {
            Spacer(Modifier.height(6.dp))
            if (!deciding) {
                TButton(
                    stringResource(R.string.docs_links_sign),
                    onClick = { deciding = true },
                    enabled = !busy,
                    modifier = Modifier.height(32.dp).testTag(TestTags.DOCUMENT_APPROVAL_SIGN),
                )
            } else {
                TTextField(
                    value = comment,
                    onValueChange = { comment = it },
                    placeholder = stringResource(R.string.docs_links_comment_hint),
                    singleLine = false,
                    fieldTag = TestTags.DOCUMENT_APPROVAL_COMMENT,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(6.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    TButton(
                        stringResource(R.string.docs_links_approve),
                        onClick = {
                            onDecide(DocumentApprovalStatus.APPROVED, comment)
                            deciding = false
                            comment = ""
                        },
                        enabled = !busy,
                        modifier = Modifier.height(32.dp).testTag(TestTags.DOCUMENT_APPROVAL_APPROVE),
                    )
                    TButton(
                        stringResource(R.string.docs_links_reject),
                        kind = TButtonKind.Secondary,
                        onClick = {
                            onDecide(DocumentApprovalStatus.REJECTED, comment)
                            deciding = false
                            comment = ""
                        },
                        enabled = !busy,
                        modifier = Modifier.height(32.dp).testTag(TestTags.DOCUMENT_APPROVAL_REJECT),
                    )
                    TButton(
                        stringResource(R.string.common_cancel),
                        kind = TButtonKind.Ghost,
                        onClick = {
                            deciding = false
                            comment = ""
                        },
                        modifier = Modifier.height(32.dp),
                    )
                }
            }
        }

        if (approval.isOpen) {
            Spacer(Modifier.height(6.dp))
            TButton(
                stringResource(R.string.docs_links_cancel_route),
                kind = TButtonKind.Ghost,
                onClick = onCancel,
                enabled = !busy,
                modifier = Modifier.height(30.dp).testTag(TestTags.documentApprovalCancel(approval.id)),
            )
        }
    }
}

/**
 * One approver's slot.
 *
 * Every step states its outcome in words — «подписал» / «отклонил» / «ждёт» —
 * and the rail on the left only reinforces it. Nothing here depends on telling
 * one hue from another, which is what the two commonest forms of colour
 * blindness take away.
 */
@Composable
private fun DocApprovalStepRow(state: DocStepState, step: DocumentApprovalStep) {
    val c = Tessera.colors
    val rail = when (state) {
        DocStepState.SIGNED, DocStepState.CURRENT -> c.primary
        DocStepState.REJECTED -> REJECTED_COLOR
        DocStepState.WAITING -> c.border
    }
    val label = when (state) {
        DocStepState.SIGNED -> R.string.docs_links_step_signed
        DocStepState.REJECTED -> R.string.docs_links_step_rejected
        DocStepState.CURRENT -> R.string.docs_links_step_current
        DocStepState.WAITING -> R.string.docs_links_step_waiting
    }

    Column(
        Modifier.fillMaxWidth().padding(vertical = 2.dp)
            .stepRail(rail)
            .padding(start = 8.dp)
            .testTag(TestTags.documentApprovalStep(step.id)),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (state == DocStepState.SIGNED) {
                IonIcon(Ion.CHECK_CIRCLE, size = 12.dp, tint = c.primary)
                Spacer(Modifier.width(4.dp))
            }
            Text(
                step.approverName.ifBlank { stringResource(R.string.docs_links_unknown_author) },
                color = if (state == DocStepState.WAITING) c.text3 else c.text1,
                fontSize = 12.sp,
            )
            Spacer(Modifier.width(6.dp))
            Text(
                stringResource(label),
                color = if (state == DocStepState.SIGNED) c.primary else c.text3,
                fontSize = 11.sp,
            )
        }
        if (step.comment.isNotBlank()) {
            Text(step.comment, color = c.text3, fontSize = 11.sp)
        }
    }
}

/** Composes a new route: what is being agreed, in which order, by whom. */
@Composable
private fun RouteComposer(
    members: List<Member>,
    busy: Boolean,
    onSubmit: (title: String, mode: String, approvers: List<String>) -> Unit,
    onCancel: () -> Unit,
) {
    val c = Tessera.colors
    var title by remember { mutableStateOf("") }
    var mode by remember { mutableStateOf(DocumentApprovalMode.SEQUENTIAL) }
    // Order matters in a sequential route, so the picked approvers are kept in
    // the order they were tapped in — that list *is* the route.
    var approvers by remember { mutableStateOf(listOf<String>()) }

    Column(Modifier.fillMaxWidth().testTag(TestTags.DOCUMENT_APPROVAL_COMPOSER)) {
        TTextField(
            value = title,
            onValueChange = { title = it },
            placeholder = stringResource(R.string.docs_links_route_title_hint),
            fieldTag = TestTags.DOCUMENT_APPROVAL_TITLE,
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(8.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            ModeChip(
                label = stringResource(R.string.docs_links_mode_sequential),
                selected = mode == DocumentApprovalMode.SEQUENTIAL,
                tag = TestTags.DOCUMENT_APPROVAL_MODE_SEQUENTIAL,
                onClick = { mode = DocumentApprovalMode.SEQUENTIAL },
            )
            ModeChip(
                label = stringResource(R.string.docs_links_mode_parallel),
                selected = mode == DocumentApprovalMode.PARALLEL,
                tag = TestTags.DOCUMENT_APPROVAL_MODE_PARALLEL,
                onClick = { mode = DocumentApprovalMode.PARALLEL },
            )
        }
        Spacer(Modifier.height(8.dp))
        Text(stringResource(R.string.docs_links_approvers), color = c.text3, fontSize = 11.sp)
        members.forEach { member ->
            val index = approvers.indexOf(member.userId)
            Row(
                Modifier.fillMaxWidth()
                    .clip(RoundedCornerShape(RadiusSm))
                    .clickableNoRipple {
                        approvers = if (index >= 0) {
                            approvers - member.userId
                        } else {
                            approvers + member.userId
                        }
                    }
                    .padding(horizontal = 6.dp, vertical = 8.dp)
                    .testTag(TestTags.documentApproverRow(member.userId)),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                // The ordinal, not a tick: in a sequential route the number *is*
                // the information — who signs after whom.
                Text(
                    if (index >= 0) "${index + 1}" else "—",
                    color = if (index >= 0) c.primary else c.text3,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.width(20.dp),
                )
                Text(
                    member.name.ifBlank { member.email },
                    color = if (index >= 0) c.text1 else c.text2,
                    fontSize = 13.sp,
                    maxLines = 1,
                )
            }
        }
        if (members.isEmpty()) {
            Text(
                stringResource(R.string.docs_links_no_members),
                color = c.text3,
                fontSize = 12.sp,
                modifier = Modifier.padding(vertical = 6.dp),
            )
        }
        Spacer(Modifier.height(8.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            TButton(
                stringResource(R.string.docs_links_submit),
                onClick = { onSubmit(title, mode, approvers) },
                enabled = approvers.isNotEmpty() && !busy,
                modifier = Modifier.height(34.dp).testTag(TestTags.DOCUMENT_APPROVAL_SUBMIT),
            )
            TButton(
                stringResource(R.string.common_cancel),
                kind = TButtonKind.Secondary,
                onClick = onCancel,
                modifier = Modifier.height(34.dp),
            )
        }
    }
}

@Composable
private fun ModeChip(label: String, selected: Boolean, tag: String, onClick: () -> Unit) {
    val c = Tessera.colors
    Box(
        Modifier.clip(RoundedCornerShape(RadiusSm))
            .background(if (selected) c.surfaceAlt else Color.Transparent)
            .border(1.dp, if (selected) c.primary else c.border, RoundedCornerShape(RadiusSm))
            .clickableNoRipple(onClick = onClick)
            .padding(horizontal = 10.dp, vertical = 6.dp)
            .testTag(tag),
    ) {
        Text(label, color = if (selected) c.text1 else c.text3, fontSize = 12.sp)
    }
}

private fun approvalStatusLabel(status: String): Int = when (status) {
    DocumentApprovalStatus.PENDING -> R.string.docs_links_status_pending
    DocumentApprovalStatus.APPROVED -> R.string.docs_links_status_approved
    DocumentApprovalStatus.REJECTED -> R.string.docs_links_status_rejected
    DocumentApprovalStatus.CANCELLED -> R.string.docs_links_status_cancelled
    else -> R.string.docs_links_status_unknown
}

// Rejection is the one outcome that has to be unmissable, and the neutral
// palette has no semantic colour for it — the same literal the journal's diff
// rails use, and the same value the web falls back to for `--t-error`.
private val REJECTED_COLOR = Color(0xFFD03050)

private fun Modifier.stepRail(color: Color): Modifier =
    drawBehind { drawRect(color, size = Size(2.dp.toPx(), size.height)) }
