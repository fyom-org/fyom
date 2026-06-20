# Full Workload - Local Provider Media Playback: Direct File URI Strategy

Target file: [docs/local-provider-implement.md](docs/local-provider-implement.md)

## Status

```text
Document status: Draft for implementation
Current desktop baseline: Tauri desktop shell + Go sidecar
Candidate desktop runtime: Wails in-process Go runtime
Playback strategy: External player launch mode
Progress strategy: Managed clients report progress through standard APIs
mpv IPC status: Explicitly out of scope for this phase
```

***

# 1. Context & Scope

The `fyom` project supports multiple storage providers for media files. The `LocalProvider` represents media files that physically reside on the same filesystem namespace that the fyom desktop process can access.

Examples:

```text
- local disks
- external USB drives
- NVMe arrays
- OS-mounted SMB shares
- OS-mounted NFS volumes
- macOS paths such as /Volumes/NAS/Movies
- Linux paths such as /mnt/media/Movies
- Windows drive paths such as Z:\Movies
- Windows UNC paths such as \\NAS\media\Movies
```

The naive implementation routes all media playback through the fyom Go HTTP backend:

```text
external player
    -> http://127.0.0.1:{port}/api/v1/media/{id}/stream
    -> Go HTTP handler
    -> os.Open / io.Copy
    -> HTTP response body
    -> external player
```

For `LocalProvider`, this is the wrong data path.

The local media file is already accessible to the external player through the same operating system filesystem namespace. Routing the file bytes through the fyom Go HTTP layer turns fyom into a media proxy, which contradicts the intended architecture.

fyom's desktop playback philosophy is:

```text
fyom is a media scheduler, not a media proxy.
```

Therefore, for desktop external-player mode, `LocalProvider` playback must use direct `file://` URI invocation.

The desired data path is:

```text
fyom
    -> authorize
    -> resolve local file path
    -> construct file:// URI
    -> launch external player

external player
    -> native OS file I/O
    -> direct seek/read/decode/playback
```

After the external player is launched, fyom exits the media byte path entirely.

This document defines the implementation strategy, boundaries, risks, verification plan, and future extension points.

***

# 2. Strategic Decision

For `LocalProvider` media items in desktop external-player mode, fyom will not proxy the video byte stream through the Go HTTP layer.

Instead, fyom performs these responsibilities:

```text
1. Authorize the user.
2. Resolve the media record.
3. Validate the local file path.
4. Construct a well-formed file:// URI.
5. Launch the configured external player.
6. Record launch success or failure.
```

fyom does not perform these responsibilities in external-player mode:

```text
- It does not stream local file bytes through HTTP.
- It does not perform io.Copy for local video playback.
- It does not re-authorize per byte range.
- It does not monitor external player progress through mpv IPC.
- It does not require a specific external player.
- It does not guarantee precise progress sync in external-player mode.
```

A `PlaybackURIResolver` abstraction provides the boundary between provider-specific URI resolution and player invocation.

Initial resolver set:

```text
- LocalFileResolver
- HTTPStreamResolver
```

Future resolver candidates:

```text
- WebDAVResolver
- SMBResolver
- MountedNetworkResolver
```

The current phase explicitly does not implement mpv JSON IPC.

***

# 3. Why This Design Exists

## 3.1 LocalProvider HTTP Proxy Is a Data Path Bug

The naive local streaming path is:

```text
external player
    -> TCP stack
    -> Go HTTP handler
    -> Go io.Copy
    -> kernel file I/O
    -> Go userspace
    -> TCP stack
    -> external player
```

Even over loopback, the path still exercises:

```text
- HTTP request parsing
- Range request handling
- TCP loopback stack
- Go userspace buffer copies
- kernel-to-userspace transitions
- userspace-to-kernel transitions
```

For high-bitrate media, especially 4K HDR content, this cost is measurable.

More importantly, random seek performance is degraded. An HTTP Range request must round-trip through the local HTTP server before the player receives the first byte. Direct local file access allows the player to use native seek primitives such as:

```text
- lseek(2) on Unix-like systems
- SetFilePointerEx on Windows
```

The perceived seek lag when routing local files through HTTP is not merely a UX defect. It is a symptom of the wrong architecture.

## 3.2 fyom Should Separate Control Plane and Data Plane

fyom should own the control plane:

```text
- authentication
- RBAC
- metadata
- library management
- provider selection
- playback URI resolution
- launch orchestration
- watch-progress persistence API for managed clients
```

The external player should own the data plane:

```text
- file I/O
- buffering
- seeking
- decoding
- subtitle discovery
- audio/video rendering
- player-specific local resume behavior
```

This is consistent with presigned URL patterns used by remote storage systems:

```text
Authorize once.
Issue a resource locator.
Step out of the byte path.
```

## 3.3 Future Flutter Clients Change the Progress Strategy

fyom is expected to support native clients built with Dart + Flutter.

In managed clients, playback progress should be collected by the client and reported to the fyom backend through standard APIs.

The long-term model is:

```text
Flutter client / browser client / future managed clients
    -> own playback state
    -> POST progress to fyom backend
    -> fyom persists progress
```

Therefore, implementing mpv-specific IPC inside the Wails desktop shell is not the correct long-term investment.

mpv IPC would be:

```text
- Wails-specific
- mpv-specific
- desktop-only
- platform-specific
- not reusable by Flutter clients
```

The preferred strategy is:

```text
External-player mode:
    launch only, no precise progress guarantee

Managed-client mode:
    client reports progress through standard backend API
```

***

# 4. Consequences

## 4.1 Positive Consequences

### Direct local playback

For local media, the external player reads directly from the filesystem.

```text
No Go io.Copy.
No HTTP Range proxy.
No loopback TCP data path.
```

### Lower CPU and memory overhead

During local playback, fyom does not move media bytes.

The Go process only performs:

```text
- authorization
- path validation
- URI construction
- player launch
```

### Native seek behavior

The external player uses native OS file seek and buffering behavior.

This is especially valuable for:

```text
- large 4K files
- HDR remux files
- high-bitrate local media
- network-mounted storage with OS-level caching
```

### Better alignment with external players

External players such as mpv, VLC, IINA, and system default openers are optimized for local file playback.

For mpv specifically, local `file://` playback can use adjacent subtitle discovery when configured with:

```text
--sub-auto=fuzzy
```

This enhancement is optional and player-specific. It does not make mpv a required dependency.

### Headless server mode remains valid

This optimization applies to desktop external-player mode where the player process shares the filesystem namespace with fyom.

Headless server mode and remote clients still require HTTP streaming where appropriate.

Examples requiring HTTP stream:

