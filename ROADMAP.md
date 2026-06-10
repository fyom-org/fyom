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
state (where did I stop?) and emotional state (do I still care?).

## Phase 1: Core Foundation & Auth ✅

Build the foundational UI shell, authentication, and the primary user action —
importing a pre-organized media library.

- [x] Login flow (JWT auth, form validation)
- [x] Setup Wizard (first-run admin creation + library creation + registration toggle)
- [x] Main layout (header, sidebar, content area)
- [x] Import view (path input, trigger button)
- [x] Job status polling component
- [x] RBAC (Admin/User roles, RequireAdmin middleware)
- [x] S3-style Presigned URLs for all media resources (HMAC-SHA256, path-bound signatures)

## Phase 2: Media Catalog & Provider Architecture ✅

Browse imported movies and shows with poster art using Jellyfin/Kodi standard
directory parsing. Decouple fyom from local filesystem assumptions via the
`MediaProvider` interface.

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

Expand beyond NAS boundaries. Allow media to live in B2, Wasabi, MinIO, or any
S3-compatible object storage.

- [x] `S3Provider` implementation (AWS SDK v2 presigned URL generation)
- [x] `S3ImportFS` implementation (ListObjectsV2 with delimiter, GetObject, HeadObject)
- [x] Import from S3 bucket (read NFO from object storage, catalog metadata locally)
- [x] CDN integration (replaceCDNHost swaps S3 host for CDN while preserving signatures)
- [x] Admin Provider CRUD API (`/api/v1/admin/providers`)
- [x] Provider config persistence (providers table, factory pattern)

**Architecture Win:** S3 perfectly aligns with fyom's no-proxying principle.
The Go server generates an S3 Presigned URL and returns it; the client streams
hundreds of Mbps directly. The server stays at <1% CPU, acting only as a
metadata dispatcher. `S3Provider` also serves as the design reference for the
`RemoteFyomProvider` signing protocol in Future Plan 1.

## Phase 4: The 3-Second Experience ✅

Transform the library from a file manager into an action-oriented media center.
Core metric: from opening the page to clicking play in under 3 seconds.

### 4.1 Action-Oriented Dashboard ✅
- [x] Dashboard View as default landing page (replacing raw library grid)
- [x] "Continue Watching" row (horizontal scroll, always first)
- [x] "Recently Added" row (horizontal scroll, freshness)
- [x] `MediaRow.vue` component (unified horizontal scroll paradigm, CSS scroll-snap)

### 4.2 Card Evolution & Hover Preview ✅
- [x] Enhanced `MediaCard`: display year, type badge, progress bar
- [x] Hover/Focus interaction: card scales up, play icon overlay
- [x] Minimal auxiliary info: cover + year/type only, restrained spacing
- [x] scaleX progress bar (GPU-composited, no layout thrash)

### 4.3 Watch Progress Tracking ✅
- [x] Backend `watch_progress` data model (position, duration, finished)
- [x] `PUT /api/v1/media/:id/progress` (player timeupdate fire-and-forget, 10s interval)
- [x] `GET /api/v1/library/continue` (in-progress items)
- [x] Progress state reflected on cards (progress bar) and detail page

### 4.4 Detail Page Simplification ✅
- [x] Visually dominant "▶ Play" button (purple glow, 18px, primary action)
- [x] Interest-based expansion: overview collapsed to 2-line clamp, click to expand
- [x] Episodes collapsed by default (season headers with episode counts, click to expand)
- [x] Resume state displayed ("▶ Resume", "Resume from 42m / 1h 58m")
- [x] Progress bar in backdrop area for partially watched items

**Design Principle:** The interface asks "what do you want to watch", not
"what do you want to manage". All browsing is horizontal scrolling + light
filtering; all states are expressed instantly through visuals.

## Phase 5: Admin Control Hub ✅

A media library admin panel should not be a collection of configurations, but a
control hub with clear status and direct operations.

- [x] Dedicated `/admin` layout (visually decoupled from user experience)
- [x] Content/Admin route decoupling (RBAC route guards, localStorage role check)
- [x] System Health Panel (library stats, import job history, storage distribution)
- [x] Provider management page (CRUD, enable/disable toggle)
- [x] Settings page (registration toggle, system configuration API)
- [x] Metadata editing (PUT /admin/media/:id, inline edit on detail page for admins)
- [x] User management (list, promote/demote, delete, last-admin protection)
- [x] Import progress UI (real-time polling with scaleX progress bar)

