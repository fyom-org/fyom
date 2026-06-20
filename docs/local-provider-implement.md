```markdown
# Local Provider Media Playback: Direct File URI Strategy (Lean Edition)

## Background (What)

The `fyom` project supports multiple storage providers for media files. The `LocalProvider` represents the case where the media file physically resides on the same filesystem that the fyom process can access — local disks, NVMe arrays, or network shares mounted at the OS level (e.g., SMB/NFS volumes appearing as `/Volumes/NAS` on macOS or `Z:\` on Windows).

The naive implementation routes all playback through the fyom Go HTTP backend: when the external player (mpv, VLC, etc.) requests a video file, it fetches it via `http://127.0.0.1:{port}/api/v1/media/{id}/stream`, and the Go process performs an `io.Copy` from the local file into the HTTP response body.

This is a fundamental architectural error for `LocalProvider`. Even over loopback, the full TCP protocol stack is exercised, causing measurable CPU overhead for high-bitrate 4K HDR content and severe random seek (drag-to-position) latency. 

fyom's stated architectural philosophy is to act as a **media scheduler, not a media proxy (or monitor)**. The `LocalProvider` playback path must be the clearest expression of this principle.

---

## Decision (What will be)

For `LocalProvider` media items, fyom will **never** proxy the video byte stream through the Go HTTP layer, nor will it monitor the external player's process. 

The Go backend performs exactly two responsibilities:
1. **Authorization**: Verify that the requesting user holds the required permission on the media item's library.
2. **Path resolution**: Validate and sanitize the stored file path, then construct a well-formed `file://` URI.

The resulting `file://` URI is passed directly to the external player via `exec.Command`. The player's process then performs native OS file I/O directly against the storage medium. **fyom immediately detaches from the player process.** It does not track the player's lifecycle, it does not poll playback progress, and it does not maintain IPC sockets. 

Resume playback functionality is delegated entirely to the external player's native mechanisms (e.g., mpv's `--resume-playback` and `watch_later` files).

---

## Consequences (Why)

### Positive

- **Zero-copy local playback.** The Go process performs no `io.Copy`. CPU and memory footprint during playback is effectively zero for the Go side.
- **Native seek latency.** The external player calls `lseek(2)` directly. Seek operations complete in microseconds.
- **Automatic subtitle discovery.** When mpv opens a `file://` URI with `--sub-auto=fuzzy`, it automatically scans the same directory for matching subtitle files. fyom does not need to enumerate subtitles.
- **Extreme engineering subtraction.** By removing IPC state syncing, we eliminate thousands of lines of cross-platform socket code, retry logic, and concurrency management. This drastically reduces maintenance burden.
- **Architectural consistency.** By not tracking local playback state in Go, we align the desktop client's behavior with the future Flutter C/S client. State tracking in a multi-platform ecosystem should rely on client-side UI reporting or the player's own memory, not host-process IPC.
- **Alignment with fyom's scheduler philosophy.** fyom issues a path credential and steps aside completely.

### Negative

- **No centralized progress tracking for desktop playback.** Because fyom does not monitor the player, the `watch_progress` table will not be updated with exact stop positions for local desktop playback. Users must rely on the external player's own resume functionality. 
- **RBAC is enforced only at path issuance time.** Once the `file://` URI is issued, the player holds an independent file descriptor. Permission revocation mid-playback will not interrupt the current session.
- **Filesystem permission dependency.** The OS user running fyom must have read access to the media directories.
- **Platform-specific path normalization required.** Windows, UNC, and POSIX paths must be handled correctly during URI construction.

---

## Engineering Details (How to do)

### 1. Define the `PlaybackInfo` contract

The struct is drastically simplified. No IPC paths, no start positions (delegated to player native memory).

```go
// internal/playback/types.go

type ProviderType string

const (
    ProviderLocal  ProviderType = "local"
    ProviderHTTP   ProviderType = "http"
    ProviderWebDAV ProviderType = "webdav"
)

// PlaybackInfo carries everything needed to invoke an external player.
type PlaybackInfo struct {
    URI       string        // e.g., file:///mnt/media/Movies/Inception.mkv
    Type      ProviderType
    ExtraArgs []string      // Provider-specific flags (e.g., HTTP headers)
}
```

