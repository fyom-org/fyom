```markdown
# fyom — Roadmap

## Design North Star

fyom is not a media server — it is a **media catalog and resource dispatcher**. 
The server never transcodes and never proxies media traffic. It only manages 
metadata and issues time-limited, signed URLs (Presigned URLs) that allow 
clients to stream directly from the source (Local Disk, S3, or Remote fyom Node).

---

# MVP: The Action-Oriented Media Center

The goal of the MVP is to transform fyom from a file manager into an immersive 
media experience with professional-grade library management. Users should be able 
to decide "what to watch" within 3 seconds, and admins should have full control 
over library organization, content lifecycle, and access permissions.

## Phase 1: Core Foundation & Auth ✅

Build the foundational UI shell, authentication, and the primary user action — 
importing a pre-organized media library.

- [x] Login flow (JWT auth, form validation)
- [x] Setup Wizard (first-run admin creation, registration toggle)
- [x] Main layout (header, sidebar, content area)
- [x] Import view (path input, trigger button)
- [x] Job status polling component
- [x] RBAC (Admin/User roles, RequireAdmin middleware)
- [x] S3-style Presigned URLs for all media resources (HMAC-SHA256)

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
- [x] `LocalProvider` implementation (wraps presign.Signer)
- [x] `ImportFS` abstraction (ReadDir, Open, Exists, Join)
- [x] `LocalImportFS` implementation (wraps os.ReadFile/filepath.WalkDir)

## Phase 3: Cloud Native & S3 Storage ✅

Expand beyond NAS boundaries. Allow media to live in B2, Wasabi, MinIO, or any 
S3-compatible object storage.

- [x] `S3Provider` implementation (AWS SDK v2 presigned URL generation)
- [x] `S3ImportFS` implementation (ListObjectsV2 with delimiter, GetObject)
- [x] Import from S3 bucket (read NFO from object storage, catalog metadata locally)
- [x] CDN integration (replaceCDNHost swaps S3 host for CDN while preserving signatures)
- [x] Admin Provider CRUD API (`/api/v1/admin/providers`)

## Phase 4: The 3-Second Experience ✅

fyom currently looks like a file manager. This phase reshaped it into an 
action-oriented media center. Core metric: from opening the page to clicking 
play in under 3 seconds.

### 4.1 Action-Oriented Dashboard ✅
- [x] Dashboard View as default landing page (replacing raw library grid)
- [x] "Continue Watching" row (horizontal scroll, always first)
- [x] "Recently Added" row (horizontal scroll, freshness)
- [x] `MediaRow.vue` component (unified horizontal scroll paradigm)

### 4.2 Card Evolution & Hover Preview ✅
- [x] Enhanced `MediaCard`: display year, type badge, progress bar
- [x] Hover/Focus interaction: card scales up, play icon overlay
- [x] Minimal auxiliary info: cover + year/type only, restrained spacing

### 4.3 Watch Progress Tracking ✅
- [x] Backend `watch_progress` data model (position, duration, finished)
- [x] `PUT /api/v1/media/:id/progress` (player timeupdate fire-and-forget)
- [x] `GET /api/v1/library/continue` (in-progress items)
- [x] Progress state reflected on cards (progress bar) and detail page

### 4.4 Detail Page Simplification ✅
- [x] Strengthen primary action: visually dominant "▶ Play" button
- [x] Interest-based expansion: overview/episodes collapsed by default
- [x] Playback state echo: resume position displayed ("Resume from 42m")
- [x] Collapsible season headers in EpisodeList with episode counts

## Phase 5: Admin Control Hub 🔧 *Current*

A media library admin panel should not be a collection of configurations, but a 
control hub with clear status and direct operations.

- [x] Dedicated `/admin` layout (visually decoupled from user experience)
- [x] System Health Panel (library stats, import job history, storage distribution)
- [x] Provider management page (CRUD, enable/disable toggle)
- [x] Content/Admin route decoupling (RBAC route guards, localStorage role check)
- [ ] Inline Operations (match/edit unrecognized media without page jumps)
- [ ] Configuration Convergence (only expose parameters that affect outcomes)

## Phase 6: Library Management & Access Control

fyom currently treats all imported media as a single flat pool with no lifecycle 
management. Admins cannot delete items, cannot organize content into separate 
libraries, and cannot control which users see which content. This phase introduces 
the Library as a first-class entity — the organizational unit that binds storage, 
metadata rules, and access permissions together.

### 6.1 Library Entity Model
- [ ] `libraries` table (id, name, type [movie|show|mixed], provider_id, source_path, 
      metadata_source [nfo|filename], created_at, updated_at)
- [ ] `media_items.library_id` foreign key (migration, backfill existing items)
- [ ] Admin CRUD API for libraries (`/api/v1/admin/libraries`)
- [ ] Admin Libraries page (create, edit, delete libraries with provider + path binding)

### 6.2 Content Lifecycle
- [ ] Delete single media item (`DELETE /api/v1/admin/media/:id` — cascades episodes for shows)
- [ ] Delete entire library (with confirmation, option to keep or remove orphans)
- [ ] Re-import / refresh library (re-scan source path, add new, mark missing items)
- [ ] Missing item detection (file no longer exists on disk → flag in UI)

### 6.3 Per-Library Access Control
- [ ] `library_permissions` table (user_id, library_id, can_view [bool])
- [ ] Library list API respects permissions (users only see libraries they can access)
- [ ] Admin UI for managing per-user library access
- [ ] Default permission: new users get access to all existing libraries (MVP-safe)

### 6.4 Library-Aware Browsing
- [ ] Sidebar library switcher (when multiple libraries exist)
- [ ] Dashboard rows scoped to accessible libraries
- [ ] Library grid filtered by `library_id` query parameter

**Design Principle:** The Library is the atomic unit of organization. Every media 
item belongs to exactly one library. Permissions are granted at the library level, 
not the item level. This gives admins the power to create distinct spaces 
(e.g., "Kids Movies" vs "Documentaries") with independent access rules, without 
the complexity of per-item permissions.

**Architecture Note:** Jellyfin's layered override model 
(global → library → item) is the right long-term target, but for MVP we only 
implement the library layer. Global settings (registration, provider config) already 
exist at the system level. Per-item overrides are a Production concern.

---

# Production: Scaling & Ecosystem

Features for hardening fyom for production deployment and wrapping the experience 
in a native desktop shell.

### Production Phase 1: Desktop Shell

Wrap everything in a native desktop application and refine the client experience
for production use.

- [ ] Tauri 2 desktop shell (wrapping the Web UI)
  > **Lifecycle prerequisite:** reserve a library-mode entry point in `cmd/fyom`
  > so the embedded Go server can run as a Tauri sidecar in Local-Only mode.
  > The HTML5 `<video>` player remains active in this phase — libmpv integration
  > is Production Phase 2.
- [ ] Local network discovery (find other fyom nodes on LAN via mDNS)
- [ ] System tray / background service management
- [ ] Responsive design improvements (mobile-friendly catalog)

---

### Production Phase 2: Native Desktop Player (libmpv)

Replace the HTML5 `<video>` player in the Tauri shell with a libmpv-powered
native player. This unlocks 4K HDR, Dolby Vision, Dolby Atmos, DTS-HD MA,
and hardware-accelerated decoding — capabilities WebKit cannot provide.

The HTML5 player is **not removed**; it remains as the fallback for the web
client and mobile. The Tauri shell detects the runtime context and routes to
the appropriate player.

Reference implementations:
[Soia](https://github.com/FengZeng/soia) (Tauri + libmpv RawWindowHandle),
[Tsukimi](https://github.com/tsukinaha/tsukimi) (Rust + GTK4 + libmpv render context).

#### Architecture: Three-Layer Integration

**Layer 1 — Rendering (Soia pattern)**

Rust FFI binds `libmpv` and sets the `wid` property to the window's
`RawWindowHandle`. The video surface renders at the OS compositor layer.
A fully transparent Vue 3 WebView overlays it for player controls (seek bar,
volume, subtitles, fullscreen). There is no IPC overhead — interaction is
microsecond-latency in-process FFI.

> Do NOT use an external `mpv` process (IPC over socket). The video window
> cannot be reliably embedded across platforms and introduces process
> lifecycle complexity.

Platform handle mapping:

| Platform | Handle | Hardware decoder |
|----------|--------|-----------------|
| Windows | `HWND` | `d3d11va`, `nvdec` |
| macOS | `NSView` | `videotoolbox` |
| Linux / X11 | `XID` | `vaapi`, `vdpau`, `nvdec` |
| Linux / Wayland | EGL surface | `drm`, `vaapi` via DMA-BUF |

> Wayland native requires `wlr-export-dmabuf` or layer-shell protocol support.
> Implement X11 + XWayland first; Wayland native is a follow-up iteration.

**Layer 2 — State (Tsukimi pattern)**

A dedicated OS thread (not `tokio::spawn` — `mpv_wait_event` is a blocking
C call and must not run on the async executor) polls the mpv event queue:

```rust
// std::thread, not tokio::spawn — mpv_wait_event is a blocking C call.
// Moving this to the tokio executor will cause the runtime to stall.
std::thread::spawn(move || {
    loop {
        let event = unsafe { mpv_wait_event(ctx, -1.0) };
        match event.event_id {
            MPV_EVENT_PROPERTY_CHANGE => { /* window.emit() → Pinia */ }
            MPV_EVENT_END_FILE        => { /* update watch progress */ }
            MPV_EVENT_SHUTDOWN        => break,
            _ => {}
        }
    }
});
```

Properties to observe: `time-pos`, `duration`, `pause`, `volume`,
`track-list` (audio/subtitle tracks), `chapter-list`.
Events are emitted to the Vue frontend via `window.emit()` and consumed
by a Pinia store that drives the player UI state.

**Layer 3 — Network**

fyom's Presigned URLs (Local HMAC, S3 SigV4) are self-authenticating — mpv
receives the URL and streams directly with no header injection needed.

For WebDAV / SMB mounts or future bearer-token scenarios (Phase 5 Federation),
inject credentials directly into mpv properties via FFI (no frontend round-trip):

```rust
mpv_set_property_string(ctx, "http-header-fields",
    "Authorization: Bearer {token}\r\nUser-Agent: fyom/1.0");
