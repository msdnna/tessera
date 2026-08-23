import { markdownToDoc } from './docImport'
import { i18n } from '@/i18n'

// Starter templates that ship with the app (#2734).
//
// They live here rather than as rows seeded into every workspace on migration:
// a seeded row is a copy per workspace that ages independently, cannot be
// improved without a data migration, and shows up in a gallery the team never
// added anything to as "their" templates. As frontend constants they are the
// same for everyone, cost no storage, and a workspace that does not want them
// simply ignores them — nothing to delete.
//
// The bodies are Markdown, converted on use through the same importer that
// reads uploaded .md files. Writing them as ProseMirror JSON would be four
// times the lines and a second thing to keep in step with the schema.
//
// Title, description AND body live in the locale bundles (#2799): a starter
// skeleton is text the app writes for the reader, so an English document should
// not open with Russian headings. Only the key and the emoji stay here.
//
// Watch the pipes in the "spec" body: `|` is vue-i18n's plural separator, so a
// Markdown table written plainly in a message compiles into plural branches and
// t() returns just the first one — an empty line. They are escaped as {'|'} in
// the JSON, which is why the tables there look the way they do.
export const BUILTIN_TEMPLATES = [
  { key: 'meeting', icon: '📋' },
  { key: 'spec', icon: '📐' },
  { key: 'retro', icon: '🔄' },
]

/** The built-in's Markdown body in the current language. */
function templateMarkdown(key) {
  return i18n.global.t(`documents.templates.${key}.body`)
}

/**
 * Builds the body of a built-in template.
 *
 * Conversion is done on use rather than at module load: the gallery only shows
 * titles, and parsing four documents through a detached editor to render a list
 * of names would be work nobody asked for.
 *
 * @param {string} key one of BUILTIN_TEMPLATES[].key
 * @returns {object|null} ProseMirror document JSON, or null for an unknown key
 */
export function builtinContent(key) {
  const tpl = BUILTIN_TEMPLATES.find((t) => t.key === key)
  return tpl ? markdownToDoc(templateMarkdown(tpl.key)) : null
}

/**
 * Shapes a built-in for the gallery so it renders next to saved templates
 * without the component having to know which is which.
 * @param {object} tpl entry of BUILTIN_TEMPLATES
 */
export function builtinCard(tpl) {
  return {
    id: `builtin:${tpl.key}`,
    builtin: true,
    key: tpl.key,
    title: i18n.global.t(`documents.templates.${tpl.key}.title`),
    description: i18n.global.t(`documents.templates.${tpl.key}.description`),
    icon: tpl.icon,
    preview: '',
  }
}

/** The built-in gallery cards. */
export function builtinCards() {
  return BUILTIN_TEMPLATES.map(builtinCard)
}
