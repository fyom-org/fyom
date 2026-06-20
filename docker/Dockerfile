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

# Build the embedded web assets first, then build a static Linux server binary.
ARG BUILD_COMMAND="cd web && pnpm install --frozen-lockfile && pnpm run build && cd .. && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o tmp/fyom ./cmd/fyom"

ENV VERSION=${VERSION}
ENV COMMIT=${COMMIT}
ENV BUILD_TIME=${BUILD_TIME}
ENV BUILD_COMMAND=${BUILD_COMMAND}

COPY . .

RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/root/.cargo/registry \
    --mount=type=cache,target=/root/.cargo/git \
    --mount=type=cache,target=/root/go/pkg/mod \
    set -eux; \
    nix develop --command bash -lc '\
      set -euo pipefail; \
      export CI=true; \
      echo "Running build command: ${BUILD_COMMAND}"; \
      eval "${BUILD_COMMAND}"; \
    '

RUN set -eux; \
    mkdir -p /out; \
    candidate=""; \
    for path in \
      "./tmp/fyom" \
      "./bin/fyom" \
      "./dist/fyom" \
      "./build/fyom" \
      "./target/x86_64-unknown-linux-musl/release/fyom" \
      "./target/release/fyom" \
      "./src-tauri/target/release/fyom" \
      "./fyom"; \
    do \
      if [ -f "$path" ] && [ -x "$path" ]; then \
        candidate="$path"; \
        break; \
      fi; \
    done; \
    if [ -z "$candidate" ]; then \
      candidate="$(find . -type f -name fyom -perm -111 \
        ! -path './.git/*' \
        ! -path './result/*' \
        ! -path './node_modules/*' \
        ! -path './web/node_modules/*' \
        | sort \
        | head -n 1 || true)"; \
    fi; \
    if [ -z "$candidate" ]; then \
      echo "Unable to locate executable named fyom after build." >&2; \
      find . -maxdepth 5 -type f -perm -111 | sort | sed 's#^\./##' | head -n 200 >&2 || true; \
      exit 1; \
    fi; \
    cp "$candidate" /out/fyom; \
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

EXPOSE 8080

VOLUME ["/app/data"]

ENTRYPOINT ["fyom-entrypoint"]
CMD ["/app/fyom"]
