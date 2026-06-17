package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fyomerrors "github.com/fyom/fyom/pkg/errors"
)

// decodeResponse is a small helper that reads the JSON body written by the
// response package into a Response struct. It fails the test if the body
// is not valid JSON or if the Content-Type header is wrong.
func decodeResponse(t *testing.T, rw *httptest.ResponseRecorder) Response {
	t.Helper()
	if ct := rw.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	var resp Response
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}
	return resp
}

// TestSuccess_EmitsStandardEnvelope verifies the success path: HTTP 200,
// code:0, message:"ok", no error_code field, data present.
func TestSuccess_EmitsStandardEnvelope(t *testing.T) {
	rw := httptest.NewRecorder()
	Success(rw, map[string]string{"hello": "world"})

	if rw.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want %d", rw.Code, http.StatusOK)
	}
	resp := decodeResponse(t, rw)
	if resp.Code != 0 {
		t.Errorf("envelope.code = %d, want 0 for success", resp.Code)
	}
	if resp.Message != "ok" {
		t.Errorf("envelope.message = %q, want %q", resp.Message, "ok")
	}
	if resp.ErrorCode != "" {
		t.Errorf("envelope.error_code = %q, want empty for success (omitempty)", resp.ErrorCode)
	}
	if resp.Data == nil {
		t.Error("envelope.data is nil, want the payload")
	}
}

// TestCreated_Emits201 verifies the 201 path.
func TestCreated_Emits201(t *testing.T) {
	rw := httptest.NewRecorder()
	Created(rw, map[string]int{"id": 42})

	if rw.Code != http.StatusCreated {
		t.Errorf("HTTP status = %d, want %d", rw.Code, http.StatusCreated)
	}
	resp := decodeResponse(t, rw)
	if resp.Code != 0 {
		t.Errorf("envelope.code = %d, want 0 for created", resp.Code)
	}
	if resp.Message != "created" {
		t.Errorf("envelope.message = %q, want %q", resp.Message, "created")
	}
	if resp.ErrorCode != "" {
		t.Errorf("envelope.error_code = %q, want empty for created (omitempty)", resp.ErrorCode)
	}
}

// TestNoContent verifies the 204 path: no body, no Content-Type.
func TestNoContent(t *testing.T) {
	rw := httptest.NewRecorder()
	NoContent(rw)

	if rw.Code != http.StatusNoContent {
		t.Errorf("HTTP status = %d, want %d", rw.Code, http.StatusNoContent)
	}
	if rw.Body.Len() != 0 {
		t.Errorf("Response body = %q, want empty", rw.Body.String())
	}
}

