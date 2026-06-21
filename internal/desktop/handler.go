// Package desktop provides HTTP routing helpers for the fyom desktop runtime.
//
// The desktop runtime serves the embedded frontend and routes API requests to
// the in-process Go backend. This keeps desktop requests same-origin while
// avoiding a separate sidecar HTTP server.
package desktop

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const (
	defaultAPIPrefix = "/api/v1/"
	distDir          = "dist"
	indexFile        = "index.html"
)

// HandlerOptions configures the desktop asset handler.
type HandlerOptions struct {
	// APIPrefix is the URL prefix routed to the backend API.
	//
	// Empty defaults to "/api/v1/".
	APIPrefix string

	// API handles API requests, typically a chi router.
	API http.Handler

	// Assets is the filesystem containing the frontend build output.
	//
	// The filesystem is expected to contain a "dist" subdirectory. This matches
	// the frontend embed package shape, where frontend.Dist embeds dist files.
	Assets fs.FS
}

// NewHandler creates an http.Handler that routes API requests to the backend
// and all other requests to the embedded frontend assets.
//
// Routing rules:
//   - /api/v1 and /api/v1/* are routed to opts.API.
//   - Existing static assets are served from Assets/dist.
//   - Unknown non-API routes fall back to index.html for SPA routing.
func NewHandler(opts HandlerOptions) (http.Handler, error) {
	apiPrefix := normalizeAPIPrefix(opts.APIPrefix)

	if opts.API == nil {
		return nil, fmt.Errorf("desktop handler requires non-nil API handler")
	}

	if opts.Assets == nil {
		return nil, fmt.Errorf("desktop handler requires non-nil assets filesystem")
	}

	distFS, err := fs.Sub(opts.Assets, distDir)
	if err != nil {
		return nil, fmt.Errorf("open embedded frontend dist filesystem: %w", err)
	}

	if !fileExists(distFS, indexFile) {
		return nil, fmt.Errorf("embedded frontend dist is missing %s", indexFile)
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIRequest(r.URL.Path, apiPrefix) {
			opts.API.ServeHTTP(w, r)
			return
		}

		assetPath, ok := cleanAssetPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		if fileExists(distFS, assetPath) {
			serveAsset(fileServer, w, r, assetPath)
			return
		}

		serveAsset(fileServer, w, r, indexFile)
	}), nil
}

func normalizeAPIPrefix(prefix string) string {
	if prefix == "" {
		return defaultAPIPrefix
	}

	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return prefix
}

func isAPIRequest(requestPath string, apiPrefix string) bool {
	apiRoot := strings.TrimSuffix(apiPrefix, "/")

	return requestPath == apiRoot || strings.HasPrefix(requestPath, apiPrefix)
}

func cleanAssetPath(requestPath string) (string, bool) {
	if requestPath == "" || requestPath == "/" {
		return indexFile, true
	}

	cleaned := path.Clean("/" + requestPath)
	cleaned = strings.TrimPrefix(cleaned, "/")

	if cleaned == "." || cleaned == "" {
		return indexFile, true
	}

	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", false
	}

	return cleaned, true
}

func serveAsset(fileServer http.Handler, w http.ResponseWriter, r *http.Request, assetPath string) {
	r2 := r.Clone(r.Context())

	u2 := *r.URL
	u2.Path = "/" + assetPath
	u2.RawPath = ""
	r2.URL = &u2
	r2.RequestURI = ""

	fileServer.ServeHTTP(w, r2)
}

func fileExists(fsys fs.FS, name string) bool {
	if name == "" {
		return false
	}

	info, err := fs.Stat(fsys, name)
	return err == nil && !info.IsDir()
}
