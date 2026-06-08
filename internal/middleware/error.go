package middleware

import (
	"net/http"

	"github.com/fyom/fyom/pkg/response"
)

// ErrorHandler is a centralized error handling middleware.
// It captures the response status code and writes a JSON error envelope
// if the handler wrote a non-xx status without a body, or if the handler
// set an error status code.
func ErrorHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(ww, r)
			// If the status is an error code and no body was written,
			// write a JSON error envelope.
			if ww.status >= 400 && !ww.bodyWritten {
				response.Error(w, ww.status, http.StatusText(ww.status))
			}
		})
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
