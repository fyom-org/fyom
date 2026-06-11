package provider

import (
	"context"
	"fmt"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/pkg/presign"
)

// LocalProvider serves media from the local filesystem.
// It generates HMAC-presigned URLs pointing to /api/v1/media/{id}/...
// The presign middleware on those routes validates exp+sig before serving.
type LocalProvider struct {
	signer *presign.Signer
}

// NewLocalProvider creates a LocalProvider backed by the given Signer.
func NewLocalProvider(signer *presign.Signer) *LocalProvider {
	return &LocalProvider{signer: signer}
}

func (p *LocalProvider) ID()   string { return "local" }
func (p *LocalProvider) Type() string { return "local" }

// SupportsRedirect returns false: LocalProvider URLs are served by this
// process and must be embedded in JSON responses, not sent as 302 redirects.
func (p *LocalProvider) SupportsRedirect() bool { return false }

func (p *LocalProvider) StreamURL(_ context.Context, item *model.MediaItem) (string, error) {
	if item.FilePath == "" {
		return "", nil
	}
	return p.signer.Generate(fmt.Sprintf("/api/v1/media/%s/stream", item.ID)), nil
}

func (p *LocalProvider) PosterURL(_ context.Context, item *model.MediaItem) (string, error) {
	if item.PosterPath == "" {
		return "", nil
	}
	return p.signer.Generate(fmt.Sprintf("/api/v1/media/%s/poster", item.ID)), nil
}

func (p *LocalProvider) BackdropURL(_ context.Context, item *model.MediaItem) (string, error) {
	if item.BackdropPath == "" {
		return "", nil
	}
	return p.signer.Generate(fmt.Sprintf("/api/v1/media/%s/backdrop", item.ID)), nil
}

func (p *LocalProvider) LogoURL(_ context.Context, item *model.MediaItem) (string, error) {
	if item.LogoPath == "" {
		return "", nil
	}
	return p.signer.Generate(fmt.Sprintf("/api/v1/media/%s/logo", item.ID)), nil
}
