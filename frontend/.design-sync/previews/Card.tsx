import { Card, Button, Icon, Switch } from '@tessera/design-system'

export const Settings = () => (
  <div style={{ width: 420 }}>
    <Card title="Уведомления" headerExtra={<Icon name="NotificationsOutline" gradient size={18} />}>
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: 10,
          color: 'var(--t-text2)',
          fontSize: 14,
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          Напоминания о сроках <Switch defaultValue />
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          Упоминания в комментариях <Switch defaultValue />
        </div>
      </div>
    </Card>
  </div>
)
export const WithFooter = () => (
  <div style={{ width: 420 }}>
    <Card
      title="Новый проект"
      footer={
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <Button quaternary>Отмена</Button>
          <Button type="primary">Создать</Button>
        </div>
      }
    >
      <span style={{ color: 'var(--t-text3)', fontSize: 13 }}>
        Проект объединяет доски, задачи и заметки внутри группы.
      </span>
    </Card>
  </div>
)