// TestErrorCode_WithDefaultMessage verifies that passing an empty message
// causes the code's canonical default message to be used. This is the
// preferred path for new code — handlers pass just the code, the response
// package fills in the English fallback.
func TestErrorCode_WithDefaultMessage(t *testing.T) {
	rw := httptest.NewRecorder()
	ErrorCode(rw, http.StatusUnauthorized, fyomerrors.CodeInvalidCredentials, "")

	if rw.Code != http.StatusUnauthorized {
		t.Errorf("HTTP status = %d, want %d", rw.Code, http.StatusUnauthorized)
	}
	resp := decodeResponse(t, rw)
	if resp.Code != http.StatusUnauthorized {
		t.Errorf("envelope.code = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
	if resp.Message != fyomerrors.CodeInvalidCredentials.DefaultMessage() {
		t.Errorf("envelope.message = %q, want default %q", resp.Message, fyomerrors.CodeInvalidCredentials.DefaultMessage())
	}
	if resp.ErrorCode != "invalid_credentials" {
		t.Errorf("envelope.error_code = %q, want %q", resp.ErrorCode, "invalid_credentials")
	}
}

// TestErrorCode_WithExplicitMessage verifies that an explicit message
// (e.g. one carrying dynamic context like "library not found: lib_123")
// overrides the default. The error_code stays stable so the client can
// still look up a translated message via api_error.<code>.
func TestErrorCode_WithExplicitMessage(t *testing.T) {
	rw := httptest.NewRecorder()
	ErrorCode(rw, http.StatusNotFound, fyomerrors.CodeLibraryNotFound, "library not found: lib_123")

	resp := decodeResponse(t, rw)
	if resp.Message != "library not found: lib_123" {
		t.Errorf("envelope.message = %q, want explicit %q", resp.Message, "library not found: lib_123")
	}
	if resp.ErrorCode != "library_not_found" {
		t.Errorf("envelope.error_code = %q, want %q", resp.ErrorCode, "library_not_found")
	}
}

// TestErrorCode_UnknownCodeFallback verifies that an unregistered code
// falls back to "internal server error" for the message (never empty),
// while still emitting the raw code string in error_code so the client
// can see what was attempted.
func TestErrorCode_UnknownCodeFallback(t *testing.T) {
	rw := httptest.NewRecorder()
	unknown := fyomerrors.Code("does_not_exist_xyz")
	ErrorCode(rw, http.StatusInternalServerError, unknown, "")

	resp := decodeResponse(t, rw)
	if resp.Message != "internal server error" {
		t.Errorf("envelope.message for unknown code = %q, want %q", resp.Message, "internal server error")
	}
	if resp.ErrorCode != "does_not_exist_xyz" {
		t.Errorf("envelope.error_code = %q, want raw %q", resp.ErrorCode, "does_not_exist_xyz")
	}
}

// TestErrorWithCode_RawString verifies the alias that accepts a raw string
// code (used by middleware/error.go where the code is computed dynamically
// from the HTTP status).
func TestErrorWithCode_RawString(t *testing.T) {
	rw := httptest.NewRecorder()
	ErrorWithCode(rw, http.StatusBadRequest, "validation_error", "")

	resp := decodeResponse(t, rw)
	if resp.Message != "validation error" {
		t.Errorf("envelope.message = %q, want %q", resp.Message, "validation error")
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("envelope.error_code = %q, want %q", resp.ErrorCode, "validation_error")
	}
}

// TestAppError_NilEmitsInternal500 verifies the nil-safety contract.
// Callers like `response.AppError(w, IsAppError(err))` may pass nil if
// the error is not an AppError; the helper must not panic and must emit
// a 500 internal_error response.
func TestAppError_NilEmitsInternal500(t *testing.T) {
	rw := httptest.NewRecorder()
	AppError(rw, nil)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("HTTP status = %d, want %d", rw.Code, http.StatusInternalServerError)
	}
	resp := decodeResponse(t, rw)
	if resp.Code != http.StatusInternalServerError {
		t.Errorf("envelope.code = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
	if resp.ErrorCode != "internal_error" {
		t.Errorf("envelope.error_code = %q, want %q", resp.ErrorCode, "internal_error")
	}
	if resp.Message != "internal server error" {
		t.Errorf("envelope.message = %q, want %q", resp.Message, "internal server error")
	}
}

// TestAppError_WithPredefinedError verifies that response.AppError
// correctly serializes a predefined sentinel *AppError (ErrNotFound etc.)
// — including its HTTP status, ErrorCode, and Message.
func TestAppError_WithPredefinedError(t *testing.T) {
	rw := httptest.NewRecorder()
	AppError(rw, fyomerrors.ErrNotFound)

	if rw.Code != http.StatusNotFound {
		t.Errorf("HTTP status = %d, want %d", rw.Code, http.StatusNotFound)
	}
	resp := decodeResponse(t, rw)
	if resp.Code != http.StatusNotFound {
		t.Errorf("envelope.code = %d, want %d", resp.Code, http.StatusNotFound)
	}
	if resp.ErrorCode != "resource_not_found" {
		t.Errorf("envelope.error_code = %q, want %q", resp.ErrorCode, "resource_not_found")
	}
	if resp.Message != "resource not found" {
		t.Errorf("envelope.message = %q, want %q", resp.Message, "resource not found")
	}
}

// TestAppError_WithNewWithCode verifies that an AppError created via
// NewWithCode (the preferred constructor for new code) is serialized
// correctly, including the auto-filled default message.
func TestAppError_WithNewWithCode(t *testing.T) {
	rw := httptest.NewRecorder()
	appErr := fyomerrors.NewWithCode(http.StatusUnauthorized, fyomerrors.CodeInvalidCredentials, "")
	AppError(rw, appErr)

	resp := decodeResponse(t, rw)
	if resp.Code != http.StatusUnauthorized {
		t.Errorf("envelope.code = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
	if resp.ErrorCode != "invalid_credentials" {
		t.Errorf("envelope.error_code = %q, want %q", resp.ErrorCode, "invalid_credentials")
	}
	if resp.Message != fyomerrors.CodeInvalidCredentials.DefaultMessage() {
		t.Errorf("envelope.message = %q, want default %q", resp.Message, fyomerrors.CodeInvalidCredentials.DefaultMessage())
	}
}

// TestAppError_LegacyEmptyErrorCode verifies that an AppError created via
// the legacy `New()` constructor (no ErrorCode) is still serialized
// correctly — the error_code field is omitted via omitempty so clients
// fall back to message-based handling. This preserves backward
// compatibility with any unmigrated call sites. The HTTP status is taken
// from the AppError.Code field (here 400).
func TestAppError_LegacyEmptyErrorCode(t *testing.T) {
	rw := httptest.NewRecorder()
	legacy := fyomerrors.New(http.StatusBadRequest, "legacy validation message")
	AppError(rw, legacy)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("HTTP status = %d, want %d (taken from AppError.Code)", rw.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, rw)
	if resp.ErrorCode != "" {
		t.Errorf("envelope.error_code = %q, want empty (legacy AppError has no code, omitempty)", resp.ErrorCode)
	}
	if resp.Message != "legacy validation message" {
		t.Errorf("envelope.message = %q, want %q", resp.Message, "legacy validation message")
	}
}

// TestAppError_ZeroHTTPStatusDefaultsTo500 verifies the defensive branch
// in response.AppError: if appErr.Code == 0 (e.g. constructed via a raw
// struct literal without setting Code), the response defaults to 500
// Internal Server Error rather than emitting HTTP 0 (which would panic
// the stdlib http package).
func TestAppError_ZeroHTTPStatusDefaultsTo500(t *testing.T) {
	rw := httptest.NewRecorder()
	zero := &fyomerrors.AppError{
		Code:      0,
		ErrorCode: fyomerrors.CodeInternal,
		Message:   "unclassified failure",
	}
	AppError(rw, zero)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("HTTP status = %d, want %d (Code=0 should default to 500)", rw.Code, http.StatusInternalServerError)
	}
	resp := decodeResponse(t, rw)
	if resp.Code != http.StatusInternalServerError {
		t.Errorf("envelope.code = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
	if resp.ErrorCode != "internal_error" {
		t.Errorf("envelope.error_code = %q, want %q", resp.ErrorCode, "internal_error")
	}
}

// TestError_Deprecated_NoErrorCode verifies the deprecated `Error` helper
// still works but emits NO error_code field. This locks the backward-compat
// behavior so we can detect if someone accidentally re-adds error_code
// population to the deprecated path.
func TestError_Deprecated_NoErrorCode(t *testing.T) {
	rw := httptest.NewRecorder()
	Error(rw, http.StatusBadRequest, "deprecated path message")

	resp := decodeResponse(t, rw)
	if resp.ErrorCode != "" {
		t.Errorf("Deprecated Error() emitted error_code = %q, want empty (deprecated path must not set error_code)", resp.ErrorCode)
	}
	if resp.Message != "deprecated path message" {
		t.Errorf("envelope.message = %q, want %q", resp.Message, "deprecated path message")
	}
	if resp.Code != http.StatusBadRequest {
		t.Errorf("envelope.code = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

// TestErrorCode_HTTPStatusSyncedWithEnvelopeCode verifies the slightly
// unusual invariant that the HTTP status code (.WriteHeader) and the
// envelope's `code` field are always equal. The frontend reads the
// envelope `code` for routing decisions (e.g. 401 → redirect to login),
// so any drift would cause subtle bugs.
func TestErrorCode_HTTPStatusSyncedWithEnvelopeCode(t *testing.T) {
	cases := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusInternalServerError,
	}
	for _, httpStatus := range cases {
		t.Run(http.StatusText(httpStatus), func(t *testing.T) {
			rw := httptest.NewRecorder()
			ErrorCode(rw, httpStatus, fyomerrors.CodeInternal, "")

			if rw.Code != httpStatus {
				t.Errorf("HTTP status = %d, want %d", rw.Code, httpStatus)
			}
			resp := decodeResponse(t, rw)
			if resp.Code != httpStatus {
				t.Errorf("envelope.code = %d, want %d (must match HTTP status)", resp.Code, httpStatus)
			}
		})
	}
}
