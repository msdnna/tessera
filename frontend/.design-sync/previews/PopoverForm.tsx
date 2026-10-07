import { PopoverForm, FormField, ColorIconPicker } from '@tessera/design-system'

export const NewProject = () => (
  <PopoverForm
    title="Новый проект"
    subtitle="в группе «Продукт»"
    primary="Создать"
    hint="tab"
    width={348}
  >
    <FormField label="Название">
      <input className="kf-input" defaultValue="Tessera Web" placeholder="Название проекта" />
    </FormField>
    <FormField label="Адрес" aside="из названия" preview="tessera-web">
      <input className="kf-input" defaultValue="tessera-web" />
    </FormField>
    <FormField label="Иконка и цвет" aside="фиолетовый">
      <ColorIconPicker name="Tessera Web" icon="code" color="#7c5cff" />
    </FormField>
  </PopoverForm>
)

export const SlugTaken = () => (
  <PopoverForm
    title="Адрес проекта"
    subtitle="Tessera Web"
    primary="Сохранить"
    hint="suggest"
    disabled
  >
    <FormField
      label="Адрес"
      preview="site"
      error="Адрес занят: его использует проект «Сайт»."
      suggestion="site-2"
    >
      <input className="kf-input error" defaultValue="site" />
    </FormField>
  </PopoverForm>
)

export const Busy = () => (
  <PopoverForm
    title="Новая доска"
    subtitle="в проекте «Tessera Web»"
    primary="Создать"
    busyLabel="Создаём…"
    busy
  >
    <FormField label="Название">
      <input className="kf-input" defaultValue="Ретро команды" disabled />
    </FormField>
  </PopoverForm>
)

export const DeleteSpace = () => (
  <PopoverForm
    title="Удалить «Студия Север» навсегда?"
    subtitle="Все проекты, доски и задачи пространства будут удалены. Отменить это нельзя."
    primary="Удалить навсегда"
    hint="активна после ввода имени"
    danger
    width={320}
  >
    <FormField label="Введите название пространства">
      <input className="kf-input" defaultValue="Студия Север" />
    </FormField>
  </PopoverForm>
)
