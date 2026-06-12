# Static Asset Serving Rules

This document defines the production static asset serving contract for fyom's
embedded frontend bundle. All rules apply to the `staticFileHandler` in
`internal/server/server.go` and the `web.Dist` embed.FS.

## Rules

1. **Never use `http.ServeFile` for embed FS.**
   `http.ServeFile` uses `os.Open` which does not know about `embed.FS`.
   Use `fs.ReadFile(fsys, path)` + `w.Write(data)` instead.

2. **MIME type is determined from the original logical filename.**
   When serving `assets/app.js.br`, the Content-Type must be
   `text/javascript; charset=utf-8`, NOT `application/brotli`.
   The `.br` / `.gz` suffix is compression, not a file type.

3. **Compression path is separate from logical request-path semantics.**
   Request `/assets/app.js` with `Accept-Encoding: br` serves
   `dist/assets/app.js.br` with `Content-Encoding: br`, but MIME and
   cache-control are derived from `assets/app.js`.

4. **Missing `/assets/...` files must 404, not SPA fallback.**
   `/assets/does-not-exist.js` returns 404 with `Cache-Control: no-store`.
   Only non-asset routes (e.g. `/library/123`) fall back to `index.html`.

5. **`index.html` must be `Cache-Control: no-cache`.**
   The SPA entry point must never be cached aggressively.

6. **Hashed assets must be `Cache-Control: public, max-age=31536000, immutable`.**
   Any file under `/assets/` with a content-hash segment in its filename
   (e.g. `index-abc123.js`) gets immutable caching.

7. **Missing static files must be `Cache-Control: no-store`.**
   404 responses for static assets include `Cache-Control: no-store`.

8. **`HEAD` must behave like `GET` for headers, but with no response body.**
   `HEAD /assets/app.js` returns the same status, Content-Type, and
   Cache-Control as `GET /assets/app.js`, but with an empty body.

## File Layout

The embedded FS root is `web/dist/` (via `//go:embed dist`).

| Logical request | FS path       |
|-----------------|---------------|
| `/`             | `dist/index.html` |
| `/assets/foo.js`| `dist/assets/foo.js` |
| `/favicon.ico`  | `dist/favicon.ico` |

## Compression Selection Order

1. If `Accept-Encoding` contains `br` and `<file>.br` exists → serve brotli
2. Else if `Accept-Encoding` contains `gzip` and `<file>.gz` exists → serve gzip
3. Else → serve uncompressed original file
