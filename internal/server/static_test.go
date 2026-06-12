package server

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/fyom/fyom/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestFS creates an in-memory FS that mimics the dist/ directory structure.
func newTestFS() fs.FS {
	return fstest.MapFS{
		"dist/index.html":                  &fstest.MapFile{Data: []byte("<html>SPA</html>")},
		"dist/assets/app-abc123.js":         &fstest.MapFile{Data: []byte("// js")},
		"dist/assets/app-abc123.js.br":      &fstest.MapFile{Data: []byte{0x01, 0x02, 0x03}},
		"dist/assets/app-abc123.js.gz":      &fstest.MapFile{Data: []byte{0x04, 0x05, 0x06}},
		"dist/assets/style-def456.css":      &fstest.MapFile{Data: []byte("body{}")},
		"dist/assets/style-def456.css.br":   &fstest.MapFile{Data: []byte{0x07, 0x08}},
		"dist/assets/style-def456.css.gz":   &fstest.MapFile{Data: []byte{0x09, 0x0a}},
		"dist/assets/pinia-B427umY2.js":     &fstest.MapFile{Data: []byte("// pinia")},
		"dist/assets/pinia-B427umY2.js.br": &fstest.MapFile{Data: []byte{0x0b}},
		"dist/assets/pinia-B427umY2.js.gz": &fstest.MapFile{Data: []byte{0x0c}},
		"dist/favicon.ico":                  &fstest.MapFile{Data: []byte{0x00, 0x00, 0x01, 0x00}},
	}
}

func setupStaticRouter(fsys fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Get("/*", staticFileHandler(noopLogger(), fsys))
	r.Head("/*", staticFileHandler(noopLogger(), fsys))
	return r
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func assertHeader(t *testing.T, rec *httptest.ResponseRecorder, name, expected string) {
	t.Helper()
	got := rec.Header().Get(name)
	assert.Contains(t, got, expected, "header %q should contain %q, got %q", name, expected, got)
}

func assertHeaderNotPresent(t *testing.T, rec *httptest.ResponseRecorder, name string) {
	t.Helper()
	got := rec.Header().Get(name)
	assert.Empty(t, got, "header %q should be absent, got %q", name, got)
}

func doRequest(t *testing.T, handler http.Handler, method, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ─── Regression tests for static asset serving (Phase 8.8) ───

func TestStatic_GetJS(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/app-abc123.js", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "text/javascript")
	assertHeaderNotPresent(t, rec, "Content-Encoding")
	assertHeader(t, rec, "Cache-Control", "immutable")
}

func TestStatic_GetJS_Brotli(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/app-abc123.js", "br")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Encoding", "br")
	assertHeader(t, rec, "Content-Type", "text/javascript")
	assertHeader(t, rec, "Vary", "Accept-Encoding")
	assertHeader(t, rec, "Cache-Control", "immutable")
	assert.NotContains(t, rec.Header().Get("Content-Type"), "brotli")
}

func TestStatic_GetJS_Gzip(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/app-abc123.js", "gzip")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Encoding", "gzip")
	assertHeader(t, rec, "Content-Type", "text/javascript")
	assertHeader(t, rec, "Vary", "Accept-Encoding")
}

func TestStatic_GetJS_NoEncoding(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/app-abc123.js", "")
	assert.Equal(t, 200, rec.Code)
	assertHeaderNotPresent(t, rec, "Content-Encoding")
	assert.Equal(t, []byte("// js"), rec.Body.Bytes())
}

func TestStatic_HeadJS(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "HEAD", "/assets/app-abc123.js", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "text/javascript")
	assertHeader(t, rec, "Cache-Control", "immutable")
	assert.Empty(t, rec.Body.Bytes())
}

func TestStatic_MissingAsset404(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/does-not-exist.js", "")
	assert.Equal(t, 404, rec.Code)
	assertHeader(t, rec, "Cache-Control", "no-store")
	assert.NotContains(t, rec.Body.String(), "<html>")
}

func TestStatic_GetCSS(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/style-def456.css", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "text/css")
	assertHeader(t, rec, "Cache-Control", "immutable")
}

