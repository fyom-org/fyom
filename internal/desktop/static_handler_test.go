package desktop

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticAssetHandler_ServesIndex(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	handler := NewStaticAssetHandler(assets)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "index") {
		t.Fatalf("expected index.html content, got: %s", w.Body.String())
	}
}

func TestStaticAssetHandler_ServesExistingAsset(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("<html>index</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}
	handler := NewStaticAssetHandler(assets)

	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "console.log") {
		t.Fatalf("expected JS content, got: %s", w.Body.String())
	}
}

func TestStaticAssetHandler_SPAFallback(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa</html>")},
	}
	handler := NewStaticAssetHandler(assets)

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "spa") {
		t.Fatalf("expected index.html fallback, got: %s", w.Body.String())
	}
}

func TestStaticAssetHandler_APINotServed(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	handler := NewStaticAssetHandler(assets)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for API route, got %d", w.Code)
	}
}

func TestStaticAssetHandler_MissingAssetFallback(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>fallback</html>")},
	}
	handler := NewStaticAssetHandler(assets)

	// Request a non-existent file — should fall back to index.html.
	req := httptest.NewRequest("GET", "/nonexistent.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "fallback") {
		t.Fatalf("expected index.html fallback, got: %s", w.Body.String())
	}
}

func TestStaticAssetHandler_DotPathBlocked(t *testing.T) {
	// Ensure path traversal is blocked.
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}
	handler := NewStaticAssetHandler(assets)

	req := httptest.NewRequest("GET", "/../../../etc/passwd", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should either be 404 (blocked) or serve index.html (SPA fallback),
	// but never serve the actual file.
	body := w.Body.String()
	if strings.Contains(body, "root:") || strings.Contains(body, "daemon:") {
		t.Fatalf("path traversal not blocked, got: %s", body)
	}
}

// ensure os and fstest are used.
var _ = os.ModeDir
