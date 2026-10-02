import { TaskCard, ColumnHeader, TesseraProvider, hueGrad } from '@tessera/design-system'

const day = (n) => new Date(Date.now() + n * 864e5).toISOString().slice(0, 10)

const columns = [
  { id: 'todo', name: 'К работе', color: '#9aa0aa' },
  { id: 'doing', name: 'В процессе', color: '#2f80ed' },
  { id: 'done', name: 'Готово', color: '#18a058' },
]
const tags = [
  { id: 't1', name: 'app::frontend', color: '#7c5cff' },
  { id: 't2', name: 'app::backend', color: '#2f80ed' },
  { id: 't3', name: 'bug', color: '#e0533d' },
  { id: 't4', name: 'design', color: '#eb2f96' },
  { id: 't5', name: 'perf', color: '#0eb0a9' },
]
const members = [
  { user_id: 'u1', name: 'Мария Соколова' },
  { user_id: 'u2', name: 'Илья Петров' },
  { user_id: 'u3', name: 'Анна Ким' },
]
const milestones = [{ id: 'm1', title: 'Релиз 0.150', due_date: day(12) }]
const board = { columns, tags, members, milestones, prefixNames: { 'app::': 'App' } }

const tasks = {
  redesign: {
    id: 'a1',
    number: 2950,
    title: 'Редизайн доски: убрать разделители, поверхности вместо линий',
    priority: 3,
    due_date: day(4),
    column_id: 'doing',
    tag_ids: ['t1', 't4'],
    assignee_ids: ['u1', 'u3'],
    created_by: 'u2',
    milestone_id: 'm1',
    estimate: 6,
    has_description: true,
  },
  crash: {
    id: 'a2',
    number: 2941,
    title: 'Падение синхронизации GitLab при пустом этапе',
    priority: 4,
    due_date: day(-2),
    column_id: 'todo',
    tag_ids: ['t2', 't3'],
    assignee_ids: ['u2'],
    created_by: 'u1',
    has_description: true,
  },
  perf: {
    id: 'a3',
    number: 2932,
    title: 'Виртуализация списка задач на мобильном',
    priority: 2,
    due_date: day(9),
    column_id: 'doing',
    tag_ids: ['t1', 't5'],
    assignee_ids: ['u3'],
    created_by: 'u3',
    estimate: 3,
  },
  plain: {
    id: 'a4',
    number: 2928,
    title: 'Обновить зависимости фронтенда',
    priority: 1,
    column_id: 'todo',
    tag_ids: [],
    assignee_ids: [],
  },
  done: {
    id: 'a5',
    number: 2915,
    title: 'Тёмная тема для экрана входа',
    priority: 2,
    column_id: 'done',
    completed_at: day(-1),
    tag_ids: ['t1'],
    assignee_ids: ['u1'],
  },
}
const subtasks = [
  {
    id: 's1',
    title: 'Токены поверхностей',
    priority: 2,
    column_id: 'done',
    completed_at: day(-1),
    assignee_ids: ['u1'],
  },
  {
    id: 's2',
    title: 'Колонки без бордеров',
    priority: 3,
    column_id: 'doing',
    assignee_ids: ['u3'],
  },
  { id: 's3', title: 'Проверка в тёмной теме', priority: 2, column_id: 'todo', assignee_ids: [] },
]

const Lane = ({ width = 296, children }) => <div style={{ width }}>{children}</div>

export const Default = () => (
  <Lane>
    <TaskCard task={tasks.redesign} {...board} />
  </Lane>
)

export const Priorities = () => (
  <Lane>
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      <TaskCard task={tasks.crash} {...board} />
      <TaskCard task={tasks.perf} {...board} />
      <TaskCard task={tasks.plain} {...board} />
    </div>
  </Lane>
)

export const WithSubtasks = () => (
  <Lane>
    <TaskCard task={tasks.redesign} subtasks={subtasks} {...board} />
  </Lane>
)

export const Compact = () => (
  <Lane>
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      <TaskCard task={tasks.redesign} cardSize="compact" {...board} />
      <TaskCard task={tasks.crash} cardSize="compact" {...board} />
    </div>
  </Lane>
)

export const Completed = () => (
  <Lane>
    <TaskCard task={tasks.done} {...board} />
  </Lane>
)

// The current board, composed the way KanbanBoard lays it out (see conventions):
// 280px columns, 12px gap, 14px radius, 3px gradient top bar in the column colour,
// a faint same-hue wash and a 1px tinted border. Cards carry their own 8px
// bottom margin; the header gets the board's 8px gap below it.
const colStyle = (c) => ({
  width: 280,
  flex: '0 0 280px',
  boxSizing: 'border-box',
  borderRadius: 14,
  padding: 10,
  border: `1px solid color-mix(in srgb, ${c.color} 12%, var(--t-border))`,
  borderTop: '3px solid transparent',
  background: `linear-gradient(color-mix(in srgb, ${c.color} 6%, var(--t-surface-alt)), color-mix(in srgb, ${c.color} 6%, var(--t-surface-alt))) padding-box, ${hueGrad(c.color)} border-box`,
  display: 'flex',
  flexDirection: 'column',
})
const byCol = (id) => Object.values(tasks).filter((t) => t.column_id === id)

const BoardView = () => (
  <div
    style={{
      display: 'flex',
      gap: 12,
      alignItems: 'flex-start',
      padding: 16,
      width: 'max-content',
      background: 'var(--t-bg)',
    }}
  >
    {columns.map((c, i) => (
      <div key={c.id} style={colStyle(c)}>
        <ColumnHeader
          column={c}
          count={byCol(c.id).length}
          first={i === 0}
          isDone={c.id === 'done'}
          editable
          style={{ marginBottom: 8, padding: '0 2px' }}
        />
        {byCol(c.id).map((t) => (
          <TaskCard key={t.id} task={t} subtasks={t.id === 'a1' ? subtasks : []} {...board} />
        ))}
      </div>
    ))}
  </div>
)

export const Board = () => <BoardView />

export const BoardDark = () => (
  <TesseraProvider mode="dark">
    <BoardView />
  </TesseraProvider>
)
