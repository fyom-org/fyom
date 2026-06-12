#!/usr/bin/env bash
# Build artifact verification — checks the production bundle for:
#   - No .map files
#   - .gz files exist for compressible assets above compression threshold
#   - index.html references files that exist in dist/assets
# Run: bash scripts/verify_bundle.sh
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="${PROJECT_ROOT}/web/dist"
ASSETS_DIR="${DIST_DIR}/assets"

PASS=0
FAIL=0
WARN=0

pass() { echo "  PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL: $1"; FAIL=$((FAIL + 1)); }
warn() { echo "  WARN: $1"; WARN=$((WARN + 1)); }

echo "=== Checking for .map files ==="
MAP_FILES=$(find "$DIST_DIR" -name "*.map" 2>/dev/null || true)
if [ -z "$MAP_FILES" ]; then
    pass "No .map files in dist"
else
    fail "No .map files in dist (found: $MAP_FILES)"
fi

echo "=== Checking .gz files for compressible assets ==="
TOTAL=0
HAS_GZ=0
HAS_BR=0
SKIP_SMALL=0

for f in "$ASSETS_DIR"/*.js "$ASSETS_DIR"/*.css; do
    [ -f "$f" ] || continue
    TOTAL=$((TOTAL + 1))
    NAME=$(basename "$f")
    SIZE=$(stat -f%z "$f" 2>/dev/null || stat -c%s "$f" 2>/dev/null || echo "0")

    if [ -f "${f}.br" ]; then
        HAS_BR=$((HAS_BR + 1))
    fi
    if [ -f "${f}.gz" ]; then
        HAS_GZ=$((HAS_GZ + 1))
        pass "$NAME has .gz"
    elif [ "$SIZE" -lt 1024 ]; then
        SKIP_SMALL=$((SKIP_SMALL + 1))
        warn "$NAME skipped .gz (size=${SIZE}B < 1024B threshold)"
    else
        fail "$NAME missing .gz (size=${SIZE}B)"
    fi
done

pass "Gzip coverage: $HAS_GZ/$TOTAL files"
if [ "$HAS_BR" -gt 0 ]; then
    pass "Brotli coverage: $HAS_BR/$TOTAL files"
else
    warn "Brotli compression not configured in Vite (only gzip)"
fi
if [ "$SKIP_SMALL" -gt 0 ]; then
    warn "$SKIP_SMALL small files (< 1024B) skipped by compression threshold"
fi

echo "=== Checking index.html asset references ==="
if [ ! -f "$DIST_DIR/index.html" ]; then
    fail "index.html exists"
else
    pass "index.html exists"

    # Extract src="..." and href="..." values for .js/.css assets
    REFS=$(grep -oP '(?:src|href)="[^"]*\.(?:js|css)"' "$DIST_DIR/index.html" 2>/dev/null \
        | sed 's/^[^"]*="//;s/"$//' \
        || true)

    if [ -z "$REFS" ]; then
        warn "No asset references found in index.html"
    else
        MISSING_COUNT=0
        for ref in $REFS; do
            # References are like "/assets/foo.js" or "assets/foo.js"
            # Resolve relative to dist/assets/
            CLEAN_REF="${ref#/assets/}"
            CLEAN_REF="${CLEAN_REF#assets/}"
            FULL_PATH="${ASSETS_DIR}/${CLEAN_REF}"

            if [ -f "$FULL_PATH" ]; then
                pass "Referenced asset exists: $ref"
            else
                fail "Referenced asset MISSING: $ref (looked at: $FULL_PATH)"
                MISSING_COUNT=$((MISSING_COUNT + 1))
            fi
        done
        if [ "$MISSING_COUNT" -eq 0 ]; then
            pass "All index.html asset references resolve"
        fi
    fi
fi

echo ""
echo "=== Results: $PASS passed, $FAIL failed, $WARN warnings ==="
if [ "$FAIL" -gt 0 ]; then
    echo "BUNDLE VERIFICATION FAILED"
    exit 1
fi
echo "BUNDLE VERIFICATION PASSED"
