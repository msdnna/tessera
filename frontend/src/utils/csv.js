// CSV for spreadsheets (#3080): RFC 4180 quoting, CRLF rows and a UTF-8 BOM, so
// Excel opens Cyrillic names correctly on double-click. Cells that a spreadsheet
// would run as a formula (= + - @ and tab/CR at the start) get a leading
// apostrophe — an exported name must not become a command (CSV injection).

const FORMULA = /^[=+\-@\t\r]/

export function csvCell(v) {
  if (v === null || v === undefined) return ''
  let s = String(v)
  if (FORMULA.test(s)) s = `'${s}`
  return /[",\r\n;]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

// toCSV turns rows into text by columns: [{ label, value: (row) => any }].
export function toCSV(rows, columns) {
  const lines = [columns.map((c) => csvCell(c.label)).join(',')]
  for (const r of rows) lines.push(columns.map((c) => csvCell(c.value(r))).join(','))
  return '﻿' + lines.join('\r\n') + '\r\n'
}
