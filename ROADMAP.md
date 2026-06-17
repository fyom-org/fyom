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
> playback is the biggest practical value unlock for fyom. Phase 9.3 and
> 9.6 have been elevated: auth truth, authorization boundaries,
> session rehydration, and self-hosted safety are now concrete
> pre-desktop requirements. Only the remaining 9.4 polish items and
> non-critical 9.6 extras stay deferred until after the desktop
> playback milestone.

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

- [x] Rebuild importer around a context-driven pipeline:
      - filesystem snapshot
      - typed candidate classification
      - metadata parsing by entity type
      - reconcile / persistence stage

- [x] Add explicit media path semantics for imported entities:
      - `root_path`
      - `primary_path`
      - `nfo_path`
      while keeping legacy `file_path` compatibility for API/storage transition

- [x] Fix episode import path handling:
      prevent `dir/file/file` double-nesting and ensure episode playable
      path points to the real media file

- [x] Fix show / movie / episode classification ownership:
      show subtree files are no longer re-imported as standalone movies,
      season/episode traversal is context-aware, and library-type policy is
      enforced during classification

- [x] Fix episode NFO application and unique ID parsing:
      - episode title/overview/rating/aired now respect episode NFO
      - `imdbid` XML tag fixed
      - IMDB / TMDB / TVDB fields no longer cross-map incorrectly

- [x] Add explicit grouping / container directory traversal:
      importer now treats wrapper / intermediate directories as transparent
      traversal layers rather than terminal unknowns, while ensuring such
      directories are never persisted as media items

- [x] Add grouping/container regression coverage:
      - one extra grouping directory
      - multiple nested grouping directories
      - grouping directory must not become persisted media
      - show-only policy under grouping directories
      - movie-only policy under grouping directories

- [x] Fix provider integrity for imported media:
      `providers.id='local'` is now seeded/made durable so library/media rows
      no longer depend on disabled foreign keys to remain valid

- [x] **BUG FIX: show re-import duplication**
      `processShowDir` generated a new UUID on every import run, bypassing
      the `INSERT OR IGNORE` dedup on `file_path`. Fixed by adding
      `FindExistingItem(library_id, file_path, type)` lookup before INSERT,
      reusing the existing show ID and calling `Update` instead. Added
      `MediaRepository.Update()` and `FindExistingItem()` methods.
      Show ID is now stable across re-imports, keeping episode `parent_id`
      foreign keys intact.

- [x] Split oversized importer implementation into stage-focused files
      inside `internal/service/` to make snapshot/classify/metadata/reconcile
      control flow inspectable and maintainable

> **Phase 9.2 is complete.** Importer robustness objectives met, key
> re-import duplication bug fixed, context-driven scan pipeline established,
> grouping/container directory traversal supported, and importer regression
> coverage significantly strengthened.
>
> **Follow-up notes:**
> - `ImportSummary` is now persisted on async import jobs and exposed through
>   job status responses, including `scanned_files`, `imported_items`,
>   `updated_items`, `skipped_files`, `parse_warnings`, and `duration_ms`.
>   Remaining UI work can wire scheduler/admin surfaces to those fields.
> - `MediaRepository.Update()` excludes `status`. NFO-derived fields
>   such as `playcount` and `last_played` are refreshed on re-import
>   by design. Future product decisions may revisit whether these
>   should be treated as user-state instead. (Not a current-phase
>   blocker.)
> - Historical pre-fix media rows may still need a dedicated repair /
>   backfill path if an existing DB already contains old corrupted
>   importer output. Preventing new corruption is complete; repairing
>   legacy corrupted rows is follow-up work.

### 9.3 API Contract & Test Cleanup

> **Priority:** before further desktop/native expansion, fyom must harden
> auth truth, authorization boundaries, and stale-session behavior.
> This section is no longer only hygiene. The minimum required scope is:
> - admin vs non-admin authorization coverage
> - deleted/missing-user token rejection
> - setup/bootstrap state must not inherit old auth
> - API/auth contract tests must reflect current DB-backed identity truth
>
> The goal is not enterprise IAM completeness. The goal is to ensure fyom's
> core security invariants hold under realistic self-hosted usage:
> browser local storage, long-lived sessions, DB resets, setup re-entry,
> desktop sidecar runtime, and mixed admin/non-admin use.

