---
bump: patch
---
- **fix(android): отказы по членству в пространстве переведены (#2932).** Четыре ответа
  сервера (`cannot change the workspace owner's role`, `cannot remove the workspace
  owner`, `cannot change your own role`, `cannot remove yourself from the workspace`) и
  два ответа о недостатке ранга (`requires admin or owner`, `requires admin`) проходили
  каталог `util/Errors.kt` насквозь и печатались сырой английской строкой. Добавлены
  сентинелами с текстами в обеих локалях.
