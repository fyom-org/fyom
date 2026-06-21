# syntax=docker/dockerfile:1.25.0

ARG NIX_IMAGE=nixos/nix:2.32.8
ARG RUNTIME_IMAGE=alpine:3.24.1

FROM ${NIX_IMAGE} AS builder

WORKDIR /src

ENV NIX_CONFIG="experimental-features = nix-command flakes"
ENV CI=true
ENV CARGO_TERM_COLOR=always
ENV CARGO_INCREMENTAL=0

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

ENV VERSION=${VERSION}
ENV COMMIT=${COMMIT}
ENV BUILD_TIME=${BUILD_TIME}

COPY . .

# Build a Linux server/container binary.
#
# Important:
# - This is not the Wails desktop build.
# - Container runtime should not depend on WebKitGTK/GTK.
# - CGO_ENABLED=0 keeps the binary suitable for Alpine runtime.
# - Output goes to dist/, not build/ or tmp/.
RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/root/.local/share/pnpm/store \
    --mount=type=cache,target=/root/go/pkg/mod \
    set -eux; \
    nix develop --command bash -lc '\
      set -euo pipefail; \
      export CI=true; \
      export CGO_ENABLED=0; \
      export GOOS=linux; \
      export GOARCH=amd64; \
      mkdir -p dist; \
      pnpm -C frontend install --frozen-lockfile; \
      pnpm -C frontend run build; \
      if [ -x scripts/delete-sourcemaps.js ] || [ -f scripts/delete-sourcemaps.js ]; then \
        node scripts/delete-sourcemaps.js frontend/dist; \
      fi; \
      go build \
        -trimpath \
        -tags server \
        -ldflags "-s -w \
          -X github.com/fyom/fyom/internal/version.Version=${VERSION} \
          -X github.com/fyom/fyom/internal/version.Commit=${COMMIT} \
          -X github.com/fyom/fyom/internal/version.BuildTime=${BUILD_TIME}" \
        -o dist/fyom \
        ./cmd/fyom; \
    '

RUN set -eux; \
    test -f /src/dist/fyom; \
    test -x /src/dist/fyom; \
    mkdir -p /out; \
    cp /src/dist/fyom /out/fyom; \
    chmod 0755 /out/fyom; \
    if command -v strip >/dev/null 2>&1; then \
      strip /out/fyom || true; \
    fi; \
    echo "Collected binary:"; \
    ls -lah /out/fyom; \
    file /out/fyom || true; \
    ldd /out/fyom || true

FROM ${RUNTIME_IMAGE} AS runtime

LABEL org.opencontainers.image.title="fyom"
LABEL org.opencontainers.image.description="fyom server container image"
LABEL org.opencontainers.image.licenses="UNSPECIFIED"

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"

ENV FYOM_CONFIG=/app/configs/fyom.yaml
ENV FYOM_DB_PATH=/app/data/fyom.db
ENV RUST_LOG=info

WORKDIR /app

RUN set -eux; \
    apk add --no-cache ca-certificates tzdata su-exec; \
    addgroup -S -g 10001 fyom; \
    adduser -S -D -H -u 10001 -G fyom fyom; \
    mkdir -p /app/data /app/configs; \
    chown -R fyom:fyom /app

COPY --from=builder /out/fyom /app/fyom
COPY --chown=fyom:fyom configs/fyom.yaml /app/configs/fyom.yaml
COPY docker/entrypoint.sh /usr/local/bin/fyom-entrypoint

RUN set -eux; \
    chmod 0755 /app/fyom; \
    chmod 0755 /usr/local/bin/fyom-entrypoint; \
    chown fyom:fyom /app/fyom; \
    chown -R fyom:fyom /app/data /app/configs

EXPOSE 27402

VOLUME ["/app/data"]

ENTRYPOINT ["fyom-entrypoint"]
CMD ["/app/fyom"]
