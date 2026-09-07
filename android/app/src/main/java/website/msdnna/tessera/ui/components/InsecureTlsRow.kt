package website.msdnna.tessera.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import website.msdnna.tessera.R
import website.msdnna.tessera.ui.TestTags

/**
 * Переключатель «не проверять сертификат сервера» (#2896).
 *
 * Живёт в двух местах и потому здесь, а не в каждом экране: на экране входа
 * (сертификат, которому телефон не верит, не пускает дальше логина) и в
 * настройках (SFU конференции берёт адрес у сервера, так что упереться в
 * недоверенную цепочку можно уже после входа).
 *
 * Подпись прямо называет цену — «соединение можно подменить»: тумблер, который
 * чинит вход одним нажатием, иначе включают не читая и оставляют навсегда.
 */
@Composable
fun InsecureTlsRow(
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    labelColor: Color,
    hintColor: Color,
    modifier: Modifier = Modifier,
) {
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Row(
            Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                stringResource(R.string.tls_insecure_label),
                color = labelColor,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(8.dp))
            TSwitch(
                checked = checked,
                onCheckedChange = onCheckedChange,
                modifier = Modifier.testTag(TestTags.TLS_INSECURE_SWITCH),
            )
        }
        Text(
            stringResource(if (checked) R.string.tls_insecure_hint_on else R.string.tls_insecure_hint_off),
            color = hintColor,
            fontSize = 11.sp,
        )
    }
}
