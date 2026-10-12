import { Modal, Button, Input, Select } from '@tessera/design-system'

export const CreateTask = () => (
  <Modal
    defaultShow
    title="Новая задача"
    width={520}
    footer={
      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
        <Button quaternary>Отмена</Button>
        <Button type="primary">Создать</Button>
      </div>
    }
  >
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <Input placeholder="Название задачи" defaultValue="Редизайн доски" />
      <Select
        options={[
          { label: 'Высокий', value: 3 },
          { label: 'Обычный', value: 2 },
        ]}
        defaultValue={3}
      />
      <Input type="textarea" placeholder="Описание" autosize={{ minRows: 3 }} />
    </div>
  </Modal>
)
