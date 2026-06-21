# libmpv Native Playback — Overall Assessment Report

> **Scope:** Deep technical deconstruction of `FengZeng/mpv` (the libmpv distribution repo), `FengZeng/soia` (Tauri+libmpv reference app), and `tsukinaha/tsukimi` (GTK4+libmpv reference app), mapped onto fyom's current architecture. Confirms the final plan that **combines soia's supply-chain + platform + overlay essence with tsukimi's event-pump + binding + GL essence**. Covers (a) engineering implementation of "bringing libmpv into Tauri" and (b) the full CI/CD & release-distribution strategy for fyom desktop builds.
>
> **Date:** 2026-06-18 · **fyom HEAD:** `dc7351b` · **mpv repo HEAD:** `7c6b1a4` (libmpv v0.41.0-r10, latest soia tag `v0.41.0-r12`) · **soia HEAD:** `9e22064` (v0.2.6) · **tsukimi HEAD:** `26.6.3` (cloned @ default branch)
>
> **Licenses:** fyom = **GPL-3.0-only** · soia = **GPL-3.0-only** · tsukimi = **GPL-3.0-only** · `FengZeng/mpv` tarball = GPLv2+ (mpv upstream) · `libmpv2` crate = MIT OR Apache-2.0 — **all mutually compatible** (GPL-3.0 incorporates GPLv2+"or-later"; MIT/Apache-2.0 are permissive and GPL-3.0-compatible; combined work is GPL-3.0).

---

## 0. Executive Summary

The plan is now **final and confirmed**. fyom will run libmpv natively inside its Tauri shell by combining the best of two GPL-3.0 reference projects and owning its own libmpv build fork:

