import { TagPicker } from '@tessera/design-system'

const TAGS = [
  { id: 't1', name: 'дизайн', color: '#f0a020', task_count: 4 },
  { id: 't2', name: 'app::web', color: '#7c5cff', task_count: 3 },
  { id: 't3', name: 'бэкенд', color: '#2f80ed', task_count: 2 },
  { id: 't4', name: 'мобильное', color: '#18a058', task_count: 1 },
  { id: 't5', name: 'Сложность::высокая', color: '#eb2f96', task_count: 1 },
  { id: 't6', name: 'фронтенд', color: '#7c5cff', task_count: 3 },
  { id: 't7', name: 'app::android', color: '#2f80ed', task_count: 0 },
]
const NAMES = { 'app::': 'Приложение' }

export const TaskTags = () => <TagPicker tags={TAGS} selected={['t1', 't2']} prefixNames={NAMES} />
export const Search = () => (
  <TagPicker tags={TAGS} selected={['t1']} prefixNames={NAMES} query="фрон" />
)
export const Create = () => (
  <TagPicker tags={TAGS} selected={['t1', 't2']} prefixNames={NAMES} query="аналитика" />
)
