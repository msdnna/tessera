import { UndoToast } from '@tessera/design-system'

const Stack = ({ children }) => (
  <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 8 }}>{children}</div>
)

export const States = () => (
  <Stack>
    <UndoToast caption="Доска «Бэклог продукта» удалена" progress={1} />
    <UndoToast caption="Колонка «Бэклог» удалена · 14 задач в «К работе»" progress={0.4} />
    <UndoToast caption="Доска «Ретро» удалена" progress={0.58} paused />
    <UndoToast caption="Доска «Бэклог продукта» удалена" status="busy" />
    <UndoToast caption="Доска «Бэклог продукта» восстановлена" status="done" />
    <UndoToast caption="Не удалось восстановить доску" status="error" />
  </Stack>
)

export const StackNewestBelow = () => (
  <Stack>
    <UndoToast caption="Проект «Сайт» в архиве" icon="archive" progress={0.2} older />
    <UndoToast caption="Колонка «Бэклог» удалена" progress={0.9} />
  </Stack>
)