```text
- browser client
- mobile client
- remote Flutter client
- remote fyom-server deployment
- external player not sharing the same filesystem namespace
```

***

## 4.2 Negative Consequences

### RBAC is enforced at URI issuance time

With direct file access, fyom authorizes before issuing the `file://` URI.

After the external player opens the file, fyom cannot re-authorize each byte range.

If permissions are revoked mid-playback, the already-launched player session may continue until the player closes the file.

This is an accepted tradeoff for desktop external-player mode.

### OS filesystem permissions become part of deployment

fyom cannot grant access to files that the OS user cannot read.

The OS user running fyom must have read access to the configured media directories.

This is a deployment responsibility.

### Byte-level access logging is lost

The HTTP proxy model can log every Range request.

Direct file playback bypasses fyom entirely after launch. fyom cannot observe byte ranges read by the player.

This is intentional.

### Precise watch progress is not guaranteed in external-player mode

Because this phase does not implement mpv IPC, fyom does not precisely know:

```text
- current playback position
- pause state
- seek events
- EOF reason
- whether the user actually watched the item
```

External-player mode may record lightweight launch metadata, but it does not guarantee watch-progress accuracy.

Precise progress sync is reserved for managed clients.

***

# 5. Non-goals

The following items are explicitly out of scope for this phase:

```text
- Do not implement mpv JSON IPC.
- Do not implement Windows named pipe integration for mpv.
- Do not implement Unix socket retry loops for player observation.
- Do not parse mpv JSON RPC events.
- Do not build real-time Wails UI progress sync from external players.
- Do not mark media as watched based on external player EOF.
- Do not make mpv a required or privileged playback backend.
- Do not replace future Flutter progress reporting with desktop player probing.
- Do not remove HTTP streaming support for browser/headless/remote clients.
- Do not make Wails migration depend on this playback change.
```

***

# 6. Target Architecture

## 6.1 Desktop External-player Mode

```text
Frontend
    |
    | play request
    v
Desktop runtime
    |
    | calls Go playback service
    v
PlaybackService
    |
    | RBAC
    | media lookup
    | provider selection
    v
PlaybackURIResolver
    |
    | LocalProvider -> file:// URI
    | HTTP provider -> http(s) URI
    v
PlayerInvoker
    |
    | exec.Command(...)
    v
External player
    |
    | direct file I/O or remote stream I/O
    v
Playback
```

## 6.2 Managed-client Mode

```text
Flutter client / browser client
    |
    | owns playback state
    v
Native or browser video player
    |
    | periodic progress events
    v
POST /api/v1/media/{mediaID}/watch-progress
    |
    v
fyom backend
    |
    v
watch_progress table
```

## 6.3 Key Architectural Boundary

```text
Playback URI resolution is common infrastructure.
External player observation is not core infrastructure.
Progress reporting belongs to managed clients.
```

***

# 7. Task Breakdown

════════════════════════════════════════════════════════════

## Part A: Define Playback Types

### Goal

Create a provider-agnostic playback contract that can be used by both desktop external-player mode and future managed clients.

### File: `internal/playback/types.go` (NEW)

```go
package playback

type ProviderType string

const (
	ProviderLocal  ProviderType = "local"
	ProviderHTTP   ProviderType = "http"
	ProviderWebDAV ProviderType = "webdav"
	ProviderSMB    ProviderType = "smb"
)

type ClientMode string

const (
	ClientModeDesktopExternal ClientMode = "desktop-external"
	ClientModeBrowser         ClientMode = "browser"
	ClientModeRemote          ClientMode = "remote"
	ClientModeManagedNative   ClientMode = "managed-native"
)

// PlaybackInfo carries everything needed to launch playback or return a
// playback target to a managed client.
//
// It must remain player-agnostic.
// Do not add mpv IPC fields to this struct.
type PlaybackInfo struct {
	// URI is the resource identifier passed to the external player or client.
	//
	// Examples:
	//   Local desktop: file:///mnt/media/Movies/Inception%20(2010).mkv
	//   Remote stream: https://fyom.example.com/api/v1/media/abc/stream?token=...
	URI string `json:"uri"`

	// Type identifies which provider produced this playback info.
	Type ProviderType `json:"type"`

	// StartPosition is optional.
	// External players may ignore this if unsupported.
	// Managed clients should use this value as their initial playback position.
	StartPosition float64 `json:"startPosition"`

	// ExtraArgs contains optional player arguments for external-player mode.
	// This field must not be used for authorization secrets unless the target
	// player invocation is explicitly trusted.
	ExtraArgs []string `json:"extraArgs,omitempty"`
}
```

### Strict Requirements

```text
- Do not include IPCSocketPath.
- Do not include mpv-specific fields.
- Do not include Wails-specific fields.
- Keep PlaybackInfo usable by future Flutter clients.
```

***

════════════════════════════════════════════════════════════

## Part B: Define PlaybackURIResolver

### Goal

Create a resolver abstraction so provider-specific URI construction is isolated from the service layer.

### File: `internal/playback/resolver.go` (NEW)

```go
package playback

import (
	"context"

	"fyom/internal/domain"
)

// PlaybackURIResolver translates an already-authorized media record into
// a PlaybackInfo suitable for the requested client mode.
//
// Implementations must not perform authorization checks.
// RBAC belongs to PlaybackService.
type PlaybackURIResolver interface {
	Supports(provider ProviderType, mode ClientMode) bool

	Resolve(
		ctx context.Context,
		media *domain.Media,
		startPos float64,
		mode ClientMode,
	) (*PlaybackInfo, error)
}
```

### Design Rule

```text
Authorization is not resolver responsibility.
Resolvers only construct playback targets.
```

***

════════════════════════════════════════════════════════════

## Part C: Implement PlaybackService

### Goal

Centralize RBAC, media lookup, resume position lookup, and resolver selection.

### File: `internal/playback/service.go` (NEW or EXTEND)

