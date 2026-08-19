import { describe, expect, it } from 'vitest'
import { normalizeOfficeHtml } from '@/utils/docOfficeHtml'

// The fragments below are copied from the HTML the LibreOffice sidecar produced
// for the document attached to задача 2755 — that is the point of this file:
// the converter's dialect is what the normaliser exists for, and a synthetic
// "well-formed" input would not exercise any of it.

const STYLE_BLOCK = `<style type="text/css">
  @page { size: 8.27in 11.69in; margin-left: 0.79in }
  p { color: #222222; line-height: 115%; text-align: left; orphans: 2 }
  p.western { font-family: "Calibri", serif }
  h1 { color: #1f4e79; page-break-after: avoid }
  h1.western { font-family: "Calibri", serif; font-size: 16pt; font-weight: bold }
  a:link { color: #000080; text-decoration: underline }
</style>`

describe('normalizeOfficeHtml', () => {
  it('leaves HTML that has none of the legacy constructs alone', () => {
    const html = '<p>Обычный абзац</p><h2>Заголовок</h2>'
    expect(normalizeOfficeHtml(html)).toBe(html)
    expect(normalizeOfficeHtml('')).toBe('')
    expect(normalizeOfficeHtml(null)).toBe('')
  })

  it('turns <font> into the inline styles the schema reads', () => {
    const out = normalizeOfficeHtml(
      '<p><font color="#1f4e79"><font size="6" style="font-size: 24pt"><b>Инструкция</b></font></font></p>',
    )
    expect(out).toContain('color: rgb(31, 78, 121)')
    // Points become pixels so one size list can cover the whole document.
    expect(out).toContain('font-size: 32px')
    expect(out).not.toContain('<font')
  })

  it('falls back to the legacy size attribute when there is no font-size', () => {
    expect(normalizeOfficeHtml('<p><font size="5">крупно</font></p>')).toContain('font-size: 24px')
  })

  it('unwraps a <font> that carries nothing worth keeping', () => {
    // 297 of these in the attached document. Left as spans they would each
    // become an attribute-less textStyle mark.
    expect(normalizeOfficeHtml('<p><font face="Calibri, serif">текст</font></p>')).toBe(
      '<p>текст</p>',
    )
  })

  it('resolves the class rules from the <style> block into inline styles', () => {
    const out = normalizeOfficeHtml(
      `${STYLE_BLOCK}<h1 class="western">Требования</h1><p class="western">Абзац</p>`,
    )
    expect(out).toContain('color: rgb(31, 78, 121)')
    expect(out).toContain('font-size: 21px')
    expect(out).not.toContain('<style')
    // The body font is not carried: it is not installed here and it is not a
    // per-paragraph choice, so pinning it would override the sheet everywhere.
    expect(out).not.toContain('Calibri')
    // Print geometry and state selectors are not formatting.
    expect(out).not.toContain('orphans')
    expect(out).not.toContain('page-break')
  })

  it('drops the default ink but keeps a colour the author chose', () => {
    const out = normalizeOfficeHtml(
      `<body text="#222222">${STYLE_BLOCK}<p class="western">Тело</p><h1 class="western">Раздел</h1></body>`,
    )
    // #222222 is Word's default text colour, repeated on every paragraph —
    // keeping it would freeze the document to the light theme.
    expect(out).not.toContain('rgb(34, 34, 34)')
    expect(out).toContain('color: rgb(31, 78, 121)')
  })

  it('drops a black that no theme could use, whatever the body says', () => {
    expect(normalizeOfficeHtml('<p><font color="#000000">чёрный</font></p>')).not.toContain('color')
  })

  it('carries <center> and align= onto the block as text-align', () => {
    const out = normalizeOfficeHtml(
      '<center><table><tr><td><p align="left">Компонент</p></td></tr></table></center>' +
        '<p align="center" style="margin-top: 0.06in"><img src="/api/uploads/a.png"></p>',
    )
    expect(out).not.toContain('<center')
    expect(out).not.toContain('align="center"')
    expect(out).toMatch(/<table[^>]*text-align: center/)
    expect(out).toMatch(/<p[^>]*text-align: center[^>]*><img/)
    expect(out).toContain('text-align: left')
  })

  it('leaves an image’s vertical align= alone instead of centring it', () => {
    const out = normalizeOfficeHtml('<p><img src="/api/uploads/a.png" align="bottom"></p>')
    expect(out).not.toContain('text-align')
  })

  it('turns a bordered paragraph into the rule Word was drawing', () => {
    const out = normalizeOfficeHtml(
      '<p align="center" style="border-top: none; border-bottom: 1.00pt solid #4f81bd">' +
        '<font color="#1f4e79"><b>Инструкция по установке</b></font></p>',
    )
    // The heading keeps its text and gains the rule under it — the whole reason
    // the horizontal lines went missing is that there is no <hr> in the source.
    expect(out).toMatch(/Инструкция по установке[\s\S]*<hr>/)
    expect(out).toContain('<b>')
  })

  it('replaces an empty bordered paragraph with the rule itself', () => {
    const out = normalizeOfficeHtml('<p style="border-bottom: 1.00pt solid #4f81bd"><br></p>')
    expect(out.replace(/\s/g, '')).toBe('<hr>')
  })

  it('ignores a border that draws nothing', () => {
    const out = normalizeOfficeHtml('<p style="border-top: none; border-bottom: 0in">текст</p>')
    expect(out).not.toContain('<hr>')
  })

  it('turns a single-cell monospace table into a code block', () => {
    // How Word writes a code listing, and how the attached document does it.
    const out = normalizeOfficeHtml(
      '<center><table width="624"><col width="608"><tr>' +
        '<td width="608" bgcolor="#f6f8fa" style="background: #f6f8fa"><p style="orphans: 0">' +
        '<font face="Consolas, serif"><font size="2" style="font-size: 9pt">sudo\n' +
        '\t\t\t\tsysctl -w vm.max_map_count=262144</font></font></p></td>' +
        '</tr></table></center>',
    )
    expect(out).toContain('<pre><code>')
    // Source-level wrapping collapses: taking those newlines literally would
    // chop the command in half.
    expect(out).toContain('sudo sysctl -w vm.max_map_count=262144')
    expect(out).not.toContain('<table')
  })

  it('keeps a table that only happens to be one cell wide', () => {
    const out = normalizeOfficeHtml('<table><tr><td><p>Обычный текст</p></td></tr></table>')
    expect(out).toContain('<table')
    expect(out).not.toContain('<pre>')
  })

  it('turns a monospace paragraph into a code block and keeps its lines', () => {
    const out = normalizeOfficeHtml(
      '<p><font face="Consolas">docker compose up -d<br>docker compose ps</font></p>',
    )
    expect(out).toContain('<pre><code>docker compose up -d\ndocker compose ps</code></pre>')
  })

  it('marks a monospace run inside a sentence as inline code', () => {
    const out = normalizeOfficeHtml(
      '<p>Добавьте строку в файл <font face="Consolas">/etc/sysctl.conf</font> и перезагрузите</p>',
    )
    expect(out).toContain('<code>/etc/sysctl.conf</code>')
    expect(out).not.toContain('<pre>')
  })

  it('drops the base size and keeps the sizes that differ from it', () => {
    // The reported symptom: every run carries a size, so the title (24pt) and
    // the headings (16pt/12pt) sorted below body text once sizes were lost.
    const body = Array.from(
      { length: 6 },
      () => '<p><font size="2" style="font-size: 11pt">строка</font></p>',
    ).join('')
    const out = normalizeOfficeHtml(
      `<p><font size="6" style="font-size: 24pt">Заголовок документа</font></p>` +
        body +
        `<p><font size="2" style="font-size: 9pt"><i>Рисунок 1</i></font></p>`,
    )
    expect(out).not.toContain('font-size: 15px')
    expect(out).toContain('font-size: 32px')
    expect(out).toContain('font-size: 12px')
  })

  it('keeps every size when none of them dominates', () => {
    const out = normalizeOfficeHtml(
      '<p><font style="font-size: 9pt">a</font></p><p><font style="font-size: 11pt">b</font></p>' +
        '<p><font style="font-size: 16pt">c</font></p><p><font style="font-size: 24pt">d</font></p>',
    )
    expect(out).toContain('font-size: 12px')
    expect(out).toContain('font-size: 15px')
    expect(out).toContain('font-size: 21px')
    expect(out).toContain('font-size: 32px')
  })
})
