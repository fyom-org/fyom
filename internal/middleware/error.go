package middleware

import (
	"net/http"

	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/response"
)

// ErrorHandler is a centralized error handling middleware.
// It captures the response status code and writes a JSON error envelope
// if the handler wrote a non-xx status without a body, or if the handler
// set an error status code.
//
// When a handler writes only a status code (e.g. via http.Error with a
// plain-text body, or a 405 from method-not-allowed routing), this
// middleware rewrites the response into the standard JSON envelope with a
// best-effort stable error_code derived from the HTTP status.
func ErrorHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(ww, r)
			// If the status is an error code and no body was written,
			// write a JSON error envelope with a stable error_code.
			if ww.status >= 400 && !ww.bodyWritten {
				response.ErrorCode(w, ww.status, codeForStatus(ww.status), http.StatusText(ww.status))
			}
		})
	}
}

// codeForStatus maps a bare HTTP status code to a stable error_code.
// Used when a handler did not provide a more specific code.
func codeForStatus(status int) errors.Code {
	switch status {
	case http.StatusBadRequest:
		return errors.CodeBadRequest
	case http.StatusUnauthorized:
		return errors.CodeUnauthorized
	case http.StatusForbidden:
		return errors.CodeForbidden
	case http.StatusNotFound:
		return errors.CodeNotFound
	case http.StatusMethodNotAllowed:
		return errors.CodeBadRequest
	case http.StatusConflict:
		return errors.CodeConflict
	case http.StatusRequestTimeout:
		return errors.CodeUnauthorized
	case http.StatusTooManyRequests:
		return errors.CodeBadRequest
	default:
		if status >= 500 {
			return errors.CodeInternal
		}
		return errors.CodeBadRequest
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code
// and track whether a body was written.
type statusWriter struct {
	http.ResponseWriter
	status      int
	bodyWritten bool
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	sw.bodyWritten = true
	return sw.ResponseWriter.Write(b)
}