func TestStatic_GetCSS_Brotli(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/style-def456.css", "br")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Encoding", "br")
	assertHeader(t, rec, "Content-Type", "text/css")
	assert.NotContains(t, rec.Header().Get("Content-Type"), "brotli")
}

func TestStatic_GetCSS_Gzip(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/style-def456.css", "gzip")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Encoding", "gzip")
	assertHeader(t, rec, "Content-Type", "text/css")
}

func TestStatic_FaviconExists(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/favicon.ico", "")
	assert.Equal(t, 200, rec.Code)
	assertHeaderNotPresent(t, rec, "immutable")
}

func TestStatic_RobotsTxtMissing(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/robots.txt", "")
	// robots.txt doesn't exist and is not under assets/ — SPA fallback
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Cache-Control", "no-cache")
	assert.Contains(t, rec.Body.String(), "<html>")
}

func TestStatic_SPARoute(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/some/spa/route", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Cache-Control", "no-cache")
	assert.Contains(t, rec.Body.String(), "<html>")
}

func TestStatic_Root(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "text/html")
	assertHeader(t, rec, "Cache-Control", "no-cache")
}

func TestStatic_IndexHTML_NoImmutable(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/index.html", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "text/html")
	assertHeaderNotPresent(t, rec, "immutable")
	assertHeader(t, rec, "Cache-Control", "no-cache")
}

func TestStatic_MethodNotAllowed(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "POST", "/assets/app-abc123.js", "")
	assert.Equal(t, 405, rec.Code)
	allow := rec.Header().Get("Allow")
	assert.NotEmpty(t, allow, "Allow header should be present on 405")
	// chi lists allowed methods; should include at least one of GET or HEAD
	assert.True(t, strings.Contains(allow, "GET") || strings.Contains(allow, "HEAD"),
		"Allow should contain GET or HEAD, got %q", allow)
}

func TestStatic_ContentTypeBasedOnOriginalName(t *testing.T) {
	r := setupStaticRouter(newTestFS())

	rec := doRequest(t, r, "GET", "/assets/app-abc123.js", "br")
	assert.Equal(t, 200, rec.Code)
	ct := rec.Header().Get("Content-Type")
	assert.True(t, strings.Contains(ct, "javascript"),
		"Content-Type for .js.br should be javascript, got %q", ct)

	// .css.br should still be text/css
	rec = doRequest(t, r, "GET", "/assets/style-def456.css", "br")
	assert.Equal(t, 200, rec.Code)
	ct = rec.Header().Get("Content-Type")
	assert.True(t, strings.Contains(ct, "css"),
		"Content-Type for .css.br should be text/css, got %q", ct)
}

// ─── MediaColumns consistency audit ───

func TestStatic_MediaColumnsConstantDefined(t *testing.T) {
	cols := repository.MediaColumns
	require.NotEmpty(t, cols)
	assert.Contains(t, cols, "id")
	assert.Contains(t, cols, "title")
	assert.Contains(t, cols, "set_overview")
	assert.Contains(t, cols, "language")
	assert.Contains(t, cols, "country_code")
	assert.Contains(t, cols, "custom_rating")
	assert.Contains(t, cols, "collection_number")
	assert.Contains(t, cols, "end_date")
	assert.Contains(t, cols, "release_date")
	assert.Contains(t, cols, "display_order")
	assert.Contains(t, cols, "original_title")
	assert.Contains(t, cols, "user_rating")
	assert.Contains(t, cols, "date_added")
	assert.Contains(t, cols, "last_played")
	assert.Contains(t, cols, "playcount")
}

// ─── Phase 9.1: additional regression tests ───

func TestStatic_HeadRoot(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "HEAD", "/", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "text/html")
	assertHeader(t, rec, "Cache-Control", "no-cache")
	assert.Empty(t, rec.Body.Bytes(), "HEAD / must not write a body")
}

