<div align="center">
<br />

# fyom - *For Your Own Media*

**A lightweight, self-hosted media catalog — no transcoding, no scraping, no fuss.**

[![License](https://img.shields.io/badge/LICENSE-GPL--3.0--only-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-Alpha-orange.svg)]()
[![Go](https://img.shields.io/badge/Go-v1.26+-00add8.svg?logo=go)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-v3-42b883.svg?logo=vuedotjs)](https://vuejs.org)
[![Wails](https://img.shields.io/badge/Wails-v3-CF2A2C?logo=wails&logoColor=CF2A2C)](https://v3.wails.io)

</div>

---

> ⚠️ **Early Access**
>
> fyom is in **early access** (alpha) stage and under active development.
>
> Core features are working, but expect rough edges, breaking changes, and missing polish.
>
> We ship fast, iterate faster, and welcome brave early adopters.
>
> If something breaks, open an issue — you're helping build this thing.

---

## What Is fyom?

**fyom is not a media server — it's a media catalog and resource dispatcher.**

Your media files are already organized. Your NFO metadata is already tagged. fyom respects that. It doesn't transcode. It doesn't proxy. It doesn't scrape. It catalogs what you have and hands your media player a time-limited, signed URL to stream directly from the source.

The result? A beautiful, snappy library interface that lets you decide **what to watch in under 3 seconds** — and then actually watches it.

**Two modes, one binary:**

- **Server/Headless** — `./fyom serve`, browser opens `http://your-server:27402`, done.
- **Desktop** — Wails shell wraps the same UI, launches your favorite external player (mpv, IINA, VLC, PotPlayer — pick your weapon).

---

## Architecture at a Glance

```
┌──────────────────────────────────────────────────────────────────┐
│                                                                  │
│  ┌────────────┐    ┌──────────────┐    ┌──────────────────────┐  │
│  │  Vue 3 UI  │◄──►│  Go Backend  │◄──►│  External Player     │  │
│  │  (embedded)│    │              │    │                      │  │
│  └────────────┘    └──────┬───────┘    └──────────────────────┘  │
│                           │                                      │
└───────────────────────────┼──────────────────────────────────────┘
                            │ SQLite (fyom.db)
                    ┌───────▼───────┐
                    │  Media Items  │
                    │  Users        │
                    │  Libraries    │
                    │  Import Jobs  │
                    └───────────────┘
```

**Technology choices that earn their keep:**

| Layer | Choice | Why |
|---|---|---|
| Backend | Go | Single binary, fast, great stdlib |
| HTTP | Chi | Lightweight, composable, stdlib-compatible |
| Frontend | Vue   + Vite | Reactive, great DX, trivially embeddable |
| Desktop | Wails   | Go lang native, tiny bundle, native window lifecycle |
| Database | SQLite (modernc) | Zero-config, file-based, no CGO dependency |
| Auth | JWT | Stateless, sidecar-friendly, works in both modes |
| Build | Taskfile | Easy to use, one `task` to rule them all |
| i18n | vue-i18n | en, zh, ja — hot-swap without refresh |

---

## Features

### ✅ What's Here Now

**Library & Catalog**
- Dark-themed poster wall grid that feels like a real media center
- NFO import (Kodi / tinyMediaManager format) — movies, shows, episodes
- Per-item metadata: genres, studios, cast, crew, ratings, tags, sets/collections
- Jellyfin-compliant NFO spec with 13 extended DB columns
- Show → Episodes hierarchy with season navigation
- Search with debounced input and AbortController
- Type filter, genre filter, sort (title, year, rating, date added)
- Dashboard with "Continue Watching", "Want to Watch", "Recently Added" rows (CSS scroll-snap horizontal)
- Media detail pages: immersive backdrop, cast avatars, guest stars, guest star sections, technical metadata
- Logo rendering (clearlogo.png / logo.png auto-discovery)

**Playback**
- HTML5 video player with Range request support (browser mode)
- External player launcher via Wails (mpv, IINA, VLC, PotPlayer — whatever your OS defaults to)
- Presigned URLs (HMAC-SHA256, path-bound signatures) — works with S3, local disk, CDN
- Watch progress tracking with auto-status transitions (none → watching → watched)

**Access Control**
- JWT authentication with bcrypt password hashing
- RBAC: Admin / User roles enforced server-side (no client-side role storage)
- Per-library access control with practical UI
- RBAC route guards verified via API on every admin navigation
- Bootstrap session bridge for desktop first-run auto-auth
- Force-password-change modal for bootstrap-created admins

**Import Pipeline**
- Context-driven scan pipeline: filesystem snapshot → classify → parse → reconcile
- Re-import idempotent (INSERT OR IGNORE with stable show IDs)
- Grouping/container directory traversal
- Import job progress with real-time polling (scaleX progress bar)
- Auto-refresh schedules (manual / hourly / 6h / daily / weekly)

**Admin**
- Dedicated `/admin` layout (visually decoupled from user experience)
- System health panel (library stats, import job history, storage distribution)
- Provider management (CRUD, enable/disable)
- User management (list, promote/demote, delete, last-admin protection)
- Library CRUD with missing-item detection
- Settings page (registration toggle, system default language, per-library auto-refresh)

**Internationalization**
- 3 locales: English, Simplified Chinese (中文), Japanese (日本語)
- Locale-aware date/time/duration/number/file-size formatting
- Dynamic `supported_locales` driven by backend
- Language switcher with flags, keyboard navigation, ARIA semantics
- No page refresh needed — hot-swap via vue-i18n

**Desktop Runtime**
- Wails system tray with close-to-tray and real quit sequencing
- Go sidecar with FYOM_READY readiness protocol and /readyz confirmation
- Graceful sidecar shutdown on quit (no orphan processes)
- `kill_on_drop(true)` defensive safeguard
- Resource URL normalization for Wails mode (relative `/api/v1/...` → `http://127.0.0.1:27403/...`)

**Security**
- Role removed from localStorage entirely — all admin checks use Pinia store populated server-side
- CORS middleware for desktop WebView (chi preflight fix)
- Strict static asset fallback (no HTML for 404s on assets)
- Immutable cache for hashed assets, `no-cache` for HTML

### 🚧 What's Coming

- See [ROADMAP](ROADMAP.md) for more information

---

## Quick Start

### Prerequisites

- **Go** 1.26+
- **Node.js** + **pnpm**
- **Wails** (For desktop builds)
- **Task** — `go install github.com/go-task/task/v3/cmd/task@latest`
- **Nix (Optional but recommended)** — <https://nixos.org> (Use `nix develop` to get into full development environment)

### Server / Headless Mode

```bash
# Clone
git clone https://github.com/fyom-org/fyom.git
cd fyom

# Build frontend + backend (one binary, embedded UI)
task build

# Run — first boot creates admin user with random password printed to stdout
./build/fyom --db-path ./build/fyom.db

# Open browser → http://localhost:27402/
```

### Desktop Mode

```bash
# Build sidecar binary first
task sidecar

# Launch Wails dev mode with mpv explicitly
task dev:desktop

# Or uses OS default player
task dev:desktop-system-player


# Build desktop bundle
task build:desktop
```

### Verify It Works

```bash
# Run all Go tests
task test

# Run frontend unit tests
cd frontend && pnpm exec vitest run

# Smoke test (production bundle)
task smoke
```

---

## Configuration

### Server [fyom.yaml](configs/fyom.yaml)

```yaml
server:
  addr: ":27402"

database:
  path: "./fyom.db"

auth:
  jwt_secret: "change-me-in-production"
  token_ttl: 24h

libraries:
  - name: "Movies"
    type: "movie"
    path: "/media/movies"
    provider: "local"
  - name: "TV Shows"
    type: "show"
    path: "/media/tv"
    provider: "local"
```

### Desktop [fyom-desktop.json](configs/fyom-desktop.json)

```json
{
  "externalPlayer": {
    "kind": "mpv",
    "program": "mpv",
    "args": [],
    "appendDefaultMpvArgs": true
  }
}
```

Environment overrides (highest priority):
- `FYOM_EXTERNAL_PLAYER` — explicit player binary path
- `FYOM_EXTERNAL_PLAYER_ARGS` — args template
- `FYOM_MPV_BIN` — mpv binary path
- `FYOM_DESKTOP_CONFIG` — explicit config file path

---

## API Contract

All endpoints live under `/api/v1/`. JWT Bearer token required for protected routes.

| Method | Path | Description |
|---|---|---|
| `POST` | `/auth/login` | Returns access token |
| `GET` | `/auth/me` | Current user info |
| `GET` | `/library` | List media (paginated, filterable) |
| `GET` | `/library/:id` | Single item detail |
| `GET` | `/media/:id/stream` | Presigned stream URL (Range supported) |
| `GET` | `/media/:id/poster` | Poster/thumbnail image |
| `GET` | `/media/:id/backdrop` | Backdrop image |
| `GET` | `/media/:id/progress` | Get watch progress |
| `PUT` | `/media/:id/progress` | Record progress (`{position,duration,finished}` or `{played:true}`) |
| `PUT` | `/media/:id/status` | Set user status (watching/want_to_watch/watched/dropped/none) |
| `POST` | `/library/import` | Trigger NFO import |
| `GET` | `/library/:id/episodes` | List episodes for a show |
| `GET` | `/health` | Health check |
| `GET` | `/version` | Build info |

---

## Project Structure

```
fyom/
├── cmd/fyom/              # Main entry point
├── configs/               # Default config files
├── build/                 # Wails build for cross-platforms
├── frontend/              # Vue 3 frontend
│   ├── src/
│   │   ├── views/         # PlayerView, LibraryView, DashboardView, admin views
│   │   ├── components/    # MediaCard, MediaRow, EpisodeList, LanguageSwitcher
│   │   ├── composables/   # useLocale, useLocaleFormat, useNotifications
│   │   ├── stores/        # user, system
│   │   ├── lib/           # API client, runtime detection, resource URL normalization
│   │   ├── plugins/       # i18n
│   │   └── locales/       # json files of locales
│   └── tests/             # Vitest unit tests
├── internal/
│   ├── handler/           # HTTP handlers (Chi)
│   ├── middleware/        # Auth, CORS, error, permissions
│   ├── model/             # Data models
│   ├── repository/        # Database access layer
│   ├── service/           # Business logic (auth, import, bootstrap)
│   └── server/            # HTTP server setup & graceful shutdown
├── pkg/
│   ├── errors/            # Taxonomy of 51 error codes across 7 domains
│   ├── locale/            # Supported locales validation
│   └── response/          # Standard API response envelope (error_code + message)
├── migrations/            # Embedded SQL migrations
└── Taskfile.yml           # Build, dev, test, lint, CI tasks
```

---

## Design Boundaries

### What fyom DOES:
- Import pre-organized media libraries (you point at a directory, fyom reads NFO/XML sidecar files)
- Catalog media items with metadata from those sidecar files
- Serve presigned URLs for direct streaming from source (local disk, S3, CDN)
- Provide a polished Vue 3 web UI for browsing, managing, and playing
- Provide a Wails desktop client wrapping the same UI with external player delegation
- User management (JWT auth, RBAC, per-library access control)

### What fyom DOES NOT do (At least not in early access stage):
- Real-time transcoding (no FFmpeg on the server)
- Metadata scraping / agent lookups (no TMDB/TVDB API calls)
- Media file probing (no ffprobe)
- Live TV / DVR
- Plugin system

---

## Contributing

Contributions are welcome — especially in:

- Bug reports and edge cases, wrong behavior (Especially in NFO parsing)
- Additional locale translations (just add a JSON file + document)
- Provider implementations (More provider are considered: SMB, Webdav and so on)
- UI/UX polish (mobile responsiveness, empty states, error handling)
- Backend reliability (Robustness and resilience in handling boundary cases)

```bash
# Minimal development workflow
nix develop       # Setup nix develop environment
task build        # Build fyom binary
task dev          # Go backend with Air hot-reload
task dev:frontend      # Vite dev server (separate terminal)
task lint         # Lint code by `golangci-lint`
task test         # Go test suite
```

---

## License

fyom is licensed under the [GPL-3.0-only](LICENSE) license.

---

## Acknowledgements

- [Jellyfin](https://github.com/jellyfin/jellyfin)
- [Kyoo](https://github.com/zoriya/Kyoo)
- [Kodi](https://github.com/xbmc/xbmc)
- [mpv](https://github.com/mpv-player/mpv)
- [tinyMediaManager](https://www.tinymediamanager.org)