- [x] Fix failing integration/auth/startup test drift and restore
      green `go test ./...` execution across the repository

- [x] Remove machine-specific hardcoded media paths from default tests;
      keep real-media corpus coverage opt-in via environment variable
      (`FYOM_TEST_MEDIA_ROOT`) rather than hardcoded `/root/...` paths

- [ ] Add API response snapshot tests for `MediaItemResponse`

- [ ] Add tests for:
      - actors filtering
      - guest stars filtering
      - logo URL generation
      - extended metadata fields
      - admin media grouping

- [x] Ensure admin and non-admin authorization behavior is covered:
      - unauthenticated access to admin routes is rejected
      - non-admin access to admin read routes is rejected or constrained
      - non-admin access to admin mutate routes is rejected
      - admin access succeeds only when backed by a current DB admin user
      - deleted/missing-user token is rejected
      - downgraded admin token is rejected after role change
      - DB reset / zero-user state does not preserve old admin access
      - setup/bootstrap state does not inherit stale admin tokens

- [x] Add auth/session regression coverage for stale-session invalidation:
      - server rejects orphaned tokens whose backing user no longer exists
      - frontend clears stored auth state on unauthorized stale-session response
      - browser-visible login state cannot outlive server-side auth truth

- [x] Add migration test path from empty DB to latest schema

- [x] Add migration test path from pre-Phase-8 DB to latest schema

- [ ] Ensure `MediaColumns` constant is used consistently
      for SELECT/Scan safety

> **Follow-up notes:**
> - This section is intentionally focused on DB-backed auth truth and
>   authorization correctness, not on advanced identity features such as
>   refresh-token rotation, multi-device session management, SSO, or
>   audit-trail expansion.
> - Frontend auth UX polish is secondary to server-side authorization
>   correctness. The server must reject stale/orphaned tokens even if the
>   browser still holds them.

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

- [x] Add debug-safe startup diagnostics:
      data directory, DB path, web asset mode, listening address

- [x] Add optional verbose logging flag:
      `--log-level debug`

- [x] **Propagate ImportSummary through async import job / API
      status responses** (follow-up from Phase 9.2):
      scheduler/admin views can now surface `scanned_files`,
      `imported_items`, `updated_items`, `skipped_files`,
      `parse_warnings`, and `duration_ms` from persisted import jobs,
      while `done_items` / `total_items` progress counters remain intact.

### 9.6 Configuration & Data Safety

> **Priority:** before broader desktop/native and public-network usage,
> fyom should provide safe operational defaults for self-hosted users.
> The minimum target is not "perfect security", but reliable safety rails:
> - predictable config precedence
> - no dangerous identity carry-over after DB/bootstrap reset
> - safe startup/runtime defaults
> - clear operator-facing configuration/docs for auth and data paths
>
> This section should bias toward secure defaults and explicit operator
> understanding, especially for local-network, family/shared, and
> accidentally-public self-hosted deployments.

- [ ] Define production config precedence:
      CLI flags > env vars > config file > defaults

- [ ] Validate data directory permissions on startup

- [ ] Add DB backup/export command or documented manual backup path

- [x] Add safe shutdown handling:
      stop scheduler, finish in-flight import safely, close DB

- [x] Ensure scheduler does not start duplicate refresh jobs
      for the same library

- [x] Prevent duplicate built-in `local` provider registration
      during server bootstrap:
      DB seed / ensure remains separate from in-memory provider
      registry registration, avoiding startup panic on fresh DBs

- [x] Add explicit auth/session safety rules for reset/bootstrap states:
      - DB reset / zero-user state must invalidate effective admin access
      - setup/bootstrap mode must not trust stale existing tokens
      - old sessions must not survive identity-store replacement
      - auth truth must remain anchored to current DB state

