// Global vitest setup (#2799, wave 0 of the i18n string extraction).
//
// Why this exists: from wave 1 on, ordinary components start calling `$t`.
// vue-test-utils resolves that off the app instance, so every `mount()` in the
// suite would throw once its component is touched — and the suite mounts
// components from ~40 spec files, most of which never opted into a plugin.
// Registering the real i18n instance once, globally, keeps those specs working
// without each of them having to learn about localisation.
//
// The *real* instance (not a stub `t: (k) => k`) is deliberate: assertions in
// the existing specs check rendered Russian text, and `ru` is the default
// locale, so they keep passing against the same strings the app shows.
import { config } from '@vue/test-utils'
import { beforeEach } from 'vitest'
import { i18n } from '@/i18n'

config.global.plugins = [i18n]

// Specs that switch the language (cx-i18n) must not leak `en` into whatever
// runs next — vitest reuses the module registry across files in a worker.
beforeEach(() => {
  i18n.global.locale.value = 'ru'
})
