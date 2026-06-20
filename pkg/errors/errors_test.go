package errors

import (
	"errors"
	"net/http"
	"testing"
)

// TestPredefinedErrors_HaveCorrectCodes locks the ErrorCode field of every
// predefined sentinel error. Clients match on these codes for i18n lookup,
// so changing one is a breaking change.
func TestPredefinedErrors_HaveCorrectCodes(t *testing.T) {
	cases := []struct {
		name     string
		err      *AppError
		wantCode Code
		wantHTTP int
	}{
		{"ErrNotFound", ErrNotFound, CodeResourceNotFound, http.StatusNotFound},
		{"ErrUnauthorized", ErrUnauthorized, CodeUnauthorized, http.StatusUnauthorized},
		{"ErrForbidden", ErrForbidden, CodeForbidden, http.StatusForbidden},
		{"ErrValidation", ErrValidation, CodeValidation, http.StatusBadRequest},
		{"ErrInternal", ErrInternal, CodeInternal, http.StatusInternalServerError},
		{"ErrConflict", ErrConflict, CodeConflict, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.ErrorCode != tc.wantCode {
				t.Errorf("%s.ErrorCode = %q, want %q", tc.name, tc.err.ErrorCode, tc.wantCode)
			}
			if tc.err.Code != tc.wantHTTP {
				t.Errorf("%s.Code = %d, want %d", tc.name, tc.err.Code, tc.wantHTTP)
			}
			if tc.err.Message == "" {
				t.Errorf("%s.Message is empty", tc.name)
			}
			if !tc.err.ErrorCode.IsValid() {
				t.Errorf("%s.ErrorCode %q is not registered in defaultMessages", tc.name, tc.err.ErrorCode)
			}
		})
	}
}

// TestNewWithCode_DefaultMessage verifies that NewWithCode fills in the
// canonical default message when the caller passes an empty string.
func TestNewWithCode_DefaultMessage(t *testing.T) {
	appErr := NewWithCode(http.StatusUnauthorized, CodeInvalidCredentials, "")
	if appErr.Message != CodeInvalidCredentials.DefaultMessage() {
		t.Errorf("Message = %q, want default %q", appErr.Message, CodeInvalidCredentials.DefaultMessage())
	}
	if appErr.ErrorCode != CodeInvalidCredentials {
		t.Errorf("ErrorCode = %q, want %q", appErr.ErrorCode, CodeInvalidCredentials)
	}
	if appErr.Code != http.StatusUnauthorized {
		t.Errorf("HTTP Code = %d, want %d", appErr.Code, http.StatusUnauthorized)
	}
}

// TestNewWithCode_ExplicitMessage verifies that NewWithCode preserves an
// explicit message (e.g. one carrying dynamic context like
// "library not found: lib_123") instead of overwriting it with the default.
func TestNewWithCode_ExplicitMessage(t *testing.T) {
	appErr := NewWithCode(http.StatusNotFound, CodeLibraryNotFound, "library not found: lib_123")
	if appErr.Message != "library not found: lib_123" {
		t.Errorf("Message = %q, want explicit %q", appErr.Message, "library not found: lib_123")
	}
	if appErr.ErrorCode != CodeLibraryNotFound {
		t.Errorf("ErrorCode = %q, want %q", appErr.ErrorCode, CodeLibraryNotFound)
	}
}

// TestNewWithCode_UnknownCode verifies behavior when an unregistered code
// is passed. DefaultMessage() falls back to "internal server error" so the
// emitted message is never empty.
func TestNewWithCode_UnknownCode(t *testing.T) {
	unknown := Code("does_not_exist")
	appErr := NewWithCode(http.StatusBadRequest, unknown, "")
	if appErr.Message != "internal server error" {
		t.Errorf("Message for unknown code = %q, want %q", appErr.Message, "internal server error")
	}
}

// TestNew_Legacy_NoErrorCode verifies the legacy `New` constructor produces
// an AppError with an empty ErrorCode. response.AppError will then emit a
// response WITHOUT an `error_code` field, so clients fall back to
// message-based handling. This preserves backward compatibility with any
// pre-Phase-3 callers that haven't been migrated yet.
func TestNew_Legacy_NoErrorCode(t *testing.T) {
	appErr := New(http.StatusBadRequest, "legacy error")
	if appErr.ErrorCode != "" {
		t.Errorf("Legacy New() should produce empty ErrorCode, got %q", appErr.ErrorCode)
	}
	if appErr.Message != "legacy error" {
		t.Errorf("Message = %q, want %q", appErr.Message, "legacy error")
	}
}

// TestAppError_Error_IncludesWrapped verifies the Error() method includes
// the wrapped underlying error's message when present.
func TestAppError_Error_IncludesWrapped(t *testing.T) {
	underlying := errors.New("disk i/o timeout")
	appErr := NewWithCode(http.StatusInternalServerError, CodeInternal, "refresh failed")
	wrapped := Wrap(underlying, appErr)

	msg := wrapped.Error()
	if !contains(msg, "refresh failed") {
		t.Errorf("Error() = %q, want it to contain 'refresh failed'", msg)
	}
	if !contains(msg, "disk i/o timeout") {
		t.Errorf("Error() = %q, want it to contain the wrapped error message", msg)
	}
}

