// «Цитировать в ответ» (#3118, PackageJ «Пузырь в тексте только для чтения»):
// the selection bubble over a read description or comment hands the picked text
// to the comment composer of the same task. The two never hold a reference to
// each other (the description and the thread are separate tabs of the modal),
// so the request travels through this one ref, which the composer watches.
import { shallowRef } from 'vue'

export const quoteRequest = shallowRef(null)

// quoteMarkdown turns picked text into a Markdown quote: every line behind «> »,
// blank lines kept as bare «>» so the quote stays one block, and a blank line
// after it for the answer.
export function quoteMarkdown(text) {
  const body = String(text || '')
    .replace(/\r\n?/g, '\n')
    .trim()
  if (!body) return ''
  return `${body
    .split('\n')
    .map((l) => (l.trim() ? `> ${l}` : '>'))
    .join('\n')}\n\n`
}

export function requestQuote(taskId, text) {
  const md = quoteMarkdown(text)
  if (!md) return false
  quoteRequest.value = { taskId, text: md, at: Date.now() }
  return true
}