### 2. Define the `PlaybackURIResolver` interface

```go
// internal/playback/resolver.go

type PlaybackURIResolver interface {
    Resolve(ctx context.Context, media *domain.Media) (*PlaybackInfo, error)
    Supports(p ProviderType) bool
}
```

The service layer selects the appropriate resolver. Note the absence of progress fetching.

```go
// internal/playback/service.go

func (s *PlaybackService) GetPlaybackInfo(
    ctx context.Context, userID string, mediaID string,
) (*PlaybackInfo, error) {

    // 1. RBAC: The single, mandatory authorization gate.
    if err := s.rbac.AssertMediaReadAccess(ctx, userID, mediaID); err != nil {
        return nil, fmt.Errorf("access denied: %w", err)
    }

    // 2. Fetch media record.
    media, err := s.repo.GetMediaByID(ctx, mediaID)
    if err != nil {
        return nil, err
    }

    // 3. Delegate to the appropriate resolver.
    for _, r := range s.resolvers {
        if r.Supports(media.ProviderType) {
            return r.Resolve(ctx, media)
        }
    }
    return nil, fmt.Errorf("no resolver for provider type: %s", media.ProviderType)
}
```

### 3. Implement `LocalFileResolver` with full path validation

Security-critical path. Every step is mandatory.

```go
// internal/playback/local_resolver.go

type LocalFileResolver struct {
    AllowedRoots []string // Absolute, real directory paths. No trailing slashes.
}

func (r *LocalFileResolver) Supports(p ProviderType) bool {
    return p == ProviderLocal
}

func (r *LocalFileResolver) Resolve(
    ctx context.Context, media *domain.Media,
) (*PlaybackInfo, error) {

    // Step A: Syntactic normalization. Catches textual traversal like "/media/../../etc/passwd".
    clean := filepath.Clean(media.FilePath)

    // Step B: Prefix check against allowed roots (pre-symlink-resolution).
    if !r.isUnderAllowedRoot(clean) {
        return nil, ErrPathTraversal
    }

    // Step C: Verify file existence. Follows symlinks, catches dangling links.
    if _, err := os.Stat(clean); err != nil {
        if os.IsNotExist(err) {
            return nil, ErrFileNotFound
        }
        return nil, fmt.Errorf("stat failed: %w", err)
    }

    // Step D: Resolve symlinks and re-validate. Catches symlink-based escapes.
    real, err := filepath.EvalSymlinks(clean)
    if err != nil {
        return nil, fmt.Errorf("symlink resolution failed: %w", err)
    }
    if !r.isUnderAllowedRoot(real) {
        return nil, ErrSymlinkTraversal
    }

    // Step E: Construct a well-formed file:// URI.
    uri, err := localPathToFileURI(real)
    if err != nil {
        return nil, fmt.Errorf("URI construction failed: %w", err)
    }

    return &PlaybackInfo{
        URI:  uri,
        Type: ProviderLocal,
    }, nil
}

func (r *LocalFileResolver) isUnderAllowedRoot(absPath string) bool {
    for _, root := range r.AllowedRoots {
        if strings.HasPrefix(absPath, root+string(filepath.Separator)) || absPath == root {
            return true
        }
    }
    return false
}
```

### 4. Cross-platform `file://` URI construction

```go
// internal/playback/uri.go

func localPathToFileURI(absPath string) (string, error) {
    slashed := filepath.ToSlash(absPath)
    var u *url.URL

    if runtime.GOOS == "windows" {
        if strings.HasPrefix(slashed, "//") {
            // UNC path: //server/share/path → file://server/share/path
            u = &url.URL{Scheme: "file", Host: "", Path: slashed}
        } else {
            // Drive letter path: C:/Movies/... → file:///C:/Movies/...
            u = &url.URL{Scheme: "file", Host: "", Path: "/" + slashed}
        }
    } else {
        // POSIX: /mnt/media/... → file:///mnt/media/...
        u = &url.URL{Scheme: "file", Host: "", Path: slashed}
    }

    // url.URL.String() percent-encodes the Path field automatically.
    return u.String(), nil
}
```

