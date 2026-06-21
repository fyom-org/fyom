package desktop

import (
	"io/fs"
	"net/http"
	"strings"
)

// HandlerOptions configures the desktop asset handler.
type HandlerOptions struct {
	// APIPrefix is the prefix that routes to the backend API.
	// Defaults to "/api/v1/".
	APIPrefix string

	// API is the handler for API requests (e.g. chi router).
	API http.Handler

	// Assets is the filesystem containing the frontend build output.
	// It should have a "dist" subdirectory (from frontend/embed.go).
	Assets fs.FS
}

// NewHandler creates an http.Handler that routes /api/v1/* to the API handler
// and everything else to the embedded static assets with SPA fallback.
func NewHandler(opts HandlerOptions) (http.Handler, error) {
	if opts.APIPrefix == "" {
		opts.APIPrefix = "/api/v1/"
	}
	if opts.API == nil {
		return nil, &configError{message: "desktop handler requires non-nil API handler"}
	}
	if opts.Assets == nil {
		return nil, &configError{message: "desktop handler requires non-nil assets filesystem"}
	}

	// Create a sub-filesystem rooted at "dist" to match frontend/embed.go structure.
	distFS, err := fs.Sub(opts.Assets, "dist")
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route API requests to the backend handler.
		if strings.HasPrefix(r.URL.Path, opts.APIPrefix) {
			opts.API.ServeHTTP(w, r)
			return
		}

		// Try to serve static asset.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if fileExists(distFS, path) {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for unknown paths.
		r2 := new(http.Request)
		*r2 = *r
		u2 := *r.URL
		u2.Path = "/"
		r2.URL = &u2
		r2.RequestURI = "/"
		fileServer.ServeHTTP(w, r2)
	}), nil
}

func fileExists(fsys fs.FS, name string) bool {
	if name == "" {
		return false
	}
	info, err := fs.Stat(fsys, name)
	return err == nil && !info.IsDir()
}

type configError struct {
	message string
}

func (e *configError) Error() string {
	return e.message
}
