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
- [x] `NFOVideo.Aspect` changed from float64 to string (Kodi format: `"16:9"`)
- [x] `logo_path` column — logo.png/clearlogo.png discovery during import, presigned URL serving

- [x] **Jellyfin NFO spec compliance** — full movie metadata alignment:
      `<sortname>`, `<releasedate>`, `<enddate>`, `<customrating>`,
      `<collectionnumber>`, `<language>`, `<countrycode>`,
      `<dateadded>`, `<lastplayed>`, `<playcount>`, `<userrating>`,
      `<displayorder>`, `<set><overview>`

- [x] 13 new DB columns (migration 0015): `set_overview`, `language`, `country_code`,
      `custom_rating`, `collection_number`, `end_date`, `release_date`,
      `display_order`, `original_title`, `user_rating`, `date_added`,
      `last_played`, `playcount`

- [x] All new fields mapped in `applyMovieNFOFields()` importer
- [x] All new fields exposed in `MediaItemResponse` API
- [x] Admin handler queries updated to use `MediaColumns` constant
      (prevents column-count mismatch on SELECT/Scan)

- [x] Frontend: movie detail page shows original title, language, country code,
      custom rating, release/end dates, play count, user rating, collection/set info

---

### 8.2 Import Pipeline: Normalization ✅

- [x] Title safety: `if nfo.Title != "" { item.Title = nfo.Title }`
      across all media types

- [x] ID merge: old-format fields folded into `UniqueIDs` slice, deduped
      (precedence: `<uniqueid type="...">` > old-format fields)

- [x] Episode `BackdropPath` set to thumbnail path for backdrop rendering
- [x] `FindLogoPath()` — discovers logo.png in show/movie directories

- [x] **NFO file selection fix** — `findMovieNFO()` now prefers `movie.nfo`
      (Kodi/Jellyfin standard) over per-episode NFO files in the same directory.
      Previously, alphabetically-first `.nfo` was selected, causing movie titles
      to be set to per-episode filenames when both `movie.nfo` and per-episode
      NFO files coexisted.

---

### 8.3 API Response Layer ✅

- [x] `ActorResponse` struct (`name`, `role`, `type`, `sort_order`, `thumb`)
- [x] `decodeActors` — filters to `type=Actor`/`""`,
      sorts by `sortorder`, limits to 6

- [x] `decodeGuestStars` — filters to `type=GuestStar`,
      sorts by `sortorder`, limits to 12

- [x] `GuestStars []ActorResponse` as distinct field in `MediaItemResponse`
- [x] `LogoURL` on Provider interface; `logo_url` in `MediaItemResponse`
- [x] All extended metadata fields exposed in `MediaItemResponse`

---

### 8.4 Frontend Metadata Display ✅

- [x] Genre tag pills, MPAA badge, tagline on detail page
- [x] Cast section with avatar-initial circles (filtered to Actors only)
- [x] GuestStars section on episode detail pages
- [x] Client-side genre filter row in `LibraryView`
- [x] Logo image rendering on media detail page
      (replaces text title when present)

- [x] Full-viewport immersive backdrop with deep blur and gradient overlay

---

### 8.5 Episode Detail UX ✅

- [x] Episode row title in `EpisodeList`
      is a router-link → `/media/:episode_id`

- [x] Detail page renders `type=episode` context:
      episode plot, S×E label, aired date, individual rating

- [x] GuestStars section on episode detail
- [x] `"← Back to show"` contextual link on episode detail pages
- [x] Episode backdrop rendered from episode thumbnail

---

### 8.6 Security Hardening ✅

- [x] Role removed from localStorage entirely —
      all admin checks use Pinia store populated server-side
      via `/auth/v1/auth/me`

- [x] Router guard verifies admin role via API call
      on each admin navigation

- [x] `isAdmin` computed in Pinia store
      derived from in-memory user object

- [x] Login page style unified with register page
      (dark card layout)

---

### 8.7 Admin UX Improvements ✅

- [x] `/admin/import` page removed —
      functionality fully superseded by Libraries page
      (create + refresh with live JobStatus progress)

- [x] Auto-refresh schedule selector per library
      (manual/hourly/6h/daily/weekly)

- [x] Server-side scheduler goroutine checks every 60s
      and triggers overdue refreshes

- [x] Admin Media view:
      episodes grouped under parent shows (expandable),
      movies standalone

- [x] Settings save fixed
      (axios 204 No Content response handling)

