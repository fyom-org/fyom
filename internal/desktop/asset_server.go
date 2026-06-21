package desktop

import (
	"net/http"
	"strings"
)

// apiPrefix is the path prefix that routes to the backend API.
const apiPrefix = "/api/v1"

// NewAssetServerHandler returns an http.Handler that dispatches:
//   - /api/v1/* → api handler (Chi router)
//   - everything else → static handler (embedded Vue assets)
//
// The api handler receives the original request including method, headers,
// body, and query string without modification.
func NewAssetServerHandler(api http.Handler, static http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiPrefix+"/") || r.URL.Path == apiPrefix {
			api.ServeHTTP(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}
