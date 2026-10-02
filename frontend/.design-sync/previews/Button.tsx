import { Button } from '@tessera/design-system'

const Row = ({ children }) => (
  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>{children}</div>
)

export const Types = () => (
  <Row>
    <Button type="primary">Создать задачу</Button>
    <Button>Отмена</Button>
    <Button tertiary>Фильтр</Button>
    <Button quaternary>Ещё</Button>
    <Button type="error" ghost>
      Удалить
    </Button>
  </Row>
)

export const Sizes = () => (
  <Row>
    <Button type="primary" size="tiny">
      Tiny
    </Button>
    <Button type="primary" size="small">
      Small
    </Button>
    <Button type="primary">Medium</Button>
    <Button type="primary" size="large">
      Large
    </Button>
  </Row>
)

export const States = () => (
  <Row>
    <Button type="primary" loading>
      Сохранение
    </Button>
    <Button type="primary" disabled>
      Недоступно
    </Button>
    <Button dashed>+ Добавить колонку</Button>
  </Row>
)
