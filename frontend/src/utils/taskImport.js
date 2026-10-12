// Task import (#3068, #3095) — the client's read of the file for the mapping
// screen and the preview. The server parses the file again on upload and is
// the authority (handlers/board_import_parse.go); the two agree on the format:
// CSV with a header row (comma, semicolon or tab), or JSON — an array of
// objects or `{ tasks: [...] }`, the shape our own export writes.

// The task fields a column can be mapped onto, in screen order.
export const IMPORT_FIELDS = [
  'title',
  'description',
  'column',
  'tags',
  'assignees',
  'priority',
  'start_date',
  'due_date',
]

// The preview shows this many rows.
export const PREVIEW_ROWS = 8

// Header names recognised per field (lower-case), for the automatic mapping.
const SYNONYMS = {
  title: ['title', 'name', 'summary', 'task', 'заголовок', 'название', 'задача', 'тема'],
  description: ['description', 'body', 'details', 'notes', 'описание', 'текст', 'подробности'],
  column: ['column', 'status', 'state', 'stage', 'list', 'колонка', 'статус', 'состояние'],
  tags: ['tags', 'labels', 'tag', 'label', 'теги', 'метки', 'тег', 'метка'],
  assignees: [
    'assignees',
    'assignee',
    'owner',
    'responsible',
    'исполнители',
    'исполнитель',
    'ответственный',
  ],
  priority: ['priority', 'importance', 'приоритет', 'важность'],
  start_date: ['start_date', 'start', 'start date', 'начало', 'дата начала', 'старт'],
  due_date: ['due_date', 'due', 'due date', 'deadline', 'срок', 'дедлайн', 'дата окончания'],
}

// sniffDelimiter picks the separator of the header line: the most frequent of
// comma, semicolon and tab outside quotes.
export function sniffDelimiter(text) {
  const nl = text.indexOf('\n')
  const line = nl < 0 ? text : text.slice(0, nl)
  const counts = { ',': 0, ';': 0, '\t': 0 }
  let quoted = false
  for (const ch of line) {
    if (ch === '"') quoted = !quoted
    else if (!quoted && ch in counts) counts[ch]++
  }
  let best = ','
  for (const d of [';', '\t']) if (counts[d] > counts[best]) best = d
  return best
}

// parseCSV reads RFC 4180 CSV into rows of cells (quoted fields, doubled
// quotes, newlines inside quotes).
export function parseCSV(text, delim = sniffDelimiter(text)) {
  const rows = []
  let row = []
  let cell = ''
  let quoted = false
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (quoted) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          cell += '"'
          i++
        } else quoted = false
      } else cell += ch
      continue
    }
    if (ch === '"' && cell === '') quoted = true
    else if (ch === delim) {
      row.push(cell)
      cell = ''
    } else if (ch === '\n' || ch === '\r') {
      if (ch === '\r' && text[i + 1] === '\n') i++
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
    } else cell += ch
  }
  if (cell !== '' || row.length) {
    row.push(cell)
    rows.push(row)
  }
  return rows
}

// uniqueColumns names blank and repeated header cells the way the server does.
function uniqueColumns(header) {
  const seen = {}
  return header.map((h, i) => {
    let name = String(h || '').trim() || `column ${i + 1}`
    if (seen[name]) {
      seen[name]++
      name = `${name} (${seen[name]})`
    } else seen[name] = 1
    return name
  })
}

function jsonCell(v) {
  if (v == null) return ''
  if (Array.isArray(v))
    return v
      .map(jsonCell)
      .filter((x) => x !== '')
      .join(', ')
  if (typeof v === 'object') return null
  return String(v).trim()
}

