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
- [x] Content/Admin route decoupling (RBAC route guards, server-side role verification)
- [x] System Health Panel (library stats, import job history, storage distribution)
- [x] Provider management page (CRUD, enable/disable toggle)
- [x] Settings page (registration toggle, system configuration API)
- [x] Metadata editing (PUT /admin/media/:id, inline edit on detail page for admins)
- [x] User management (list, promote/demote, delete, last-admin protection)
- [x] Import progress UI (real-time polling with scaleX progress bar)

## Phase 6: Library Management & Access Control ✅

- [x] `libraries` table + `media_items.library_id` (migration, backfill)
- [x] Admin CRUD API for libraries (`/api/v1/admin/libraries`)
- [x] Admin Libraries page (create, delete, refresh, check-missing, auto-refresh schedule)
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

### 8.1 NFO Parser & Data Model ✅

- [x] Kodi-standard `<ratings>` block (child `<value>/<votes>`, named rating sets)
- [x] `<uniqueid type="...">` multi-source ID parsing (new Kodi format)
- [x] Old-format ID fields: `<imdb_id>`, `<tvdbid>`, `<tmdbid>`, `<id>` (classic Kodi)
- [x] `<set>` support (franchise/collection grouping pointer, with `<overview>`)
- [x] Multi-episode NFO file support (`ParseEpisodeNFOs` — splits on
      `<episodedetails>`, single-episode fallback)

- [x] Deep metadata fields: genres, studios, mpaa, tagline, outline, premiered,
      set_name, directors, credits, tags, countries

- [x] Actor extraction from `<actor>` blocks (name, role, type, sortorder, thumb)
- [x] Technical stream metadata from `<fileinfo>`
      (video codec/res/fps, audio codec/channels/lang, subtitle lang list)

- [x] JSON string storage in SQLite for variable-length arrays/objects
      (deliberate MVP trade-off — see Architecture Note below)

- [x] `actorsToJSON`, `uniqueIDsToJSON`, `subtitlesToJSON`, `stringsToJSON`
      helpers in importer

- [x] `NFOActor.Type` field — distinguishes Actor/GuestStar/Producer/Director/Writer
- [x] `NFOActor.SortOrder` xml tag corrected to `xml:"sortorder"`
- [x] `NFOVideo.Aspect` changed from float64 to string (Kodi format: "16:9")
- [x] `logo_path` column — logo.png/clearlogo.png discovery during import, presigned URL serving

- [x] **Jellyfin NFO spec compliance** — full movie metadata alignment:
      `<sortname>`, `<releasedate>`, `<enddate>`, `<customrating>`,
      `<collectionnumber>`, `<language>`, `<countrycode>`,
      `<dateadded>`, `<lastplayed>`, `<playcount>`, `<userrating>`,
      `<displayorder>`, `<set><overview>`

- [x] 13 new DB columns (migration 0015): set_overview, language, country_code,
      custom_rating, collection_number, end_date, release_date, display_order,
      original_title, user_rating, date_added, last_played, playcount

- [x] All new fields mapped in `applyMovieNFOFields()` importer
- [x] All new fields exposed in `MediaItemResponse` API
- [x] Admin handler queries updated to use `MediaColumns` constant
      (prevents column-count mismatch on SELECT/Scan)

- [x] Frontend: movie detail page shows original title, language, country code,
      custom rating, release/end dates, play count, user rating, collection/set info

### 8.2 Import Pipeline: Normalization ✅

- [x] Title safety: `if nfo.Title != "" { item.Title = nfo.Title }` across all media types
- [x] ID merge: old-format fields folded into UniqueIDs slice, deduped
      (precedence: `<uniqueid type="...">` > old-format fields)
- [x] Episode `BackdropPath` set to thumbnail path for backdrop rendering
- [x] `FindLogoPath()` — discovers logo.png in show/movie directories

### 8.3 API Response Layer ✅

- [x] `ActorResponse` struct (name, role, type, sort_order, thumb)
- [x] `decodeActors` — filters to `type=Actor`/`""`, sorts by `sortorder`, limits to 6
- [x] `decodeGuestStars` — filters to `type=GuestStar`, sorts by `sortorder`, limits to 12
- [x] `GuestStars []ActorResponse` as distinct field in `MediaItemResponse`
- [x] `LogoURL` on Provider interface; `logo_url` in `MediaItemResponse`
- [x] All extended metadata fields exposed in `MediaItemResponse`

