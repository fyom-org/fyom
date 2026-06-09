# fyom — Roadmap

## Design North Star

fyom is not a media server — it is a **media catalog and resource dispatcher**. 
The server never transcodes and never proxies media traffic. It only manages 
metadata and issues time-limited, signed URLs (Presigned URLs) that allow 
clients to stream directly from the source (Local Disk, S3, or Remote fyom Node).

---

## Phase 1: Core Foundation & Import (Complete)

Build the foundational UI shell, authentication, and the primary user action — importing a pre-organized media library.

- [x] Login flow (JWT auth, form validation)
- [x] Setup Wizard (first-run admin creation, registration toggle)
- [x] Main layout (header, sidebar, content area)
- [x] Import view (path input, trigger button)
- [x] Job status polling component
- [x] RBAC (Admin/User roles, path restrictions lifted for MVP)
- [x] API client for library endpoints

## Phase 2: Media Catalog & Browsing (Complete)

Browse imported movies and shows with poster art using Jellyfin/Kodi standard directory parsing.

- [x] Library list view (dark-themed poster wall grid)
- [x] NFO Parser (Kodi/tinyMediaManager XML format for Movies, Shows, Episodes)
- [x] Detail page for individual media items (backdrop, metadata, overview)
- [x] Show → Episodes hierarchy navigation
- [x] Native HTML5 video player (autoplay, fullscreen, seek via Range requests)
- [x] S3-style Presigned URLs for all media resources (HMAC-SHA256, zero header dependency)
- [ ] Search and filter controls

## Phase 3: Provider Architecture & Abstraction (Current)

Decouple fyom from local filesystem assumptions. Introduce the `MediaProvider` interface 
to unify local, remote, and cloud storage under a single dispatch layer.

- [ ] Define `MediaProvider` interface (`GetMetadata`, `GetPresignedStreamURL`, `GetPresignedPosterURL`)
- [ ] Refactor local importer/file serving into `LocalProvider` (wraps current logic)
- [ ] Provider configuration in system settings (mount multiple providers)
- [ ] Dynamic URL dispatch: API responses route stream/poster URLs via the correct provider

## Phase 4: Cloud Native & S3 Storage

Expand beyond NAS boundaries. Allow media to live in B2, Wasabi, MinIO, or any S3-compatible object storage. fyom becomes a pure metadata catalog while storage scales infinitely in the cloud.

- [ ] S3 Provider implementation (`S3Provider`)
- [ ] S3 Presigned URL generation (using S3 SDK, direct client-to-S3 streaming)
- [ ] Import from S3 bucket (read NFO from object storage, catalog metadata locally)
- [ ] Bandwidth cost optimization (S3 CDN integration via Presigned URL parameters)

**Architecture Win:** S3 perfectly aligns with fyom's "no server transcoding/proxying" principle. 
The Go server generates an S3 Presigned URL and returns it to the client. The client streams 
hundreds of Mbps directly from S3. The Go server remains at <1% CPU and minimal memory, 
acting only as a metadata dispatcher.

## Phase 5: Federation & Remote Nodes

Break down data silos between friends. Connect multiple fyom instances together without 
duplicating terabytes of files.

- [ ] Remote fyom Provider implementation (`RemoteFyomProvider`)
- [ ] Peer token exchange (authenticate with a friend's fyom instance)
- [ ] Metadata proxying (cache remote library metadata locally for fast browsing)
- [ ] 302 Redirect streaming (local fyom redirects client directly to remote fyom's Presigned URL)
- [ ] Cross-instance watch status synchronization (optional)

**Architecture Win:** Zero local bandwidth consumption for remote content. When a user hits "Play" 
on a remote item, the local fyom server simply returns an HTTP 302 redirect to the remote fyom's 
Presigned stream URL. The client pulls the video stream directly from the remote server over P2P.

## Phase 6: Desktop Shell & Ecosystem

Refine the client experience and wrap everything in a native desktop application.

- [ ] Tauri 2 desktop shell integration (wrapping the Web UI)
- [ ] Local network discovery (find other fyom nodes on LAN)
- [ ] System tray / background service management
- [ ] Responsive design improvements (mobile-friendly catalog)
- [ ] Global search (across local, S3, and federated providers)
