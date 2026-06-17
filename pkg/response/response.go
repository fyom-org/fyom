// Package response provides helpers for writing JSON HTTP responses.
//
// The standard envelope is:
//
//	{
//	  "code":        200,              // HTTP status code
//	  "message":     "ok",             // human-readable English fallback
//	  "error_code":  "",               // stable machine code (errors only)
//	  "data":        {...}             // payload (success only)
//	}
//
// Clients should prefer `error_code` over `message` for i18n lookup, since
// `message` is English-only and may change between releases.
package response

import (
	"encoding/json"
	"net/http"

	"github.com/fyom/fyom/pkg/errors"
)

// Response is the standard API response envelope.
type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	ErrorCode string      `json:"error_code,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// Success writes a successful response.
func Success(w http.ResponseWriter, data interface{}) {
	JSON(w, 200, Response{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// Created writes a 201 response.
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, 201, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(204)
}

// Error writes an error response WITHOUT a stable error_code.
//
// Deprecated: prefer ErrorCode or ErrorWithCode for new code — clients can
// only do i18n lookup when error_code is present. This helper is kept for
// backward compatibility with call sites that have not yet been migrated.
func Error(w http.ResponseWriter, code int, message string) {
	JSON(w, code, Response{
		Code:    code,
		Message: message,
	})
}

// ErrorCode writes an error response with a stable error_code.
//
// If message is empty, the code's canonical default English message is used.
// Pass an explicit message when you want to include dynamic context (e.g.
// "library not found: lib_123"); the error_code stays stable so the client
// can still look up a translated message.
func ErrorCode(w http.ResponseWriter, httpStatus int, code errors.Code, message string) {
	if message == "" {
		message = code.DefaultMessage()
	}
	JSON(w, httpStatus, Response{
		Code:      httpStatus,
		Message:   message,
		ErrorCode: code.String(),
	})
}

// ErrorWithCode is an alias for ErrorCode that accepts a raw string code.
//
// Use this when the code is computed dynamically (e.g. propagated from an
// AppError). Prefer the typed ErrorCode helper at static call sites.
func ErrorWithCode(w http.ResponseWriter, httpStatus int, code string, message string) {
	if message == "" {
		message = errors.Code(code).DefaultMessage()
	}
	JSON(w, httpStatus, Response{
		Code:      httpStatus,
		Message:   message,
		ErrorCode: code,
	})
}

// AppError writes an error response from an *errors.AppError.
//
// If appErr is nil, a generic 500 internal_error is emitted.
// If appErr.ErrorCode is empty (legacy AppError), the response omits
// error_code and clients will fall back to message-based handling.
func AppError(w http.ResponseWriter, appErr *errors.AppError) {
	if appErr == nil {
		ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}
	if appErr.Code == 0 {
		appErr.Code = http.StatusInternalServerError
	}
	if appErr.Message == "" {
		appErr.Message = appErr.ErrorCode.DefaultMessage()
	}
	JSON(w, appErr.Code, Response{
		Code:      appErr.Code,
		Message:   appErr.Message,
		ErrorCode: appErr.ErrorCode.String(),
	})
}
