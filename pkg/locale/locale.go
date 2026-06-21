// Package locale defines the supported locales for fyom's i18n system.
//
// This package is the single source of truth for locale codes on the backend.
// The frontend mirrors this list in frontend/src/plugins/i18n.ts (SUPPORTED_LOCALES).
// When adding a new locale, update BOTH locations and add a JSON file in
// frontend/src/locales/.
//
// See docs/i18n.md for the full i18n architecture and locale resolution chain.
package locale

// SupportedLocales is the list of locale codes the backend accepts.
//
// Order matters: the first entry (DefaultLocale) is used when no preference
// can be determined.
//
// Phase 7: en + zh + ja. Additional locales require:
//   - A new frontend/src/locales/<code>.json file
//   - An entry in frontend/src/plugins/i18n.ts BUNDLED_LOCALES + LOCALE_DISPLAY_LABELS
//   - An entry here
var SupportedLocales = []string{"en", "zh", "ja"}

// DefaultLocale is the fallback locale when no preference is determined.
// Always the source locale (English) so a fresh install renders deterministically.
const DefaultLocale = "en"

// IsValid returns true if the given code is in SupportedLocales.
//
// Used by handlers to validate PUT /auth/me/preferences { preferred_language }
// before persisting to the database.
func IsValid(code string) bool {
	if code == "" {
		// Empty string is valid — it means "clear preference, use system default".
		return true
	}

	for _, supported := range SupportedLocales {
		if code == supported {
			return true
		}
	}

	return false
}