```

This hands all network buffering and retry logic to mpv's internal stream
layer, enabling seek-to-play on 4K files over remote mounts.

#### Task List

- [ ] Rust `libmpv` FFI bindings (use `libmpv-sys` crate or vendor the C headers)
- [ ] `RawWindowHandle` extraction from Tauri window; platform-specific `wid` injection
- [ ] Dedicated `std::thread` event loop — `mpv_wait_event` → `window.emit()`
- [ ] Tauri commands: `player_open(url)`, `player_seek(pos)`, `player_pause()`,
  `player_set_volume(v)`, `player_set_track(id, type)`
- [ ] Vue 3 transparent player overlay (seek bar, volume, track selector, fullscreen)
- [ ] Pinia `usePlayerStore` — driven by Tauri events, not local component state
- [ ] Runtime player selection: libmpv in Tauri shell, HTML5 on web / mobile
- [ ] Watch progress write-back on `MPV_EVENT_END_FILE` and periodic `time-pos`
- [ ] X11 implementation + XWayland fallback (Wayland native: follow-up)
- [ ] Platform CI: Windows (HWND + d3d11va), macOS (NSView + videotoolbox),
  Linux / X11 (XID + vaapi)

---

### Production Phase 3: Advanced Metadata & Discovery

- [ ] Full Kodi NFO template support (actors, ratings, `<uniqueid>`, fanart sets)
- [ ] Jellyfin `<uniqueid>` multi-source ID resolution (IMDb, TMDb, TVDb)
- [ ] `<MediaInfo>` technical metadata display (codec, resolution, audio channels,
  HDR format, bit depth)
- [ ] Collection / franchise grouping (Marvel, Star Wars, etc.)
- [ ] Smart playlists / saved filters
- [ ] SQLite FTS5 full-text search (replaces current `LIKE '%q%'` scan)
  > Migration: `CREATE VIRTUAL TABLE media_fts USING fts5(...)` + triggers.
  > The `TODO(fts5)` anchor in `media_paged.go` marks the call site.

---

# Future Plan

Architectural visions for the next era. These require significant design work 
before implementation and depend on MVP being stable and complete.

## Future Plan 1: Enhanced Fetures
- [ ] Global search (across local, S3, and federated providers)
- [ ] Per-item metadata overrides (layered override: global → library → item)
- [ ] NFO write-back (Jellyfin-style bidirectional sync)

## Future Plan 2: Federation & Remote Nodes

Break down data silos between friends. Connect multiple fyom instances together
without duplicating terabytes of files.

- [ ] `RemoteFyomProvider` implementation (`SupportsRedirect() → true`)
  > Add `SupportsRedirect()` to the `Provider` interface before this phase.
  > At this point only two implementations exist (Local + S3), so the interface
  > change is minimal.
- [ ] Peer token exchange (authenticate with a remote fyom instance)
- [ ] Metadata proxying (cache remote library metadata locally for fast browsing)
- [ ] 302 Redirect streaming (local fyom returns `Location:` pointing to remote
  Presigned URL — zero local bandwidth consumed)
- [ ] Cross-instance watch status sync (requires Phase 2 local progress schema)

**Architecture note:** When a user hits Play on a remote item, the local fyom
server reads `SupportsRedirect() == true`, calls `GetPresignedStreamURL()` on
the `RemoteFyomProvider`, and returns HTTP 302. The client pulls the video
stream directly from the remote server — the local node never touches media bytes.

```