- [x] Library page empty-state flash eliminated
      (loading guard fix)

---

### 8.8 Static Asset Serving ✅

- [x] **embed FS serving** —
      replaced `http.ServeFile`
      (which internally uses `os.Open` and is unaware of `embed.FS`)
      with `fs.ReadFile(web.Dist, ...)` + direct response streaming.
      This was the root cause of static asset 404s.

- [x] **Content-Type detection** —
      MIME type is now derived from the original asset filename
      (e.g. `.css`, `.js`) instead of compressed filenames
      (`.css.br`, `.js.gz`) which previously caused invalid MIME responses.

- [x] **Compression negotiation** —
      brotli → gzip → raw fallback chain,
      all served directly from embed FS with correct
      `Content-Encoding` and `Vary: Accept-Encoding` headers.

- [x] **HEAD support** —
      added explicit `HEAD` route handling
      (chi `r.Get()` alone does not handle HEAD),
      fixing CSS preload failures and browser asset probing.

- [x] **Cache-Control policy** —
      versioned `assets/*` use immutable long-term caching,
      HTML uses `no-cache`,
      404 responses use `no-store`.

- [x] **Strict static asset fallback** —
      requests under `assets/` never fall back to SPA `index.html`;
      missing assets now correctly return 404,
      preventing CSS preload requests from receiving HTML.

- [x] **Strict non-asset file handling** —
      requests for non-SPA files with extensions
      (`favicon.ico`, `robots.txt`, `manifest.webmanifest`, etc.)
      now return proper 404s instead of incorrectly falling back
      to `index.html`.

- [x] **Immutable cache scope fix** —
      immutable caching now applies only to hashed files under `assets/`,
      preventing accidental long-term caching of `index.html`.

- [x] **NFO file selection** —
      `findMovieNFO()` prefers `movie.nfo`
      over per-episode NFO files that may coexist
      in the same directory.

- [x] **auth_test.go build fix** —
      added missing `libPermRepo` argument
      to `NewAuthService`.

---

## Architecture Note — JSON Storage & the Query Debt

Genres, actors, and other variable-length fields are stored as JSON strings in
SQLite (no join tables). This was a deliberate MVP decision:
fast delivery, zero schema complexity, survives re-import without migration.

Known limitation:
server-side filtering on these fields requires either deserializing rows in
application memory or using SQLite's `json_each()` operator
(SQLite ≥ 3.38, zero schema change).

Current client-side genre filtering is unaffected.

### Resolution Path

- **Now through Production Phase 1**
  client-side filtering is sufficient;
  `json_each()` can be introduced as a drop-in solution
  if server-side filtering becomes necessary before Phase 3.

- **Production Phase 3**
  FTS5 full-text search becomes the primary discovery layer.
  Genre/actor filtering can then be implemented via:
  - SQLite `json_each()` queries
  - or a dedicated `media_genres` join table

  Final approach intentionally deferred until
  real-world query performance becomes the constraint.

---

## Phase 9: Stabilization & Release Readiness

> **Execution Strategy:** Phase 9.1 and 9.2 are complete. The next major
> product milestone is **Tauri shell + sidecar + libmpv** — desktop-native
> playback is the biggest practical value unlock for fyom. The remainder
> of Phase 9 is **not a full blocker** for that work. Only a minimal
> pre-desktop guardrail subset should be completed before desktop
> development begins; the bulk of 9.3, 9.4, and 9.6 is intentionally
> deferred until after the desktop playback milestone.

### 9.1 Static Asset & Build Reliability

- [x] Add regression tests for embedded static asset serving
      (`index.html`, JS, CSS, `.br`, `.gz`, missing assets)

- [x] Verify `HEAD` and `GET` behavior for all static asset types
      (`.js`, `.css`, `.html`, `.svg`, `.json`, `.ico`)

- [x] Add smoke test script for production bundle:
      - `/`
      - `/assets/index-*.js`
      - `/assets/*.css`
      - brotli response
      - gzip response
      - missing asset 404
      - SPA route fallback

- [x] Ensure `index.html` always uses `Cache-Control: no-cache`

- [x] Ensure hashed `assets/*` always use
      `Cache-Control: public, max-age=31536000, immutable`

- [x] Ensure missing static files return `404` with `Cache-Control: no-store`

- [x] Add build artifact verification:
      - no `.map` files in production bundle
      - `.br` and `.gz` exist for compressible assets
      - `index.html` references files that exist in `dist/assets`