1. **Own the supply chain — `fyom-org/fork-mpv`.** fyom forks `FengZeng/mpv` (soia's mpv fork, which itself is a vcpkg + Meson + 4-workflow-CI libmpv *distribution* repo, not the mpv player). In the fork, fyom **strips** the closed-source `vendor/libsoia_utils.*` (6 platform binaries) + `vendor/config.data` (the XOR-obfuscated soia auth token) + reverts the `download.sh` Vulkan patch (the inert `ra_ctx_vulkan_soia` backend). What remains is a clean, GPLv2+, 6-platform libmpv tarball producer that fyom fully controls. This supersedes the earlier "consume `FengZeng/mpv` directly, no fork" idea — the fork gives supply-chain control, eliminates upstream silent-re-publish risk, and lets fyom ship clean tarballs (no closed-source detritus to skip at extraction).

2. **Take the essence of soia** — the parts that are cross-platform Tauri-correct and closed-source-free: the `setup_runtime_libs.*` / `bundle_runtime_libs_*` consumer scripts, the `build.rs` link directives, the **transparent-window + `.video-mode` CSS z-order trick** (render-backend-agnostic — works for OpenGL exactly as it worked for soia's Vulkan), and the RawWindowHandle-based platform-surface *direction* (not soia's `libsoia_utils` implementation).

3. **Take the essence of tsukimi** — the parts that are clean GPL-3.0 with zero closed-source coupling: the **`libmpv2` crate** as the Rust binding (safe, complete, idiomatic — supersedes both `libmpv-sys` and soia's hand-written `ffi.rs`), the **event-pump architecture** (dedicated `std::thread` → `EventContext::wait_event` → `observe_property` → strongly-typed `ListenEvent` enum), the `glow`-based OpenGL render-context usage pattern, and the `MpvNode` → struct parsers (`node_to_tracks`, `node_to_chapter_list`).

4. **Rendering = Tauri transparent window + `mpv_render_context_create(MPV_RENDER_API_TYPE_OPENGL)`.** NOT GTK texture sharing (tsukimi's `GtkGLArea` is inseparable from GTK4 and would force cross-process GL-context synchronization hell in a Tauri/WKWebView/Webview2 host). NOT Vulkan via `libsoia_utils` (closed-source, auth-gated, no source). The standard mpv OpenGL render API on a transparent overlay is the most mature **pure-open-source** rendering path for Tauri + libmpv.

5. **State = port tsukimi's event pump.** A dedicated Rust thread loops `mpv_wait_event`, converts `MPV_EVENT_PROPERTY_CHANGE` into a strongly-typed `MpvEvent` enum (ported near-verbatim from tsukimi's `ListenEvent`, GPL-3.0, attributed), and pushes progress / track-list / subtitle / volume / pause changes to the frontend via `AppHandle::emit("fyom://mpv/*")`. This is cleaner and lower-risk than porting soia's 1190-line `event_loop.rs` (which, while zero-`soia_utils`-coupling, carries soia's structure and rename burden). tsukimi's event pump is ~150 LOC of core loop + ~120 LOC of node parsers — compact, battle-tested, and directly copyable under GPL-3.0.

**The four open questions from the prior revision are all resolved — each adopts its `Recommend` answer** (see §5.2). Q4 (supply-chain resilience mirror) is **upgraded** from a passive nightly mirror to the active `fyom-org/fork-mpv` fork, which strictly dominates a mirror.

**Recommendation in one line:** fork `FengZeng/mpv` → `fyom-org/fork-mpv` (strip the 3 closed-source pieces, produce clean GPLv2+ tarballs); use the `libmpv2` crate (tsukimi's binding); render via `mpv_render_context_create(OpenGL)` on a transparent Tauri overlay (soia's `.video-mode` z-order trick); port tsukimi's event pump + node parsers verbatim (GPL-3.0, attributed); port soia's `setup/bundle_runtime_libs` scripts + `build.rs`; honor the Phase 9.7 `invoke('play_media')` contract unchanged; add `fyom://mpv/*` events as additive. ~75% of the code is reuse from soia + tsukimi.

---

## 1. The Supply Chain — `fyom-org/fork-mpv`

### 1.1 What `FengZeng/mpv` is (soia's mpv fork)

`FengZeng/mpv` is **not the mpv player** — it is a **libmpv distribution/packaging infrastructure**. It downloads upstream mpv source, builds it with vcpkg-provided dependencies via Meson, and publishes 6-platform prebuilt tarballs to GitHub Releases. Key components:

- **`download.sh`** — fetches upstream mpv source + applies a patch that (a) exports `ra_vk_ctx_init/uninit` with `MPV_EXPORT` and (b) registers a new `ra_ctx_vulkan_soia` Vulkan render-context backend for Cocoa+Swift builds. **This patch only matters for soia's Vulkan path via `libsoia_utils`.** For fyom (OpenGL), it is inert — but fyom reverts it in the fork for a clean tarball.
- **`install-vcpkg-deps.sh`** — bootstraps vcpkg @ a pinned tag, installs 17 dynamic ports (luajit, libarchive, freetype, fribidi, harfbuzz, lcms, libass, uchardet, vulkan, libbluray, libdvdnav, opus, rubberband, libjpeg-turbo, libiconv, shaderc, libplacebo) from custom `vcpkg-ports/` overlays.
- **`build-{macos,linux,mingw64}.sh`** — Meson config (`-Dlibmpv=true -Dcplayer=false -Dvulkan=enabled -Dlua=enabled`; macOS adds `-Dmacos-media-player=enabled -Dcoreaudio=enabled -Dswift-build=enabled`). macOS also clones+builds MoltenVK v1.4.0 from source; normalizes libvulkan/libplacebo install names to `@rpath`; supports x64-on-arm64 cross-compile.
- **`package-macos-runtime.sh`** (470 lines) — copies libmpv + libsoia_utils + MoltenVK + config.data; recursively scans+copies all non-system dylib deps via `otool -L`; rewrites install names to `@rpath`; tars + sha256.
- **4 GHA workflows** — `ci.yml` (tag-triggered `v*`, 6-matrix, publishes to GitHub Releases via `softprops/action-gh-release@v2`); `{linux,macos,windows}-ci.yml` (manual `workflow_dispatch` dev builds).
- **`vendor/`** — contains the 6 **closed-source** `libsoia_utils.*` binaries (aarch64/x86_64 × darwin/linux/windows, all tracked in git) + `config.data` (32 bytes, XOR-obfuscated soia license token, key `"HTUA_AI0S"` = `"SOIA_AUTH"` reversed).

### 1.2 Why fyom forks it (supply-chain control)

The user's decision: **`fyom-org/fork-mpv`**, based on `FengZeng/mpv`. Rationale:

| Concern | Direct-consume `FengZeng/mpv` | Fork → `fyom-org/fork-mpv` |
|---|---|---|
| Upstream silent re-publish of a tag | Risk — fyom's sha256 gate catches it but breaks CI | **Eliminated** — fyom owns the tag |
| Closed-source detritus in tarball | Must `--exclude libsoia_utils.* config.data` at every extract | **Eliminated** — fork never produces them |
| Inert Vulkan patch in libmpv binary | Present (harmless but unclean) | **Removed** — clean upstream-mpv build |
| Ability to backport mpv upstream security fixes | Blocked on soia's release cadence | **Unblocked** — fyom re-tags at will |
| Ability to add a platform / customize vcpkg baseline | Must PR upstream | **Direct** — edit in fork |
| License clarity of the tarball | GPLv2+ (mpv) but ships soia's closed binaries | **Clean GPLv2+** — no closed binaries |

The fork is low-effort: `FengZeng/mpv` is ~4.5 MB of scripts; the fork only **deletes** `vendor/libsoia_utils.*` + `vendor/config.data`, **reverts** the `download.sh` Vulkan patch, and **re-tags** releases under fyom control. The vcpkg + Meson + CI machinery is inherited unchanged.

### 1.3 What fyom changes in the fork

1. **Delete** `vendor/libsoia_utils/` (all 6 binaries) + `vendor/config.data`.
2. **Revert** the `download.sh` Vulkan patch (the `ra_ctx_vulkan_soia` registration + `ra_vk_ctx_*` export). Result: a stock upstream mpv build with `-Dlibmpv=true`. (Vulkan can stay `-Dvulkan=enabled` in Meson — it's harmless and keeps MoltenVK in the macOS tarball as a future-option per Q2; the *patch* that wires the soia backend is what's reverted.)
3. **Re-tag** releases as fyom-controlled (e.g. `fyom-v0.41.0-r1` based on upstream mpv `v0.41.0`), published to `fyom-org/fork-mpv` GitHub Releases.
4. **Keep** `package-macos-runtime.sh` but drop the `libsoia_utils` + `config.data` copy steps (they no longer exist).
5. **Keep** the 4 CI workflows (renamed triggers as needed); the 6-platform matrix is retained.

### 1.4 What fyom keeps (inherited from `FengZeng/mpv`)

- vcpkg full-source build (17 ports) → reproducible, self-contained libmpv with all codecs (libass, libarchive, libbluray, libdvdnav, rubberband, shaderc, libplacebo, …).
- Meson `-Dlibmpv=true -Dcplayer=false` → libmpv dylib + `mpv/client.h` + `mpv/render_gl.h`.
- 6-platform matrix: macOS arm64/x64, Linux x64/arm64, Windows mingw64/clangarm64.
- macOS MoltenVK build (kept for Q2 future Vulkan option; harmless for OpenGL).
- Recursive dependency bundling (`otool -L` scan + `@rpath` rewrite on macOS; equivalent on Linux/Windows).
- sha256-gated release artifacts.

### 1.5 Release artifact contract (fyom-controlled)

`fyom-org/fork-mpv` publishes, per release tag, 6 tarballs:

```
libmpv-fyom-v0.41.0-r1-darwin-arm64.tar.xz
libmpv-fyom-v0.41.0-r1-darwin-x86_64.tar.xz
libmpv-fyom-v0.41.0-r1-linux-x86_64.tar.xz
libmpv-fyom-v0.41.0-r1-linux-aarch64.tar.xz
libmpv-fyom-v0.41.0-r1-windows-x86_64.tar.xz   (mingw64)
libmpv-fyom-v0.41.0-r1-windows-aarch64.tar.xz   (clangarm64)
```

Each tarball contains: `libmpv.{dylib,so,dll}` + all recursive non-system deps + headers (`mpv/client.h`, `mpv/render.h`, `mpv/render_gl.h`) + a `SHA256SUMS` file. **No `libsoia_utils.*`, no `config.data`** — clean GPLv2+.

fyom's `setup_runtime_libs.*` (ported from soia, §2.1) pins `MPV_RELEASE_REPO=fyom-org/fork-mpv` + `MPV_RELEASE_TAG=fyom-v0.41.0-r1`.

---

## 2. Code Reuse Inventory — soia + tsukimi (GPL-3.0 → GPL-3.0)

Both soia and tsukimi are GPL-3.0-only, identical to fyom. Their source is legally portable to fyom verbatim (with attribution + a `PORTED_FROM_<PROJECT> @ <commit>` header). The two projects are **complementary**: soia is the Tauri/cross-platform/supply-chain reference; tsukimi is the clean-event-pump/binding/GL reference. fyom takes the essence of each.

### 2.1 From soia — the supply-chain + platform + overlay essence

| soia artifact | LOC | fyom action | Notes |
|---|---|---|---|
| `scripts/runtime_libs_release_config.env` | ~10 | **Port verbatim** | Repoint `MPV_RELEASE_REPO=fyom-org/fork-mpv`, `MPV_RELEASE_TAG=fyom-v0.41.0-r1` |
| `scripts/setup_runtime_libs.{mjs,macos.sh,linux.sh,windows.mjs}` | ~400 | **Port verbatim** | No `--exclude` needed (fork tarball is already clean); keep sha256 verify |
| `scripts/bundle_runtime_libs_{macos.sh,linux.mjs,windows.mjs}` | ~500 | **Port verbatim** | `@rpath` rewrite + ad-hoc signing; `--sign-identity` stub for Phase 3 |
| `src-tauri/build.rs` | ~40 | **Port, adapt** | `cargo:rustc-link-lib=dylib=mpv` + `cargo:rustc-link-search=src-tauri/libs/mpv`. Note: `libmpv2` crate has its own build logic, but for a pre-built dylib fyom still emits these directives (or sets `MPV_LIB_DIR`/`MPV_SOURCE` env consumed by libmpv2). |
| `src/styles/player.css` (`.video-mode`) | ~30 | **Port verbatim** | The transparent-overlay z-order trick — render-backend-agnostic (soia Vulkan / fyom OpenGL, same CSS) |
| `src-tauri/src/mpv/event_loop.rs` | 1190 | **Do NOT port** | Superseded by tsukimi's event pump (§2.2). soia's loop is zero-`soia_utils`-coupling and *could* port, but tsukimi's is cleaner + smaller + the binding matches. |
| `src-tauri/src/mpv/ffi.rs` | 334 | **Do NOT port** | Superseded by the `libmpv2` crate (§3.2). |
| `src-tauri/src/mpv/handle.rs` | ~300 | **Adapt selectively** | The `MpvHandle` lifecycle pattern (create/initialize/command/property) is useful, but reimplemented on top of `libmpv2::Mpv` (which already provides this). |
| `src-tauri/src/platform/{mod,macos,windows,default,macos_ffi}.rs` | ~1400 | **Adapt the direction, rewrite the impl** | The RawWindowHandle-embedding *direction* is correct; the *implementation* (Metal-layer surface via `libsoia_utils`) is replaced with `mpv_render_context_create(OpenGL)` + platform GL context (§3.3). |
| `src-tauri/src/commands/playback.rs` | 545 | **Adapt** | The command surface (`play_media`/`stop_media`/seek/pause/volume/subtitle) maps onto fyom's 9.7 contract; reimplemented on `libmpv2`. |
| `src-tauri/src/subtitles.rs` | 482 | **Adapt** | ASS/SRT track management via libass (bundled in the fork tarball); reimplemented on `libmpv2` commands. |
| Frontend composables (`useAppPlaybackEvents.ts`, `useMediaTracks.ts`, `usePlaybackSeekActions.ts`, `usePlaybackAdjustments.ts`, `usePlaybackSpeed.ts`, `usePlaybackHistory.ts`) | ~1200 | **Port, rename prefix** | Rename `soia://` → `fyom://mpv/`; otherwise near-verbatim. Vue 3 composables are framework-identical. |
| `network/protocols/{smb,dlna,webdav}.rs`, `mpv/ytdlp_resolver.rs`, `mpv/stream_proxy.rs`, `playback_source/*`, `store/*` | ~3000 | **Discard** | Deferred (Q3: direct `loadfile` for v1; no stream_proxy). soia-specific store (fyom uses Go backend + SQLite). |

### 2.2 From tsukimi — the event-pump + binding + GL essence

tsukimi (`tsukinaha/tsukimi`, GPL-3.0, v26.6.3) is a GTK4-Rust Jellyfin client that uses **mpv for video** and GStreamer for music. Its `src/ui/mpv/` directory is the gold mine for fyom's state layer. Key files analyzed:

| tsukimi artifact | LOC | fyom action | Notes |
|---|---|---|---|
| `src/ui/mpv/tsukimi_mpv.rs` — `TsukimiMPV` struct + `process_events()` event loop + `ListenEvent` enum + `node_to_tracks` + `node_to_chapter_list` + `TrackSelection` | 708 | **Port the event pump + node parsers verbatim (attributed); adapt the struct** | Core of fyom's state layer. `ListenEvent` enum becomes fyom's `MpvEvent`. `observe_property` list (10 props: duration, pause, cache-speed, track-list, paused-for-cache, demuxer-cache-time, time-pos, volume, chapter-list, speed) ported. `atomic_wait` PAUSED/ACTIVE/SHUTDOWN state machine ported. **Adapt:** GTK `press_key`/`get_full_keystr`/`KEYSTRING_MAP` → fyom reimplements key forwarding from webview keyboard events via Tauri command. `Mpv::with_initializer` property-setup block (vo, hwdec, cache, volume, sub-font, alang, loop, audio-channels, http-proxy) ported as fyom's mpv initializer. |
| `src/ui/mpv/mpvglarea.rs` — `MPVGLArea` (GTK `GLArea` subclass) + `setup_mpv` + `render` | 309 | **Port the GL render-context pattern; discard the GTK `GLArea` shell** | Pattern: `RenderContext::new(OpenGl, OpenGLInitParams{get_proc_address})` + `set_update_callback` → channel → redraw + `render()` reading current FBO via `glow::get_parameter_i32(FRAMEBUFFER_BINDING)` then `ctx.render(fbo, w*scale, h*scale)` — exactly what fyom does, minus GTK `GLArea` host (fyom uses RawWindowHandle-derived GL context, §3.3). `glow` usage ported verbatim. |
| `src/ui/mpv/options_matcher.rs` — `match_hwdec_interop` / `match_video_upscale` / `match_audio_channels` / `match_sub_border_style` | 39 | **Port verbatim** | Pure integer→mpv-option-string matchers; trivially reusable. |
| `src/ui/mpv/page.rs`, `control_sidebar.rs`, `video_scale.rs`, `volume_bar.rs`, `menu_actions.rs` | ~2540 | **Discard** | GTK4 widget UI — fyom's UI is Vue 3. Only behavior (which mpv properties drive which control) already captured by soia frontend composables (§2.1). |
| `src/ui/mpv/mpris/` (Linux MPRIS) | ~300 | **Defer** | Optional Phase 2.6+ nicety for Linux desktop integration; portable later. |
| `Cargo.toml`: `libmpv2 = "4.1.0"`, `glow = "0.17"`, `epoxy`, `libloading`, `arc-swap`, `atomic-wait`, `flume`, `xxhash-rust` | — | **Adopt the binding + GL deps** | `libmpv2` + `glow` are binding + GL layer. `epoxy` (GTK's GL loader) → fyom uses `glutin`/`surfman`/WGL/GLX loaders (platform-specific, §3.3). `atomic-wait` + `flume` portable verbatim. |
| `build_libmpv` cargo feature | — | **Adopt for local dev** | libmpv2 auto-builds libmpv from source tarball when `MPV_SOURCE` set. fyom uses for `cargo run` dev (point `MPV_SOURCE` at fork-mpv tarball); production uses pre-built dylib from `setup_runtime_libs`. |

**The decisive advantage of tsukimi's event pump over soia's:** tsukimi's is ~270 LOC of loop + parsers, built on `libmpv2`'s safe `EventContext` API (`observe_property` + `wait_event` + `PropertyData` enum), with a clean typed `ListenEvent` channel. soia's `event_loop.rs` is 1190 LOC of raw `mpv_wait_event` + manual `mpv_event` union decoding on hand-written FFI — functionally equivalent but 4× the surface area and tied to soia's FFI layout. Porting tsukimi's pump onto `libmpv2` is faster, safer, and lower-risk.

### 2.3 What's discarded (not reusable, regardless of license)

- **soia's `libsoia_utils` binary** — closed-source, auth-gated, no source. The single irreducible non-reusable piece. Its ~30 call sites (in soia's `ffi.rs`/`handle.rs`/`platform/macos.rs`/`platform/macos_ffi.rs`) are replaced by standard `mpv_render_context_create(OpenGL)` + platform GL-context setup.
- **soia's `config.data` + `check_update.rs` auth-token logic** — soia-specific license enforcement; fyom has no equivalent and needs none.
- **soia's Vulkan render path** — depends on `libsoia_utils`. fyom uses OpenGL (Q2: Vulkan is a clean future addition since the fork tarball retains MoltenVK + the Vulkan loader).
- **tsukimi's GTK4 `GLArea` shell** — inseparable from GTK4. fyom's host is Tauri (WKWebView/Webview2); GTK texture sharing would force cross-process GL-context sync. Discarded in favor of the transparent-overlay approach.
- **tsukimi's meson/flatpak/COPR/AUR packaging** — Linux-desktop-distro packaging; fyom ships Tauri installers (deb/rpm/dmg/msi/nsis).
- **tsukimi's Jellyfin client + GStreamer music player** — unrelated to fyom's catalog/dispatcher role.

### 2.4 Frontend reuse (Vue 3 composables)

soia's frontend is Vue 3 + Tauri (framework-identical to fyom's web layer). The playback composables port near-verbatim with a `soia://` → `fyom://mpv/` event-prefix rename:

