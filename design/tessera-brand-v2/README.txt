tessera — фирменные ассеты (v2)
================================================================

Логотип: строчная «t» + плитка-тессера в углу (без монограммы «mt»).
Надпись: полное написание «tessera» — всегда строчными, заглавная «T» не используется.
Шрифт:  Fredoka 600 — и знак «t», и надпись. Один вес во всех ассетах.
        Контуры переведены в кривые (path) — шрифт для отрисовки НЕ нужен.
Цвет:   диагональный градиент (низ-лево → верх-право)
        #6D5FE0 (0%) → #7C6CFF (50%, основной тон) → #9183FF (100%)
        Плоский основной: #7C6CFF · Чёрный: #1A1722
        Тёмно-фиолетовый (инверс на фоне): #4B3BB3 · Тень: #180B46
Скругление иконки: 23.5 % от стороны.


СТРУКТУРА
----------------------------------------------------------------
svg/                              векторы (масштабируются без потерь)
  icon.svg                        иконка: градиент + белый знак + мягкая тень
  icon-on-white.svg               белая плитка + градиентный знак
  mark-gradient.svg               только знак «t», градиент (для светлого фона)
  mark-white.svg                  только знак, белый (для тёмного фона)
  mark-purple.svg                 только знак, плоский #7C6CFF
  mark-black.svg                  только знак, #1A1722 (монохром)
  wordmark.svg                    надпись «tessera», #251F42
  wordmark-gradient.svg           надпись, градиент
  wordmark-white.svg              надпись, белая (для тёмного фона)
  wordmark-deep.svg               надпись, #4B3BB3 (для фиолетового фона)
  logo-horizontal.svg             знак + tessera (на светлом)
  logo-horizontal-on-purple.svg   то же на фиолетовом
  logo-vertical.svg               вертикальная компоновка
  loader-tessera-white.svg        плитка лоадера (бел.) — крутить через CSS/Lottie
  loader-tessera-purple.svg       плитка лоадера (фиол.)

loader/                           анимированный лоадер (референс для реализации)
  loader.html                     живая реализация — открыть в браузере
  tessera-loader.js               движок без зависимостей: mountTesseraLoader(el, opts)
  loader-states.json              геометрия всех кадров + тайминги (источник истины)
  README.md                       спецификация: модель, состояния, тайминги, web + Android

png/                              растровые превью
  icon-1024.png  icon-512.png  icon-192.png

android/                          готово к копированию в app/src/main/res/
  mipmap-mdpi/ic_launcher.png         48×48
  mipmap-hdpi/ic_launcher.png         72×72
  mipmap-xhdpi/ic_launcher.png        96×96
  mipmap-xxhdpi/ic_launcher.png       144×144
  mipmap-xxxhdpi/ic_launcher.png      192×192
  mipmap-anydpi-v26/ic_launcher.xml         адаптивная иконка (API 26+)
  mipmap-anydpi-v26/ic_launcher_round.xml   круглая адаптивная иконка
  drawable/ic_launcher_background.xml       фон адаптивной иконки (градиент)
  drawable/ic_launcher_foreground.xml       передний слой (белый знак «t», + monochrome для тем Android 13)
  drawable/ic_notification.xml              иконка уведомления (белый силуэт, 24dp, тинтуется системой)
  drawable/ic_favicon.xml                   цветной знак-favicon (48dp, градиент + белый знак)
  drawable/ic_wordmark.xml                  надпись «tessera» в кривых (для сплеша / «о приложении»)
  drawable-mdpi…xxxhdpi/ic_stat_tessera.png растровые иконки уведомления (24/36/48/72/96, белый силуэт)
  ic_launcher-512.png                       для Google Play Store

preview.html                      визуальная сводка всех ассетов


КАК ПОДКЛЮЧИТЬ (Android)
----------------------------------------------------------------
1. Скопируйте содержимое android/ в app/src/main/res/, сохранив имена папок.
2. AndroidManifest.xml:
     <application
        android:icon="@mipmap/ic_launcher"
        android:roundIcon="@mipmap/ic_launcher_round" ... >
3. Уведомления: setSmallIcon(R.drawable.ic_notification).
   Силуэт уже белый и прозрачный — Android сам красит его в нужный цвет.
   (Альтернатива растром: drawable-*/ic_stat_tessera.png.)
4. Favicon в вебе: используйте svg/icon.svg или png/icon-512.png
   (<link rel="icon" type="image/svg+xml" href="icon.svg">).

ЛОАДЕР
----------------------------------------------------------------
Полный анимированный лоадер (t → тессера закрывает t → крутится и разворачивается
в виды: канбан, список, таймлайн, гант, матрица) живёт в «Tessera Logo v2.html» (файл проекта).
Здесь — статичная плитка-тессера для простого CSS/Lottie-вращения:
  @keyframes spin { to { transform: rotate(360deg); } }
  .loader { animation: spin 1.5s cubic-bezier(.68,0,.32,1) infinite; }
Сплеш — белая плитка на фиолетовом; загрузка экранов — фиолетовая на белом.
