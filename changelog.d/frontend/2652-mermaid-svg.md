---
bump: patch
---
- **security(web): mermaid-диаграммы проходят через DOMPurify перед вставкой в DOM (#2652).**
  Результат `mermaid.render()` попадал в `innerHTML` в обход общей санитизации —
  единственное место, где отображаемый контент шёл мимо `renderRich()`. Mermaid
  инициализирован с `securityLevel:'strict'`, так что активной дыры не было, но
  defence-in-depth восстановлен: новый `sanitizeSvgFragment()` чистит SVG тем же
  DOMPurify и отдаёт уже разобранные узлы, а не строку — без повторного парсинга,
  на котором и строятся mXSS-обходы. Подписи узлов (`<foreignObject>`) и инлайновый
  `<style>` диаграммы сохраняются.