```go
package playback

import (
	"context"
	"fmt"

	"fyom/internal/domain"
)

type RBACService interface {
	AssertMediaReadAccess(ctx context.Context, userID string, mediaID string) error
}

type MediaRepository interface {
	GetMediaByID(ctx context.Context, mediaID string) (*domain.Media, error)
	GetWatchProgress(ctx context.Context, userID string, mediaID string) (*domain.WatchProgress, error)
}

type PlaybackService struct {
	rbac      RBACService
	repo      MediaRepository
	resolvers []PlaybackURIResolver
}

func NewPlaybackService(
	rbac RBACService,
	repo MediaRepository,
	resolvers []PlaybackURIResolver,
) (*PlaybackService, error) {
	if rbac == nil {
		return nil, fmt.Errorf("rbac service is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("media repository is required")
	}
	if len(resolvers) == 0 {
		return nil, fmt.Errorf("at least one playback resolver is required")
	}

	return &PlaybackService{
		rbac:      rbac,
		repo:      repo,
		resolvers: resolvers,
	}, nil
}

func (s *PlaybackService) GetPlaybackInfo(
	ctx context.Context,
	userID string,
	mediaID string,
	mode ClientMode,
) (*PlaybackInfo, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if mediaID == "" {
		return nil, fmt.Errorf("mediaID is required")
	}
	if mode == "" {
		return nil, fmt.Errorf("client mode is required")
	}

	// RBAC must complete before any file path, storage URI, or credential is
	// constructed or returned.
	if err := s.rbac.AssertMediaReadAccess(ctx, userID, mediaID); err != nil {
		return nil, fmt.Errorf("access denied: %w", err)
	}

	media, err := s.repo.GetMediaByID(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("get media: %w", err)
	}

	startPos := 0.0
	progress, err := s.repo.GetWatchProgress(ctx, userID, mediaID)
	if err == nil && progress != nil {
		startPos = progress.PositionSeconds
	}

	provider := ProviderType(media.ProviderType)

	for _, resolver := range s.resolvers {
		if resolver.Supports(provider, mode) {
			return resolver.Resolve(ctx, media, startPos, mode)
		}
	}

	return nil, fmt.Errorf("no playback resolver for provider=%s mode=%s", provider, mode)
}
```

### Important Clarification

RBAC may internally query the database to determine library ownership or permissions.

The invariant is not that no database query happens before RBAC. The invariant is:

```text
No file path, provider URI, storage credential, or playback URI may be disclosed or constructed before RBAC succeeds.
```

***

════════════════════════════════════════════════════════════

## Part D: Implement LocalFileResolver

### Goal

Resolve authorized `LocalProvider` media into safe `file://` URIs for desktop external-player mode.

### File: `internal/playback/errors.go` (NEW)

```go
package playback

import "errors"

var (
	ErrPathTraversal    = errors.New("path is outside allowed roots")
	ErrSymlinkTraversal = errors.New("symlink resolves outside allowed roots")
	ErrFileNotFound     = errors.New("media file not found")
	ErrNotRegularFile   = errors.New("media path is not a regular file")
	ErrFileNotReadable  = errors.New("media file is not readable")
	ErrAllowedRootsEmpty = errors.New("allowed roots must not be empty")
)
```

### File: `internal/playback/local_resolver.go` (NEW)

```go
package playback

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"fyom/internal/domain"
)

type LocalFileResolver struct {
	allowedRoots []string
}

// NewLocalFileResolver creates a LocalFileResolver with normalized,
// absolute, symlink-resolved roots.
//
// Each allowed root must be a directory.
func NewLocalFileResolver(roots []string) (*LocalFileResolver, error) {
	if len(roots) == 0 {
		return nil, ErrAllowedRootsEmpty
	}

	normalized := make([]string, 0, len(roots))

	for _, root := range roots {
		if root == "" {
			return nil, fmt.Errorf("allowed root must not be empty")
		}

		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve allowed root absolute path: %w", err)
		}

		clean := filepath.Clean(abs)

		stat, err := os.Stat(clean)
		if err != nil {
			return nil, fmt.Errorf("stat allowed root %q: %w", clean, err)
		}

		if !stat.IsDir() {
			return nil, fmt.Errorf("allowed root is not a directory: %s", clean)
		}

		real, err := filepath.EvalSymlinks(clean)
		if err != nil {
			return nil, fmt.Errorf("resolve allowed root symlink %q: %w", clean, err)
		}

		normalized = append(normalized, normalizePathForCompare(real))
	}

	return &LocalFileResolver{
		allowedRoots: normalized,
	}, nil
}

func (r *LocalFileResolver) Supports(provider ProviderType, mode ClientMode) bool {
	return provider == ProviderLocal && mode == ClientModeDesktopExternal
}

func (r *LocalFileResolver) Resolve(
	ctx context.Context,
	media *domain.Media,
	startPos float64,
	mode ClientMode,
) (*PlaybackInfo, error) {
	if media == nil {
		return nil, fmt.Errorf("media is required")
	}

	if media.FilePath == "" {
		return nil, fmt.Errorf("media file path is required")
	}

	// Step A: Convert to absolute path and clean syntactic traversal.
	abs, err := filepath.Abs(media.FilePath)
	if err != nil {
		return nil, fmt.Errorf("resolve media absolute path: %w", err)
	}

	clean := filepath.Clean(abs)
	cleanCompare := normalizePathForCompare(clean)

	// Step B: Early prefix validation before symlink resolution.
	// This catches obvious textual traversal and prefix collision cases.
	if !r.isUnderAllowedRoot(cleanCompare) {
		return nil, ErrPathTraversal
	}

	// Step C: Stat follows symlinks. This catches missing files and dangling
	// symlinks before EvalSymlinks.
	if _, err := os.Stat(clean); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("stat media file: %w", err)
	}

	// Step D: Resolve symlinks and validate again.
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return nil, fmt.Errorf("resolve media symlink: %w", err)
	}

	realCompare := normalizePathForCompare(real)

	if !r.isUnderAllowedRoot(realCompare) {
		return nil, ErrSymlinkTraversal
	}

	// Step E: Ensure the final target is a regular file.
	stat, err := os.Stat(real)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("stat resolved media file: %w", err)
	}

	if !stat.Mode().IsRegular() {
		return nil, ErrNotRegularFile
	}

	// Step F: Optional readability check.
	file, err := os.Open(real)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFileNotReadable, err)
	}
	_ = file.Close()

	// Step G: Construct file:// URI.
	uri, err := localPathToFileURI(real)
	if err != nil {
		return nil, fmt.Errorf("construct file URI: %w", err)
	}

	return &PlaybackInfo{
		URI:           uri,
		Type:          ProviderLocal,
		StartPosition: startPos,
	}, nil
}

func (r *LocalFileResolver) isUnderAllowedRoot(path string) bool {
	for _, root := range r.allowedRoots {
		if samePathOrUnderRoot(path, root) {
			return true
		}
	}
	return false
}

func samePathOrUnderRoot(path string, root string) bool {
	path = normalizePathForCompare(path)
	root = normalizePathForCompare(root)

	if path == root {
		return true
	}

	separator := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		separator = `\`
	}

	return strings.HasPrefix(path, root+separator)
}

func normalizePathForCompare(path string) string {
	clean := filepath.Clean(path)

	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}

	return clean
}
```

