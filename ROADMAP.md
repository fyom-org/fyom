```markdown
# fyom — Roadmap

## Design North Star

fyom is not a media server — it is a **media catalog and resource dispatcher**.
The server never transcodes and never proxies media traffic. It only manages
metadata and issues time-limited, signed URLs (Presigned URLs) that allow
clients to stream directly from the source (Local Disk, S3, or Remote fyom Node).

---

# MVP: The Action-Oriented Media Center

The goal of the MVP is to deliver an immersive media experience with
professional-grade library management. Users decide "what to watch" within
3 seconds. Admins have full control over library organization, content
lifecycle, and access permissions. Every media item carries both physical
state (where did I stop?) and emotional state (do I still care?), enriched
by deep, Kodi-compliant metadata.

## Phase 1: Core Foundation & Auth ✅

- [x] Login flow (JWT auth, form validation)
- [x] Setup Wizard (first-run admin creation + library creation + registration toggle)
- [x] Main layout (header, sidebar, content area)
- [x] Import view (path input, trigger button)
- [x] Job status polling component
- [x] RBAC (Admin/User roles, RequireAdmin middleware)
- [x] S3-style Presigned URLs for all media resources (HMAC-SHA256, path-bound signatures)

## Phase 2: Media Catalog & Provider Architecture ✅

- [x] Library list view (dark-themed poster wall grid)
- [x] NFO Parser (Kodi/tinyMediaManager XML format for Movies, Shows, Episodes)
- [x] Detail page for individual media items (backdrop, metadata, overview)
- [x] Show → Episodes hierarchy navigation
- [x] Native HTML5 video player (autoplay, fullscreen, seek via Range requests)
- [x] Search, Type Filter & Sort (Debounced input, AbortController, CASE WHEN SQL)
- [x] `MediaProvider` interface & Registry (concurrency-safe, graceful degradation)
- [x] `LocalProvider` implementation (wraps presign.Signer, SupportsRedirect → false)
- [x] `ImportFS` abstraction (ReadDir, Open, Exists, Join)
- [x] `LocalImportFS` implementation (wraps os.ReadDir, os.Open, os.Stat)
- [x] Season 0 serialization fix (*int + omitempty pattern for JSON zero-values)

## Phase 3: Cloud Native & S3 Storage ✅

- [x] `S3Provider` implementation (AWS SDK v2 presigned URL generation)
- [x] `S3ImportFS` implementation (ListObjectsV2 with delimiter, GetObject, HeadObject)
- [x] Import from S3 bucket (read NFO from object storage, catalog metadata locally)
- [x] CDN integration (replaceCDNHost swaps S3 host for CDN while preserving signatures)
- [x] Admin Provider CRUD API (`/api/v1/admin/providers`)
- [x] Provider config persistence (providers table, factory pattern)

## Phase 4: The 3-Second Experience ✅

- [x] Dashboard View as default landing page
- [x] "Continue Watching" row (horizontal scroll, always first)
- [x] "Recently Added" row (horizontal scroll, freshness)
- [x] `MediaRow.vue` component (unified horizontal scroll paradigm, CSS scroll-snap)
- [x] Enhanced `MediaCard` (year, type badge, progress bar, hover preview)
- [x] Watch progress tracking (position, duration, finished, 10s fire-and-forget)
- [x] Visually dominant "▶ Play" button, collapsed overview/episodes, resume state

## Phase 5: Admin Control Hub ✅

- [x] Dedicated `/admin` layout (visually decoupled from user experience)
- [x] Content/Admin route decoupling (RBAC route guards, localStorage role check)
- [x] System Health Panel (library stats, import job history, storage distribution)
- [x] Provider management page (CRUD, enable/disable toggle)
- [x] Settings page (registration toggle, system configuration API)
- [x] Metadata editing (PUT /admin/media/:id, inline edit on detail page for admins)
- [x] User management (list, promote/demote, delete, last-admin protection)
- [x] Import progress UI (real-time polling with scaleX progress bar)

## Phase 6: Library Management & Access Control ✅

- [x] `libraries` table + `media_items.library_id` (migration, backfill)
- [x] Admin CRUD API for libraries (`/api/v1/admin/libraries`)
- [x] Admin Libraries page (create, delete, refresh, check-missing)
- [x] "local" provider accepted as built-in; Setup wizard creates first library
- [x] Content Lifecycle (cascade/orphan delete, INSERT OR IGNORE refresh, missing detection)
- [x] Per-Library Access Control (`library_permissions`, auto-grant, matrix UI)
- [x] Library-Aware Browsing (sidebar switcher, filtered grid, library tags)

## Phase 7: User Status & Intent ✅

- [x] `user_media_status` table (watching, want_to_watch, watched, dropped, none)
- [x] Status API endpoints (PUT/GET status, GET by-status)
- [x] Auto-transition: none/want → watching on play; → watched on finish (respects dropped)
- [x] Status in API responses (bulk attachment per page)
- [x] MediaCard status icon (top-left, colored circle, click to cycle)
- [x] Status filter in LibraryView (colored buttons)
- [x] Status toggle on detail page (with clear option)
- [x] Dashboard "Want to Watch" row (Continue → Want → Recent)

## Phase 8: Rich Metadata & NFO Compliance ✅

- [x] Kodi-standard NFO parsing (`<ratings>` with child `<value>`, `<uniqueid>`, `<set>`)
- [x] Multi-episode NFO support (`ParseEpisodeNFOs` splitting on `<episodedetails`)
- [x] Deep metadata extraction (actors, genres, studios, uniqueids, mpaa, tagline, set_name)
- [x] Technical metadata from `<fileinfo>` (video/audio/subtitle streams)
- [x] JSON string storage in SQLite for arrays/objects (no join tables)
- [x] API response decoding (JSON strings → typed arrays/objects)
- [x] Frontend genre tags, cast display (avatar initials), MPAA badge, tagline
- [x] Client-side genre filter in LibraryView

---

# Production: Scaling & Ecosystem

## Production Phase 1: Desktop Shell & Tauri

- [ ] Tauri 2 desktop shell (wrapping the Web UI)
- [ ] Local network discovery (find other fyom nodes on LAN via mDNS)
- [ ] System tray / background service management
- [ ] Responsive design improvements (mobile-friendly catalog)
- [ ] Global search (across local, S3, and federated providers)

## Production Phase 2: Native Playback with libmpv

- [ ] libmpv integration via Tauri plugin
- [ ] MPV_EVENT_END_FILE → auto-set status 'watched'
- [ ] Hardware-accelerated decoding (GPU passthrough)
- [ ] Subtitle rendering (ASS/SRT with libass)
- [ ] Audio passthrough (DTS/AC3 to receiver)

## Production Phase 3: Polish & Metadata

- [ ] Per-item metadata overrides (layered override: global → library → item)
- [ ] NFO write-back (Jellyfin-style bidirectional sync)
- [ ] Server-side genre filtering (query param, not client-side)
- [ ] Show-level status aggregation
- [ ] by-status pagination
- [ ] Fix failing integration/auth tests (constructor signature drift)

---

# Future Plan

## Future Plan 1: Federation & Remote Nodes

- [ ] `RemoteFyomProvider` implementation (`SupportsRedirect() → true`)
- [ ] Peer token exchange
- [ ] Metadata proxying (cache remote library metadata locally)
- [ ] 302 Redirect streaming (zero local bandwidth)
- [ ] Cross-instance watch status sync

## Future Plan 2: Advanced Discovery

- [ ] FTS5 full-text search (SQLite virtual table for sub-second search)
- [ ] Collection / franchise grouping (from set_name field)
- [ ] Deduplication detection (same movie in multiple libraries)

## Future Plan 3: Multi-User Experience

- [ ] Watch history timeline (per-user activity feed)
- [ ] Social features (share status, recommend to friends)
- [ ] Parental controls (content ratings, time-based access)
```
