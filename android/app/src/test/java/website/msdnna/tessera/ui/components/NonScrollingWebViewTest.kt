package website.msdnna.tessera.ui.components

import android.view.View
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/**
 * The guard layer of #2781. Robolectric doesn't render a document inside a
 * WebView, so the height-reporting fix itself can't be asserted here — but the
 * view-level scroll pinning is plain [View] machinery and is exactly what keeps
 * a late height report from clipping text mid-line.
 */
@RunWith(RobolectricTestRunner::class)
class NonScrollingWebViewTest {
    private fun view() = NonScrollingWebView(ApplicationProvider.getApplicationContext())

    @Test
    fun `scrolling the view vertically leaves the content pinned`() {
        val web = view()
        web.scrollTo(0, 400)
        assertThat(web.scrollY).isEqualTo(0)
    }

    @Test
    fun `horizontal scrolling still works`() {
        val web = view()
        web.scrollTo(120, 400)
        assertThat(web.scrollX).isEqualTo(120)
        assertThat(web.scrollY).isEqualTo(0)
    }

    @Test
    fun `over-scroll is off so no glow or drag steal on the edges`() {
        assertThat(view().overScrollMode).isEqualTo(View.OVER_SCROLL_NEVER)
    }
}
