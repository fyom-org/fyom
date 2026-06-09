# fyom — Roadmap

## Design North Star

fyom is not a media server — it is a **media catalog and resource dispatcher**.
The server never transcodes and never proxies media traffic. It only manages
metadata and issues time-limited, signed URLs (Presigned URLs) that allow
clients to stream directly from the source (Local Disk, S3, or Remote fyom Node).

---

## Phase 1: Core Foundation & Import ✅

Build the foundational UI shell, authentication, and the primary user action —
importing a pre-organized media library.

- [x] Login flow (JWT auth, form validation)
- [x] Setup Wizard (first-run admin creation, registration toggle)
- [x] Main layout (header, sidebar, content area)
- [x] Import view (path input, trigger button)
- [x] Job status polling component
- [x] RBAC (Admin/User roles, path restrictions lifted for MVP)
- [x] API client for library endpoints

---

## Phase 2: Media Catalog & Browsing ✅

Browse imported movies and shows with poster art using Jellyfin/Kodi standard
directory parsing.

- [x] Library list view (dark-themed poster wall grid)
- [x] NFO Parser (Kodi/tinyMediaManager XML format for Movies, Shows, Episodes)
- [x] Detail page for individual media items (backdrop, metadata, overview)
- [x] Show → Episodes hierarchy navigation
- [x] Native HTML5 video player (autoplay, fullscreen, seek via Range requests)
- [x] S3-style Presigned URLs for all media resources (HMAC-SHA256, zero header dependency)
- [ ] Search and filter controls *(client-side filter on fetched catalog — complete before Phase 3)*
- [ ] Local watch progress persistence (playback position, watched flag — SQLite)
  > Required data foundation for Phase 5 cross-instance sync. Schema should be
  > settled before Provider abstraction is introduced.

---

## Phase 3: Provider Architecture & Abstraction 🔧 *Current*

Decouple fyom from local filesystem assumptions. Introduce the `MediaProvider`
interface to unify local, remote, and cloud storage under a single dispatch layer.

**`MediaProvider` interface contract:**

```go
type MediaProvider interface {
    GetMetadata(ctx context.Context, item *MediaItem) (*MediaMetadata, error)
    GetPresignedStreamURL(ctx context.Context, item *MediaItem, ttl time.Duration) (string, error)
    GetPresignedPosterURL(ctx context.Context, item *MediaItem, ttl time.Duration) (string, error)

    // SupportsRedirect signals whether this provider returns a URL suitable
    // for an HTTP 302 redirect rather than a locally-signed URL.
    // LocalProvider → false  |  S3Provider → false  |  RemoteFyomProvider → true
    SupportsRedirect() bool
}
```

> `SupportsRedirect()` is defined now so the Phase 5 `RemoteFyomProvider`
> can be implemented without touching the interface.

- [ ] Define `MediaProvider` interface (`GetMetadata`, `GetPresignedStreamURL`, `GetPresignedPosterURL`, `SupportsRedirect`)
- [ ] Refactor local importer/file serving into `LocalProvider` (wraps current logic)
- [ ] Provider registry and configuration in system settings (mount multiple providers)
- [ ] Dynamic URL dispatch: API responses route stream/poster URLs via the correct provider

---

## Phase 4: Cloud Native & S3 Storage

Expand beyond NAS boundaries. Allow media to live in B2, Wasabi, MinIO, or any
S3-compatible object storage. fyom becomes a pure metadata catalog while storage
scales infinitely in the cloud.

- [ ] `S3Provider` implementation
- [ ] S3 Presigned URL generation (AWS SDK — direct client-to-S3 streaming)
- [ ] Import from S3 bucket (read NFO from object storage, catalog metadata locally)
- [ ] Bandwidth cost optimization (CDN integration via Presigned URL parameters)

**Architecture note:** S3 aligns perfectly with fyom's no-proxying principle.
The Go server generates an S3 Presigned URL and returns it; the client streams
hundreds of Mbps directly. The server stays at <1% CPU, acting only as a
metadata dispatcher. `S3Provider` also serves as the design reference for the
`RemoteFyomProvider` signing protocol in Phase 5.

---

## Phase 5: Federation & Remote Nodes

Break down data silos between friends. Connect multiple fyom instances together
without duplicating terabytes of files.

- [ ] `RemoteFyomProvider` implementation (`SupportsRedirect() → true`)
- [ ] Peer token exchange (authenticate with a remote fyom instance)
- [ ] Metadata proxying (cache remote library metadata locally for fast browsing)
- [ ] 302 Redirect streaming (local fyom returns `Location:` pointing to remote Presigned URL — zero local bandwidth)
- [ ] Cross-instance watch status sync (requires Phase 2 local progress schema)

**Architecture note:** When a user hits Play on a remote item, the local fyom
server reads `SupportsRedirect() == true`, calls `GetPresignedStreamURL()` on
the `RemoteFyomProvider`, and returns an HTTP 302. The client pulls the video
stream directly from the remote server — the local node never touches media bytes.

---

## Phase 6: Desktop Shell & Ecosystem

Wrap everything in a native desktop application and refine the client experience.

- [ ] Tauri 2 desktop shell (wrapping the Web UI)
  > **Lifecycle note:** in Local-Only mode the embedded Go server must be
  > launchable as a Tauri sidecar (not only as a standalone process). Reserve
  > a library-mode entry point in `cmd/fyom` before this phase so the Tauri
  > integration does not require core refactoring.
- [ ] Local network discovery (find other fyom nodes on LAN via mDNS)
- [ ] System tray / background service management
- [ ] Responsive design improvements (mobile-friendly catalog)
- [ ] Global search (across local, S3, and federated providers)
