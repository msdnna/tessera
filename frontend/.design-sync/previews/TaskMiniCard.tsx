import { TaskMiniCard } from '@tessera/design-system'

const day = (n) => new Date(Date.now() + n * 864e5).toISOString().slice(0, 10)
const tags = [
  { id: 't1', name: 'app::frontend', color: '#7c5cff' },
  { id: 't3', name: 'bug', color: '#e0533d' },
]
const members = [
  { user_id: 'u1', name: 'Мария Соколова' },
  { user_id: 'u2', name: 'Илья Петров' },
]
const Box = ({ children }) => (
  <div
    style={{
      width: 300,
      padding: 10,
      borderRadius: 10,
      background: 'var(--t-surface)',
      boxShadow: '0 6px 24px rgba(0,0,0,0.12)',
    }}
  >
    {children}
  </div>
)

export const Default = () => (
  <Box>
    <TaskMiniCard
      task={{
        id: 'a2',
        number: 2941,
        title: 'Падение синхронизации GitLab при пустом этапе',
        priority: 4,
        due_date: day(-2),
        tag_ids: ['t1', 't3'],
        assignee_ids: ['u1', 'u2'],
      }}
      tags={tags}
      members={members}
    />
  </Box>
)

export const Minimal = () => (
  <Box>
    <TaskMiniCard
      task={{ id: 'a4', number: 2928, title: 'Обновить зависимости фронтенда', priority: 1 }}
    />
  </Box>
)