**Design Principle:** The admin panel tells you if the system is healthy and
where exceptions are. You only need one click to fix them.

## Phase 6: Library Management & Access Control ✅

fyom treats all imported media as a single flat pool. This phase introduces the
Library as a first-class entity — the organizational unit that binds storage,
metadata rules, and access permissions together.

### 6.1 Library Entity Model ✅
- [x] `libraries` table (id, name, type, provider_id, source_path, metadata_source)
- [x] `media_items.library_id` foreign key (migration, backfill)
- [x] Admin CRUD API for libraries (`/api/v1/admin/libraries`)
- [x] Admin Libraries page (create, delete, refresh, check-missing)
- [x] "local" provider accepted as built-in (not stored in providers table)
- [x] Setup wizard creates first library (optional, enabled by default)

### 6.2 Content Lifecycle ✅
- [x] Delete single media item (`DELETE /api/v1/admin/media/:id` — cascades episodes + progress)
- [x] Delete entire library (cascade/orphan modes via prompt)
- [x] Re-import / refresh library (INSERT OR IGNORE for idempotent scanning)
- [x] Missing item detection (`media_items.status` field, check-missing endpoint)
- [x] Missing Items admin page (list, filter by library, batch delete)
- [x] Missing items hidden from user-facing views (status='available' filter)