func TestStatic_HeadMatchesGetHeaders(t *testing.T) {
	r := setupStaticRouter(newTestFS())

	// GET
	getRec := doRequest(t, r, "GET", "/assets/app-abc123.js", "")
	// HEAD
	headRec := doRequest(t, r, "HEAD", "/assets/app-abc123.js", "")

	assert.Equal(t, getRec.Code, headRec.Code, "status codes must match")
	assert.Equal(t, getRec.Header().Get("Content-Type"), headRec.Header().Get("Content-Type"),
		"Content-Type must match")
	assert.Equal(t, getRec.Header().Get("Cache-Control"), headRec.Header().Get("Cache-Control"),
		"Cache-Control must match")
	assert.Empty(t, headRec.Body.Bytes(), "HEAD must not write a body")
}

func TestStatic_GetSVG(t *testing.T) {
	fsys := fstest.MapFS{
		"dist/index.html":    &fstest.MapFile{Data: []byte("<html></html>")},
		"dist/assets/logo.svg": &fstest.MapFile{Data: []byte("<svg></svg>")},
	}
	r := setupStaticRouter(fsys)
	rec := doRequest(t, r, "GET", "/assets/logo.svg", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "image/svg+xml")
	assertHeader(t, rec, "Cache-Control", "immutable")
}

func TestStatic_GetJSON(t *testing.T) {
	fsys := fstest.MapFS{
		"dist/index.html":               &fstest.MapFile{Data: []byte("<html></html>")},
		"dist/assets/manifest.webmanifest": &fstest.MapFile{Data: []byte(`{"name":"test"}`)},
	}
	r := setupStaticRouter(fsys)
	rec := doRequest(t, r, "GET", "/assets/manifest.webmanifest", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Content-Type", "application/json")
}

func TestStatic_MissingAsset_NoStore(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/totally-missing.js", "")
	assert.Equal(t, 404, rec.Code)
	assertHeader(t, rec, "Cache-Control", "no-store")
	assert.NotContains(t, rec.Body.String(), "<html>", "404 must not return SPA HTML")
}

func TestStatic_IndexHTML_NeverImmutable(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/index.html", "")
	assert.Equal(t, 200, rec.Code)
	assertHeaderNotPresent(t, rec, "immutable")
	assertHeader(t, rec, "Cache-Control", "no-cache")
}

func TestStatic_HashedAsset_NeverNoCache(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/app-abc123.js", "")
	assert.Equal(t, 200, rec.Code)
	assertHeader(t, rec, "Cache-Control", "immutable")
	// Must never be no-cache
	assert.NotContains(t, rec.Header().Get("Cache-Control"), "no-cache")
}

func TestStatic_MissingAsset_Never200(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/does-not-exist.css", "")
	assert.NotEqual(t, 200, rec.Code, "missing asset must not return 200")
	assert.Equal(t, 404, rec.Code)
}

func TestStatic_MissingAsset_NeverSPAFallback(t *testing.T) {
	r := setupStaticRouter(newTestFS())
	rec := doRequest(t, r, "GET", "/assets/missing.js", "")
	assert.NotContains(t, rec.Body.String(), "<html>", "missing asset must not return SPA HTML")
}

func TestStatic_CompressedAsset_MIME_NotEncoding(t *testing.T) {
	r := setupStaticRouter(newTestFS())

	// When serving brotli, MIME must still be based on .js not .br
	rec := doRequest(t, r, "GET", "/assets/app-abc123.js", "br")
	assert.Equal(t, 200, rec.Code)
	ct := rec.Header().Get("Content-Type")
	assert.Contains(t, ct, "javascript", "MIME must be javascript, not brotli")
	assert.NotContains(t, ct, "brotli", "MIME must not mention brotli")

	// When serving gzip, MIME must still be based on .css not .gz
	rec = doRequest(t, r, "GET", "/assets/style-def456.css", "gzip")
	assert.Equal(t, 200, rec.Code)
	ct = rec.Header().Get("Content-Type")
	assert.Contains(t, ct, "css", "MIME must be text/css, not gzip")
	assert.NotContains(t, ct, "gzip", "MIME must not mention gzip")
}