### 8.4 Frontend Metadata Display ✅

- [x] Genre tag pills, MPAA badge, tagline on detail page
- [x] Cast section with avatar-initial circles (filtered to Actors only)
- [x] GuestStars section on episode detail pages
- [x] Client-side genre filter row in LibraryView
- [x] Logo image rendering on media detail page (replaces text title when present)
- [x] Full-viewport immersive backdrop with deep blur and gradient overlay

### 8.5 Episode Detail UX ✅

- [x] Episode row title in EpisodeList is a router-link → `/media/:episode_id`
- [x] Detail page renders `type=episode` context: episode plot, S×E label,
      aired date, individual rating
- [x] GuestStars section on episode detail
- [x] "← Back to show" contextual link on episode detail pages
- [x] Episode backdrop rendered from episode thumbnail

### 8.6 Security Hardening ✅

- [x] Role removed from localStorage entirely — all admin checks use Pinia store
      populated server-side via `/auth/v1/auth/me`
- [x] Router guard verifies admin role via API call on each admin navigation
- [x] `isAdmin` computed in Pinia store derived from in-memory user object
- [x] Login page style unified with register page (dark card layout)

### 8.7 Admin UX Improvements ✅

- [x] `/admin/import` page removed — functionality fully superseded by
      Libraries page (create + refresh with live JobStatus progress)
- [x] Auto-refresh schedule selector per library (manual/hourly/6h/daily/weekly)
- [x] Server-side scheduler goroutine checks every 60s and triggers overdue refreshes
- [x] Admin Media view: episodes grouped under parent shows (expandable), movies standalone
- [x] Settings save fixed (axios 204 No Content response handling)
- [x] Library page empty-state flash eliminated (loading guard fix)

---

**Architecture Note — JSON Storage & the Query Debt**

Genres, actors, and other variable-length fields are stored as JSON strings in
SQLite (no join tables). This was a deliberate MVP decision: fast delivery,
zero schema complexity, survives re-import without migration.

Known limitation: server-side filtering on these fields requires either
deserializing rows in application memory or using SQLite's `json_each()`
operator (available SQLite ≥ 3.38, zero schema change). Current client-side
genre filtering is unaffected.

Resolution path:
- **Now through Production Phase 1**: client-side filtering is sufficient;
  `json_each()` available as a drop-in if a server-side filter endpoint is
  needed before Phase 3.
- **Production Phase 3**: FTS5 full-text search covers the primary discovery
  use case. Genre/actor server-side filtering addressed at that point —
  either via `json_each()` queries or a targeted `media_genres` join table.
  Decision deferred until actual query performance becomes the constraint.

---

# Production: Scaling & Ecosystem

## Production Phase 1: Desktop Shell & Tauri

- [ ] Tauri 2 desktop shell (wrapping the Web UI)
- [ ] Go sidecar: `--sidecar` mode, fixed port 27403, `FYOM_READY` signal
- [ ] Tauri system tray, window lifecycle, close-to-tray behavior
- [ ] Frontend API base URL adapts to Tauri vs browser context
- [ ] Build workflow: `make sidecar`, `make dev`, `make tauri-build`
- [ ] Local network discovery (find other fyom nodes on LAN via mDNS)
- [ ] Responsive design improvements (mobile-friendly catalog)
- [ ] Global search (across local, S3, and federated providers)

## Production Phase 2: Native Playback with libmpv

- [ ] libmpv integration via Tauri plugin
- [ ] MPV_EVENT_END_FILE → auto-set status 'watched'
- [ ] Hardware-accelerated decoding (GPU passthrough)
- [ ] Subtitle rendering (ASS/SRT with libass)
- [ ] Audio passthrough (DTS/AC3 to receiver)
- [ ] RawWindowHandle / transparent window overlay

## Production Phase 3: Polish & Metadata

- [ ] Per-item metadata overrides (layered override: global → library → item)
- [ ] NFO write-back (Jellyfin-style bidirectional sync)
- [ ] Server-side genre filtering (query param, not client-side)
- [ ] Show-level status aggregation
- [ ] by-status pagination
- [ ] Fix failing integration/auth tests (constructor signature drift)
- [ ] Code signing / notarization

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