- [ ] Document secure deployment expectations for self-hosted use:
      - local-only vs LAN vs public exposure
      - reverse proxy / TLS expectations
      - desktop sidecar vs browser runtime auth boundaries
      - safe handling of long-lived browser sessions

- [ ] Add config documentation for:
      - data directory
      - server address
      - auth/session settings
      - library paths
      - object storage providers

> **Follow-up notes:**
> - This section is not intended to introduce full enterprise-grade
>   security controls. It is meant to ensure safe defaults and clear
>   behavior for real fyom deployment modes.
> - Advanced security controls such as session revocation sets, token
>   epochs, multi-device session management, rate limiting, or hardened
>   public-edge policies can follow later if fyom's deployment surface
>   expands further.

### 9.7 Native Playback Failure Fallback (Pre-Desktop Guardrail)

> **Priority:** guardrail-only before desktop. Ensures no black screen if native
> player init fails. Full libmpv implementation remains in Production Phase 2.

- [x] Add explicit native player state model:
      `idle / initializing / ready / failed / unavailable`
- [x] Add `attempted` flag to distinguish browser-by-default from
      browser-as-fallback-after-native-failure
- [x] Reuse repo's canonical `isTauriEnvironment()` for runtime detection
      instead of ad-hoc globals
- [x] Add `tryInitializeNativePlayer()` bridge function encapsulating
      invoke/catch/failure-mapping in one place
- [x] PlayerView renders loading state during native init, HTML5 fallback
      on failure, native surface on success
- [x] Visible fallback banner on native init failure:
      `Native player unavailable, using browser playback`
- [x] Fallback banner only shown when native was attempted and failed;
      not shown during normal browser playback
- [x] PlayerView attempts native init exactly once per mount lifecycle;
      no retry loops
- [x] Wire vitest into frontend: `package.json` test script,
      `vitest.config.ts`, jsdom environment
- [x] Add 5 component-level tests for PlayerView fallback behavior:
      browser-default, loading, fallback-on-failure, native-success, no-retry
- [x] Add 25 helper/bridge tests for native player state model
- [x] All 30 frontend tests pass (`vitest run`); frontend build passes

> **Follow-up notes:**
> - `tryInitializeNativePlayer` calls `invoke('play_media')` which is a
>   placeholder. The actual libmpv Tauri command name and contract will be
>   defined in Production Phase 2.
> - Desktop runtime E2E verification not possible in current environment
>   (Tauri CLI not available). Component tests validate the fallback path
>   through mocking.
> - Historical corrupted DB rows from pre-fix importer output still need
>   a dedicated repair/backfill path (out of scope for this guardrail task).

### 9.8 First-Run Bootstrap & Entry Routing Simplification

> **Priority:** high. The historical `/setup` browser flow has become a
> disproportionate source of frontend route complexity and state-machine
> fragility. fyom should stop treating first-run bootstrap as a browser
> route concern and instead move initial admin provisioning into the backend.
>
> The goal of this phase is to:
> - remove `/setup` from normal frontend routing
> - simplify production entry semantics to `anonymous -> /login`,
>   `authenticated -> /`
> - keep bootstrap logic in the backend where it belongs
> - preserve database portability across machines
> - support both server/headless mode and Tauri desktop mode without
>   reintroducing route-state pollution

- [x] **Remove `/setup` as a frontend/browser route**
      - `/setup` must no longer exist as a normal route in the SPA
      - direct browser access to `/setup` must no longer act as a valid
        initialization flow
      - initialized systems must never route anonymous users to `/setup`

- [x] **Replace browser setup with backend first-run bootstrap**
      - when `userCount == 0`, backend performs initial admin bootstrap
      - frontend should no longer depend on `/api/v1/system/initialize`
        as part of normal browser first-run flow
      - bootstrap becomes a backend/system concern, not a route concern

