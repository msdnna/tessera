// renderRich runs two decorations over the same content — the mention chip and the
// «#N» task link — and the second one reads what the first one wrote. Chained inside a
// single walk, linkTaskRefs treated the freshly spliced <span> as prose: a member named
// «Отдел #5» ended up with an <a> inside its own data-label, which closes the attribute
// at the chip's first quote. These pin the order and the skip that keep the two apart
// (#2932); ut-md-decorations-inside-tags.spec.js covers the same breakout one layer out,
// through an image's alt and a link's title.
import { describe, it, expect } from 'vitest'
import { renderRich } from '@/utils/markdown'

const DEPT = [{ id: 'u1', label: 'Отдел #5', display: 'Отдел #5' }]
const IVAN = [{ id: 'u2', label: 'ivan', display: 'Иван Петров' }]

function parse(html) {
  return new DOMParser().parseFromString(html, 'text/html')
}

describe('renderRich: one decoration must not read the other as text', () => {
  it('keeps a chip whose name carries a «#N» intact', () => {
    const doc = parse(renderRich('Привет @Отдел #5, глянь', DEPT, { taskRefs: true }))
    const chip = doc.querySelector('span.mention')
    expect(chip).toBeTruthy()
    // The label is what the hover card resolves the chip by; a breakout truncates it
    // at «Отдел <a class=» and hands the rest to the parser as attributes.
    expect(chip.getAttribute('data-label')).toBe('Отдел #5')
    expect(chip.getAttribute('data-id')).toBe('u1')
    expect(chip.hasAttribute('data-task-ref')).toBe(false)
    expect(chip.hasAttribute('href')).toBe(false)
    expect(chip.textContent).toBe('@Отдел #5')
  })

  it('draws no leftover markup as visible text', () => {
    const text = parse(renderRich('Привет @Отдел #5, глянь', DEPT, { taskRefs: true })).body
      .textContent
    expect(text).toBe('Привет @Отдел #5, глянь\n')
  })

  it('renders the chip the same way with and without the task-ref pass', () => {
    const src = 'Привет @Отдел #5, глянь'
    expect(renderRich(src, DEPT, { taskRefs: true })).toBe(renderRich(src, DEPT))
  })

  it('does not link a «#N» that belongs to a chip', () => {
    const doc = parse(renderRich('@Отдел #5', DEPT, { taskRefs: true }))
    expect(doc.querySelector('span.mention a')).toBeNull()
  })

  it('still links a «#N» that follows a chip', () => {
    const doc = parse(renderRich('@ivan #12 глянь', IVAN, { taskRefs: true }))
    expect(doc.querySelector('span.mention').getAttribute('data-id')).toBe('u2')
    const ref = doc.querySelector('a.task-ref')
    expect(ref.getAttribute('data-task-ref')).toBe('12')
    expect(ref.textContent).toBe('#12')
  })

  it('leaves a stored legacy chip and its «#N» alone', () => {
    const stored =
      '<p><span data-type="mention" data-id="u1" data-label="Отдел #5">@Отдел #5</span></p>'
    const doc = parse(renderRich(stored, DEPT, { taskRefs: true }))
    const chip = doc.querySelector('span[data-type="mention"]')
    expect(chip.getAttribute('data-label')).toBe('Отдел #5')
    expect(doc.querySelector('a.task-ref')).toBeNull()
  })

  it('still steps over a code sample', () => {
    const doc = parse(renderRich('глянь `#2550` и @ivan', IVAN, { taskRefs: true }))
    expect(doc.querySelector('code').textContent).toBe('#2550')
    expect(doc.querySelector('a.task-ref')).toBeNull()
    expect(doc.querySelector('span.mention').getAttribute('data-id')).toBe('u2')
  })
})