### Security Requirements

Every local file must pass:

```text
- filepath.Abs
- filepath.Clean
- allowed root prefix check before symlink resolution
- os.Stat
- filepath.EvalSymlinks
- allowed root prefix check after symlink resolution
- regular file check
- optional readability check
- file:// URI construction through url.URL
```

Do not remove any of these checks without replacing them with an equivalent or stronger control.

***

════════════════════════════════════════════════════════════

## Part E: Implement Cross-platform file:// URI Construction

### Goal

Construct correct `file://` URIs for POSIX paths, Windows drive-letter paths, and Windows UNC paths.

### File: `internal/playback/uri.go` (NEW)

```go
package playback

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func localPathToFileURI(absPath string) (string, error) {
	return localPathToFileURIForGOOS(runtime.GOOS, absPath)
}

func localPathToFileURIForGOOS(goos string, absPath string) (string, error) {
	if absPath == "" {
		return "", errors.New("path is required")
	}

	if goos == "windows" {
		return windowsPathToFileURI(absPath)
	}

	slashed := filepath.ToSlash(absPath)
	if !strings.HasPrefix(slashed, "/") {
		return "", errors.New("POSIX file URI requires an absolute path")
	}

	return (&url.URL{
		Scheme: "file",
		Path:   slashed,
	}).String(), nil
}

func windowsPathToFileURI(path string) (string, error) {
	slashed := strings.ReplaceAll(path, `\`, `/`)

	// Normalize extended-length Windows paths:
	//   //?/C:/path -> C:/path
	//   //?/UNC/server/share/path -> //server/share/path
	if strings.HasPrefix(slashed, "//?/UNC/") {
		slashed = "//" + strings.TrimPrefix(slashed, "//?/UNC/")
	} else if strings.HasPrefix(slashed, "//?/") {
		slashed = strings.TrimPrefix(slashed, "//?/")
	}

	// UNC path:
	//   //NAS/media/movie.mkv -> file://NAS/media/movie.mkv
	if strings.HasPrefix(slashed, "//") {
		trimmed := strings.TrimPrefix(slashed, "//")
		parts := strings.SplitN(trimmed, "/", 2)

		if len(parts) == 0 || parts[0] == "" {
			return "", errors.New("invalid UNC path")
		}

		host := parts[0]
		pathPart := "/"

		if len(parts) == 2 && parts[1] != "" {
			pathPart += parts[1]
		}

		return (&url.URL{
			Scheme: "file",
			Host:   host,
			Path:   pathPart,
		}).String(), nil
	}

	// Drive-letter path:
	//   C:/Movies/File.mkv -> file:///C:/Movies/File.mkv
	if len(slashed) >= 2 && slashed[1] == ':' {
		return (&url.URL{
			Scheme: "file",
			Path:   "/" + slashed,
		}).String(), nil
	}

	return "", errors.New("unsupported Windows path format")
}
```

### Expected URI Examples

| OS      | Input path                    | Output URI                                                      |
| ------- | ----------------------------- | --------------------------------------------------------------- |
| Linux   | `/mnt/media/Movie (2010).mkv` | `file:///mnt/media/Movie%20(2010).mkv`                          |
| macOS   | `/Volumes/NAS/影视/测试.mkv`      | `file:///Volumes/NAS/%E5%BD%B1%E8%A7%86/%E6%B5%8B%E8%AF%95.mkv` |
| Windows | `C:\Movies\Movie (2010).mkv`  | `file:///C:/Movies/Movie%20(2010).mkv`                          |
| Windows | `\\NAS\media\movie.mkv`       | `file://NAS/media/movie.mkv`                                    |

### Important Rule

Never construct `file://` URIs by string concatenation.

Do not use:

```go
uri := "file://" + path
```

Always use `url.URL`.

***

════════════════════════════════════════════════════════════

## Part F: Implement HTTPStreamResolver

### Goal

Preserve HTTP stream behavior for browser, remote, and headless modes.

### File: `internal/playback/http_resolver.go` (NEW or EXTEND)

```go
package playback

import (
	"context"
	"fmt"
	"net/url"

	"fyom/internal/domain"
)

type HTTPStreamResolver struct {
	BaseURL string
}

func NewHTTPStreamResolver(baseURL string) (*HTTPStreamResolver, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	return &HTTPStreamResolver{
		BaseURL: baseURL,
	}, nil
}

func (r *HTTPStreamResolver) Supports(provider ProviderType, mode ClientMode) bool {
	switch mode {
	case ClientModeBrowser, ClientModeRemote, ClientModeManagedNative:
		return true
	default:
		return provider != ProviderLocal
	}
}

func (r *HTTPStreamResolver) Resolve(
	ctx context.Context,
	media *domain.Media,
	startPos float64,
	mode ClientMode,
) (*PlaybackInfo, error) {
	if media == nil {
		return nil, fmt.Errorf("media is required")
	}

	base, err := url.Parse(r.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}

	// Adjust this path to match the actual fyom stream endpoint.
	base.Path = fmt.Sprintf("/api/v1/media/%s/stream", media.ID)

	return &PlaybackInfo{
		URI:           base.String(),
		Type:          ProviderHTTP,
		StartPosition: startPos,
	}, nil
}
```

### Important Rule

`LocalProvider` should use `file://` only when the client mode is `ClientModeDesktopExternal`.

The same local media may still use HTTP stream for:

```text
- browser client
- remote client
- managed native client without shared filesystem
- headless server mode
```

***

════════════════════════════════════════════════════════════

## Part G: Implement External Player Invoker

### Goal

Launch the configured external player without binding to mpv IPC.

### File: `internal/playback/player.go` (NEW)

```go
package playback

type PlayerKind string

const (
	PlayerKindMPV     PlayerKind = "mpv"
	PlayerKindVLC     PlayerKind = "vlc"
	PlayerKindIINA    PlayerKind = "iina"
	PlayerKindDefault PlayerKind = "default"
	PlayerKindCustom  PlayerKind = "custom"
)

type PlayerProfile struct {
	Kind        PlayerKind `json:"kind"`
	Command     string     `json:"command"`
	DefaultArgs []string   `json:"defaultArgs,omitempty"`
}

type LaunchResult struct {
	Started bool   `json:"started"`
	Message string `json:"message,omitempty"`
}
```

### File: `internal/playback/player_args.go` (NEW)