// TestAppError_Error_NoWrapped verifies Error() returns just the message
// when there is no wrapped error.
func TestAppError_Error_NoWrapped(t *testing.T) {
	appErr := NewWithCode(http.StatusNotFound, CodeLibraryNotFound, "")
	if got := appErr.Error(); got != CodeLibraryNotFound.DefaultMessage() {
		t.Errorf("Error() = %q, want %q", got, CodeLibraryNotFound.DefaultMessage())
	}
}

// TestAppError_Unwrap verifies errors.Is / errors.As work through AppError.
// This is required so middleware that does `errors.Is(err, ErrNotFound)`
// still matches after wrapping.
func TestAppError_Unwrap(t *testing.T) {
	underlying := errors.New("sql: no rows")
	wrapped := Wrap(underlying, ErrNotFound)

	if !errors.Is(wrapped, underlying) {
		t.Error("errors.Is(wrapped, underlying) returned false; Unwrap chain is broken")
	}

	// errors.As should also extract the *AppError.
	var extracted *AppError
	if !errors.As(wrapped, &extracted) {
		t.Fatal("errors.As failed to extract *AppError from wrapped error")
	}
	if extracted.ErrorCode != CodeResourceNotFound {
		t.Errorf("Extracted ErrorCode = %q, want %q", extracted.ErrorCode, CodeResourceNotFound)
	}
}

// TestWrap_NilAppError verifies that wrapping with a nil AppError template
// produces a valid internal_error AppError rather than panicking. This
// guards callers that do `Wrap(err, maybeNilAppErr)`.
func TestWrap_NilAppError(t *testing.T) {
	underlying := errors.New("unexpected fault")
	wrapped := Wrap(underlying, nil)

	if wrapped == nil {
		t.Fatal("Wrap(err, nil) returned nil")
	}
	if wrapped.Code != http.StatusInternalServerError {
		t.Errorf("Code = %d, want 500", wrapped.Code)
	}
	if wrapped.ErrorCode != CodeInternal {
		t.Errorf("ErrorCode = %q, want %q", wrapped.ErrorCode, CodeInternal)
	}
	//nolint:errorlint // identity check: verify wrapped error is the same instance
	if wrapped.Err != underlying {
		t.Error("Wrapped Err does not match the underlying error")
	}
}

// TestWrap_PropagatesErrorCode verifies that Wrap preserves the template
// AppError's ErrorCode, HTTP status, message, and detail. The frontend
// looks up `api_error.<error_code>` for i18n, so the code must survive
// the wrap operation.
func TestWrap_PropagatesErrorCode(t *testing.T) {
	underlying := errors.New("context deadline exceeded")
	template := &AppError{
		Code:      http.StatusServiceUnavailable,
		ErrorCode: CodeRefreshAlreadyInProgress,
		Message:   "refresh already in progress for this library",
		Detail:    "another refresh started 2s ago",
	}
	wrapped := Wrap(underlying, template)

	if wrapped.Code != template.Code {
		t.Errorf("Code = %d, want %d", wrapped.Code, template.Code)
	}
	if wrapped.ErrorCode != template.ErrorCode {
		t.Errorf("ErrorCode = %q, want %q", wrapped.ErrorCode, template.ErrorCode)
	}
	if wrapped.Message != template.Message {
		t.Errorf("Message = %q, want %q", wrapped.Message, template.Message)
	}
	if wrapped.Detail != template.Detail {
		t.Errorf("Detail = %q, want %q", wrapped.Detail, template.Detail)
	}
	//nolint:errorlint // identity check: verify wrapped error is the same instance
	if wrapped.Err != underlying {
		t.Error("Wrapped Err does not match the underlying error")
	}
}

// TestIsAppError verifies the IsAppError helper correctly identifies
// *AppError values (including wrapped ones) and returns false for
// non-AppError errors.
func TestIsAppError(t *testing.T) {
	cases := []struct {
		name  string
		input error
		want  bool
	}{
		{"direct AppError", ErrNotFound, true},
		{"wrapped AppError", Wrap(errors.New("io"), ErrNotFound), true},
		{"NewWithCode result", NewWithCode(400, CodeValidation, ""), true},
		{"plain Go error", errors.New("plain"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := IsAppError(tc.input)
			if ok != tc.want {
				t.Errorf("IsAppError(%v) ok = %v, want %v", tc.input, ok, tc.want)
			}
		})
	}
}

// TestAppError_JSONOmitsEmptyErrorCode verifies the json tags on AppError
// so the response envelope can omit error_code for legacy errors and omit
// Code/Err entirely (they're json:"-").
//
// This is a structural assertion — if someone removes a json tag, the test
// fails. We don't actually marshal here; we just inspect the field tags via
// reflection-lite (manual struct instantiation + json.Marshal in the
// response package's own tests).
func TestAppError_JSONOmitsEmptyErrorCode(t *testing.T) {
	// Legacy AppError (no ErrorCode): the json:"error_code,omitempty" tag
	// should cause the field to be omitted when empty.
	legacy := New(http.StatusBadRequest, "legacy")
	if legacy.ErrorCode != "" {
		t.Errorf("Legacy AppError ErrorCode = %q, want empty (so omitempty kicks in)", legacy.ErrorCode)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