### 5. External player invocation (`PlayerInvoker`)

The invoker is now incredibly lean. It translates `PlaybackInfo` into an `exec.Command`, starts the process, and **immediately returns**. It does not wait, it does not monitor, it does not spawn goroutines.

```go
// internal/playback/invoker.go

type PlayerInvoker struct {
    PlayerBinary string // e.g., "mpv", resolved via exec.LookPath at startup
}

func (inv *PlayerInvoker) Launch(info *PlaybackInfo) error {
    args := inv.buildArgs(info)
    cmd := exec.Command(inv.PlayerBinary, args...)

    // Start the process and detach. Fire and forget.
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("failed to start player: %w", err)
    }

    // Intentionally not calling cmd.Wait() and not tracking the process.
    // The OS will reap the process when it exits, and the player manages
    // its own state and resume memory.
    return nil
}

func (inv *PlayerInvoker) buildArgs(info *PlaybackInfo) []string {
    args := []string{
        info.URI,
        "--no-terminal",
    }

    if info.Type == ProviderLocal {
        // Rely entirely on the player's native resume memory (e.g., mpv's watch_later).
        // --resume-playback is harmless if no saved state exists.
        args = append(args, "--resume-playback")
        // Enable fuzzy subtitle discovery in adjacent directory.
        args = append(args, "--sub-auto=fuzzy")
    }

    args = append(args, info.ExtraArgs...)
    return args
}
```

---

## Key Invariants

**Invariant 1: RBAC is enforced before any path information is disclosed.**
The `GetPlaybackInfo` service method must call `rbac.AssertMediaReadAccess` as its very first operation. A `file://` URI is a credential.

**Invariant 2: Every path returned by `LocalFileResolver` must survive both `filepath.Clean` prefix validation AND `filepath.EvalSymlinks` prefix validation.**
`filepath.Clean` catches textual traversal. `filepath.EvalSymlinks` catches symlink-based escapes. Both are load-bearing.

**Invariant 3: The `file://` URI must be constructed exclusively via `url.URL`, never by string concatenation.**
String concatenation does not percent-encode spaces, CJK characters, or parentheses, leading to silent player failures.

**Invariant 4: fyom must not track the external player's lifecycle.**
Once `exec.Command.Start()` succeeds, fyom's responsibility ends. Do not implement IPC, do not poll `cmd.ProcessState`, and do not attempt to sync progress back to the database for local desktop playback.

---

## Boundary Conditions

**Windows drive letter paths.** `C:\Movies\File.mkv` → `file:///C:/Movies/File.mkv`. The triple slash is required for the empty host component.

**Windows UNC paths.** `\\NAS\media\file.mkv` → `file://NAS/media/file.mkv`. The UNC server name becomes the URI host component (RFC 8089). `AllowedRoots` validation must compare against the normalized forward-slash form.

**CJK and special characters.** `url.URL` percent-encodes the entire `Path` field automatically. No additional encoding step is required.

**Network-mounted volumes.** An SMB share mounted at `/Volumes/NAS` is indistinguishable from a local path. `LocalFileResolver` will serve it via `file://`. If custom authentication is required, introduce an `SMBResolver` in the future.

**Player resume state isolation.** Because fyom does not track progress, resume functionality is tied to the specific external player's local configuration directory (e.g., `~/.config/mpv/watch_later/`). If a user switches players, they lose their resume position. This is an accepted trade-off for architectural simplicity.

---

## Failure Modes

**File deleted between path validation and player open.** `os.Stat` succeeds, but mpv fails to open the file. mpv displays its own error dialog and exits. fyom does not monitor the exit code, so no Wails event is emitted. The user simply sees the player's error and returns to fyom.

**OS-level permission denied.** The fyom process owner lacks read permission. mpv's `open(2)` will fail with `EACCES`. The error surfaces through mpv's own error reporting, not through fyom.

**`AllowedRoots` is empty.** If initialized with an empty slice, `isUnderAllowedRoot` always returns `false`. This must be caught at startup via configuration validation.