```go
package playback

import "fmt"

func buildStartPositionArg(kind PlayerKind, startPosition float64) string {
	if startPosition <= 0 {
		return ""
	}

	switch kind {
	case PlayerKindMPV:
		return fmt.Sprintf("--start=%f", startPosition)
	case PlayerKindVLC:
		return fmt.Sprintf("--start-time=%f", startPosition)
	default:
		return ""
	}
}

func buildLocalPlaybackArgs(kind PlayerKind) []string {
	switch kind {
	case PlayerKindMPV:
		return []string{"--sub-auto=fuzzy"}
	default:
		return nil
	}
}
```

### File: `internal/playback/invoker.go` (NEW)

```go
package playback

import (
	"context"
	"fmt"
	"log"
	"os/exec"
)

type LaunchEventSink interface {
	OnPlaybackProcessExit(mediaID string, err error)
}

type PlayerInvoker struct {
	Profile PlayerProfile
	Sink    LaunchEventSink
}

func NewPlayerInvoker(profile PlayerProfile, sink LaunchEventSink) (*PlayerInvoker, error) {
	if profile.Kind == "" {
		return nil, fmt.Errorf("player kind is required")
	}

	if profile.Command == "" {
		return nil, fmt.Errorf("player command is required")
	}

	return &PlayerInvoker{
		Profile: profile,
		Sink:    sink,
	}, nil
}

func (inv *PlayerInvoker) Launch(
	ctx context.Context,
	info PlaybackInfo,
	mediaID string,
) (LaunchResult, error) {
	if info.URI == "" {
		return LaunchResult{}, fmt.Errorf("playback URI is required")
	}

	args := inv.buildArgs(info)

	// Do not use request-scoped cancellation for external player lifetime.
	// The player should continue running after the launch request returns.
	cmd := exec.Command(inv.Profile.Command, args...)

	if err := cmd.Start(); err != nil {
		return LaunchResult{}, fmt.Errorf("failed to start external player: %w", err)
	}

	// Reap the process to avoid zombies.
	// This is not playback observation and does not provide progress sync.
	go func() {
		err := cmd.Wait()
		if inv.Sink != nil {
			inv.Sink.OnPlaybackProcessExit(mediaID, err)
			return
		}
		if err != nil {
			log.Printf("external player exited with error for media %s: %v", mediaID, err)
		}
	}()

	return LaunchResult{
		Started: true,
		Message: "external player started",
	}, nil
}

func (inv *PlayerInvoker) buildArgs(info PlaybackInfo) []string {
	args := make([]string, 0, len(inv.Profile.DefaultArgs)+len(info.ExtraArgs)+4)

	args = append(args, inv.Profile.DefaultArgs...)

	if startArg := buildStartPositionArg(inv.Profile.Kind, info.StartPosition); startArg != "" {
		args = append(args, startArg)
	}

	if info.Type == ProviderLocal {
		args = append(args, buildLocalPlaybackArgs(inv.Profile.Kind)...)
	}

	args = append(args, info.ExtraArgs...)

	// Put the URI last for broad compatibility with player argument parsers.
	args = append(args, info.URI)

	return args
}
```

### Explicit Non-goal

Do not add:

```text
--input-ipc-server
```

Do not create:

```text
IPCSocketPath
```

Do not start:

```text
runIPCSession
```

Do not add dependency on:

```text
github.com/Microsoft/go-winio
```

***

════════════════════════════════════════════════════════════

## Part H: Add Future Watch Progress API for Managed Clients

### Goal

Define the future progress synchronization direction for Flutter and other managed clients.

This does not need to be used by external-player mode.

### File: `internal/watch/types.go` (NEW or EXTEND)

```go
package watch

type ProgressState string

const (
	ProgressStatePlaying ProgressState = "playing"
	ProgressStatePaused  ProgressState = "paused"
	ProgressStateEnded   ProgressState = "ended"
	ProgressStateStopped ProgressState = "stopped"
)

type SaveProgressInput struct {
	UserID          string
	MediaID         string
	PositionSeconds float64
	DurationSeconds float64
	State           ProgressState
	ClientType      string
}
```

### File: `internal/watch/service.go` (NEW or EXTEND)

```go
package watch

import (
	"context"
	"fmt"
)

type RBACService interface {
	AssertMediaReadAccess(ctx context.Context, userID string, mediaID string) error
}

type Repository interface {
	SavePosition(ctx context.Context, userID string, mediaID string, positionSeconds float64) error
	MarkWatched(ctx context.Context, userID string, mediaID string, durationSeconds float64) error
}

type Service struct {
	rbac RBACService
	repo Repository
}

func NewService(rbac RBACService, repo Repository) (*Service, error) {
	if rbac == nil {
		return nil, fmt.Errorf("rbac service is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("watch repository is required")
	}

	return &Service{
		rbac: rbac,
		repo: repo,
	}, nil
}

func (s *Service) SaveProgress(ctx context.Context, input SaveProgressInput) error {
	if input.UserID == "" {
		return fmt.Errorf("userID is required")
	}
	if input.MediaID == "" {
		return fmt.Errorf("mediaID is required")
	}

	if err := s.rbac.AssertMediaReadAccess(ctx, input.UserID, input.MediaID); err != nil {
		return fmt.Errorf("access denied: %w", err)
	}

	if input.PositionSeconds < 0 {
		return fmt.Errorf("positionSeconds must be non-negative")
	}

	if input.DurationSeconds > 0 && input.PositionSeconds > input.DurationSeconds {
		input.PositionSeconds = input.DurationSeconds
	}

	if input.State == ProgressStateEnded {
		return s.repo.MarkWatched(ctx, input.UserID, input.MediaID, input.DurationSeconds)
	}

	return s.repo.SavePosition(ctx, input.UserID, input.MediaID, input.PositionSeconds)
}
```

### File: `internal/http/dto/watch_progress.go` (NEW or EXTEND)

```go
package dto

type SaveWatchProgressRequest struct {
	PositionSeconds float64 `json:"positionSeconds"`
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
	State           string  `json:"state"`
	ClientType      string  `json:"clientType,omitempty"`
}
```

### Target HTTP Endpoint

```text
POST /api/v1/media/{mediaID}/watch-progress
```

This endpoint is intended for:

```text
- future Flutter desktop client
- future Flutter mobile client
- browser managed player
- any client that owns playback state
```

It is not required for external-player launch mode.

***

# 8. Key Invariants

## Invariant 1: RBAC before disclosure

RBAC must complete before any local path, storage URI, provider credential, or playback URI is disclosed or constructed.

A `file://` URI is a credential.

Issuing it to an unauthorized caller is equivalent to granting filesystem read access for that file.

## Invariant 2: Local paths require double validation

Every local path must survive both:

```text
- syntactic validation after Abs + Clean
- real path validation after EvalSymlinks
```

These checks close different attack surfaces.

`filepath.Clean` handles textual traversal.

