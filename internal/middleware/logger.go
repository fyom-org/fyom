package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logger returns a middleware that logs requests via slog.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := &responseWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(ww, r)

			latency := time.Since(start)
			status := ww.status
			clientIP := r.RemoteAddr
			method := r.Method
			path := r.URL.Path
			raw := r.URL.RawQuery

			if raw != "" {
				path = path + "?" + raw
			}

			attrs := []slog.Attr{
				slog.Int("status", status),
				slog.String("method", method),
				slog.String("path", path),
				slog.String("ip", clientIP),
				slog.Duration("latency", latency),
			}

			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			} else if status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "http_request", attrs...)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