- [x] Document embed FS serving rules:
      - never use `http.ServeFile` for embed FS
      - MIME type based on original filename
      - compression path separate from logical request path

- [x] **BUG FIX: static asset MIME type for `.webmanifest`**
      `detectContentType` did not recognize `.webmanifest` and returned
      `application/octet-stream` instead of `application/json`.
      Fixed by treating `.webmanifest` as JSON in the static asset
      content type switch in `internal/server/server.go`.

### 9.2 Importer Robustness & Idempotency

- [x] Re-import idempotency test:
      repeated library refresh does not duplicate media, actors, unique IDs,
      genres, or technical metadata

- [x] NFO fallback behavior tests:
      - `movie.nfo`
      - per-file movie NFO
      - mixed movie + episode NFO files
      - missing title
      - old-format IDs
      - new-format unique IDs

- [x] Episode import edge cases:
      - multi-episode NFO
      - missing season/episode numbers
      - special episodes
      - episode thumbnail backdrop fallback

- [x] Logo discovery tests:
      - `logo.png`
      - `clearlogo.png`
      - movie directory
      - show directory

- [x] Import error reporting:
      failed NFO parse should be visible in job status/logs
      without aborting entire library import

- [x] Add import summary:
      scanned files, imported items, updated items, skipped files,
      parse warnings, duration

- [x] **BUG FIX: show re-import duplication**
      `processShowDir` generated a new UUID on every import run, bypassing
      the `INSERT OR IGNORE` dedup on `file_path`. Fixed by adding
      `FindExistingItem(library_id, file_path, type)` lookup before INSERT,
      reusing the existing show ID and calling `Update` instead. Added
      `MediaRepository.Update()` and `FindExistingItem()` methods.
      Show ID is now stable across re-imports, keeping episode `parent_id`
      foreign keys intact.

> **Phase 9.2 is complete.** Importer robustness objectives met, key
> re-import duplication bug fixed.
>
> **Follow-up notes:**
> - `ImportSummary` currently returns from synchronous `ImportLibrary`
>   only. It is not yet propagated through the async import job / API
>   status surfaces used by the scheduler and admin views. (Tracks
>   under 9.5 Observability — structured import summary in job
>   responses.)
> - `MediaRepository.Update()` excludes `status`. NFO-derived fields
>   such as `playcount` and `last_played` are refreshed on re-import
>   by design. Future product decisions may revisit whether these
>   should be treated as user-state instead. (Not a current-phase
>   blocker.)

### 9.3 API Contract & Test Cleanup

> **Priority:** hygiene-only before desktop. Only fix currently red/failing
> integration-auth tests and constructor-signature drift issues. The rest
> of 9.3 is deferred until after the desktop playback milestone.

- [ ] Fix failing integration/auth tests (constructor signature drift)

- [ ] Fix all failing integration/auth tests
      caused by constructor signature drift

- [ ] Add API response snapshot tests for `MediaItemResponse`

- [ ] Add tests for:
      - actors filtering
      - guest stars filtering
      - logo URL generation
      - extended metadata fields
      - admin media grouping

- [ ] Ensure admin and non-admin authorization behavior is covered

- [ ] Add migration test path from empty DB to latest schema

- [ ] Add migration test path from pre-Phase-8 DB to latest schema

- [ ] Ensure `MediaColumns` constant is used consistently
      for SELECT/Scan safety

### 9.4 Frontend Reliability & UX Polish

> **Priority:** most of 9.4 is intentionally deferred until after the
> desktop playback milestone. Only blocker-level fixes discovered while
> enabling desktop playback should be done before that milestone.

- [ ] Add production-mode frontend smoke test:
      load app, login, open library, open media detail,
      open episode detail, open settings

- [ ] Verify dynamic route chunks load correctly in fresh/incognito browser

- [ ] Verify CSS preload works for all lazy-loaded views

- [ ] Add empty/error/loading states for:
      - library detail
      - media detail
      - provider unavailable
      - failed import job
      - missing poster/backdrop/logo

- [ ] Responsive design improvements (mobile-friendly catalog)

- [ ] Mobile catalog pass:
      - library grid
      - media cards
      - detail backdrop
      - episode list
      - admin library page

- [ ] Normalize frontend API error handling
      with consistent toast/error display

### 9.5 Observability & Diagnostics