`filepath.EvalSymlinks` handles symlink escape.

Both are mandatory.

## Invariant 3: URI construction must use url.URL

`file://` URIs must be constructed through `url.URL`.

String concatenation is forbidden.

## Invariant 4: External-player mode is launch-only

External-player mode launches a configured player and does not require player IPC.

It may reap the child process to avoid zombies, but it must not depend on player-specific control protocols for correctness.

## Invariant 5: Progress is client-reported in managed clients

Precise playback progress belongs to managed clients.

The standard path is:

```text
client playback state
    -> POST watch-progress API
    -> fyom backend persistence
```

***

# 9. Boundary Conditions

## Windows drive-letter paths

Input:

```text
C:\Movies\File.mkv
```

Normalized:

```text
C:/Movies/File.mkv
```

URI:

```text
file:///C:/Movies/File.mkv
```

## Windows UNC paths

Input:

```text
\\NAS\media\file.mkv
```

Normalized:

```text
//NAS/media/file.mkv
```

URI:

```text
file://NAS/media/file.mkv
```

The UNC server name becomes the URI host component.

## CJK and special characters

Examples:

```text
/Volumes/NAS/影视/测试.mkv
/mnt/media/Movie (2010).mkv
/mnt/media/file#name?.mkv
```

The `url.URL` struct must handle percent encoding.

Do not pre-encode the path before passing it to `url.URL`.

## Network-mounted volumes

An SMB or NFS share mounted at an OS path is treated as local from fyom's perspective.

Example:

```text
/Volumes/NAS/Movies/file.mkv
```

If the external player can read the same mounted path, `LocalFileResolver` may return a `file://` URI.

If per-stream authentication is required, do not use `LocalFileResolver`. Implement a dedicated resolver.

## Symlinks inside allowed roots

Valid:

```text
/media/movies/link.mkv -> /media/movies/actual.mkv
```

Invalid:

```text
/media/movies/escape.mkv -> /etc/shadow
```

The second case must fail after `EvalSymlinks`.

## Windows long paths

Deeply nested Windows paths may hit MAX\_PATH limitations unless long path support is enabled.

This is a deployment and packaging consideration.

Document it for Windows users with very deep media directory structures.

***

# 10. Failure Modes

## File deleted between validation and player open

`LocalFileResolver` may validate successfully, but the file may be deleted before the external player opens it.

Expected behavior:

```text
- fyom launches the player.
- player fails to open file.
- player displays its own error or exits.
- fyom may log process exit.
- fyom does not attempt to recover through HTTP proxy.
```

## OS permission denied

`os.Stat` may succeed, but `os.Open` or the player open may fail due to ACLs or filesystem permissions.

Expected behavior:

```text
- resolver should fail early if os.Open fails.
- player may still fail if permission changes after validation.
- UI should show a launch failure only when fyom itself fails to launch the process.
```

## External player not found

If the configured command cannot be found or started:

```text
PlayerInvoker.Launch returns an error.
```

Expected UI behavior:

```text
Show user-facing player configuration error.
```

## External player exits with non-zero status

fyom reaps the process to avoid zombies.

Non-zero exit may be logged or sent to a lightweight event sink.

This is not progress synchronization.

## AllowedRoots is empty

This must be rejected at startup.

Do not allow a `LocalFileResolver` with empty allowed roots.

## Browser mode accidentally receives file:// URI

This is a security and compatibility bug.

Browser and remote clients must receive HTTP stream URLs unless they are explicitly trusted managed clients sharing the same filesystem namespace.

***

# 11. How to Verify

════════════════════════════════════════════════════════════

## Unit Test: file:// URI construction

### File: `internal/playback/uri_test.go` (NEW)

```go
package playback

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalPathToFileURIForPOSIX(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: "/mnt/media/Movie (2010).mkv",
			want:  "file:///mnt/media/Movie%20(2010).mkv",
		},
		{
			input: "/Volumes/NAS/影视/测试.mkv",
			want:  "file:///Volumes/NAS/%E5%BD%B1%E8%A7%86/%E6%B5%8B%E8%AF%95.mkv",
		},
		{
			input: "/media/file#name?.mkv",
			want:  "file:///media/file%23name%3F.mkv",
		},
	}

	for _, c := range cases {
		got, err := localPathToFileURIForGOOS("linux", c.input)
		require.NoError(t, err)
		require.Equal(t, c.want, got)
	}
}

func TestLocalPathToFileURIForWindowsDriveLetter(t *testing.T) {
	got, err := localPathToFileURIForGOOS("windows", `C:\Movies\Movie (2010).mkv`)
	require.NoError(t, err)
	require.Equal(t, "file:///C:/Movies/Movie%20(2010).mkv", got)
}

func TestLocalPathToFileURIForWindowsUNC(t *testing.T) {
	got, err := localPathToFileURIForGOOS("windows", `\\NAS\media\movie.mkv`)
	require.NoError(t, err)
	require.Equal(t, "file://NAS/media/movie.mkv", got)
}

func TestLocalPathToFileURIRejectsRelativePOSIX(t *testing.T) {
	_, err := localPathToFileURIForGOOS("linux", "relative/movie.mkv")
	require.Error(t, err)
}

func TestLocalPathToFileURIRejectsUnsupportedWindowsPath(t *testing.T) {
	_, err := localPathToFileURIForGOOS("windows", `relative\movie.mkv`)
	require.Error(t, err)
}
```

***

## Unit Test: LocalFileResolver path traversal

### File: `internal/playback/local_resolver_test.go` (NEW)

```go
package playback

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fyom/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestLocalFileResolverResolvesFileUnderAllowedRoot(t *testing.T) {
	root := t.TempDir()

	filePath := filepath.Join(root, "movie.mkv")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0644))

	resolver, err := NewLocalFileResolver([]string{root})
	require.NoError(t, err)

	media := &domain.Media{
		ID:           "media-1",
		ProviderType: string(ProviderLocal),
		FilePath:     filePath,
	}

	info, err := resolver.Resolve(context.Background(), media, 0, ClientModeDesktopExternal)
	require.NoError(t, err)
	require.Equal(t, ProviderLocal, info.Type)
	require.Contains(t, info.URI, "file://")
}

func TestLocalFileResolverRejectsPrefixCollision(t *testing.T) {
	parent := t.TempDir()

	root := filepath.Join(parent, "media")
	other := filepath.Join(parent, "media2")

	require.NoError(t, os.Mkdir(root, 0755))
	require.NoError(t, os.Mkdir(other, 0755))

	filePath := filepath.Join(other, "movie.mkv")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0644))

	resolver, err := NewLocalFileResolver([]string{root})
	require.NoError(t, err)

	media := &domain.Media{
		ID:           "media-1",
		ProviderType: string(ProviderLocal),
		FilePath:     filePath,
	}

	_, err = resolver.Resolve(context.Background(), media, 0, ClientModeDesktopExternal)
	require.ErrorIs(t, err, ErrPathTraversal)
}

func TestLocalFileResolverRejectsDirectory(t *testing.T) {
	root := t.TempDir()

	resolver, err := NewLocalFileResolver([]string{root})
	require.NoError(t, err)

	media := &domain.Media{
		ID:           "media-1",
		ProviderType: string(ProviderLocal),
		FilePath:     root,
	}

	_, err = resolver.Resolve(context.Background(), media, 0, ClientModeDesktopExternal)
	require.ErrorIs(t, err, ErrNotRegularFile)
}

func TestNewLocalFileResolverRejectsEmptyRoots(t *testing.T) {
	_, err := NewLocalFileResolver(nil)
	require.ErrorIs(t, err, ErrAllowedRootsEmpty)
}
```

