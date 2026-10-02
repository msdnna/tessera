import { BrandLogo, TesseraProvider } from '@tessera/design-system'

export const Lockup = () => <BrandLogo height={32} />
export const MarkOnly = () => <BrandLogo height={36} wordmark={false} />
export const WordmarkOnly = () => <BrandLogo height={26} mark={false} />
export const AccentFollows = () => (
  <div style={{ display: 'flex', gap: 24 }}>
    <TesseraProvider accent="teal">
      <BrandLogo height={26} />
    </TesseraProvider>
    <TesseraProvider accent="pink">
      <BrandLogo height={26} />
    </TesseraProvider>
  </div>
)