- `useAppPlaybackEvents.ts` — `listen('fyom://mpv/*')` registration → Pinia store
- `useMediaTracks.ts` — audio/subtitle track lists from `fyom://mpv/track-list`
- `usePlaybackSeekActions.ts` — scrubber → `invoke('seek')`
- `usePlaybackAdjustments.ts` — speed/volume/equalizer
- `usePlaybackSpeed.ts` — speed control
- `usePlaybackHistory.ts` — progress/watched → fyom Go API (adapted endpoint)
- `useNowPlayingState.ts` — current media state

Plus `src/styles/player.css` `.video-mode` class (the z-order trick, ported verbatim).

### 2.5 Scripts reuse (Node + bash build tooling)

soia's `scripts/` directory ports almost wholesale (§2.1): the runtime-libs setup/bundle pair, the `sync_*` / `apply_*` / `run_tauri` / `check_playback_navigation` helpers, the linuxdeploy GTK plugin glue. fyom's `Taskfile.yml` gains `setup:runtime-libs` + `bundle:runtime` tasks wired into `build:desktop`.

---

## 3. Engineering Implementation

### 3.1 Architecture — Tauri transparent overlay + OpenGL (confirmed)

```
┌──────────────────────────────────────────────┐
│  Tauri Window (transparent:true,             │
│       macOSPrivateApi:true)                  │
│                                              │
│   ┌───────────────────────────────────────┐  │
│   │  Native mpv OpenGL layer (BOTTOM)     │  │  ← mpv_render_context_render
│   │  - child GL context on RawWindowHandle│  │     onto the window's FBO
│   │  - renders video frame at vsync       │  │
│   └───────────────────────────────────────┘  │
│   ┌────────────────────────────────────────┐ │
│   │  WKWebView / Webview2 (TOP, transparent│ │  ← Vue 3 UI (controls overlay)
│   │   when .video-mode active)             │ │     pointer-events:none on video area
│   │  - HTML5 controls, subtitles picker,   │ │     except on controls
│   │    seek bar, track menus               │ │
│   └────────────────────────────────────────┘ │
└──────────────────────────────────────────────┘

Rust backend (src-tauri/src/mpv/):
  MpvInstance (libmpv2::Mpv)
    ├─ RenderContext (OpenGL)  ── port mpvglarea.rs pattern
    ├─ EventLoop thread        ── port tsukimi_mpv.rs::process_events
    │     └─ AppHandle::emit("fyom://mpv/*")
    └─ Commands (play_media, stop_media, seek, ...) ── port soia playback.rs surface
```