// parseImportFile reads the file's text into `{ columns, rows }`, each row a
// column → cell object. Throws an Error with a `code` the screen translates.
export function parseImportFile(text, name = '') {
  const src = String(text || '').replace(/^\uFEFF/, '')
  if (!src.trim()) throw Object.assign(new Error('empty'), { code: 'empty' })
  const isJson = /\.json$/i.test(name) || /^\s*[[{]/.test(src)
  if (isJson) {
    let data
    try {
      data = JSON.parse(src)
    } catch {
      throw Object.assign(new Error('json'), { code: 'json' })
    }
    const items = Array.isArray(data) ? data : Array.isArray(data?.tasks) ? data.tasks : null
    if (!items) throw Object.assign(new Error('json'), { code: 'jsonShape' })
    const columns = []
    const rows = items.map((it) => {
      const row = {}
      for (const k of Object.keys(it || {}).sort()) {
        const v = jsonCell(it[k])
        if (v == null) continue
        if (!columns.includes(k)) columns.push(k)
        row[k] = v
      }
      return row
    })
    return { columns, rows }
  }
  const table = parseCSV(src)
  if (!table.length) throw Object.assign(new Error('empty'), { code: 'empty' })
  const columns = uniqueColumns(table[0])
  const rows = []
  for (const rec of table.slice(1)) {
    if (rec.every((c) => !String(c).trim())) continue
    const row = {}
    columns.forEach((c, i) => (row[c] = String(rec[i] ?? '').trim()))
    rows.push(row)
  }
  return { columns, rows }
}

// autoMapping maps every field onto the first column whose header is one of
// its synonyms (or the field's own key). A column is used once.
export function autoMapping(columns) {
  const out = {}
  const used = new Set()
  for (const field of IMPORT_FIELDS) {
    const names = [field, ...SYNONYMS[field]]
    const hit = columns.find((c) => !used.has(c) && names.includes(c.trim().toLowerCase()))
    if (hit) {
      out[field] = hit
      used.add(hit)
    }
  }
  return out
}

// The priorities the server understands (parseImportPriority): our export
// names, Russian words, 0–4. Returns the level or null for an unknown value;
// '' is "not set".
const PRIORITY_WORDS = {
  none: 0,
  нет: 0,
  low: 1,
  низкий: 1,
  normal: 2,
  medium: 2,
  средний: 2,
  обычный: 2,
  high: 3,
  высокий: 3,
  urgent: 4,
  critical: 4,
  срочный: 4,
  критический: 4,
}
export function parsePriority(s) {
  const v = String(s || '')
    .trim()
    .toLowerCase()
  if (!v) return 0
  if (v in PRIORITY_WORDS) return PRIORITY_WORDS[v]
  if (/^[0-4]$/.test(v)) return Number(v)
  return null
}

// parseDate mirrors the server: ISO (date or date-time) or ДД.ММ.ГГГГ. Returns
// an ISO date string for the preview, null for an unknown value, '' for empty.
export function parseDate(s) {
  const v = String(s || '').trim()
  if (!v) return ''
  let m = v.match(/^(\d{4})-(\d{2})-(\d{2})/)
  if (m) return `${m[1]}-${m[2]}-${m[3]}`
  m = v.match(/^(\d{1,2})[./](\d{1,2})[./](\d{4})$/)
  if (m) return `${m[3]}-${m[2].padStart(2, '0')}-${m[1].padStart(2, '0')}`
  return null
}

// splitList splits a tags / assignees cell like the server does.
export function splitList(s) {
  const seen = new Set()
  const out = []
  for (const p of String(s || '').split(/[,;\n]/)) {
    const v = p.trim()
    if (v && !seen.has(v.toLowerCase())) {
      seen.add(v.toLowerCase())
      out.push(v)
    }
  }
  return out
}

// previewRows reads the first rows through the mapping the way the import will:
// each field's value plus the problems the server would warn about.
// ctx: { columns: [{ name }], members: [{ email, name }] }.
export function previewRows(rows, mapping, ctx = {}, limit = PREVIEW_ROWS) {
  const colNames = new Set((ctx.columns || []).map((c) => String(c.name).trim().toLowerCase()))
  const people = new Set()
  for (const m of ctx.members || []) {
    if (m.email) people.add(m.email.toLowerCase())
    if (m.name) people.add(m.name.trim().toLowerCase())
  }
  const cell = (row, f) => (mapping[f] ? String(row[mapping[f]] ?? '').trim() : '')
  return rows.slice(0, limit).map((row) => {
    const warn = []
    const title = cell(row, 'title')
    if (!title) warn.push('title')
    const column = cell(row, 'column')
    if (column && colNames.size && !colNames.has(column.toLowerCase())) warn.push('column')
    const priority = parsePriority(cell(row, 'priority'))
    if (priority == null) warn.push('priority')
    const due = parseDate(cell(row, 'due_date'))
    const start = parseDate(cell(row, 'start_date'))
    if (due == null || start == null) warn.push('date')
    const assignees = splitList(cell(row, 'assignees'))
    if (people.size && assignees.some((a) => !people.has(a.toLowerCase()))) warn.push('assignee')
    return {
      title,
      description: cell(row, 'description'),
      column,
      tags: splitList(cell(row, 'tags')),
      assignees,
      priority,
      start_date: start,
      due_date: due,
      warn,
    }
  })
}
