package locale

import "testing"

// TestIsValid_AcceptsSupportedLocales verifies every code in SupportedLocales
// passes IsValid. If a new locale is added to SupportedLocales but a test
// case isn't added here, TestIsValid_AcceptsSupportedLocales will still pass
// (it iterates the slice), but TestIsValid_Table will not — that's by design
// so the table-driven test documents every accepted code.
func TestIsValid_AcceptsSupportedLocales(t *testing.T) {
	if len(SupportedLocales) == 0 {
		t.Fatal("SupportedLocales is empty — at least one locale (the default) must be supported")
	}
	for _, code := range SupportedLocales {
		if !IsValid(code) {
			t.Errorf("IsValid(%q) = false, want true (code is in SupportedLocales)", code)
		}
	}
}

// TestIsValid_Table documents the full IsValid decision matrix:
//   - empty string is valid (means "clear preference, use system default")
//   - every code in SupportedLocales is valid
//   - case-sensitive: "EN" is NOT valid (callers must normalize first)
//   - unknown codes are invalid
func TestIsValid_Table(t *testing.T) {
	cases := []struct {
		name string
		code string
		want bool
	}{
		{"empty (clear preference)", "", true},
		{"English", "en", true},
		{"Chinese", "zh", true},
		{"Japanese", "ja", true},
		// Case-sensitivity: locale codes are lowercase by convention.
		// Callers that accept user input MUST normalize to lowercase
		// before calling IsValid.
		{"uppercase EN (not normalized)", "EN", false},
		{"uppercase ZH (not normalized)", "ZH", false},
		{"uppercase JA (not normalized)", "JA", false},
		// Common BCP-47 tags with region/script subtags — these are NOT
		// valid; the caller must extract just the primary subtag.
		{"zh-CN (use primary subtag only)", "zh-CN", false},
		{"en-US (use primary subtag only)", "en-US", false},
		{"ja-JP (use primary subtag only)", "ja-JP", false},
		// Unknown / unsupported.
		{"French (not bundled)", "fr", false},
		{"German (not bundled)", "de", false},
		{"Spanish (not bundled)", "es", false},
		{"Korean (not bundled)", "ko", false},
		{"Arabic (not bundled)", "ar", false},
		{"Hindi (not bundled)", "hi", false},
		{"Portuguese (not bundled)", "pt", false},
		{"Russian (not bundled)", "ru", false},
		// Malformed inputs.
		{"whitespace only", "   ", false},
		{"number", "123", false},
		{"code with space", "en zh", false},
		{"code with hyphen but no region", "en-", false},
		{"emoji", "🇨🇳", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValid(tc.code); got != tc.want {
				t.Errorf("IsValid(%q) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

// TestDefaultLocale_IsInSupportedLocales verifies that DefaultLocale is
// itself a valid locale. If someone changes DefaultLocale to a code not in
// SupportedLocales, this test fails — otherwise the system would fall back
// to an invalid locale and produce English-from-the-source-locale strings
// as a last resort.
func TestDefaultLocale_IsInSupportedLocales(t *testing.T) {
	if DefaultLocale == "" {
		t.Fatal("DefaultLocale is empty — must be a concrete locale code")
	}
	if !IsValid(DefaultLocale) {
		t.Errorf("DefaultLocale %q is not in SupportedLocales %v", DefaultLocale, SupportedLocales)
	}
}

// TestSupportedLocales_ContainsDefaultLocale verifies the same invariant
// from the SupportedLocales angle: the first entry should typically be the
// default (so admins see the default at the top of dropdowns).
func TestSupportedLocales_ContainsDefaultLocale(t *testing.T) {
	found := false
	for _, code := range SupportedLocales {
		if code == DefaultLocale {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DefaultLocale %q is not present in SupportedLocales %v", DefaultLocale, SupportedLocales)
	}
}

// TestSupportedLocales_NoEmptyOrDuplicateEntries guards against accidental
// corruption of the SupportedLocales slice (e.g. someone appending "" or
// duplicating "en"). The backend serializes this slice directly into the
// GET /system/status response — duplicates would render duplicate options
// in the admin SettingsView dropdown.
func TestSupportedLocales_NoEmptyOrDuplicateEntries(t *testing.T) {
	seen := make(map[string]bool, len(SupportedLocales))
	for _, code := range SupportedLocales {
		if code == "" {
			t.Error("SupportedLocales contains an empty string entry")
			continue
		}
		if seen[code] {
			t.Errorf("SupportedLocales contains duplicate entry %q", code)
		}
		seen[code] = true
	}
}

// TestSupportedLocales_AllLowercase enforces the lowercase-ASCII convention
// for locale codes. The frontend's matchBrowserLanguageTag() lowercases
// navigator.language before matching, so any uppercase letter in
// SupportedLocales would never match (the comparison is case-sensitive on
// both sides after lowercasing the input).
func TestSupportedLocales_AllLowercase(t *testing.T) {
	for _, code := range SupportedLocales {
		for _, r := range code {
			if !(r >= 'a' && r <= 'z') {
				t.Errorf("SupportedLocales entry %q contains non-lowercase-ASCII character %q", code, r)
			}
		}
	}
}
