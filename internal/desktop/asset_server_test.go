package desktop

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestAssetServerHandler_APIRequests(t *testing.T) {
	// Fake API handler that records the request path.
	var apiPath string
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("api:" + r.URL.Path))
	})

	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	staticHandler := NewStaticAssetHandler(assets)
	handler := NewAssetServerHandler(apiHandler, staticHandler)

	tests := []struct {
		name       string
		path       string
		wantAPI    bool
		wantAPIPath string
	}{
		{"health endpoint", "/api/v1/health", true, "/api/v1/health"},
		{"media endpoint", "/api/v1/media", true, "/api/v1/media"},
		{"media with query", "/api/v1/media?id=123", true, "/api/v1/media"},
		{"unknown api path", "/api/v1/unknown", true, "/api/v1/unknown"},
		{"root path", "/settings", false, ""},
		{"assets path", "/assets/app.js", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiPath = ""
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if tt.wantAPI {
				if apiPath != tt.wantAPIPath {
					t.Errorf("expected API handler to receive %q, got %q", tt.wantAPIPath, apiPath)
				}
				if w.Code != http.StatusOK {
					t.Errorf("expected 200, got %d", w.Code)
				}
				if !strings.Contains(w.Body.String(), "api:") {
					t.Errorf("expected API response, got: %s", w.Body.String())
				}
			} else {
				if apiPath != "" {
					t.Errorf("expected static handler, but API handler received: %q", apiPath)
				}
			}
		})
	}
}

func TestAssetServerHandler_PreservesMethodAndHeaders(t *testing.T) {
	var gotMethod, gotContentType string
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	staticHandler := NewStaticAssetHandler(assets)
	handler := NewAssetServerHandler(apiHandler, staticHandler)

	req := httptest.NewRequest("POST", "/api/v1/media", strings.NewReader(`{"id":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if gotMethod != "POST" {
		t.Errorf("expected POST method, got %s", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected application/json, got %s", gotContentType)
	}
}

func TestAssetServerHandler_StaticFallback(t *testing.T) {
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("api"))
	})

	assets := fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("<html>spa</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}
	staticHandler := NewStaticAssetHandler(assets)
	handler := NewAssetServerHandler(apiHandler, staticHandler)

	// Unknown Vue route should fall back to index.html, not API handler.
	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "spa") {
		t.Errorf("expected SPA fallback, got: %s", w.Body.String())
	}
}

func TestAssetServerHandler_APIv1ExactPrefix(t *testing.T) {
	var apiCalled bool
	apiHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		apiCalled = true
	})

	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	staticHandler := NewStaticAssetHandler(assets)
	handler := NewAssetServerHandler(apiHandler, staticHandler)

	// /api/v1 (exact, no trailing slash) should go to API handler.
	req := httptest.NewRequest("GET", "/api/v1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !apiCalled {
		t.Error("expected API handler to be called for /api/v1")
	}
}

func TestAssetServerHandler_NonAPIPath(t *testing.T) {
	var apiCalled bool
	apiHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		apiCalled = true
	})

	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	staticHandler := NewStaticAssetHandler(assets)
	handler := NewAssetServerHandler(apiHandler, staticHandler)

	// /api/other should NOT go to API handler (it's /api/, not /api/v1/).
	req := httptest.NewRequest("GET", "/api/other", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if apiCalled {
		t.Error("API handler should not be called for /api/other")
	}
}
