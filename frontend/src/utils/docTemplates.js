import { markdownToDoc } from './docImport'

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

export const BUILTIN_TEMPLATES = [
  {
    key: 'meeting',
    title: 'Протокол совещания',
    description: 'Участники, повестка, решения и задачи по итогам',
    icon: '📋',
    markdown: `# Протокол совещания

**Дата:**
**Участники:**

## Повестка

1.
2.

## Обсуждение

## Решения

- [ ] Решение — ответственный, срок

## Задачи по итогам

- [ ]
`,
  },
  {
    key: 'spec',
    title: 'Техническое задание',
    description: 'Задача, объём работ, критерии приёмки и риски',
    icon: '📐',
    markdown: `# Техническое задание

## Постановка задачи

## Что входит в объём

-

## Что не входит

-

## Критерии приёмки

- [ ]

## Риски и открытые вопросы

| Вопрос | Кто отвечает |
|---|---|
|  |  |
`,
  },
  {
    key: 'retro',
    title: 'Ретроспектива',
    description: 'Что получилось, что мешало, что меняем',
    icon: '🔄',
    markdown: `# Ретроспектива

**Период:**

## Что получилось

-

## Что мешало

-

## Что меняем

- [ ] Действие — ответственный
`,
  },
]

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
  return tpl ? markdownToDoc(tpl.markdown) : null
}

/**
 * Shapes a built-in for the gallery so it renders next to saved templates
 * without the component having to know which is which.
 * @param {object} t entry of BUILTIN_TEMPLATES
 */
export function builtinCard(t) {
  return {
    id: `builtin:${t.key}`,
    builtin: true,
    key: t.key,
    title: t.title,
    description: t.description,
    icon: t.icon,
    preview: '',
  }
}

/** The built-in gallery cards. */
export function builtinCards() {
  return BUILTIN_TEMPLATES.map(builtinCard)
}
