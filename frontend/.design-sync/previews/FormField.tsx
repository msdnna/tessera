import { FormField } from '@tessera/design-system'

// A field outside its PopoverForm, on the form's own surface and width.
const Surface = ({ children }) => (
  <div
    style={{
      width: 316,
      padding: 16,
      borderRadius: 16,
      background: 'var(--t-pop)',
      display: 'flex',
      flexDirection: 'column',
      gap: 14,
    }}
  >
    {children}
  </div>
)

export const WithPreview = () => (
  <Surface>
    <FormField label="Адрес" aside="из названия" preview="tessera-web">
      <input className="kf-input" defaultValue="tessera-web" />
    </FormField>
  </Surface>
)

export const ErrorWithSuggestion = () => (
  <Surface>
    <FormField
      label="Адрес"
      preview="site"
      error="Адрес занят: его использует проект «Сайт»."
      suggestion="site-2"
    >
      <input className="kf-input error" defaultValue="site" />
    </FormField>
  </Surface>
)

export const WithMessage = () => (
  <Surface>
    <FormField
      label="Хранить записи, дней"
      message="Записи этой конференции удалятся автоматически через столько дней. 0 — хранить без срока."
    >
      <input className="kf-input" type="number" defaultValue={30} />
    </FormField>
  </Surface>
)
