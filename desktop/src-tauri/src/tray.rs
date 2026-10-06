// Tray menu wording (#2932).
//
// The tray is the only interface this crate draws itself. It exists before the
// webview has loaded anything, so vue-i18n — which owns every other string the
// user reads — cannot reach it: whatever is typed here is what ships, in one
// language, next to a window the user may well have switched to the other.
//
// So the wording lives in one table with both languages side by side, and gets
// picked twice:
//
//   * at startup, from the OS locale, because that is the only hint available
//     before the user's own preference has been read back from the server;
//   * again as soon as the webview knows the reader's language, over the
//     `set_tray_language` command (frontend/src/utils/desktopTray.js).
//
// The fallback is Russian — the product default the webview falls back to as
// well (FALLBACK_LOCALE in frontend/src/i18n), so an unknown locale leaves both
// halves of the app saying the same thing rather than disagreeing.

use tauri::menu::MenuItem;
use tauri::Wry;

/// The languages the tray ships in. Mirrors SUPPORTED_LOCALES in the webview —
/// a language offered in the interface but missing here would silently fall back
/// to Russian in the tray alone.
pub const FALLBACK: &str = "ru";

/// The wording of one tray menu, in one language.
pub struct Labels {
    pub show: &'static str,
    pub quit: &'static str,
}

/// labels renders the menu in `lang`, which must already be normalized.
pub fn labels(lang: &str) -> Labels {
    match lang {
        "en" => Labels {
            show: "Open Tessera",
            quit: "Quit",
        },
        _ => Labels {
            show: "Открыть Tessera",
            quit: "Выход",
        },
    }
}

/// normalize_lang reduces a language tag to one of the shipped languages.
///
/// It takes the primary subtag, so the POSIX `en_US.UTF-8`, the BCP 47 `en-GB`
/// and a bare `en` all land on the same place; anything we do not ship (`de`,
/// the locale-less `C`/`POSIX`, an empty value) lands on the fallback.
pub fn normalize_lang(raw: &str) -> &'static str {
    let primary = raw
        .trim()
        .split(['.', '@', '_', '-'])
        .next()
        .unwrap_or_default()
        .to_ascii_lowercase();
    match primary.as_str() {
        "en" => "en",
        _ => FALLBACK,
    }
}

/// pick_language takes the first value that actually carries a locale, in the
/// order POSIX gives them precedence: LC_ALL over LC_MESSAGES over LANG. A
/// variable that is set but empty does not count as an answer — that is how a
/// shell spells "unset" — so the search continues past it.
pub fn pick_language<'a>(values: impl IntoIterator<Item = Option<&'a str>>) -> &'static str {
    for v in values {
        match v {
            Some(s) if !s.trim().is_empty() => return normalize_lang(s),
            _ => continue,
        }
    }
    FALLBACK
}

/// os_language is pick_language over the environment. On Windows none of these
/// are normally set and the tray starts on the fallback — a second of Russian
/// until the webview reports the reader's language, which is what the webview
/// itself starts on too.
pub fn os_language() -> &'static str {
    let vars = ["LC_ALL", "LC_MESSAGES", "LANG"].map(std::env::var);
    pick_language(vars.iter().map(|v| v.as_deref().ok()))
}

/// The tray items whose text has to follow the reader's language. Held in Tauri's
/// managed state so the command can reach them long after setup() has returned.
pub struct Tray {
    pub show: MenuItem<Wry>,
    pub quit: MenuItem<Wry>,
}

/// set_tray_language re-labels the tray in the language the webview is showing.
///
/// Called on every language switch, including the one at startup, so the tray
/// agrees with the window even when the OS locale said something else.
#[tauri::command]
pub fn set_tray_language(lang: String, tray: tauri::State<'_, Tray>) -> Result<(), String> {
    let l = labels(normalize_lang(&lang));
    tray.show.set_text(l.show).map_err(|e| e.to_string())?;
    tray.quit.set_text(l.quit).map_err(|e| e.to_string())?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_shipped_language_has_its_own_wording() {
        // A language added to the table but left with the Russian text would pass
        // "the shell holds no display text" and still show the wrong language.
        let ru = labels("ru");
        let en = labels("en");
        for (a, b) in [(ru.show, en.show), (ru.quit, en.quit)] {
            assert!(!a.trim().is_empty() && !b.trim().is_empty());
            assert_ne!(a, b, "the two languages share the same wording: {a:?}");
        }
        assert!(
            !en.show
                .chars()
                .any(|c| ('\u{0400}'..='\u{04FF}').contains(&c)),
            "en tray menu is in Russian: {:?}",
            en.show
        );
    }

    #[test]
    fn a_locale_is_reduced_to_its_primary_subtag() {
        for tag in ["en", "EN", "en-GB", "en_US.UTF-8", "  en_US  "] {
            assert_eq!(normalize_lang(tag), "en", "{tag}");
        }
        for tag in ["ru_RU.UTF-8", "ru", "ru_RU@quot"] {
            assert_eq!(normalize_lang(tag), "ru", "{tag}");
        }
    }

    #[test]
    fn a_language_we_do_not_ship_lands_on_the_fallback() {
        // Including the locale-less values a service or a bare container hands us.
        for tag in ["de_DE.UTF-8", "fr", "C", "POSIX", "", "   ", "english"] {
            assert_eq!(normalize_lang(tag), FALLBACK, "{tag}");
        }
    }

    #[test]
    fn lc_all_outranks_lc_messages_outranks_lang() {
        assert_eq!(
            pick_language([
                Some("en_US.UTF-8"),
                Some("ru_RU.UTF-8"),
                Some("ru_RU.UTF-8")
            ]),
            "en"
        );
        assert_eq!(
            pick_language([None, Some("en_GB.UTF-8"), Some("ru_RU.UTF-8")]),
            "en"
        );
        assert_eq!(pick_language([None, None, Some("en")]), "en");
    }

    #[test]
    fn an_empty_variable_is_not_an_answer() {
        // `LC_ALL=` is how a shell spells "unset", and POSIX reads it that way;
        // treating it as a value would pin every such session to the fallback
        // even with LANG=en_US.UTF-8 right behind it.
        assert_eq!(
            pick_language([Some(""), Some("  "), Some("en_US.UTF-8")]),
            "en"
        );
        assert_eq!(pick_language([None, None, None]), FALLBACK);
    }
}
