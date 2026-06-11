// Package provider defines the MediaProvider interface and the in-process
// Registry that maps provider IDs to their implementations.
package provider

import (
	"context"
	"sync"

	"github.com/fyom/fyom/internal/model"
)

// Provider generates time-limited resource URLs for a specific storage backend.
// Implementations must be safe for concurrent use.
//
// URL generation is the only responsibility of this interface.
// Serving files is a LocalProvider implementation detail hidden behind
// /api/v1/media/ routes — do NOT add Serve() or CanServe() here.
//
// Context usage:
//   LocalProvider      — ignores ctx (synchronous HMAC generation)
//   S3Provider         — passes ctx to AWS SDK for timeout/cancellation
//   RemoteFyomProvider — passes ctx to outbound HTTP calls
type Provider interface {
	// ID returns the unique identifier for this provider instance.
	// Examples: "local", "wasabi-main", "friend-alice"
	ID() string

	// Type returns the provider type string.
	// Examples: "local", "s3", "remote_fyom"
	Type() string

	// SupportsRedirect reports whether StreamURL/PosterURL/BackdropURL/LogoURL return
	// URLs suitable for an HTTP 302 Location header rather than inline JSON.
	//
	//   LocalProvider       → false  (URLs served by this process)
	//   S3Provider          → false  (direct S3 presigned URLs, no redirect needed)
	//   RemoteFyomProvider  → true   (302 to peer's presigned URL; zero local bandwidth)
	//
	// Handlers must check this before deciding how to return URLs.
	// Do NOT use type assertions as a substitute.
	SupportsRedirect() bool

	// StreamURL returns a time-limited URL for streaming the media file.
	// Returns ("", nil) if the item has no streamable file path.
	StreamURL(ctx context.Context, item *model.MediaItem) (string, error)

	// PosterURL returns a time-limited URL for the poster image.
	// Returns ("", nil) if the item has no poster path.
	PosterURL(ctx context.Context, item *model.MediaItem) (string, error)

	// BackdropURL returns a time-limited URL for the backdrop image.
	// Returns ("", nil) if the item has no backdrop path.
	BackdropURL(ctx context.Context, item *model.MediaItem) (string, error)

	// LogoURL returns a time-limited URL for the logo image.
	// Returns ("", nil) if the item has no logo path.
	LogoURL(ctx context.Context, item *model.MediaItem) (string, error)
}

// Registry manages all registered providers.
// Safe for concurrent reads and writes.
type Registry struct {
	providers map[string]Provider
	mu        sync.RWMutex
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider to the registry.
// Panics if the ID is already registered — this is a programming error
// that must be caught at startup, not at request time.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[p.ID()]; exists {
		panic("provider already registered: " + p.ID())
	}
	r.providers[p.ID()] = p
}

// Get returns the provider for the given ID.
// Returns (nil, false) if not found. Callers must handle the not-found case
// explicitly — do NOT use MustGet in any request-handling path.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// List returns all registered providers in unspecified order.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}
