// Running a workspace command (#3069, CmdsWin). Every action is a call to an
// endpoint the app uses anyway, so the server's invariants (positions, journal,
// tags of the project, recurrence…) hold exactly as when a person does it.
//
//   create_task      a task on the open board (first open column): the template's
//                    title, description, priority, tags (by name — the project's
//                    tags of that name; the rest are reported), subtasks; params
//                    may add tags, override the priority, assign «me», pin the
//                    current stage
//   open_link        the URL in a new tab
//   create_document  a document from a document template (in the board's project
//                    when a board is open)
//   run_pipeline     not yet (#3099) — the row says «Скоро»
import { stageState } from './milestones'

export const MACRO_ACTIONS = ['create_task', 'open_link', 'create_document', 'run_pipeline']
export const SOON_ACTIONS = { run_pipeline: 3099 }

// taskPlan — what create_task makes, as data (pure, tested).
export function taskPlan(macro, template) {
  const p = macro?.params || {}
  const tpl = template || {}
  const tags = [...(tpl.tags || []), ...(Array.isArray(p.tags) ? p.tags : [])]
  const seen = new Set()
  return {
    title: String(p.title || tpl.title || macro?.name || '').trim(),
    description: tpl.description || '',
    priority: Number.isInteger(p.priority) ? p.priority : tpl.priority || 0,
    tags: tags.filter((n) => {
      const k = String(n).trim().toLowerCase()
      if (!k || seen.has(k)) return false
      seen.add(k)
      return true
    }),
    subtasks: (tpl.subtasks || []).filter((s) => String(s).trim()),
    assignMe: p.assignee === 'me',
    currentStage: p.milestone === 'current',
  }
}

// The column a new task goes to: the leftmost one that is not «done».
export function firstOpenColumn(columns, doneColumnId) {
  const sorted = [...(columns || [])].sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
  return sorted.find((c) => c.id !== doneColumnId) || sorted[0] || null
}

// The stage «текущий» means: the active stage closest to its due date.
export function currentStage(milestones, today) {
  const active = (milestones || []).filter((m) => stageState(m, today) === 'active')
  active.sort((a, b) => String(a.due_date || '9999').localeCompare(String(b.due_date || '9999')))
  return active[0] || null
}

// What the command needs before it can run, or '' when it can: 'board' (a
// create_task without a board open), 'soon' (an action not built yet).
export function macroBlocked(macro, { boardId } = {}) {
  if (SOON_ACTIONS[macro?.action]) return 'soon'
  if (macro?.action === 'create_task' && !boardId) return 'board'
  return ''
}

/**
 * runMacro executes a command. `ctx`: { wsId, me, board: { id, projectId,
 * columns, doneColumnId, milestones }, templates, docTemplates, api: { tasks,
 * boards, projects, documents }, open(url), go(path) }. Resolves to
 * { path?, missedTags? }.
 */
export async function runMacro(macro, ctx) {
  const p = macro.params || {}
  if (macro.action === 'open_link') {
    ctx.open(p.url)
    return {}
  }
  if (macro.action === 'create_document') {
    const tpl = (ctx.docTemplates || []).find((x) => x.id === p.template_id)
    const { data } = await ctx.api.documents.create(ctx.wsId, {
      title: String(p.title || tpl?.title || tpl?.name || macro.name),
      template_id: tpl ? tpl.id : undefined,
      project_id: ctx.board?.projectId || undefined,
    })
    const path = `/documents/${data.slug || data.id}`
    ctx.go(path)
    return { path }
  }
  if (macro.action !== 'create_task') throw new Error('unsupported')

  const board = ctx.board
  const column = firstOpenColumn(board.columns, board.doneColumnId)
  const template = (ctx.templates || []).find((x) => x.id === p.template_id) || null
  const plan = taskPlan(macro, template)
  const { data: task } = await ctx.api.boards.createTask(board.id, {
    column_id: column.id,
    title: plan.title || macro.name,
    description: plan.description,
    priority: plan.priority,
  })
  const missedTags = []
  if (plan.tags.length) {
    const { data: tags } = await ctx.api.projects.tags(board.projectId)
    const byName = new Map((tags || []).map((t) => [t.name.toLowerCase(), t]))
    for (const name of plan.tags) {
      const tag = byName.get(String(name).toLowerCase())
      if (tag) await ctx.api.tasks.addTag(task.id, tag.id)
      else missedTags.push(name)
    }
  }
  if (plan.assignMe && ctx.me) await ctx.api.tasks.addAssignee(task.id, ctx.me)
  if (plan.currentStage) {
    const stage = currentStage(board.milestones)
    if (stage) await ctx.api.tasks.setMilestone(task.id, stage.id)
  }
  for (const title of plan.subtasks) {
    await ctx.api.boards.createTask(board.id, { column_id: column.id, title, parent_id: task.id })
  }
  const path = `/board/${board.id}?task=${task.number ?? task.id}`
  ctx.go(path)
  return { path, missedTags, task }
}