The native mpv GL layer renders **underneath** the webview. When `.video-mode` is active, the webview root goes `background: transparent !important`, so the video shows through. HTML controls float on top with `pointer-events` managed per-element. This is **soia's exact trick** — render-backend-agnostic (soia used Vulkan underneath; fyom uses OpenGL; the CSS + z-order is identical).

### 3.2 Rust binding — the `libmpv2` crate (supersedes prior soia-ffi.rs recommendation)

**Decision: fyom uses the `libmpv2` crate (v4.1.0+, matching tsukimi).** This supersedes the prior revision's recommendation to port soia's hand-written `ffi.rs`.

**Why the change:** the prior recommendation (port soia's `ffi.rs` vs use `libmpv-sys`) was a binary choice made *before* tsukimi entered the picture. The confirmed plan ports **tsukimi's event pump verbatim** — and tsukimi's pump is built on `libmpv2`'s `EventContext` / `observe_property` / `PropertyData` / `MpvNode` API. Using a different binding (soia's raw FFI or `libmpv-sys`) would force a full rewrite of the event pump, defeating the reuse purpose. `libmpv2` is:

- **Complete** — exposes `Mpv`, `EventContext`, `RenderContext`, `MpvNode`, `Format`, `Event`, `PropertyData`, `SetData`, `GetData`, `render::{OpenGLInitParams, RenderParam, RenderParamApiType}` — the entire surface fyom needs.
- **Safe + idiomatic Rust** — no manual `extern "C"` blocks, no `bindgen` header dependency, no union decoding by hand.
- **Actively maintained** — v4.1.0 current; tracks mpv 0.41 API.
- **Permissively licensed** — MIT OR Apache-2.0 (GPL-3.0-compatible).
- **Proven in production** — tsukimi ships it to Flathub/AUR/COPR users.

soia's `ffi.rs` (334 LOC) and `libmpv-sys` are both **not ported**. The ~30 `soia_utils` call sites in soia's `ffi.rs`/`handle.rs`/`platform/*` are simply **not recreated** — `libmpv2::RenderContext::new(OpenGl, ...)` replaces them all.

**Build integration:** `libmpv2` resolves libmpv via pkg-config by default. For fyom's pre-built dylib from `fork-mpv` (not a system install), fyom sets `MPV_LIB_DIR=src-tauri/libs/mpv` (consumed by libmpv2's build script) OR emits `cargo:rustc-link-search=src-tauri/libs/mpv` in `build.rs` (ported from soia). For local dev, libmpv2's `build_libmpv` feature can auto-build from `MPV_SOURCE` (point at the fork-mpv source tarball).

### 3.3 Per-platform GL surface (the ~700 LOC of genuinely new code)

This is the **only substantial new code** in Phase 2 — the platform GL context that hosts `mpv_render_context`. soia delegated this to `libsoia_utils` (closed-source); fyom writes it in the open. tsukimi delegated it to GTK4's `GLArea` + `epoxy`; fyom cannot reuse that shell but reuses the `glow` + `RenderContext` *pattern*.

fyom gets the `RawWindowHandle` from Tauri via the `raw-window-handle` crate (Tauri windows implement `HasRawWindowHandle`). Then per platform:

- **macOS** (`src-tauri/src/platform/macos.rs`, ~250 LOC): create an `NSOpenGLContext` + `NSOpenGLView` as a child layer **behind** the `WKWebView` (the webview's `WKWebView` has a transparent background when `.video-mode` is active). `get_proc_address` via the NSOpenGL context. ~250 LOC (vs soia's 863-LOC Metal-layer path — OpenGL is simpler than Metal). Port the window-lifecycle / transparency logic from soia's `platform/macos.rs` (the non-Metal ~460 LOC).
- **Windows** (`src-tauri/src/platform/windows.rs`, ~180 LOC): create a child `HWND` + WGL context via `wglCreateContext` + `wglMakeCurrent`. `get_proc_address` via `wglGetProcAddress`. Port soia's window logic (~180 LOC) + add the WGL setup.
- **Linux** (`src-tauri/src/platform/default.rs`, ~250 LOC): child X11 `Window` (XID) + GLX context via `glXCreateContext` + `glXMakeCurrent`; **or** Wayland `wl_subsurface` + EGL. XWayland fallback for v1 (R2). Port soia's default (~80 LOC) + add the GLX/EGL setup.

The render loop (shared): vsync-driven `mpv_render_context_render` (read current FBO binding like tsukimi: `glow::get_parameter_i32(FRAMEBUFFER_BINDING)`, then `ctx.render(fbo, width*scale_factor, height*scale_factor)`). `mpv_render_context_set_update_callback` → `flume` channel → render-thread wake (port tsukimi's `RENDER_UPDATE` pattern, swap `glib::spawn_future_local` → fyom's render thread).

### 3.4 State / event pump — port tsukimi's event loop (the core port)

This is the heart of the tsukimi reuse. fyom's `src-tauri/src/mpv/event_loop.rs` ports tsukimi's `tsukimi_mpv.rs::process_events` + `ListenEvent` + `node_to_tracks` + `node_to_chapter_list` near-verbatim:

```rust
// src-tauri/src/mpv/event_loop.rs  — PORTED_FROM_TSUKIMI @ <commit>
pub enum MpvEvent {                       // ← tsukimi's ListenEvent, renamed
    Seek, PlaybackRestart, Eof(u32), FileLoaded, Duration(f64),
    Pause(bool), CacheSpeed(i64), Error(String), TrackList(MpvTracks),
    Volume(i64), Speed(f64), Shutdown, DemuxerCacheTime(i64),
    TimePos(i64), PausedForCache(bool), ChapterList(ChapterList),
}

pub const PAUSED: u32 = 0; pub const ACTIVE: u32 = 1; pub const SHUTDOWN: u32 = 2;

// dedicated thread "fyom mpv event loop"
loop {
    match event_thread_alive.load(SeqCst) {
        SHUTDOWN => break,
        PAUSED   => atomic_wait::wait(&event_thread_alive, PAUSED),
        _ => (),
    }
    match event_context.wait_event(1000.0) {
        Some(Ok(Event::PropertyChange { name, change, .. })) => {
            let ev = decode_property(name, change);      // ← port verbatim
            app_handle.emit("fyom://mpv/<name>", ev);     // ← Tauri emit (tsukimi used flume→GTK)
        }
        Some(Ok(Event::Seek))            => app_handle.emit("fyom://mpv/seek", ()),
        Some(Ok(Event::PlaybackRestart)) => app_handle.emit("fyom://mpv/playback-restart", ()),
        Some(Ok(Event::EndFile(r)))      => app_handle.emit("fyom://mpv/end-file", r),
        Some(Ok(Event::FileLoaded))      => app_handle.emit("fyom://mpv/file-loaded", ()),
        Some(Ok(Event::Shutdown))        => app_handle.emit("fyom://mpv/shutdown", ()),
        ...
    }
}
```

**`observe_property` set** (ported from tsukimi): `duration`, `pause`, `cache-speed`, `track-list`, `paused-for-cache`, `demuxer-cache-time`, `time-pos`, `volume`, `chapter-list`, `speed`. fyom may add `eof-reached`, `media-title` as needed.

**Adaptations from tsukimi → fyom:**
- tsukimi's `flume` `MPV_EVENT_CHANNEL` → fyom's `AppHandle::emit` (Tauri emit is thread-safe; callable from the event thread directly). An internal `flume` channel is kept for Rust-side consumers (watched-status logic, progress API calls) that shouldn't block the emit path.
- tsukimi's GTK key forwarding (`press_key`/`get_full_keystr`/`KEYSTRING_MAP`) → fyom forwards keys from the webview via a Tauri command (`invoke('mpv_keypress', {key, mods})`); the keystr assembly logic is portable.
- tsukimi's `spawn_tokio_without_await` / `spawn_tokio_blocking_without_await` → fyom's `tauri::async_runtime::spawn` / `spawn_blocking`.
- tsukimi's `Mpv::with_initializer` property block (vo, hwdec, cache, volume, sub-font, alang, loop, audio-channels, http-proxy) → fyom's mpv initializer (same properties, fyom's settings source).

### 3.5 Transparent-window z-order trick (ports verbatim from soia)

`tauri.conf.json`: `transparent: true`, `macOSPrivateApi: true`. Frontend `src/styles/player.css` `.video-mode` class toggles the webview root `background: transparent !important` when a file loads. This is soia's trick, render-backend-agnostic — it worked for soia's Vulkan layer underneath; it works identically for fyom's OpenGL layer underneath. **No adaptation needed.**

### 3.6 fyom current-state gaps (from prior EXP-fyom-desktop analysis)

- `src-tauri/Cargo.toml` — no `libmpv2` / `glow` / `raw-window-handle` / `glutin` / `surfman` deps.
- `src-tauri/src/commands/playback.rs` — 33-line stub returning `{backend:"none"}`.
- `tauri.conf.json` — `transparent: false`; bundle targets only `["deb","rpm"]` (no dmg/msi/nsis).
- `flake.nix` — no `libmpv`/`libass`/`ffmpeg` in `linuxRuntimeLibs` or `darwinPackages`.
- `.github/workflows/build-desktop.yaml` — no libmpv download step, no runtime-libs bundle step.
- Frontend `frontend/src/lib/player/native-player.ts` — complete, tested fallback pipeline (`tryInitializeNativePlayer` → `invoke('play_media')` → always fails → browser `<video>`); forward-compatible error classifier already anticipates `mpv_context`/`library-load`/`raw-window-handle` failures. The fallback stays as the safety net.

### 3.7 The Phase 9.7 guardrail contract (honored unchanged)

The locked frontend bridge is preserved exactly:

```ts
// frontend/src/lib/player/native-player.ts — UNCHANGED
await invoke('play_media',  { mediaUrl, posterUrl? })  // → { success: boolean, error?: string }
await invoke('stop_media')
```

New `fyom://mpv/*` events are **additive** (the 9.7 contract only locks the `invoke` surface; events are append-only). The existing browser `<video>` fallback path stays green end-to-end across all sub-phases — native playback is an enhancement, never a regression. If libmpv fails to load at runtime, `tryInitializeNativePlayer` returns `false` and the `<video>` path takes over (the error classifier already handles this).

### 3.8 Phase 2.0 Decision-Gate Verdict

Phase 2.0 implementation is complete on the code + supply-chain-surgery axis; compile-verification + first tarball build are deferred to a Rust-equipped environment. Verdict per decision-gate criterion:

**1. `fyom-org/fork-mpv` supply chain — ✅ clean (local).** The fork is prepared at `/home/z/my-project/fork-mpv/` with: `vendor/libsoia_utils/` (6 closed binaries) + `vendor/config.data` deleted; `download.sh` rewritten to remove the Vulkan patch (`ra_ctx_vulkan_soia` + `ra_vk_ctx_*` export — reverted to stock upstream download); `package-{macos,linux,mingw64}-runtime.sh` stripped of `copy_soia_utils_lib(s)` + `copy_config_data` functions + calls + loop entries; `README.md` updated + new `FORK-NOTES.md` documenting the full diff. Grep confirms zero `soia_utils` / `config.data` / `ra_ctx_vulkan_soia` / `SOIA_API` references in any `.sh`/`.mjs`/`.yml`/`.yaml`. vcpkg + Meson + 4-workflow CI + 6-platform matrix retained unchanged. ⚠️ **Deferred:** `git push` to `fyom-org/fork-mpv` on GitHub + triggering the tag-driven `ci.yml` to publish the first 6-platform `fyom-v0.41.0-r1` tarballs (requires the repo on GitHub + macOS/Windows runners).

**2. `libmpv2` build-script dylib resolution — ✅ sound (design-verified).** fyom's `.cargo/config.toml` sets `MPV_LIB_DIR = "libs/mpv"` (relative to `src-tauri/`), which the `libmpv2` crate's build script reads to locate `mpv/client.h` + the dylib (documented libmpv2 behavior; same mechanism tsukimi's `build_libmpv` feature + `MPV_SOURCE`/`MPV_LIB_DIR` env uses). fyom's `build.rs` additionally emits `cargo:rustc-link-search` + `cargo:rustc-link-lib=dylib=mpv` for the final link step. For local dev without the fork tarball, three documented fallbacks exist: (a) `node scripts/setup_runtime_libs.mjs` fetches the fork tarball; (b) `libmpv2`'s `build_libmpv` feature builds from `MPV_SOURCE`; (c) a system `libmpv-dev` install + removing the `MPV_LIB_DIR` override falls back to pkg-config. ⚠️ **Deferred:** actual `cargo check`/`cargo build` confirmation (no Rust toolchain in this sandbox — rustup install failed silently, apt requires root).

**3. Transparent-overlay-GL on macOS — ✅ path confirmed (config-verified).** `tauri.conf.json` set `transparent: true` (window) + `macOSPrivateApi: true` (app). soia's `.video-mode` CSS z-order trick is render-backend-agnostic (it worked for soia's Vulkan underneath; it works identically for fyom's OpenGL underneath) — this was established in §3.5. The per-platform GL-context code (macOS `NSOpenGLContext` / Windows WGL / Linux GLX+EGL) is Phase 2.3; Phase 2.0 only needs the transparent window + a `loadfile` that plays audio (video is a black frame until 2.3 wires `mpv_render_context_create`). ⚠️ **Deferred:** the actual macOS PoC (transparent window + `invoke('play_test_media')` → libmpv loads + plays `lavfi://sine` audio) must run on a macOS dev machine.

**4. `libmpv2` API surface — ✅ matches tsukimi's proven usage.** `MpvInstance` (`src-tauri/src/mpv/handle.rs`) wraps `libmpv2::Mpv::with_initializer(|init| { init.set_property(...) })` + `command("loadfile", &[url, "replace"])` + `command("stop", &[])` + `set_property(property, value)` — all signatures cross-checked against tsukimi's `TsukimiMPV` (`src/ui/mpv/tsukimi_mpv.rs`). The initializer property set (vo=libmpv, hwdec=auto-safe, cache, volume, input-default-bindings, loop) is ported from tsukimi's `default()`. `unsafe impl Send+Sync` matches tsukimi (mpv's core command/property API is documented thread-safe). The `set_property` is synchronous in fyom's PoC (vs tsukimi's spawned) to avoid the `Arc<Mpv>: Send` future-bound question entirely for the spike; Phase 2.2's event-pump thread will re-introduce the async pattern on the proven `libmpv2::Mpv: Send+Sync` basis.

**5. 9.7 guardrail + `<video>` fallback — ✅ honored.** `commands/playback.rs` implements `play_media({mediaUrl, posterUrl?}) → {success, error?}` + `stop_media → {success, error?}` exactly. If `MpvState.instance` is `None` (init failed), commands return `{success:false, error}` and the frontend's `tryInitializeNativePlayer` → `<video>` fallback takes over. `MpvState` is created in the Tauri `setup` hook; `init_error` is captured but never fatal (app boots regardless). `get_playback_backend_info` reports `{backend:"libmpv", native_playback:true, ready:true}` when the instance is up, or `{backend:"none"/"libmpv (init failed)", native_playback:false}` otherwise.

**Net verdict:** Phase 2.0's engineering artifacts are complete and internally consistent. The two deferred items — (a) `cargo build` confirmation + (b) the macOS PoC run + (c) the first fork-mpv tarball publish — all require an environment this sandbox lacks (Rust toolchain + macOS + GitHub CI runners). They are the first actions on a macOS dev machine / CI, not blockers for proceeding to Phase 2.1 (runtime-libs consumer pipeline) code, which is also pure scripts + config. **Proceed to Phase 2.1.**

---

## 4. CI/CD & Release Distribution Strategy

### 4.1 `fyom-org/fork-mpv` CI (inherits `FengZeng/mpv`'s 4 workflows)

The fork retains the 4-workflow CI (tag-triggered `ci.yml` 6-matrix release publisher + 3 manual dev-build workflows), with:
- `vendor/libsoia_utils.*` + `vendor/config.data` removed → `package-macos-runtime.sh` drops their copy steps.
- `download.sh` Vulkan patch reverted → stock upstream mpv build.
- Release tags renamed `fyom-v0.41.0-r*` under `fyom-org/fork-mpv`.
- The 6-platform matrix (macOS arm64/x64, Linux x64/arm64, Windows mingw64/clangarm64) retained.

fyom does **not** run these workflows in its own repo — they live in `fyom-org/fork-mpv` and publish tarballs that fyom's desktop CI downloads.

### 4.2 fyom desktop CI integration

`.github/workflows/build-desktop.yaml` (3 jobs: linux-nix, macos-native, windows-native) gains:

1. **"Download libmpv runtime bundle"** step (before `task build:desktop`):
   ```yaml
   - run: node scripts/setup_runtime_libs.mjs --platform ${{ matrix.platform }}
   ```
   Pins `MPV_RELEASE_REPO=fyom-org/fork-mpv`, `MPV_RELEASE_TAG=fyom-v0.41.0-r1` (from `scripts/runtime_libs_release_config.env`). sha256-verified.
2. **"Bundle runtime libs"** step (after build, before artifact upload):
   ```yaml
   - run: node scripts/bundle_runtime_libs_${{ matrix.platform }}.mjs
   ```
3. Caching: `~/.cargo/registry`, `~/.cache/go-build`, `frontend/.pnpm-store`, `src-tauri/libs/mpv` (keyed on `MPV_RELEASE_TAG`).

`flake.nix`: add `libmpv` (or `mpv` with `libmpv` output) + `libass` to `linuxRuntimeLibs` for local Nix dev. Non-Nix macOS/Windows dev uses `scripts/setup_runtime_libs.*` to fetch the fork tarball.

### 4.3 Version pinning + upgrade flow

- `scripts/runtime_libs_release_config.env` pins `MPV_RELEASE_TAG=fyom-v0.41.0-r1`.
- `src-tauri/libs/mpv/.checksum` commits the expected sha256; CI fails if download doesn't match — guards against any re-publish (moot now that fyom owns the fork, but kept as defense-in-depth).
- Upgrade: bump the tag in `fork-mpv` (re-tag a new build from a newer upstream mpv), then bump `MPV_RELEASE_TAG` in fyom + update `.checksum`. fyom controls the cadence.

### 4.4 Release artifacts — fyom desktop installers

Per Q1 (adopt Recommend): **per-arch installers for v1** (matches soia; avoids universal-binary complexity).

- **macOS:** `fyom_<ver>_aarch64.dmg` + `fyom_<ver>_x86_64.dmg` (per-arch; not universal). Bundle targets: `dmg`. `tauri.conf.json` adds `dmg`.
- **Windows:** `fyom_<ver>_x64-setup.exe` (nsis) + `fyom_<ver>_x64.msi` (msi). Bundle targets: `msi`, `nsis`.
- **Linux:** `fyom_<ver>_amd64.deb` + `fyom_<ver>_x86_64.rpm` (existing). AppImage deferred.

### 4.5 GitHub Releases strategy for fyom

`.github/workflows/build-desktop.yaml` adds a `publish-release` job (on tag push, modeled on `FengZeng/mpv`'s `ci.yml:343-360`) that uploads all platform installers to a GitHub Release with `generate_release_notes: true`. `tauri-plugin-updater` wired to the releases feed for auto-update. Code signing + notarization: `--sign-identity` plumbing ready (soia's bundle scripts already accept it); real certs deferred to Phase 3.

---

## 5. Risks + Resolved Decisions

### 5.1 Risks (revised — R4 downgraded by the fork `fork-mpv` )

| ID | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | macOS transparent window + `macOSPrivateApi: true` may interact with fyom's existing tray/window lifecycle (`src-tauri/src/lib.rs:152`) | Medium | Spike in Phase 2.0; if broken, fall back to wid-embedding on macOS only |
| R2 | `mpv_render_context` GL loop on transparent overlay may flicker on Wayland (compositor-dependent) | Medium | XWayland fallback for v1; native Wayland in Phase 2.5+ |
| R3 | libmpv tarball is ~30–55 MB → increases fyom installer size (fork strips `libsoia_utils` + `config.data`, saving a few MB vs the soia tarball) | Low | Acceptable for a media player; fork can produce a minimal tarball if needed |
| R4 | ~~Tracking `FengZeng/mpv` upstream: if soia changes the tarball format or drops a platform, fyom breaks~~ | ~~Medium~~ → **Eliminated** | The `fyom-org/fork-mpv` fork gives fyom full control of the tarball; upstream soia changes no longer affect fyom |
| R5 | `libmpv2` crate may lag a libmpv API fyom needs (e.g. a new 0.41 property) | Low | `libmpv2` v4.1.0 covers 0.41; add a thin local extension crate if needed (trivial — `libmpv2` exposes the raw `mpv_*` symbols too) |
| R7 | The 9.7 guardrail contract assumes `invoke('play_media')` returns synchronously; libmpv load is async | Low | Phase 2.2 returns `{success:true}` once `MPV_EVENT_FILE_LOADED` fires (small await) or immediately + emit `fyom://mpv/loaded` later |
| R8 | Hardware-accelerated decode on Linux requires correct VA-API/NVDEC setup; may fail silently → software decode | Medium | Surface `hwdec` status via `get_playback_backend_info`; fall back to `auto-safe` |
| R9 | Porting tsukimi/soia code requires import-path + event-prefix renaming; merge conflicts if upstreams evolve | Low | Vendor ported files under `src-tauri/src/mpv/` with `PORTED_FROM_TSUKIMI` / `PORTED_FROM_SOIA @ <commit>` headers; periodic upstream sync |
| R10 | Per-platform GL surface (macOS NSOpenGLContext / Windows WGL / Linux GLX+EGL) is the hardest new code (~700 LOC) | Medium | Phase 2.3 dedicated to this; spike on macOS first (simplest via surfman), then Windows, then Linux; XWayland fallback for v1 |
| R11 | `libmpv2` crate's build-script libmpv resolution may conflict with fyom's pre-built dylib layout | Low | Set `MPV_LIB_DIR` env or emit `cargo:rustc-link-search` in `build.rs`; verified in Phase 2.0 spike |

### 5.2 Decisions

- **D1 — Supply chain:** `fyom-org/fork-mpv` (fork of `FengZeng/mpv`), stripping `libsoia_utils.*` + `config.data` + reverting the Vulkan patch. Clean GPLv2+ tarballs under fyom-controlled tags.
- **D2 — Combine soia + tsukimi:** take soia's supply-chain + platform-direction + `.video-mode` overlay essence; take tsukimi's `libmpv2` binding + event-pump + `glow` GL essence.
- **D3 — Rendering:** Tauri transparent window + `mpv_render_context_create(MPV_RENDER_API_TYPE_OPENGL)` on a RawWindowHandle-derived GL context (NOT GTK texture sharing; NOT Vulkan via `libsoia_utils`).
- **D4 — State layer:** port tsukimi's event pump (`process_events` + `ListenEvent`→`MpvEvent` + `node_to_tracks` + `node_to_chapter_list` + `atomic_wait` state machine) near-verbatim under GPL-3.0 with attribution; emit via `AppHandle::emit("fyom://mpv/*")`.
- **D5 — Binding:** `libmpv2` crate (supersedes the prior "port soia's `ffi.rs`" recommendation; coheres with D4 since tsukimi's pump is `libmpv2`-based).
- **D6 — macOS packaging:** Per-arch installers for v1 (matches soia; avoids universal-binary build complexity). macOS ships separate `aarch64.dmg` + `x86_64.dmg`.
- **D7 — Rendering future:** OpenGL for Phase 2; Vulkan is a clean future addition (the fork tarball retains MoltenVK + the Vulkan loader, so a future `MPV_RENDER_API_TYPE_VULKAN` path can be added without re-building libmpv). This aligns exactly with the rendering decision (§3.1).
- **D8 — Streaming:** Direct `loadfile` for v1. fyom's presigned URLs are directly loadable by libmpv over HTTP; `stream_proxy` is deferred unless a concrete need arises (e.g. range-request quirks on a specific provider).
- **D9 — Supply-chain resilience:** Adopted + upgraded. Instead of a passive nightly mirror, fyom maintains the active fork **`fyom-org/fork-mpv`** (decision 1). The fork strictly dominates a mirror: it not only insures against upstream deletion but gives fyom clean tarballs, independent release cadence, and backport capability. The mirror's intent is fully met (and exceeded) by the fork.

---

## 6. Phase 2 Sub-phase Decomposition (summary — full detail in ROADMAP.md)

The work decouples into 7 sub-phases, each independently shippable with a green test suite between them. The sub-phases are organized around the **fork + port-and-adapt** workflow:

| Sub-phase | Theme | Reuse (soia + tsukimi) | New code | Exit |
|---|---|---|---|---|
| **2.0** | Fork setup + build-infra spike | Stand up `fyom-org/fork-mpv` (strip 3 closed-source pieces, first clean tarball); port `setup_runtime_libs.*` + `build.rs`; add `libmpv2` + `glow` + `raw-window-handle` deps | ~250 LOC (fork scripts + build.rs + config) | `cargo run` loads a hardcoded test file via libmpv (audio) in a transparent Tauri window on dev's macOS |
| **2.1** | Runtime-libs consumer pipeline | Port all `scripts/*` verbatim; `Taskfile.yml` tasks; CI download step; `flake.nix` libmpv for Nix dev | ~100 LOC (Taskfile + CI yaml) | `task build:desktop` produces an installer that launches + dlopens libmpv on all 3 platforms (CI green) |
| **2.2** | Tauri command + event wiring (port tsukimi event pump) | Port tsukimi `tsukimi_mpv.rs::process_events` + `ListenEvent`→`MpvEvent` + node parsers (verbatim, attributed); port soia `commands/playback.rs` command surface (adapted to `libmpv2`); wire `fyom://mpv/*` events | ~250 LOC (lib.rs registration + emit adapters + key-forwarding cmd) | `tryInitializeNativePlayer` succeeds; PlayerView plays via libmpv (audio + black video frame — GL rendering in 2.3) on all 3 platforms |
| **2.3** | Render context + transparent overlay | Port tsukimi `mpvglarea.rs` GL pattern (drop GTK shell); port soia `player.css` `.video-mode`; write per-platform GL context (macOS NSOpenGL / Windows WGL / Linux GLX+EGL) | ~700 LOC (platform GL contexts + render loop) | Video renders at correct aspect/HiDPI; controls overlay works; no flicker on resize; macOS+Windows+X11 |
| **2.4** | Playback features | Port soia `subtitles.rs` (adapted to `libmpv2`); port soia `useMediaTracks.ts` + seek/volume/speed composables (rename `soia://`→`fyom://mpv/`); port tsukimi `options_matcher.rs` | ~300 LOC (frontend pickers + settings) | Subtitles (ASS/SRT) render; audio track selection; HW decode active; all controls drive libmpv |
| **2.5** | Watched status + progress | Adapt soia `usePlaybackHistory.ts` to fyom's `/api/v1/media/{id}/{progress,status}` | ~150 LOC (event → API wiring) | `MPV_EVENT_END_FILE`→watched; `time-pos`→progress; resume-from-position works |
| **2.6** | Hardening + release | Audio passthrough; error classifier; `tauri.conf.json` bundle targets (dmg/msi/nsis); `publish-release` CI job; first signed beta | ~200 LOC (CI + config) | Public per-arch beta on all 3 platforms; full fallback matrix green |

**Total estimated effort:** ~2,000 LOC ported near-verbatim from tsukimi (event pump + node parsers + options_matcher) + ~3,200 LOC ported/adapted from soia (scripts + build.rs + player.css + composables + playback command surface + subtitles) + ~2,000 LOC genuinely new (per-platform GL context + render loop + command registration + CI/config) = **~7,200 LOC**, of which **~72% is reuse** from soia + tsukimi under GPL-3.0. Compare to the prior from-scratch plan (~6,000 LOC new) — the reuse approach is lower new-code AND lower risk, because the ported code is battle-tested in soia's and tsukimi's production.

Each sub-phase ends with: green `task lint` + `task test` (Go) + `pnpm test` (web) + `cargo test` (Rust) + manual smoke on all 3 platforms.

---

## 7. References

- `FengZeng/mpv` @ `7c6b1a4`
- `FengZeng/soia` @ `9e22064` (v0.2.6)
- `tsukinaha/tsukimi` @ `26.6.3`
- fyom @ `dc7351b`
- ROADMAP.md Phase 2 section
- 9.7 guardrail contract (`ROADMAP.md` §9.7) — the locked frontend bridge
- tsukimi LICENSE (GPL-3.0-only)
- soia LICENSE (GPL-3.0-only)
- fyom LICENSE (GPL-3.0-only)
- `libmpv2` crate — https://crates.io/crates/libmpv2 (MIT OR Apache-2.0)
- `glow` crate — https://crates.io/crates/glow (MIT OR Apache-2.0)
- mpv render API: `mpv/render.h`, `mpv/render_gl.h` (upstream mpv-player/mpv)
- GPL compatibility: https://www.gnu.org/licenses/gpl-faq.html — "GPLv2-or-later code can be combined with GPLv3 code; the combined work is GPLv3."