### 6.3 Per-Library Access Control ✅
- [x] `library_permissions` table (user_id, library_id, can_view)
- [x] Library list API respects permissions (users only see accessible libraries)
- [x] Admin Permissions page (user × library matrix with toggle)
- [x] Auto-grant: new users get access to all existing libraries
- [x] Auto-grant: new libraries grant access to all existing users
- [x] Permission middleware (ResolvePermissions, allowedLibraryIDs in context)
- [x] 404 not 403 for inaccessible items (don't leak existence)

### 6.4 Library-Aware Browsing ✅
- [x] Sidebar library switcher (2+ libraries, emoji icons by type)
- [x] Dashboard rows scoped to accessible libraries
- [x] Library grid filtered by `library_id` query parameter
- [x] Library name in page title + breadcrumb navigation
- [x] Library tags on dashboard cards (when 2+ libraries exist)
- [x] Single-library grace: no library UI chrome when only 1 library
- [x] `library_id` in API responses for frontend library mapping

**Design Principle:** The Library is the atomic unit of organization. Every media
item belongs to exactly one library. Permissions are granted at the library level,
not the item level. This gives admins the power to create distinct spaces
(e.g., "Kids Movies" vs "Documentaries") with independent access rules.

## Phase 7: User Status & Intent 🔧 *Current*

watch_progress tracks physical playback position. User status tracks
emotional intent. Both are needed for a complete media experience.

### 7.1 Status Data Model
- [x] `user_media_status` table (user_id, media_item_id, status, created_at, updated_at)
- [x] Status enum: watching, want_to_watch, watched, dropped, none (default)
- [x] `idx_ums_user_status` index on (user_id, status) for query performance
- [ ] PUT /api/v1/media/:id/status — set status
- [ ] GET /api/v1/media/:id/status — get status for current user
- [ ] GET /api/v1/library/by-status — items filtered by status
- [ ] Auto-transition: playing an item sets status to 'watching' (if none/want)
- [ ] Auto-transition: video ended + finished=true sets status to 'watched'
- [ ] Auto-transition respects 'dropped' — never overrides manual intent

### 7.2 Status in Browsing UI
- [ ] Status icon on MediaCard (top-left, colored circle, click to cycle)
- [ ] Status filter in LibraryView toolbar (All / Watching / Want / Watched / Dropped)
- [ ] Status toggle on detail page (one-click set)
- [ ] Event-driven prop updates (emit only, parent manages data source)

### 7.3 Status-Aware Dashboard
- [ ] "Continue Watching" row shows items with status=watching
- [ ] "Want to Watch" row shows items with status=want_to_watch
- [ ] Row order: Continue → Want → Recent
- [ ] Rows merge physical progress with emotional intent

**Design Note:** Status and progress are complementary. Progress answers
"where did I stop?" Status answers "do I still care?" An item can have
progress but be 'dropped', or be 'want_to_watch' with no progress.

**Tauri Integration Point:** When libmpv fires MPV_EVENT_END_FILE,
the desktop client calls PUT /media/:id/status with {status:'watched'}.
The backend model already exists — zero new work needed.

**Known MVP Limitations:**
- Status filter and Type filter cannot combine (by-status ignores type param)
- Show vs Episode status semantics undefined (both are independent for now)
- by-status endpoint has no pagination (hard limit 20)

---

# Production: Scaling & Ecosystem

Features for hardening fyom for production deployment and wrapping the
experience in a native desktop shell.

## Production Phase 1: Desktop Shell & Tauri

Wrap the Web UI in a native desktop application. The Go server runs as a
Tauri sidecar for local-only mode.

- [ ] Tauri 2 desktop shell (wrapping the Web UI)
  > **Lifecycle note:** in Local-Only mode the embedded Go server must be
  > launchable as a Tauri sidecar. Reserve a library-mode entry point in
  > `cmd/fyom` so the Tauri integration does not require core refactoring.
- [ ] Local network discovery (find other fyom nodes on LAN via mDNS)
- [ ] System tray / background service management
- [ ] Responsive design improvements (mobile-friendly catalog)
- [ ] Global search (across local, S3, and federated providers)

## Production Phase 2: Native Playback with libmpv

Replace the HTML5 `<video>` player with libmpv for professional-grade playback.

- [ ] libmpv integration via Tauri plugin
- [ ] MPV_EVENT_END_FILE → auto-set status 'watched'
- [ ] Hardware-accelerated decoding (GPU passthrough)
- [ ] Subtitle rendering (ASS/SRT with libass)
- [ ] Audio passthrough (DTS/AC3 to receiver)
- [ ] Playback speed control (0.5x – 2.0x)
- [ ] Chapter navigation from NFO `<epbookmark>` data

**Architecture Note:** libmpv is the same engine powering mpv, Celluloid, and
IINA. It handles every codec, every container, every subtitle format. fyom's
"no server transcoding" principle means the player must handle everything —
libmpv is the only player engine that can.

## Production Phase 3: Polish & Metadata

- [ ] Per-item metadata overrides (layered override: global → library → item)
- [ ] NFO write-back (Jellyfin-style bidirectional sync)
- [ ] Collection / franchise grouping (Marvel, Star Wars, etc.)
- [ ] Smart playlists / saved filters
- [ ] Status + Type filter combination
- [ ] Show-level status aggregation (show watched when all episodes watched)
- [ ] by-status pagination

---

# Future Plan

Architectural visions for the next era. These require significant design work
before implementation and depend on MVP being stable and complete.

## Future Plan 1: Federation & Remote Nodes

Break down data silos between friends. Connect multiple fyom instances together
without duplicating terabytes of files.

- [ ] `RemoteFyomProvider` implementation (`SupportsRedirect() → true`)
- [ ] Peer token exchange (authenticate with a remote fyom instance)
- [ ] Metadata proxying (cache remote library metadata locally for fast browsing)
- [ ] 302 Redirect streaming (local fyom returns `Location:` pointing to remote
  Presigned URL — zero local bandwidth)
- [ ] Cross-instance watch status sync (requires MVP Phase 7 status model)

**Architecture Note:** When a user hits Play on a remote item, the local fyom
server reads `SupportsRedirect() == true`, calls the provider, and returns an
HTTP 302. The client pulls the video stream directly from the remote server —
the local node never touches media bytes.

## Future Plan 2: Advanced Metadata & Discovery

- [ ] Full Kodi NFO template support (actors, ratings, uniqueid, fanart)
- [ ] Jellyfin `<uniqueid>` multi-source ID resolution (IMDb, TMDb, TVDb)
- [ ] `<MediaInfo>` technical metadata (codec, resolution, audio channels)
- [ ] FTS5 full-text search (SQLite virtual table for sub-second search)
- [ ] Deduplication detection (same movie in multiple libraries)

## Future Plan 3: Multi-User Experience

- [ ] Watch history timeline (per-user activity feed)
- [ ] Social features (share status, recommend to friends)
- [ ] Parental controls (content ratings, time-based access)
- [ ] Per-user watchlist (curated collection independent of status)
```

---
