package errors

import (
	"strings"
	"testing"
)

// TestCode_DefaultMessage_KnownCodes verifies that every Code constant
// declared in codes.go has a corresponding entry in the defaultMessages map.
//
// This is a critical invariant: if a code is declared but has no default
// message, callers using NewWithCode(httpStatus, code, "") would silently
// get "internal server error", which is misleading. The test fails the
// build in that case so the omission is caught at compile-test time rather
// than surfacing as a confusing user-facing message in production.
func TestCode_DefaultMessage_KnownCodes(t *testing.T) {
	codes := []Code{
		// Generic
		CodeInternal, CodeValidation, CodeInvalidJSON, CodeBadRequest, CodeConflict,
		// Auth
		CodeUnauthorized, CodeForbidden, CodeInvalidCredentials,
		CodeRegistrationDisabled, CodeAdminRoleRequired, CodeLocalhostOnly,
		CodeMissingAuthHeader, CodeInvalidAuthHeader, CodeTokenMissingSubject,
		CodeFailedToIssueToken,
		// Bootstrap
		CodeNoBootstrapToken, CodeNoBootstrapSession, CodeAlreadyInitialized,
		// Resource lookup
		CodeNotFound, CodeResourceNotFound, CodeLibraryNotFound,
		CodeProviderNotFound, CodeMissingID,
		// Validation specifics
		CodeNameRequired, CodeDisplayNameRequired, CodeDisplayNameTooLong,
		CodeIDRequired, CodeIDTooLong, CodeIDHasSpaces, CodeIDLocalReserved,
		CodeNewPasswordRequired, CodeOldPasswordRequired, CodeLibraryIDRequired,
		CodeUserIDAndLibraryIDRequired, CodeConfigInvalidJSON, CodeTypeInvalid,
		CodeUnsupportedLocale, CodeInvalidStatus, CodeInvalidProgress,
		CodeInvalidMode, CodeUnknownSetting, CodeImportFromProviderTypeUnsupported,
		// Provider ops
		CodeFailedToCreateProvider, CodeFailedToUpdateProvider,
		CodeFailedToDeleteProvider, CodeFailedToLoadProviderConfig,
		CodeFailedToCreateS3Client,
		// Library ops
		CodeRefreshAlreadyInProgress, CodeOrphanModeDeleteItemsFirst,
		// Media ops
		CodeMediaItemNotShow, CodeCannotUpdateProgressForShow,
	}

	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			if !code.IsValid() {
				t.Errorf("Code %q is declared but not registered in defaultMessages", code)
				return
			}
			msg := code.DefaultMessage()
			if msg == "" {
				t.Errorf("Code %q has an empty DefaultMessage", code)
			}
			if msg == "internal server error" && code != CodeInternal {
				t.Errorf("Code %q fell back to 'internal server error' — add it to defaultMessages", code)
			}
		})
	}
}

// TestCode_DefaultMessage_UnknownCode verifies the fallback behavior for
// codes that are not registered (e.g. a typo, or a code from a newer server
// version that this client doesn't know about).
func TestCode_DefaultMessage_UnknownCode(t *testing.T) {
	unknown := Code("does_not_exist_xyz")
	if unknown.IsValid() {
		t.Error("IsValid returned true for an unregistered code")
	}
	if got := unknown.DefaultMessage(); got != "internal server error" {
		t.Errorf("DefaultMessage for unknown code = %q, want %q", got, "internal server error")
	}
}

// TestCode_DefaultMessage_EmptyCode verifies that the zero-value Code also
// falls back to the internal-error message rather than returning an empty
// string. This protects response.AppError and response.ErrorCode from
// emitting an empty `message` field when called with an unclassified error.
func TestCode_DefaultMessage_EmptyCode(t *testing.T) {
	var empty Code
	if empty.IsValid() {
		t.Error("IsValid returned true for the zero-value Code")
	}
	if got := empty.DefaultMessage(); got != "internal server error" {
		t.Errorf("DefaultMessage for empty code = %q, want %q", got, "internal server error")
	}
}

// TestCode_String verifies the fmt.Stringer implementation returns the raw
// string value (no quotes, no transformation). The frontend uses this value
// verbatim as the i18n lookup key, so any transformation would break
// translation lookups.
func TestCode_String(t *testing.T) {
	cases := []struct {
		code Code
		want string
	}{
		{CodeInternal, "internal_error"},
		{CodeInvalidCredentials, "invalid_credentials"},
		{CodeLibraryNotFound, "library_not_found"},
		{Code(""), ""},
		{Code("custom"), "custom"},
	}
	for _, tc := range cases {
		if got := tc.code.String(); got != tc.want {
			t.Errorf("Code(%q).String() = %q, want %q", tc.code, got, tc.want)
		}
	}
}

