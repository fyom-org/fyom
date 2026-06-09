// Package errors defines fyom-specific error types and helpers.
package errors

import (
	"errors"
	"fmt"
)

// AppError represents a domain error with an HTTP status code.
type AppError struct {
	Code    int    `json:"-"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Err     error  `json:"-"`
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
	ErrNotFound     = &AppError{Code: 404, Message: "resource not found"}
	ErrUnauthorized = &AppError{Code: 401, Message: "unauthorized"}
	ErrForbidden    = &AppError{Code: 403, Message: "forbidden"}
	ErrValidation   = &AppError{Code: 400, Message: "validation error"}
	ErrInternal     = &AppError{Code: 500, Message: "internal server error"}
	ErrConflict     = &AppError{Code: 409, Message: "resource already exists"}
)

// New creates a new AppError with the given code and message.
func New(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Wrap wraps an existing error with an AppError.
func Wrap(err error, appErr *AppError) *AppError {
	return &AppError{
		Code:    appErr.Code,
		Message: appErr.Message,
		Err:     err,
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
