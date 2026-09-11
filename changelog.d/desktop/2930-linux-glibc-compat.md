---
bump: patch
---
- **fix(desktop): Linux-релиз собираем под старый glibc — работает на Astra Linux SE 1.8
  и других старых дистрибутивах (#2930).** AppImage, собранный на dev-хосте (Ubuntu 24.04
  / glibc 2.39), падал с ошибкой glibc на системах постарше, т.к. glibc обратно-, но не
  прямо-совместим. Теперь `make desktop-release-linux` собирает `.deb` и AppImage в
  контейнере на Ubuntu 22.04 (glibc 2.35 + webkit2gtk-4.1) — бинарь запускается на любом
  glibc ≥ 2.35 (Astra SE 1.8 / Debian 12, Ubuntu 22.04+, RHEL/Alma 9, Fedora). Обычный
  `make desktop-release` (сборка на хосте) остаётся для локальной проверки.
