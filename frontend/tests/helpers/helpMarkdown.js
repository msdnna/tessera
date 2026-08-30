// Shared by the two help-content guards (#2823): both scan articles for image
// links, and both used to scan the code samples too.
//
// That was fine while no article had a reason to *print* Markdown, and stopped
// being fine with the article about the Markdown editor — it has to show the
// reader what an image insert looks like, and `![имя](адрес)` read as a link to
// a screenshot named «адрес» that was never committed. Stripping fenced blocks
// and inline code first keeps the guard on real links: a sample inside backticks
// is never one.
export function withoutCode(md) {
  return md.replace(/```[\s\S]*?```/g, '').replace(/`[^`\n]*`/g, '')
}