- [x] **Server mode first-run bootstrap**
      - on first boot with zero users:
        - create admin user `admin`
        - generate a strong random password
        - print credentials once to stdout/stderr in a clearly readable block
      - frontend unauthenticated landing remains `/login`
      - no browser setup wizard/page is involved

- [x] **Desktop mode first-run bootstrap**
      - on first boot with zero users:
        - create a local admin user
        - issue a local bootstrap session for the Tauri desktop runtime
      - desktop app should enter the main UI directly on first run
      - browser `/setup` does not exist here either
      - bootstrap session must remain local-only and must not become a
        remote/public auth bypass

- [x] **Introduce `password_change_required` as a user attribute**
      - add persistent user-level field:
        - `password_change_required BOOLEAN NOT NULL DEFAULT FALSE`
      - bootstrap-created admin accounts must be created with
        `password_change_required = true`
      - this flag is a user/auth concern, not a system/bootstrap route state

- [x] **Extend auth responses with password-change requirement**
      - login/session bootstrap responses must include:
        - `access_token`
        - current `user`
        - `password_change_required`
      - both server-mode manual login and desktop-mode bootstrap session
        must converge on the same frontend auth state shape

- [x] **Replace `/setup` UX with a blocking password-change modal**
      - when `userStore.isAuthenticated && userStore.requiresPasswordChange`,
        show a global blocking modal/overlay from `App.vue`
      - the modal is:
        - not a route
        - not directly addressable by URL
        - not dismissible without completing the password change
      - after successful password update:
        - backend sets `password_change_required = false`
        - frontend updates current user state
        - modal disappears automatically

- [x] **Ensure migration-safe credential semantics**
      - credentials must live in the database, not only in local device state
      - desktop auto-session is only a convenience layer
      - if the database is moved to another machine:
        - server mode: user logs in with the password they set
        - desktop mode: if no local session exists, user logs in normally
      - database portability must not depend on the original device retaining
        a hidden session secret

- [x] **Simplify frontend entry-state semantics**
      - after this refactor, frontend system state should no longer model
        `needs_setup` as a stable routing destination
      - production route semantics should simplify to:
        - initialized + anonymous -> `/login`
        - initialized + authenticated -> `/`
        - authenticated + non-admin -> no `/admin`
      - `/setup` must not remain as a generic or accidental fallback

- [x] **Refactor route gate and route revalidation around the simplified model**
      - route decisions must be based on:
        - system initialized truth
        - auth session truth
        - current user role
      - removing `/setup` should reduce route leakage and eliminate a major
        source of route/bootstrap ambiguity
      - login success and logout must revalidate the current route correctly

- [x] **Deprecate or demote `/api/v1/system/initialize`**
      - frontend must stop using it as a normal browser bootstrap endpoint
      - if retained, it should be clearly scoped to controlled internal/admin
        or recovery use only
      - it must not imply that `/setup` remains part of the normal UI flow

- [x] **Add backend bootstrap regression coverage**
      - zero-user server bootstrap creates admin exactly once
      - zero-user desktop bootstrap creates admin and local bootstrap session
      - existing users prevent repeated bootstrap
      - generated credentials are not static/predictable
      - stdout credential output is emitted once, not on every restart

- [x] **Add frontend regression coverage for the post-`/setup` world**
      - initialized fresh anonymous session lands on `/login`
      - `/setup` is no longer reachable as a normal route
      - login success lands on `/`
      - logout from protected routes lands on `/login`
      - password-change modal appears when required
      - password-change modal blocks app use until completion
      - completion clears `password_change_required` and unlocks the app

> **Follow-up notes:**
> - This phase intentionally does **not** introduce guest/public capability
>   policy yet. Public browsing, demo behavior, and guest/user/admin
>   capability configuration should be layered on top of the simplified
>   entry/auth model afterward.
> - The `password_change_required` flow is intentionally modeled as a
>   user-level auth property, not a route or system bootstrap state.
> - Desktop bootstrap convenience must never supersede the long-term truth
>   that database-backed credentials are the portable source of identity.

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
