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
    confirm: { title: 'Точно удалить «Tessera Web»?', sub: '3 доски со всеми задачами · без возврата' },
  },
]

export const Project = () => <ActionMenu header="Tessera Web" headerAside="Продукт" items={PROJECT} />

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
      { key: 'right', label: 'Сдвинуть вправо', icon: 'ArrowForwardOutline', caps: ['⌘', '⇧', '→'] },
      { key: 'add', label: 'Новая колонка справа', icon: 'AddOutline' },
      { gap: true },
      { key: 'delete', label: 'Удалить колонку', icon: 'TrashOutline', danger: true, caps: ['⌫'] },
    ]}
  />
)
