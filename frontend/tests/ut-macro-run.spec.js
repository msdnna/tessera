import { describe, it, expect, vi } from 'vitest'
import { currentStage, firstOpenColumn, macroBlocked, runMacro, taskPlan } from '@/utils/macroRun'

const TPL = {
  id: 'tpl',
  title: 'Баг: ',
  description: '## Шаги\n- [ ] воспроизвести',
  tags: ['type::bug', 'Срочно'],
  priority: 2,
  subtasks: ['Тест', ' '],
}

describe('macro run (#3069)', () => {
  it('plans a task from the template and the params', () => {
    const plan = taskPlan(
      {
        name: 'Новый баг',
        params: { template_id: 'tpl', tags: ['срочно', 'web'], priority: 3, assignee: 'me' },
      },
      TPL,
    )
    expect(plan).toEqual({
      title: 'Баг:',
      description: TPL.description,
      priority: 3,
      tags: ['type::bug', 'Срочно', 'web'],
      subtasks: ['Тест'],
      assignMe: true,
      currentStage: false,
    })
  })

  it('picks the first open column and the active stage due soonest', () => {
    const cols = [
      { id: 'done', position: 0 },
      { id: 'todo', position: 1 },
    ]
    expect(firstOpenColumn(cols, 'done').id).toBe('todo')
    const ms = [
      { id: 'late', state: 'active', due_date: '2026-12-01' },
      { id: 'soon', state: 'active', due_date: '2026-11-01' },
      { id: 'shut', state: 'closed', due_date: '2026-10-20' },
    ]
    expect(currentStage(ms).id).toBe('soon')
  })

  it('needs a board for a task and a pipeline', () => {
    expect(macroBlocked({ action: 'create_task' }, {})).toBe('board')
    expect(macroBlocked({ action: 'create_task' }, { boardId: 'b' })).toBe('')
    expect(macroBlocked({ action: 'run_pipeline' }, {})).toBe('board')
    expect(macroBlocked({ action: 'open_link' }, {})).toBe('')
  })

  it('runs the board project pipeline (#3099)', async () => {
    const runCI = vi.fn(async () => ({ data: { id: 77, project: 'team/web' } }))
    const res = await runMacro(
      { action: 'run_pipeline', params: { ref: 'main', variables: { ENV: 'staging' } } },
      { board: { id: 'b1' }, api: { boards: { runCI } } },
    )
    expect(runCI).toHaveBeenCalledWith('b1', {
      mode: 'run',
      ref: 'main',
      variables: { ENV: 'staging' },
      integration_id: undefined,
    })
    expect(res.pipeline.id).toBe(77)
  })

  it('creates the task, its tags, assignee and subtasks through the API', async () => {
    const api = {
      boards: {
        createTask: vi.fn(async (_b, d) => ({
          data: { id: d.parent_id ? 'sub' : 't1', number: 7 },
        })),
      },
      projects: { tags: vi.fn(async () => ({ data: [{ id: 'tg1', name: 'type::bug' }] })) },
      tasks: { addTag: vi.fn(), addAssignee: vi.fn(), setMilestone: vi.fn() },
    }
    const go = vi.fn()
    const res = await runMacro(
      { action: 'create_task', name: 'Новый баг', params: { template_id: 'tpl', assignee: 'me' } },
      {
        me: 'u1',
        templates: [TPL],
        board: {
          id: 'b1',
          projectId: 'p1',
          columns: [{ id: 'c1', position: 0 }],
          doneColumnId: null,
        },
        api,
        go,
      },
    )
    expect(api.boards.createTask).toHaveBeenNthCalledWith(1, 'b1', {
      column_id: 'c1',
      title: 'Баг:',
      description: TPL.description,
      priority: 2,
    })
    expect(api.tasks.addTag).toHaveBeenCalledWith('t1', 'tg1')
    expect(res.missedTags).toEqual(['Срочно'])
    expect(api.tasks.addAssignee).toHaveBeenCalledWith('t1', 'u1')
    expect(api.boards.createTask).toHaveBeenLastCalledWith('b1', {
      column_id: 'c1',
      title: 'Тест',
      parent_id: 't1',
    })
    expect(go).toHaveBeenCalledWith('/board/b1?task=7')
  })

  it('opens a link in a new tab', async () => {
    const open = vi.fn()
    await runMacro(
      { action: 'open_link', params: { url: 'https://grafana.example.com' } },
      { open },
    )
    expect(open).toHaveBeenCalledWith('https://grafana.example.com')
  })
})