### Optional Unix-only symlink escape test

### File: `internal/playback/local_resolver_symlink_test.go` (NEW)

```go
//go:build !windows

package playback

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fyom/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestLocalFileResolverRejectsSymlinkEscape(t *testing.T) {
	parent := t.TempDir()

	root := filepath.Join(parent, "media")
	outside := filepath.Join(parent, "outside")

	require.NoError(t, os.Mkdir(root, 0755))
	require.NoError(t, os.Mkdir(outside, 0755))

	outsideFile := filepath.Join(outside, "secret.mkv")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret"), 0644))

	linkPath := filepath.Join(root, "escape.mkv")
	require.NoError(t, os.Symlink(outsideFile, linkPath))

	resolver, err := NewLocalFileResolver([]string{root})
	require.NoError(t, err)

	media := &domain.Media{
		ID:           "media-1",
		ProviderType: string(ProviderLocal),
		FilePath:     linkPath,
	}

	_, err = resolver.Resolve(context.Background(), media, 0, ClientModeDesktopExternal)
	require.ErrorIs(t, err, ErrSymlinkTraversal)
}
```

***

## Unit Test: PlayerInvoker does not add IPC args

### File: `internal/playback/invoker_test.go` (NEW)

```go
package playback

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlayerInvokerBuildArgsForMPVLocalDoesNotUseIPC(t *testing.T) {
	inv := &PlayerInvoker{
		Profile: PlayerProfile{
			Kind:        PlayerKindMPV,
			Command:     "mpv",
			DefaultArgs: []string{"--no-terminal"},
		},
	}

	info := PlaybackInfo{
		URI:           "file:///mnt/media/movie.mkv",
		Type:          ProviderLocal,
		StartPosition: 12.5,
	}

	args := inv.buildArgs(info)

	require.Contains(t, args, "--no-terminal")
	require.Contains(t, args, "--sub-auto=fuzzy")
	require.Contains(t, args, "--start=12.500000")
	require.Contains(t, args, "file:///mnt/media/movie.mkv")

	for _, arg := range args {
		require.NotContains(t, arg, "input-ipc-server")
	}
}
```

***

## Integration Test: RBAC gate

```bash
TOKEN=$(curl -s -X POST http://localhost:27402/api/v1/auth/login \
  -d '{"username":"guest","password":"guest"}' | jq -r .token)

curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${TOKEN}" \
  http://localhost:27402/api/v1/media/MEDIA_IN_FORBIDDEN_LIBRARY/playback-info
```

Expected:

```text
403
```

No local file path should appear in:

```text
- response body
- logs
- frontend error object
```

***

## Integration Test: LocalProvider desktop external mode returns file://

Use a media item backed by `LocalProvider`.

```bash
curl -s \
  -H "Authorization: Bearer ${TOKEN}" \
  "http://localhost:27402/api/v1/media/${MEDIA_ID}/playback-info?mode=desktop-external" | jq .
```

Expected:

```json
{
  "uri": "file:///...",
  "type": "local",
  "startPosition": 0
}
```

Must not return:

```text
http://127.0.0.1:27402/api/v1/media/.../stream
```

***

## Integration Test: Browser mode still returns HTTP stream

```bash
curl -s \
  -H "Authorization: Bearer ${TOKEN}" \
  "http://localhost:27402/api/v1/media/${MEDIA_ID}/playback-info?mode=browser" | jq .
```

Expected:

```json
{
  "uri": "http://127.0.0.1:27402/api/v1/media/...",
  "type": "http"
}
```

***

## Integration Test: external player receives file:// URI

In a test build, replace the configured player command with a stub script that writes all args to a file.

Example stub:

```bash
#!/usr/bin/env bash
printf '%s\n' "$@" > /tmp/fyom-player-args.txt
exit 0
```

Trigger playback from desktop UI.

Then verify:

```bash
cat /tmp/fyom-player-args.txt
```

Expected:

```text
contains file:///...
does not contain /api/v1/media/.../stream
does not contain --input-ipc-server
```

***

## Log Verification: no HTTP Range requests for local desktop playback

Enable HTTP access logging.

Trigger desktop external playback for a local media item.

Expected:

```text
No GET /api/v1/media/{id}/stream requests during local external-player playback.
No HTTP Range requests for local desktop external-player playback.
```

Metadata requests are allowed.

***

# 12. Verification Commands

Run:

```bash
go test ./...
```

Expected:

```text
exits 0
```

Run:

```bash
go test ./internal/playback
```

Expected:

```text
exits 0
```

Run frontend build:

```bash
npm ci
npm run build
```

Expected:

```text
frontend builds successfully
no runtime-specific import breakage
```

If Wails shell exists:

```bash
cd desktop/wails
wails build
```

Expected:

```text
Wails build succeeds
no mpv IPC dependency is introduced
```

If Tauri shell exists:

```bash
cd desktop/tauri
npm run tauri build
```

Expected:

```text
Tauri build succeeds
existing sidecar behavior remains intact
```

***

# 13. Edge Cases & Known Risks

## Risk: file:// URI leaks local path

A `file://` URI contains the local filesystem path.

Mitigation:

```text
- Only issue after RBAC succeeds.
- Do not return file:// to remote/browser clients.
- Do not log file:// URIs at info level.
- Redact file paths in user-facing errors where appropriate.
```

## Risk: symlink escape

A file under an allowed root may be a symlink to a path outside allowed roots.

Mitigation:

```text
- EvalSymlinks must be mandatory.
- Validate allowed root after symlink resolution.
```

## Risk: Windows path comparison

Windows paths are typically case-insensitive.

