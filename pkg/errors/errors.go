// Package errors defines fyom-specific error types and helpers.
package errors

import (
	"errors"
	"fmt"
)

// AppError represents a domain error with an HTTP status code and a stable
// machine-readable ErrorCode.
type AppError struct {
	// Code is the HTTP status code (e.g. 404, 409).
	Code int `json:"-"`
	// ErrorCode is the stable, machine-readable code (see codes.go).
	// Empty means "unclassified" — clients fall back to Message.
	ErrorCode Code `json:"error_code,omitempty"`
	// Message is the human-readable English fallback.
	Message string `json:"message"`
	// Detail is an optional longer explanation (also English fallback).
	Detail string `json:"detail,omitempty"`
	// Err is the wrapped underlying error; never serialized.
	Err error `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Predefined errors
var (
	ErrNotFound     = &AppError{Code: 404, ErrorCode: CodeResourceNotFound, Message: "resource not found"}
	ErrUnauthorized = &AppError{Code: 401, ErrorCode: CodeUnauthorized, Message: "unauthorized"}
	ErrForbidden    = &AppError{Code: 403, ErrorCode: CodeForbidden, Message: "forbidden"}
	ErrValidation   = &AppError{Code: 400, ErrorCode: CodeValidation, Message: "validation error"}
	ErrInternal     = &AppError{Code: 500, ErrorCode: CodeInternal, Message: "internal server error"}
	ErrConflict     = &AppError{Code: 409, ErrorCode: CodeConflict, Message: "resource already exists"}
)

// New creates a new AppError with the given HTTP status code and message.
// ErrorCode is left empty; prefer NewWithCode for new code.
func New(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// NewWithCode creates a new AppError with a stable error_code.
// If message is empty, the code's canonical default message is used.
func NewWithCode(httpStatus int, code Code, message string) *AppError {
	if message == "" {
		message = code.DefaultMessage()
	}
	return &AppError{Code: httpStatus, ErrorCode: code, Message: message}
}

// Wrap wraps an existing error with an AppError.
func Wrap(err error, appErr *AppError) *AppError {
	if appErr == nil {
		return &AppError{
			Code:      500,
			ErrorCode: CodeInternal,
			Message:   CodeInternal.DefaultMessage(),
			Err:       err,
		}
	}
	return &AppError{
		Code:      appErr.Code,
		ErrorCode: appErr.ErrorCode,
		Message:   appErr.Message,
		Detail:    appErr.Detail,
		Err:       err,
	}
}

// IsAppError checks if an error is an AppError and returns it.
func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
