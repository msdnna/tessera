import { Tabs } from '@tessera/design-system'

const taskTabs = [
  { name: 'desc', label: 'Описание' },
  { name: 'subtasks', label: 'Подзадачи' },
  { name: 'comments', label: 'Комментарии' },
  { name: 'files', label: 'Файлы' },
  { name: 'history', label: 'История' },
]
const Box = ({ children }) => <div style={{ width: 560 }}>{children}</div>

export const Line = () => (
  <Box>
    <Tabs tabs={taskTabs} defaultValue="comments" />
  </Box>
)
export const Segment = () => (
  <Box>
    <Tabs
      type="segment"
      tabs={[
        { name: 'board', label: 'Доска' },
        { name: 'list', label: 'Список' },
        { name: 'calendar', label: 'Календарь' },
      ]}
    />
  </Box>
)
