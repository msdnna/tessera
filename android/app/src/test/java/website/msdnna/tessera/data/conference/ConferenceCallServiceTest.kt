package website.msdnna.tessera.data.conference

import android.Manifest
import android.app.Application
import android.content.pm.ServiceInfo
import androidx.test.core.app.ApplicationProvider
import com.google.common.truth.Truth.assertThat
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config

/**
 * The call's foreground service, at the one moment it can take the app down
 * (#2896).
 *
 * From Android 14 a foreground service typed `camera` is refused — loudly, out of
 * `startForeground` on the main thread — to an app without the camera runtime
 * permission, and the same for `microphone` without `RECORD_AUDIO`. The room
 * screen connects *first* and asks for the microphone second, and the camera is
 * only ever asked for from the toolbar, so the service reliably starts holding
 * neither. Declaring both regardless is why the app died the instant somebody
 * tapped «Войти».
 *
 * Robolectric does not enforce the platform check, so these specs assert the two
 * things that decide it: the type handed to `startForeground`, and whether the
 * service is started at all.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class ConferenceCallServiceTest {
    private val app: Application get() = ApplicationProvider.getApplicationContext()

    @Before
    fun denyEverything() {
        shadowOf(app).denyPermissions(Manifest.permission.RECORD_AUDIO, Manifest.permission.CAMERA)
    }

    @Test
    fun `the type covers only what the phone granted`() {
        shadowOf(app).grantPermissions(Manifest.permission.RECORD_AUDIO)

        // Not `microphone or camera`: the camera permission has not been asked
        // for yet at this point in a call, and naming it is the crash.
        assertThat(ConferenceCallService.grantedTypes(app))
            .isEqualTo(ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE)
    }

    @Test
    fun `granting the camera later widens the type`() {
        shadowOf(app).grantPermissions(Manifest.permission.RECORD_AUDIO, Manifest.permission.CAMERA)

        assertThat(ConferenceCallService.grantedTypes(app)).isEqualTo(
            ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE or ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA,
        )
    }

    @Test
    fun `a call with no media permission does not start the service`() {
        ConferenceCallService.apply(app, needed = true)

        // The call still connects — a meeting you can only listen to is a
        // meeting. What it does not get is a service the platform would refuse.
        assertThat(shadowOf(app).nextStartedService).isNull()
    }

    @Test
    fun `the microphone alone is enough to start it`() {
        shadowOf(app).grantPermissions(Manifest.permission.RECORD_AUDIO)

        ConferenceCallService.apply(app, needed = true)

        val started = shadowOf(app).nextStartedService
        assertThat(started).isNotNull()
        assertThat(started!!.component?.className).isEqualTo(ConferenceCallService::class.java.name)
    }

    @Test
    fun `a service started without a permission goes away instead of going foreground`() {
        val service = Robolectric.buildService(ConferenceCallService::class.java).create().get()

        service.onStartCommand(null, 0, 1)

        // Neither half is optional: staying up without calling startForeground is
        // the platform killing the process a few seconds later.
        assertThat(shadowOf(service).lastForegroundNotification).isNull()
        assertThat(shadowOf(service).isStoppedBySelf).isTrue()
    }

    @Test
    fun `with the microphone granted it does go foreground`() {
        shadowOf(app).grantPermissions(Manifest.permission.RECORD_AUDIO)
        val service = Robolectric.buildService(ConferenceCallService::class.java).create().get()

        service.onStartCommand(null, 0, 1)

        assertThat(shadowOf(service).lastForegroundNotification).isNotNull()
        assertThat(shadowOf(service).isStoppedBySelf).isFalse()
    }
}
