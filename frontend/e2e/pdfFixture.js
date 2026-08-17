// A minimal, real PDF built byte by byte (#2733).
//
// The PDF viewer is the one part of D8 that unit tests deliberately cannot
// cover: utils/docPdf.js is written free of pdf.js so it stays testable without
// a canvas, which means "pdf.js actually renders a page" is only ever shown in
// a browser. That test needs a file, and the options were a committed binary
// blob or this.
//
// Generating it wins: a checked-in PDF is opaque in review and in diffs, while
// the bytes below are readable and say exactly what the fixture contains — one
// page, one line of text. The xref offsets are computed rather than hard-coded,
// because a stale offset is precisely the kind of corruption pdf.js reports as
// a vague parse failure and a reader would then blame on the viewer.

/**
 * A one-page PDF containing `text`.
 * @param {string} [text] ASCII only — the fixture uses base-14 Helvetica, which
 *   has no Cyrillic glyphs. Rendering Cyrillic is the converter sidecar's job
 *   (it ships fonts for exactly that reason) and is not what this exercises.
 * @returns {Buffer}
 */
export function minimalPdf(text = 'Tessera PDF fixture') {
  const escaped = String(text).replace(/([\\()])/g, '\\$1')
  const stream = `BT /F1 18 Tf 20 120 Td (${escaped}) Tj ET\n`

  const objects = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] ' +
      '/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>',
    `<< /Length ${Buffer.byteLength(stream, 'latin1')} >>\nstream\n${stream}endstream`,
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
  ]

  let pdf = '%PDF-1.4\n'
  const offsets = []
  objects.forEach((body, i) => {
    offsets.push(Buffer.byteLength(pdf, 'latin1'))
    pdf += `${i + 1} 0 obj\n${body}\nendobj\n`
  })

  const xrefAt = Buffer.byteLength(pdf, 'latin1')
  pdf += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`
  for (const off of offsets) pdf += `${String(off).padStart(10, '0')} 00000 n \n`
  pdf += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xrefAt}\n%%EOF\n`

  return Buffer.from(pdf, 'latin1')
}
