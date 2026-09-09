package website.msdnna.tessera.e2e

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import website.msdnna.tessera.ui.AppRoot
import website.msdnna.tessera.ui.TestTags

/**
 * История изменений по тапу на версию в футере сайдбара (#2858) и блок «какой
 * билд запущен» внутри неё (#2859).
 *
 * Единственный спек, который идёт через настоящий шелл и настоящий drawer: путь
 * тут и есть предмет проверки. Строка версии лежит **внутри** блока пользователя,
 * у которого свой клик («открыть настройки»), и юнит-тесты этого не видят —
 * вложенный кликабельный узел легко проглатывается внешним, и тогда тап молча
 * открывает не то. Заодно это доказывает, что тег и текст остались на одном узле:
 * `clickable` сливает свою семантику, и тег на родителе оказался бы на узле без
 * подписи (та же грабля, что ловил счётчик #2853).
 *
 * Экран пинится большим намеренно: дефолтные для Robolectric 320x470 обрезают
 * низ drawer'а, и тап по футеру терялся бы за кромкой без единой ошибки.
 */
@RunWith(RobolectricTestRunner::class)
// The «+» is load-bearing: without it these qualifiers *replace* the tier's set
// (`e2e/robolectric.properties`) instead of adding to it, so the class would
// quietly lose the `ru` locale pin and `xhdpi` along with the screen size it
// actually wanted. It also costs a third Robolectric sandbox in the tier.
@Config(qualifiers = "+w800dp-h1200dp")
class ChangelogHistoryE2eTest {
    private val e2e = E2eRule()
    private val compose = createComposeRule()

    @get:Rule
    val rules: RuleChain = RuleChain.outerRule(e2e).around(compose)

    @Test
    fun `tapping the version in the sidebar footer opens the full changelog`() {
        compose.setContent { AppRoot() }
        // `awaitShell`, not `awaitTag`: this is the tier's only spec that boots the
        // real gate with a live session, so its first frame waits on a network call
        // and a stalled connect leaves a retry screen that never resolves itself.
        compose.awaitShell()

        compose.onNodeWithTag(TestTags.TOP_MENU).performClick()
        compose.awaitTag(TestTags.SIDEBAR_VERSION)

        compose.onNodeWithTag(TestTags.SIDEBAR_VERSION).performClick()

        // Карточка сама по себе доказывала бы мало: её же рисует и обычное «Что
        // нового». Блок сборки есть ТОЛЬКО в режиме истории — по нему и опознаём,
        // что открылась именно она.
        compose.awaitTag(TestTags.WHATS_NEW_CARD)
        compose.awaitTag(TestTags.WHATS_NEW_BUILD)

        // Закрытие истории ничего не подтверждает — карточка просто уходит.
        compose.onNodeWithTag(TestTags.WHATS_NEW_DISMISS).performClick()
        compose.awaitNoTag(TestTags.WHATS_NEW_CARD)
    }
}
