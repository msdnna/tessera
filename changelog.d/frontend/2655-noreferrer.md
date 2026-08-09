---
bump: patch
---
- **fix(web): rel="noreferrer" на внешних ссылках с target="_blank" (#2655).**
  GitLab-ссылки (карточка задачи, модалка, этапы, журнал синхронизации) открывались
  в новой вкладке с `rel="noopener"`, но без `noreferrer` — заголовок Referer
  утекал на сторонний origin. Добавлено во всех 5 местах `target="_blank"`.
