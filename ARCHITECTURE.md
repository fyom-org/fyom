# fyom — Architecture

## 1. Project Identity

**Name**: fyom (For Your Own Media)
**Tagline**: A lightweight, self-hosted media catalog — no server-side transcoding, no metadata scraping. You bring the files, we bring the library.

## 2. Design Principles

1. **No server-side transcoding** — the server only serves files; playback is entirely client-decoded.
2. **No media scanning / metadata scraping** — assumes files are already organized and tagged by tools like tinyMediaManager. fyom is a pure *importer*.
3. **Dual-mode access** — C/S via web UI (browser) AND a local Tauri desktop client. Both talk to the same REST API.
4. **Single-binary deployment** — Go backend embeds the built Vue frontend; one binary, one SQLite file.
5. **Batteries included, but removable** — sensible defaults, minimal config, but every component is replaceable.

## 3. Technology Layer

| Layer          | Choice                    | Rationale                                      |
|----------------|---------------------------|------------------------------------------------|
| Language       | Go 1.26+                  | Single binary, fast, great stdlib              |
| HTTP Framework | Gin                       | Mature, fast, middleware ecosystem             |
| Frontend       | Vue 3 + Vite              | Lightweight, great DX, easy to embed           |
| Desktop Shell  | Tauri 2                   | Rust-based, tiny bundle, native feel           |
| Database       | SQLite (via modernc.org)  | Zero-config, file-based, no CGO dependency     |
| Migrations     | golang-migrate            | Industry standard, supports embedded FS        |
| Config         | Koanf (YAML/ENV/flags)    | Multi-source, struct-mapped                    |
| Logging        | slog (stdlib)             | Structured, performant, Go 1.21+ native        |
| Linting        | golangci-lint             | Comprehensive, configurable                    |
| Build          | Makefile                  | Simple, universal                              |

## 4. Project Boundaries

### What fyom DOES:
- Import pre-organized media libraries (user points at a directory, fyom reads NFO/XML metadata files).
- Catalog media items (movies, TV shows, episodes) with metadata from sidecar files.
- Serve media files via HTTP (range-request support for seeking).
- Provide a REST API for library management.
- Provide a Vue 3 web UI for browsing and playback.
- Provide a Tauri desktop client wrapping the same API.
- User management (simple username/password, JWT auth).

### What fyom DOES NOT do:
- Real-time transcoding (no FFmpeg on server).
- Metadata scraping / agent lookups (no TMDB/TVDB API calls).
- Media file scanning / probing (no ffprobe).
- Live TV / DVR.
- Plugin system (v1).

## 5. API Contract (v1)

All endpoints under `/api/v1/`. JSON request/response. JWT Bearer <REDACTED>

### Auth
| Method | Path           | Description          |
|--------|----------------|----------------------|
| POST   | /auth/login    | Returns access token |
| POST   | /auth/refresh  | Refresh token        |
| GET    | /auth/me       | Current user info    |

### Library
| Method | Path                     | Description              |
|--------|--------------------------|--------------------------|
| GET    | /library                 | List all media items     |
| GET    | /library/:id             | Get single item          |
| POST   | /library/import          | Trigger import from path |
| DELETE | /library/:id             | Remove item from catalog |

### Media Playback
| Method | Path                | Description                    |
|--------|---------------------|--------------------------------|
| GET    | /media/:id/stream   | Stream file (Range support)    |
| GET    | /media/:id/poster   | Serve poster/thumbnail image   |

### System
| Method | Path          | Description          |
|--------|---------------|----------------------|
| GET    | /health       | Health check         |
| GET    | /version      | Build version info   |

## 6. Data Model (v1)

### users
```sql
CREATE TABLE users (
    id         TEXT PRIMARY KEY,   -- UUID
    username   TEXT NOT NULL UNIQUE,
    password   TEXT NOT NULL,      -- bcrypt hash
    role       TEXT NOT NULL DEFAULT 'user',  -- admin | user
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### media_items
```sql
CREATE TABLE media_items (
    id          TEXT PRIMARY KEY,   -- UUID
    type        TEXT NOT NULL,      -- movie | episode | show
    title       TEXT NOT NULL,
    sort_title  TEXT,
    year        INTEGER,
    overview    TEXT,
    rating      REAL,
    duration    INTEGER,            -- seconds
    file_path   TEXT NOT NULL UNIQUE,
    poster_path TEXT,
    backdrop_path TEXT,
    parent_id   TEXT,               -- episode -> show
    season      INTEGER,            -- episode only
    episode     INTEGER,            -- episode only
    metadata_source TEXT,           -- e.g. "nfo", "xml"
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (parent_id) REFERENCES media_items(id)
);
```

### import_jobs
```sql
CREATE TABLE import_jobs (
    id          TEXT PRIMARY KEY,
    source_path TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | running | done | error
    total_items INTEGER DEFAULT 0,
    done_items  INTEGER DEFAULT 0,
    error_msg   TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
```

## 7. Directory Structure

```
fyom/
├── cmd/
│   └── fyom/           # Main entry point
│       └── main.go
├── internal/
│   ├── config/         # Configuration loading (koanf)
│   ├── handler/        # HTTP handlers (Gin)
│   ├── middleware/     # Gin middleware (auth, logging, recovery)
│   ├── model/          # Data models
│   ├── repository/     # Database access layer
│   ├── service/        # Business logic
│   └── server/         # HTTP server setup & graceful shutdown
├── pkg/
│   ├── logger/         # Structured logger setup
│   ├── errors/         # Unified error types
│   └── response/       # Standard API response helpers
├── web/                # Vue 3 frontend
│   ├── src/
│   ├── package.json
│   └── vite.config.ts
├── migrations/         # SQL migration files
├── configs/            # Default config files
├── scripts/            # Build/dev scripts
├── Makefile
├── go.mod / go.sum
├── .golangci.yml
└── README.md
```

## 8. Deployment Modes

### Mode A: Server + Web UI (C/S)
```
./fyom serve --config fyom.yaml
  -> Starts HTTP server on :8080
  -> Serves embedded Vue frontend at /
  -> Serves REST API at /api/v1/
  -> Browser accesses http://server:8080
```

### Mode B: Tauri Desktop Client (Local Only)
```
./fyom desktop
  -> Opens Tauri window
  -> Connects to local fyom server (or remote)
  -> Same API, native feel
```

### Mode C: Headless Server + External Client
```
./fyom serve --no-ui
  -> API only, no embedded frontend
  -> Tauri client or any HTTP client connects
```