// TestCode_NamingConvention enforces the snake_case naming convention
// documented in codes.go. Renaming a code is a breaking change for clients
// (the frontend looks up `api_error.<error_code>` in locale JSONs), so any
// drift should fail the build.
//
// Allowed pattern: lowercase ASCII letters, digits, and underscores.
// Must start with a letter. Must not contain consecutive underscores.
// Must not end with an underscore.
func TestCode_NamingConvention(t *testing.T) {
	codes := []Code{
		CodeInternal, CodeValidation, CodeInvalidJSON, CodeBadRequest, CodeConflict,
		CodeUnauthorized, CodeForbidden, CodeInvalidCredentials,
		CodeRegistrationDisabled, CodeAdminRoleRequired, CodeLocalhostOnly,
		CodeMissingAuthHeader, CodeInvalidAuthHeader, CodeTokenMissingSubject,
		CodeFailedToIssueToken, CodeNoBootstrapToken, CodeNoBootstrapSession,
		CodeAlreadyInitialized, CodeNotFound, CodeResourceNotFound,
		CodeLibraryNotFound, CodeProviderNotFound, CodeMissingID,
		CodeNameRequired, CodeDisplayNameRequired, CodeDisplayNameTooLong,
		CodeIDRequired, CodeIDTooLong, CodeIDHasSpaces, CodeIDLocalReserved,
		CodeNewPasswordRequired, CodeOldPasswordRequired, CodeLibraryIDRequired,
		CodeUserIDAndLibraryIDRequired, CodeConfigInvalidJSON, CodeTypeInvalid,
		CodeUnsupportedLocale, CodeInvalidStatus, CodeInvalidProgress,
		CodeInvalidMode, CodeUnknownSetting, CodeImportFromProviderTypeUnsupported,
		CodeFailedToCreateProvider, CodeFailedToUpdateProvider,
		CodeFailedToDeleteProvider, CodeFailedToLoadProviderConfig,
		CodeFailedToCreateS3Client, CodeRefreshAlreadyInProgress,
		CodeOrphanModeDeleteItemsFirst, CodeMediaItemNotShow,
		CodeCannotUpdateProgressForShow,
	}
	for _, code := range codes {
		s := string(code)
		if s == "" {
			t.Errorf("Code value is empty")
			continue
		}
		// First char must be a lowercase letter.
		if !isLowerASCII(s[0]) {
			t.Errorf("Code %q must start with a lowercase letter", s)
		}
		// Last char must not be underscore.
		if s[len(s)-1] == '_' {
			t.Errorf("Code %q must not end with an underscore", s)
		}
		// No consecutive underscores.
		if strings.Contains(s, "__") {
			t.Errorf("Code %q must not contain consecutive underscores", s)
		}
		// Only [a-z0-9_].
		for i := 0; i < len(s); i++ {
			c := s[i]
			if !(isLowerASCII(c) || (c >= '0' && c <= '9') || c == '_') {
				t.Errorf("Code %q contains invalid character %q at position %d", s, c, i)
			}
		}
	}
}

// TestCode_DefaultMessage_NoInternalLeak verifies that no default message
// leaks internal implementation details (file paths, stack traces, SQL,
// JWT structure, etc.). The `message` field is sent to clients as an
// English fallback and must be safe to display.
func TestCode_DefaultMessage_NoInternalLeak(t *testing.T) {
	unsafeFragments := []string{
		"sql", "stack", "trace", "exception", "internal server",
		"jwt", "token structure", "undefined", "null",
		".go:", "panic:", "goroutine",
	}

	for code, msg := range defaultMessages {
		lower := strings.ToLower(msg)
		for _, frag := range unsafeFragments {
			// "internal server error" is the canonical fallback for CodeInternal
			// itself and is allowed — it's a user-facing label, not a leak.
			if code == CodeInternal && frag == "internal server" {
				continue
			}
			if strings.Contains(lower, frag) {
				t.Errorf("Code %q default message %q contains unsafe fragment %q", code, msg, frag)
			}
		}
	}
}

func isLowerASCII(c byte) bool {
	return c >= 'a' && c <= 'z'
}
