package desktop

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// NewStaticAssetHandler returns an http.Handler that serves static files from
// the given fs.FS. Unknown paths fall back to index.html for Vue history mode
// routing. API paths (starting with /api/) are not handled and return 404.
func NewStaticAssetHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do not serve API routes as static assets.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly.
		cleanPath := path.Clean(r.URL.Path)
		if cleanPath == "/" {
			cleanPath = "index.html"
		} else {
			cleanPath = strings.TrimPrefix(cleanPath, "/")
		}

		// Check if the file exists in the embedded FS.
		if _, err := fs.Stat(assets, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for unknown routes.
		// Use ServeContent to avoid 301 redirects from http.FileServer.
		indexData, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(string(indexData)))
	})
}
