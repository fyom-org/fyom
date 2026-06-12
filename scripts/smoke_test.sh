#!/usr/bin/env bash
# Production bundle smoke test — verifies the embedded static asset server
# serves index.html, JS/CSS chunks (with brotli/gzip), and correctly 404s
# missing assets. Run: bash scripts/smoke_test.sh
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
DIST_DIR="${PROJECT_ROOT}/web/dist"
BUILD_DIR=$(mktemp -d)
SERVER_BIN="${BUILD_DIR}/fyom-server"
SERVER_PID=""

cleanup() {
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

echo "=== Building frontend ==="
cd "$PROJECT_ROOT/web"
npm run build:full

echo "=== Building Go binary ==="
cd "$PROJECT_ROOT"
CGO_ENABLED=0 go build -o "$SERVER_BIN" ./cmd/fyom/

echo "=== Starting test server ==="
"$SERVER_BIN" > /tmp/smoke_server.log 2>&1 &
SERVER_PID=$()

### Wait for server to be ready (poll until log shows "server starting")
for i in $(seq 1 50); do
    if grep -q "server starting" /tmp/smoke_server.log 2>/dev/null; then
        break
    fi
    sleep 0.1
done

if ! grep -q "server starting" /tmp/smoke_server.log 2>/dev/null; then
    echo "ERROR: Server did not start within 5 seconds"
    cat /tmp/smoke_server.log
    exit 1
fi

### Extract actual port from log
ACTUAL_PORT=$(grep -oP 'addr":"[^"]+' /tmp/smoke_server.log 2>/dev/null | head -1 | grep -oP ':\K\d+' || true)
if [ -z "$ACTUAL_PORT" ]; then
    echo "ERROR: Could not determine server port from log"
    cat /tmp/smoke_server.log
    exit 1
fi
echo "Server listening on port $ACTUAL_PORT"

BASE="http://127.0.0.1:${ACTUAL_PORT}"
PASS=0
FAIL=0

check() {
    local desc="$1"
    local expected="$2"
    local actual="$3"
    if [ "$actual" = "$expected" ]; then
        echo "  PASS: $desc"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc (expected '$expected', got '$actual')"
        FAIL=$((FAIL + 1))
    fi
}

contains() {
    local haystack="$1"
    local needle="$2"
    echo "$haystack" | grep -q "$needle"
}

echo "=== Testing / ==="
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/" 2>/dev/null || echo "000")
check "GET / returns 200" "200" "$HTTP_CODE"
CT=$(curl -s -I "$BASE/" 2>/dev/null | grep -i content-type | tr -d '\r' || true)
if contains "$CT" "text/html"; then
    check "GET / has text/html" "true" "true"
else
    check "GET / has text/html" "true" "false (got: $CT)"
fi
CC=$(curl -s -I "$BASE/" 2>/dev/null | grep -i cache-control | tr -d '\r' || true)
if contains "$CC" "no-cache"; then
    check "GET / has no-cache" "true" "true"
else
    check "GET / has no-cache" "true" "false (got: $CC)"
fi

echo "=== Testing /assets/ ==="
### Find a JS chunk that HAS a .br file (not all chunks are compressed)
### If no .br files exist in this build, try .gz instead, then raw
JS_FILE=""
JS_HAS_BR=false
JS_HAS_GZ=false
for candidate in "$DIST_DIR/assets"/*.js; do
    [ -f "$candidate" ] || continue
    if [ -f "${candidate}.br" ]; then
        JS_FILE="$candidate"
        JS_HAS_BR=true
        break
    fi
    if [ -f "${candidate}.gz" ]; then
        JS_FILE="$candidate"
        JS_HAS_GZ=true
    fi
done
if [ -z "$JS_FILE" ]; then
    echo "ERROR: No JS files found in $DIST_DIR/assets"
    exit 1
fi
JS_NAME=$(basename "$JS_FILE")
echo "  Testing with: $JS_NAME (br=$JS_HAS_BR, gz=$JS_HAS_GZ)"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/assets/$JS_NAME" 2>/dev/null || echo "000")
check "GET /assets/$JS_NAME returns 200" "200" "$HTTP_CODE"
CC=$(curl -s -I "$BASE/assets/$JS_NAME" 2>/dev/null | grep -i cache-control | tr -d '\r' || true)
if contains "$CC" "immutable"; then
    check "GET /assets/$JS_NAME has immutable" "true" "true"
else
    check "GET /assets/$JS_NAME has immutable" "true" "false (got: $CC)"
fi

echo "=== Testing brotli ==="
if [ "$JS_HAS_BR" = "true" ]; then
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Accept-Encoding: br" "$BASE/assets/$JS_NAME" 2>/dev/null || echo "000")
    check "GET /assets/$JS_NAME with br returns 200" "200" "$HTTP_CODE"
    CE=$(curl -s -I -H "Accept-Encoding: br" "$BASE/assets/$JS_NAME" 2>/dev/null | grep -i content-encoding | tr -d '\r' || true)
    if contains "$CE" "br"; then
        check "br response has Content-Encoding: br" "true" "true"
    else
        check "br response has Content-Encoding: br" "true" "false (got: $CE)"
    fi
    CT=$(curl -s -I -H "Accept-Encoding: br" "$BASE/assets/$JS_NAME" 2>/dev/null | grep -i content-type | tr -d '\r' || true)
    if contains "$CT" "javascript"; then
        check "br response has JS MIME" "true" "true"
    else
        check "br response has JS MIME" "true" "false (got: $CT)"
    fi
else
    echo "  SKIP: No .br files in this build (brotli compression not configured in Vite)"
fi

echo "=== Testing gzip ==="
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Accept-Encoding: gzip" "$BASE/assets/$JS_NAME" 2>/dev/null || echo "000")
check "GET /assets/$JS_NAME with gzip returns 200" "200" "$HTTP_CODE"
CE=$(curl -s -I -H "Accept-Encoding: gzip" "$BASE/assets/$JS_NAME" 2>/dev/null | grep -i content-encoding | tr -d '\r' || true)
if contains "$CE" "gzip"; then
    check "gzip response has Content-Encoding: gzip" "true" "true"
else
    check "gzip response has Content-Encoding: gzip" "true" "false (got: $CE)"
fi

echo "=== Testing 404 ==="
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/assets/nonexistent-xyz.js" 2>/dev/null || echo "000")
check "GET /assets/nonexistent-xyz.js returns 404" "404" "$HTTP_CODE"
CC=$(curl -s -I "$BASE/assets/nonexistent-xyz.js" 2>/dev/null | grep -i cache-control | tr -d '\r' || true)
if contains "$CC" "no-store"; then
    check "404 has no-store" "true" "true"
else
    check "404 has no-store" "true" "false (got: $CC)"
fi

echo "=== Testing SPA fallback ==="
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/some/spa/deep/route" 2>/dev/null || echo "000")
check "GET /some/spa/deep/route returns 200" "200" "$HTTP_CODE"
CT=$(curl -s "$BASE/some/spa/deep/route" 2>/dev/null | head -c 50 || true)
if contains "$CT" "<html"; then
    check "SPA route serves HTML" "true" "true"
else
    check "SPA route serves HTML" "true" "false (got: $CT)"
fi

echo "=== Testing HEAD ==="
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -I "$BASE/assets/$JS_NAME" 2>/dev/null || echo "000")
check "HEAD /assets/$JS_NAME returns 200" "200" "$HTTP_CODE"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
if [ "$FAIL" -gt 0 ]; then
    cat /tmp/smoke_server.log
    echo "SMOKE TEST FAILED"
    exit 1
fi
echo "SMOKE TEST PASSED"