> **Priority:** the pre-desktop guardrail subset is `/healthz`, `/readyz`,
> and `/version`. The remaining structured logging / diagnostics work
> can follow after the desktop playback milestone.

- [x] Add `/healthz` endpoint
- [x] Add `/readyz` endpoint for future Tauri sidecar readiness
- [x] Add `/version` endpoint:
      version, commit, build time, Go version, frontend asset hash
- [ ] Add structured logs for:
      - server start
      - database open/migrate
      - library scan start/end
      - import warnings
      - static asset 404s
      - auth failures

- [ ] Add debug-safe startup diagnostics:
      data directory, DB path, web asset mode, listening address

- [ ] Add optional verbose logging flag:
      `--log-level debug`

- [ ] **Propagate ImportSummary through async import job / API
      status responses** (follow-up from Phase 9.2):
      scheduler/admin views should surface scanned_files,
      imported_items, parse_warnings, and duration from
      `ImportSummary`, not just the current done/total counters.
      This may require adding summary columns to import_jobs
      or a separate endpoint.

### 9.6 Configuration & Data Safety

> **Priority:** the pre-desktop guardrail subset is basic safe shutdown
> handling and preventing duplicate refresh jobs for the same library.
> The rest of 9.6 is deferred until after the desktop playback milestone.

- [ ] Define production config precedence:
      CLI flags > env vars > config file > defaults

- [ ] Validate data directory permissions on startup

- [ ] Add DB backup/export command or documented manual backup path

- [x] Add safe shutdown handling:
      stop scheduler, finish in-flight import safely, close DB

- [x] Ensure scheduler does not start duplicate refresh jobs
      for the same library

- [ ] Add config documentation for:
      - data directory
      - server address
      - auth/session settings
      - library paths
      - object storage providers


# Production: Scaling & Ecosystem

## Production Phase 1: Desktop Shell & Tauri

- [x] Tauri 2 desktop shell (wrapping the Web UI)
- [x] Go sidecar `--sidecar` mode with fixed loopback port `127.0.0.1:27403`
- [x] `FYOM_READY` readiness signal and `/readyz` confirmation flow
- [x] Desktop DB path resolution (`fyom.db` colocated with desktop app executable)
- [x] Sidecar bootstrap / runtime lifecycle stabilization
- [x] Tauri system tray, window lifecycle, close-to-tray behavior, and real quit sequencing
- [x] Frontend API base URL adapts to Tauri vs browser runtime
- [x] Desktop auth/network hardening (CORS / preflight handling for sidecar-backed API flows)
- [x] Runtime-aware media/resource URL normalization for Tauri desktop
- [x] Build workflow: `task sidecar`, `task dev:desktop`, `task build:desktop`

> Phase 1 desktop runtime is now functionally in place.
> Tauri shell, Go sidecar bootstrap, desktop DB path handling, tray/window lifecycle,
> explicit sidecar shutdown on real quit, runtime-aware frontend API routing,
> desktop auth flow hardening, and media/resource URL normalization are implemented.
> Remaining work in this phase should stay limited to small runtime polish and
> follow-up hardening, not new architectural expansion.

## Production Phase 2: Native Playback with libmpv

- [ ] libmpv integration via Tauri plugin or private fyom native playback module
- [ ] MPV_EVENT_END_FILE → auto-set status 'watched'
- [ ] Hardware-accelerated decoding (GPU passthrough)
- [ ] Subtitle rendering (ASS/SRT with libass)
- [ ] Audio passthrough (DTS/AC3 to receiver)
- [ ] RawWindowHandle / transparent window overlay

## Production Phase 3: Polish UI/UX & Metadata

- [ ] Per-item metadata overrides (layered override: global → library → item)
- [ ] NFO write-back (Jellyfin-style bidirectional sync)
- [ ] Server-side genre filtering (query param, not client-side)
- [ ] Show-level status aggregation
- [ ] by-status pagination
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

- [ ] Local network discovery (find other fyom nodes on LAN via mDNS)
- [ ] FTS5 full-text search (SQLite virtual table for sub-second search)
- [ ] Collection / franchise grouping (from set_name field)
- [ ] Deduplication detection (same movie in multiple libraries)
- [ ] Global search across local, object storage, and federated providers

## Future Plan 3: Multi-User Experience

- [ ] Watch history timeline (per-user activity feed)
- [ ] Social features (share status, recommend to friends)
- [ ] Parental controls (content ratings, time-based access)