---

## How to Verify

**Unit test: `file://` URI construction**

```go
func TestLocalPathToFileURI(t *testing.T) {
    cases := []struct{ input, want string }{
        {"/mnt/media/Movie (2010).mkv",     "file:///mnt/media/Movie%20(2010).mkv"},
        {"/Volumes/NAS/影视/测试.mkv",        "file:///Volumes/NAS/%E5%BD%B1%E8%A7%86/%E6%B5%8B%E8%AF%95.mkv"},
        {"/media/file&name.mkv",             "file:///media/file&name.mkv"},
    }
    for _, c := range cases {
        got, err := localPathToFileURI(c.input)
        require.NoError(t, err)
        require.Equal(t, c.want, got)
    }
}
```

**Unit test: path traversal prevention**

```go
func TestLocalFileResolver_PathTraversal(t *testing.T) {
    r := &LocalFileResolver{AllowedRoots: []string{"/media"}}

    media := &domain.Media{FilePath: "/media/../../etc/passwd", ProviderType: ProviderLocal}
    _, err := r.Resolve(context.Background(), media)
    require.ErrorIs(t, err, ErrPathTraversal)

    media2 := &domain.Media{FilePath: "/media2/file.mkv", ProviderType: ProviderLocal}
    _, err = r.Resolve(context.Background(), media2)
    require.ErrorIs(t, err, ErrPathTraversal)
}
```

**Integration test: verify `file://` URI is what mpv receives**

Replace `exec.Command` with a stub that writes its arguments to a temp file:

```bash
# After triggering playback via the Wails UI:
cat /tmp/fyom-last-invocation.txt
# Expected first argument: file:///path/to/actual/file.mkv
# Must NOT be: http://127.0.0.1:27402/api/v1/media/.../stream
```

**Log verification: confirm no HTTP Range requests**

Enable Chi access logging. During local file playback, no requests to `/api/v1/media/*/stream` should appear in the log.

---

## Key Points and Boundary (What should do, what should not do)

### What should DO

- **Enforce RBAC as the first operation in `GetPlaybackInfo`.** The path is the credential; it must be gated.
- **Run `filepath.Clean` followed by `filepath.EvalSymlinks`, then re-validate.** Both steps are mandatory to close traversal attack surfaces.
- **Use `url.URL{Scheme: "file", Path: ...}.String()` for URI construction.**
- **Pass `--sub-auto=fuzzy` and `--resume-playback` for `ProviderLocal` invocations.** Leverage the player's native capabilities.
- **Implement `PlaybackURIResolver` as an interface.** Future providers (WebDAV, SMB) must be addable without modifying the service layer.
- **Validate `AllowedRoots` is non-empty at application startup.**

### What should NOT do

- **Do not open a TCP listener for media streaming in desktop mode for `LocalProvider` items.** 
- **Do not construct `file://` URIs by string formatting or concatenation.**
- **Do not skip `filepath.EvalSymlinks`.**
- **Do not implement IPC, state polling, or progress syncing for local desktop playback.** Rely on the external player's native resume memory. The architectural cost of maintaining cross-platform IPC outweighs the benefit of centralized progress tracking for a local desktop app.
- **Do not implement per-chunk re-authorization.** Authorize once, then trust the OS file descriptor.

---

## Implementation Notes (What things left for the future)

**`SMBResolver` & `WebDAVResolver`: direct URI issuance.**
mpv supports native `smb://` and `https://` URIs with auth header injection (`--http-header-fields`). Future resolvers can issue these URIs directly to the player, bypassing Go entirely, just like `LocalFileResolver`.

**Manual "Mark as Watched" UI flow.**
Since automatic progress tracking is removed for desktop playback, the UI should provide a prominent "Mark as Watched" / "Mark as Unwatched" toggle. This keeps the fyom catalog accurate without requiring process monitoring.

**Multi-instance playback.**
The current design is strictly fire-and-forget. If future requirements demand preventing multiple concurrent player instances, a simple PID lockfile mechanism can be introduced, entirely separate from complex IPC state syncing.
```
