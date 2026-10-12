import { TesseraProvider, Button, TagPill, UserAvatar, Icon } from '@tessera/design-system'

const Sample = ({ label }) => (
  <div
    style={{
      background: 'var(--t-bg)',
      padding: 14,
      borderRadius: 12,
      display: 'flex',
      flexDirection: 'column',
      gap: 10,
      width: 300,
    }}
  >
    <div
      style={{
        background: 'var(--t-surface)',
        borderRadius: 10,
        padding: 12,
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          color: 'var(--t-text1)',
          fontWeight: 600,
        }}
      >
        <Icon name="AlbumsOutline" gradient size={18} /> {label}
      </div>
      <div style={{ color: 'var(--t-text3)', fontSize: 13 }}>
        Поверхность, текст и акцент берутся из провайдера
      </div>
      <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        <TagPill tag={{ name: 'design', color: '#eb2f96' }} />
        <TagPill tag={{ name: 'app::frontend', color: '#7c5cff' }} />
        <span style={{ flex: 1 }} />
        <UserAvatar name="Мария Соколова" stacked />
        <UserAvatar name="Илья Петров" stacked />
      </div>
      <div style={{ display: 'flex', gap: 8 }}>
        <Button type="primary" size="small">
          Создать
        </Button>
        <Button size="small" quaternary>
          Отмена
        </Button>
      </div>
    </div>
  </div>
)

export const LightAndDark = () => (
  <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
    <TesseraProvider mode="light">
      <Sample label="Светлая тема" />
    </TesseraProvider>
    <TesseraProvider mode="dark">
      <Sample label="Тёмная тема" />
    </TesseraProvider>
  </div>
)

export const AccentSchemes = () => (
  <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
    {['purple', 'blue', 'teal', 'green', 'orange', 'red', 'pink'].map((a) => (
      <TesseraProvider key={a} accent={a}>
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 6,
            width: 96,
          }}
        >
          <Button type="primary" size="small">
            {a}
          </Button>
          <span className="accent-grad-text" style={{ fontWeight: 700, fontSize: 15 }}>
            Tessera
          </span>
        </div>
      </TesseraProvider>
    ))}
  </div>
)