Mitigation:

```text
- Normalize comparison paths to lower-case on Windows.
- Test drive-letter and UNC paths.
```

## Risk: remote client receives file://

A remote client cannot use a server-local `file://` URI.

Mitigation:

```text
- Resolver selection must include ClientMode.
- Only desktop external-player mode may receive LocalProvider file://.
```

## Risk: external player cannot access mounted path

The fyom process may see a path that the external player cannot access due to sandboxing, app bundle constraints, or OS permissions.

Mitigation:

```text
- Validate readability with os.Open.
- Surface launch failures clearly.
- Document OS permission requirements.
```

## Risk: no precise progress in external-player mode

Without player IPC, fyom cannot know actual playback progress.

Mitigation:

```text
- Treat external-player mode as launch-only.
- Use managed client progress API for precise progress.
- Provide manual mark-watched or resume controls if needed.
- Let mpv/VLC/IINA handle their own local resume where configured.
```

## Risk: Wails migration conflates with playback improvement

Wails and LocalProvider file:// solve different problems.

Mitigation:

```text
- Implement LocalProvider file:// independently from Wails.
- Compare Tauri+file:// against Wails+file:// when evaluating runtime migration.
```

***

# 14. What Should Be Done

```text
- Implement PlaybackInfo without IPC fields.
- Implement PlaybackURIResolver.
- Implement PlaybackService with RBAC-first behavior.
- Implement LocalFileResolver for desktop external-player mode.
- Implement HTTPStreamResolver for browser/headless/remote modes.
- Implement cross-platform file:// URI construction.
- Implement PlayerInvoker as launch-only.
- Reap external player process to avoid zombies.
- Add watch-progress API for managed clients.
- Verify no HTTP Range requests happen for local desktop external playback.
- Verify no --input-ipc-server argument is added.
```

***

# 15. What Should Not Be Done

```text
- Do not implement mpv IPC in this phase.
- Do not add IPCSocketPath to PlaybackInfo.
- Do not add Windows named pipe support for player observation.
- Do not parse mpv JSON RPC.
- Do not emit real-time playback progress from external players.
- Do not mark watched based on external player process exit.
- Do not proxy LocalProvider bytes through HTTP in desktop external-player mode.
- Do not return file:// URIs to browser or remote clients.
- Do not construct file:// URIs by string concatenation.
- Do not skip EvalSymlinks.
- Do not let Wails migration depend on this playback implementation.
```

***

# 16. Future Work

## Flutter managed playback

Future Dart + Flutter clients should implement managed playback.

They should:

```text
- receive PlaybackInfo
- play media using native Flutter/video stack
- track playback position
- report progress through watch-progress API
- mark watched through backend API
```

## WebDAVResolver

A future `WebDAVResolver` may issue direct WebDAV URLs.

If authentication headers are required, it may use provider-specific `ExtraArgs` only for trusted external-player mode.

For managed clients, credentials should be delivered through a safer client-specific mechanism.

## SMBResolver

A future `SMBResolver` may generate `smb://` URIs for players that support native SMB access.

This should be separate from `LocalFileResolver`.

Mounted SMB shares that appear as local OS paths may continue to use `LocalFileResolver` if OS-level authentication is sufficient.

## Optional player observers

Player observation may be reconsidered in the future as an optional plugin system.

If implemented, it must remain:

```text
- optional
- player-specific
- not part of PlaybackInfo
- not required for LocalProvider file:// playback
- not required for Wails migration
```

mpv IPC may be one such future plugin, but it is not part of this phase.

## Manual watched controls

Because external-player mode does not guarantee progress sync, the UI may provide:

```text
- mark as watched
- mark as unwatched
- continue from last managed progress
- play from beginning
```

***

# 17. Strict Constraints

## Runtime Constraints

```text
- Do not remove existing Tauri behavior while implementing this.
- Do not require Wails for LocalProvider direct playback.
- Do not make Wails migration depend on player invocation changes.
- Do not remove HTTP stream endpoints needed by browser/headless/remote clients.
```

## Security Constraints

```text
- RBAC must complete before playback URI construction.
- Local file paths must be validated against allowed roots before and after symlink resolution.
- Local file paths must not be logged at info level.
- file:// URIs must not be returned to unauthorized or remote clients.
```

## Player Constraints

```text
- External player mode must remain player-agnostic.
- mpv may receive lightweight optional args such as --sub-auto=fuzzy.
- mpv must not be required.
- mpv IPC must not be implemented in this phase.
- VLC, IINA, system opener, and custom players must remain possible.
```

## Progress Constraints

```text
- External-player mode does not guarantee precise progress.
- watch_progress accuracy is guaranteed only for managed clients that report progress.
- Do not invent progress based on player process lifetime.
- Do not mark watched merely because the external player exited successfully.
```

## Documentation and Commit Constraints

```text
- Code comments must be written in English.
- Commit messages must be written in English.
- Chinese product documentation is allowed.
- Do not include unrelated refactors in the same change.
```

Suggested commit messages:

```text
docs(playback): define LocalProvider direct file URI strategy
feat(playback): add provider-aware playback URI resolver
feat(playback): add LocalProvider file URI resolver
feat(playback): add launch-only external player invoker
feat(watch): add managed client watch progress service
test(playback): verify local file URI and traversal protection
test(playback): ensure external player launch does not use IPC
```

***

# 18. Final Recommendation

The recommended implementation order is:

```text
P0: PlaybackInfo without IPC fields
P0: PlaybackURIResolver abstraction
P0: LocalFileResolver with strict path validation
P0: Cross-platform file:// URI construction
P0: Launch-only PlayerInvoker
P1: HTTPStreamResolver client-mode fallback
P1: Tauri desktop integration with file:// local playback
P1: No HTTP Range verification for local desktop playback
P2: Wails runtime evaluation using the same playback service
P2: Managed watch-progress API for future Flutter clients
```

The final architecture should be:

```text
fyom backend:
    authorization
    metadata
    provider resolution
    playback URI construction
    watch-progress persistence API

desktop shell:
    local orchestration
    external player launch

external player:
    native playback
    seeking
    decoding
    subtitle handling
    optional self-managed resume

future Flutter client:
    managed playback
    progress reporting
    cross-client resume
```

The core decision is:

```text
LocalProvider direct file:// playback is a required data-path correction.

mpv IPC is not required and should not be implemented in this phase.

Progress synchronization belongs to managed clients through standard APIs, not to Wails-specific external-player probing.
```

This keeps fyom aligned with its core philosophy:

```text
fyom is a media scheduler; more precisely, it is a **media catalog and resource dispatcher**. not a media proxy or player monitor.
```
