# Хранилище самообновления Android

Релизная сборка (`tools/build-android-release.sh`) кладёт сюда подписанный APK и
манифест `latest.json`; nginx фронтенда раздаёт их по пути `/apks/`, а
Android-приложение самообновляется с `<server>/apks/latest.json`. APK в gitignore;
каталог оставлен, чтобы у сборочного скрипта и bind-mount nginx была цель.
