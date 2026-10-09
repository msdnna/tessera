import { ActionMenu } from '@tessera/design-system'

const PROJECT = [
  { key: 'open', label: 'Открыть', icon: 'OpenOutline', caps: ['↵'] },
  { key: 'board', label: 'Новая доска', icon: 'AddOutline', caps: ['⇧', 'N'] },
  { gap: true },
  { key: 'rename', label: 'Переименовать', icon: 'CreateOutline', caps: ['F2'] },
  { key: 'icon', label: 'Иконка и цвет…', icon: 'ColorPaletteOutline', caps: ['I'] },
  { key: 'slug', label: 'Сменить адрес…', icon: 'LinkOutline', caps: ['U'] },
  { key: 'move', label: 'Перенести в пространство…', icon: 'SwapHorizontalOutline', caps: ['M'] },
  { gap: true },
  { key: 'link', label: 'Копировать ссылку', icon: 'CopyOutline', caps: ['⌘', '⇧', 'C'] },
  { key: 'settings', label: 'Настройки проекта', icon: 'SettingsOutline', stub: 2971 },
  { gap: true },
  { key: 'archive', label: 'Архивировать', icon: 'ArchiveOutline', stub: 3005 },
  {
    key: 'delete',
    label: 'Удалить проект',
    icon: 'TrashOutline',
    danger: true,
    caps: ['⌘', '⌫'],
    confirm: {
      title: 'Точно удалить «Tessera Web»?',
      sub: '3 доски со всеми задачами · без возврата',
    },
  },
]

export const Project = () => (
  <ActionMenu header="Tessera Web" headerAside="Продукт" items={PROJECT} />
)

export const ProjectArmed = () => (
  <ActionMenu header="Tessera Web" headerAside="Продукт" items={PROJECT} armed="delete" />
)

export const Column = () => (
  <ActionMenu
    header="На рассмотрении"
    headerAside="3 задачи"
    width={284}
    items={[
      { key: 'rename', label: 'Переименовать', icon: 'CreateOutline', caps: ['F2'] },
      { key: 'color', label: 'Цвет', icon: 'ColorPaletteOutline', swatch: '#f0a020' },
      { key: 'done', label: 'Колонка «Готово»', icon: 'CheckmarkDoneOutline', toggle: false },
      { gap: true },
      { key: 'collapse', label: 'Свернуть', icon: 'ChevronBackOutline', caps: ['H'] },
      {
        key: 'right',
        label: 'Сдвинуть вправо',
        icon: 'ArrowForwardOutline',
        caps: ['⌘', '⇧', '→'],
      },
      { key: 'add', label: 'Новая колонка справа', icon: 'AddOutline' },
      { gap: true },
      { key: 'delete', label: 'Удалить колонку', icon: 'TrashOutline', danger: true, caps: ['⌫'] },
    ]}
  />
)

const TASK = [
  { key: 'open', label: 'Открыть', icon: 'OpenOutline', caps: ['↵'] },
  { key: 'done', label: 'Отметить выполненной', icon: 'CheckmarkDoneOutline', caps: ['E'] },
  {
    key: 'priority',
    label: 'Приоритет',
    icon: 'FlagOutline',
    caps: ['P'],
    subWidth: 220,
    children: [
      { key: 'p0', label: 'Без приоритета', dot: '#9aa0aa', caps: ['0'] },
      { key: 'p1', label: 'Низкий', dot: '#5b9bd5', caps: ['1'] },
      { key: 'p2', label: 'Обычный', dot: '#3aa675', caps: ['2'], checked: true },
      { key: 'p3', label: 'Высокий', dot: '#e0a418', caps: ['3'] },
      { key: 'p4', label: 'Срочный', dot: '#e0533d', caps: ['4'] },
    ],
  },
  {
    key: 'move',
    label: 'Переместить в колонку',
    icon: 'ArrowForwardOutline',
    children: [
      { key: 'c1', label: 'К работе', dot: '#0eb0a9', checked: true },
      { key: 'c2', label: 'В процессе', dot: '#2f80ed' },
      { key: 'c3', label: 'На рассмотрении', dot: '#f0a020' },
      { key: 'c4', label: 'Готово', dot: '#18a058' },
    ],
  },
  { key: 'subtask', label: 'Создать подзадачу', icon: 'GitBranchOutline', caps: ['⇧', 'N'] },
  { gap: true },
  { key: 'archive', label: 'В архив', icon: 'ArchiveOutline' },
  {
    key: 'delete',
    label: 'Удалить',
    icon: 'TrashOutline',
    danger: true,
    confirm: { title: 'Удалить #142?', sub: 'Вместе с 3 подзадачами · без возврата' },
  },
]

export const TaskSubmenu = () => (
  <div style={{ width: 540, height: 330 }}>
    <ActionMenu header="#142 Экран онбординга" items={TASK} width={290} openSub="priority" />
  </div>
)

export const Workspace = () => (
  <div style={{ width: 540, height: 250 }}>
    <ActionMenu
      header="Tessera Demo"
      width={290}
      openSub="integrations"
      items={[
        { key: 'members', label: 'Участники', icon: 'PeopleOutline' },
        {
          key: 'integrations',
          label: 'Интеграции',
          icon: 'ExtensionPuzzleOutline',
          children: [
            {
              key: 'gitlab',
              label: 'GitLab',
              icon: 'LogoGitlab',
              note: '2 конфликта',
              noteWarn: true,
            },
            { key: 'estimation', label: 'Оценка задач', icon: 'SpeedometerOutline' },
            { key: 'commands', label: 'Команды редактора', icon: 'TerminalOutline' },
          ],
        },
        {
          key: 'appearance',
          label: 'Оформление пространства',
          icon: 'ColorPaletteOutline',
          caps: ['I'],
        },
        { gap: true },
        { key: 'create', label: 'Новое пространство', icon: 'AddOutline', caps: ['⇧', 'N'] },
        { key: 'delete', label: 'Удалить пространство', icon: 'TrashOutline', danger: true },
      ]}
    />
  </div>
)
